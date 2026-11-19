// settlement_pay.go — 结算单的付款流水（v0.6.0）。
//
// 粉丝反馈「结算不可以自己填多少钱，有时不一定一次性工资结算完」。
// 处理口径：应付仍由系统算（工资条要能逐条对账，人不能把总额改成任意数），
// 「这次实际给了多少」用付款流水表达，一张结算单可分多笔；差额是本单的尚欠。
//
// 设计依据与不变式见 docs/06-结算分期借支工地补贴需求评估.md §3。
package worklog

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

// paymentInput 登记付款的请求体。
type paymentInput struct {
	AmountCents int64  `json:"amount_cents"`
	Method      string `json:"method"`
	PaidOn      string `json:"paid_on"`
	Note        string `json:"note"`
}

// settlementPaymentStats 一批结算单的付款聚合（一次 GROUP BY 出全部，避免列表 N+1）。
type settlementPaymentStats struct {
	SettlementID uint  `json:"-"`
	Cnt          int64 `json:"cnt"`
	Total        int64 `json:"total"`
}

func loadPaymentStats(db *gorm.DB, settlementIDs []uint) map[uint]settlementPaymentStats {
	out := map[uint]settlementPaymentStats{}
	if len(settlementIDs) == 0 {
		return out
	}
	var rows []settlementPaymentStats
	db.Model(&SettlementPayment{}).
		Select("settlement_id, COUNT(*) AS cnt, COALESCE(SUM(amount_cents),0) AS total").
		Where("settlement_id IN ?", settlementIDs).Group("settlement_id").Scan(&rows)
	for _, r := range rows {
		out[r.SettlementID] = r
	}
	return out
}

// paymentLabelOf 付款状态中文标签（导出与列表提示共用）。
func paymentLabelOf(status string) string {
	switch status {
	case PayStatusUnpaid:
		return "未付"
	case PayStatusPartial:
		return "部分支付"
	case PayStatusPaid:
		return "已付清"
	}
	return "已付清"
}

// listSettlementPayments 结算单详情的付款流水。员工可读自己那张单的流水（只读）。
func listSettlementPayments(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		var item Settlement
		if err := db.Where("user_id = ? AND id = ?", ac.scopeUser(), c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "结算单不存在")
			return
		}
		if sw, ok := ac.workerScope(); ok && item.WorkerID != sw {
			forbidden(c)
			return
		}
		var payments []SettlementPayment
		if err := db.Where("settlement_id = ?", item.ID).
			Order("paid_on ASC, id ASC").Find(&payments).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		fillPaymentNames(db, payments, item.WorkerName)
		response.Success(c, gin.H{
			"items": payments, "paid_cents": item.PaidCents, "pay_status": item.PayStatus,
		})
	}
}

// fillPaymentNames 补工人姓名与结算单号；工资条号沿用前端 JS%06d 的展示口径。
func fillPaymentNames(db *gorm.DB, payments []SettlementPayment, workerName string) {
	if len(payments) == 0 {
		return
	}
	ids := make([]uint, 0, len(payments))
	for _, p := range payments {
		ids = append(ids, p.WorkerID)
	}
	var workers []Worker
	db.Where("id IN ?", ids).Find(&workers)
	names := map[uint]string{}
	for _, w := range workers {
		names[w.ID] = w.Name
	}
	for i := range payments {
		if n, ok := names[payments[i].WorkerID]; ok {
			payments[i].WorkerName = n
		} else if workerName != "" {
			payments[i].WorkerName = workerName
		}
		payments[i].SettlementNo = fmt.Sprintf("JS%06d", payments[i].SettlementID)
	}
}

// createPayment 登记一笔付款（仅老板，与「谁能创建/撤销结算」同一权限口径）。
func createPayment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() {
			forbidden(c)
			return
		}
		dsUID := ac.scopeUser()
		var item Settlement
		if err := db.Where("user_id = ? AND id = ?", dsUID, c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "结算单不存在")
			return
		}
		var in paymentInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		paid, err := validatePaymentInput(&item, in)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		if in.Method == "" {
			in.Method = "cash"
		}
		if in.PaidOn == "" {
			in.PaidOn = time.Now().Format("2006-01-02")
		}
		var created SettlementPayment
		err = db.Transaction(func(tx *gorm.DB) error {
			// 重算口径下并发登记是安全的：两笔都在各自事务里 SUM，不会互相覆盖成同一值。
			var cur Settlement
			if err := tx.Where("id = ?", item.ID).First(&cur).Error; err != nil {
				return err
			}
			p := SettlementPayment{
				UserID: dsUID, SettlementID: cur.ID, WorkerID: cur.WorkerID, AmountCents: paid,
				Method: in.Method, PaidOn: in.PaidOn,
				Note: limitRunes(strings.TrimSpace(in.Note), 60), CreatedBy: uid,
			}
			if err := tx.Create(&p).Error; err != nil {
				return err
			}
			created = p
			if err := recalcSettlementPaid(tx, cur.ID); err != nil {
				return err
			}
			return tx.Where("id = ?", cur.ID).First(&cur).Error
		})
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		fillPaymentNames(db, []SettlementPayment{created}, item.WorkerName)
		response.Success(c, created)
	}
}

// validatePaymentInput 校验一笔付款。超过剩余应付直接拒，避免把工资条做成负欠款。
func validatePaymentInput(s *Settlement, in paymentInput) (int64, error) {
	if in.AmountCents <= 0 {
		return 0, errors.New("付款金额需大于 0")
	}
	if in.AmountCents > maxUnitPriceCents {
		return 0, errors.New("付款金额过大，请核对")
	}
	if in.PaidOn != "" && !validDate(in.PaidOn) {
		return 0, errors.New("付款日期格式应为 YYYY-MM-DD")
	}
	if in.Method != "" {
		if _, ok := methodLabels[in.Method]; !ok {
			return 0, errors.New("付款方式不正确")
		}
	}
	if s.PayableCents <= 0 {
		return 0, errors.New("本单没有应付金额，不需要登记付款")
	}
	if in.AmountCents > s.PayableCents-s.PaidCents {
		return 0, fmt.Errorf("超出剩余应付 %s，请核对", centsToYuanString(s.PayableCents-s.PaidCents))
	}
	return in.AmountCents, nil
}

// deletePayment 删掉一笔登记错的付款并重算已付（仅老板）。
func deletePayment(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() {
			forbidden(c)
			return
		}
		dsUID := ac.scopeUser()
		settlementID := utils.Atoi(c.Param("id"), 0)
		paymentID := utils.Atoi(c.Param("pid"), 0)
		if settlementID <= 0 || paymentID <= 0 {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		err := db.Transaction(func(tx *gorm.DB) error {
			res := tx.Where("id = ? AND settlement_id = ? AND user_id = ?", paymentID, settlementID, dsUID).
				Delete(&SettlementPayment{})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return errors.New("付款记录不存在")
			}
			return recalcSettlementPaid(tx, uint(settlementID))
		})
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// updateSettlementAdjust 修改结算单的调整项（仅老板）。
//
// 记错了抹零金额、事后又补了罚款这类修正是真实需求。改完必须重算：
//
//	应付 = 记工 − 借支 + 调整，且已付不能超过新应付（否则这一单变成负欠款，
//	工资条上说不通）。调整原因一并改，保证工资条自解释。
func updateSettlementAdjust(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() {
			forbidden(c)
			return
		}
		var item Settlement
		if err := db.Where("user_id = ? AND id = ?", ac.scopeUser(), c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "结算单不存在")
			return
		}
		var in struct {
			AdjustCents int64  `json:"adjust_cents"`
			AdjustNote  string `json:"adjust_note"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if in.AdjustCents < -maxUnitPriceCents || in.AdjustCents > maxUnitPriceCents {
			response.ErrorBadRequest(c, "调整金额过大，请核对")
			return
		}
		payable := item.WorkAmountCents - item.AdvanceAmountCents + in.AdjustCents
		if payable < 0 {
			response.ErrorBadRequest(c, "调整后应付为负，请同时核对借支或改为撤销重结")
			return
		}
		if payable < item.PaidCents {
			response.ErrorBadRequest(c, fmt.Sprintf("调整后应付 %s 小于已付 %s，请先删掉多余的付款记录",
				centsToYuanString(payable), centsToYuanString(item.PaidCents)))
			return
		}
		err := db.Model(&item).Updates(map[string]interface{}{
			"adjust_cents":  in.AdjustCents,
			"adjust_note":   limitRunes(strings.TrimSpace(in.AdjustNote), 60),
			"payable_cents": payable,
			"pay_status":    payStatusOf(payable, item.PaidCents),
		}).Error
		if err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		if err := db.Where("id = ?", item.ID).First(&item).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		fillSettlementDerived(&item, 0)
		response.Success(c, item)
	}
}

// dueSettlementRow 待付（欠薪）汇总的一行。
type dueSettlementRow struct {
	SettlementID uint   `json:"settlement_id"`
	SettlementNo string `json:"settlement_no"`
	WorkerID     uint   `json:"worker_id"`
	WorkerName   string `json:"worker_name"`
	ProjectName  string `json:"project_name"`
	PeriodStart  string `json:"period_start"`
	PeriodEnd    string `json:"period_end"`
	PayableCents int64  `json:"payable_cents"`
	PaidCents    int64  `json:"paid_cents"`
	DueCents     int64  `json:"due_cents"`
	SettledAt    string `json:"settled_at"`
}

// settlementsDue 已结算但还没付完的单子（欠薪看板）。
//
// 与 unsettled（未结算记工）是两码事，两个数不能混：未结算是「还没算账」，
// 待付是「账已算清、钱还没给全」。
func settlementsDue(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		q := db.Model(&Settlement{}).Where("user_id = ? AND pay_status IN ?", ac.scopeUser(),
			[]string{PayStatusUnpaid, PayStatusPartial})
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("worker_id = ?", sw)
		} else if v := c.Query("worker_id"); v != "" {
			q = q.Where("worker_id = ?", v)
		}
		var items []Settlement
		if err := q.Omit("details").Order("settled_at ASC, id ASC").Limit(500).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		rows := make([]dueSettlementRow, 0, len(items))
		var totalDue int64
		for _, s := range items {
			due := s.PayableCents - s.PaidCents
			if due <= 0 {
				continue
			}
			totalDue += due
			rows = append(rows, dueSettlementRow{
				SettlementID: s.ID, SettlementNo: fmt.Sprintf("JS%06d", s.ID),
				WorkerID: s.WorkerID, WorkerName: s.WorkerName, ProjectName: s.ProjectName,
				PeriodStart: s.PeriodStart, PeriodEnd: s.PeriodEnd,
				PayableCents: s.PayableCents, PaidCents: s.PaidCents, DueCents: due,
				SettledAt: s.SettledAt.Format("2006-01-02"),
			})
		}
		response.Success(c, gin.H{"rows": rows, "total_due_cents": totalDue, "count": len(rows)})
	}
}
