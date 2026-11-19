package worklog

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

// settlementDetail 结算单明细快照。
type settlementDetail struct {
	Records  []detailRecord  `json:"records"`
	Advances []detailAdvance `json:"advances"`
	// AdjustNote 调整原因（v0.6.0）。金额本身存在 Settlement.AdjustCents 上，
	// 这里只快照原因，让历史工资条解释得出「为什么和明细差这点钱」。
	AdjustNote string `json:"adjust_note,omitempty"`
}

type detailRecord struct {
	RecordID       uint   `json:"record_id"`
	Date           string `json:"date"`
	Type           string `json:"type"`
	TypeLabel      string `json:"type_label"`
	Quantity       string `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	// 补贴/扣款快照（v0.6.0）。工资条必须能写成「工钱 + 补贴 − 扣款」，
	// 只给一个含补贴的合计，工人对不上账。
	WageCents    int64  `json:"wage_cents"`
	SubsidyCents int64  `json:"subsidy_cents,omitempty"`
	SubsidyNote  string `json:"subsidy_note,omitempty"`
	DeductCents  int64  `json:"deduct_cents,omitempty"`
	AmountCents  int64  `json:"amount_cents"`
	ProjectName  string `json:"project_name,omitempty"`
	PieceName    string `json:"piece_name,omitempty"`
	PieceUnit    string `json:"piece_unit,omitempty"`
	Note         string `json:"note,omitempty"`
}

type detailAdvance struct {
	AdvanceID   uint   `json:"advance_id"`
	Date        string `json:"date"`
	AmountCents int64  `json:"amount_cents"`
	Method      string `json:"method"`
	MethodLabel string `json:"method_label"`
	// 借支归属工地（v0.6.0）：历史工资条要能看出「这 2000 是哪个工地出的」。
	ProjectName string `json:"project_name,omitempty"`
	Note        string `json:"note,omitempty"`
}

type workerUnsettled struct {
	WorkerID           uint   `json:"worker_id"`
	WorkerName         string `json:"worker_name"`
	RecordCount        int    `json:"record_count"`
	WorkAmountCents    int64  `json:"work_amount_cents"`
	AdvanceCount       int    `json:"advance_count"`
	AdvanceAmountCents int64  `json:"advance_amount_cents"`
	PayableCents       int64  `json:"payable_cents"`
	EarliestDate       string `json:"earliest_date"`
	LatestDate         string `json:"latest_date"`
	// 按项目结算时，未指定工地的借支不参与本次抵扣；这两个字段让前端能
	// 明说「另有 N 笔共 X 元将在按人结算时抵扣」，避免老板以为漏扣又去记一遍。
	LooseAdvanceCount int   `json:"loose_advance_count"`
	LooseAdvanceCents int64 `json:"loose_advance_cents"`
}

// settleScope 构造「某工人 + 某时间段 + 某工地」的未结算记工与借支查询。
//
// summary / preview / doSettle 三处的取数口径必须完全一致，否则「预览看到的」和
// 「实际结算挂走的」会不是同一批记录。历史上这套条件在三个函数里各抄了一遍，
// 已经分叉出 bug（按项目结算时借支漏过滤），所以这里收敛成一个函数。
// settleScope 构造「某工人 + 某时间段 + 某工地」的未结算记工与借支查询。
//
// summary / preview / doSettle 三处的取数口径必须完全一致，否则「预览看到的」和
// 「实际结算挂走的」会不是同一批记录。历史上这套条件在三个函数里各抄了一遍，
// 已经分叉出 bug（按项目结算时借支漏过滤），所以这里收敛成一个函数。
func settleScope(db *gorm.DB, uid, workerID uint, start, end string, projectID *uint) (rq, aq *gorm.DB) {
	rq = db.Model(&WorkRecord{}).Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL AND status = ?", uid, workerID, RecordStatusApproved)
	aq = db.Model(&Advance{}).Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL", uid, workerID)
	if start != "" {
		rq = rq.Where("date >= ?", start)
		aq = aq.Where("date >= ?", start)
	}
	if end != "" {
		rq = rq.Where("date <= ?", end)
		aq = aq.Where("date <= ?", end)
	}
	if projectID != nil {
		// 按项目结算：记工只取该工地，借支也只取该工地（未分工地的留给按人结算兜底）。
		rq = rq.Where("project_id = ?", *projectID)
		aq = aq.Where("project_id = ?", *projectID)
	}
	return rq, aq
}

// unsettledSummary 计算某工人指定时间段内未结算的记工与借支汇总。
//
// 抵扣口径（v0.6.0，粉丝反馈「借支分不清是哪个工地出的」，见 docs/06 §4.2）：
//
//	projectID == nil（按人结算）→ 该工人全部未结算借支；
//	projectID != nil（按项目结算）→ 只算 project_id = projectID 的借支；
//	未指定工地（NULL）的借支不进任何一次项目结算，只由按人结算兜底抵扣。
//
// 一笔借支只会挂到一张结算单（settlement_id 锁定），两条路径互斥、不会重复扣。
func unsettledSummary(db *gorm.DB, uid, workerID uint, start, end string, projectID *uint) (*workerUnsettled, error) {
	var worker Worker
	if err := db.Where("user_id = ? AND id = ?", uid, workerID).First(&worker).Error; err != nil {
		return nil, fmt.Errorf("工人不存在")
	}
	rq, aq := settleScope(db, uid, workerID, start, end, projectID)
	var workTotal, advanceTotal int64
	var recordCount, advanceCount int64
	if err := rq.Select("COALESCE(SUM(amount_cents),0)").Scan(&workTotal).Error; err != nil {
		return nil, err
	}
	if err := rq.Count(&recordCount).Error; err != nil {
		return nil, err
	}
	if err := aq.Select("COALESCE(SUM(amount_cents),0)").Scan(&advanceTotal).Error; err != nil {
		return nil, err
	}
	if err := aq.Count(&advanceCount).Error; err != nil {
		return nil, err
	}
	var minRec, maxRec struct{ V *string }
	rq.Select("MIN(date) AS v").Scan(&minRec)
	var minAdv struct{ V *string }
	aq.Select("MIN(date) AS v").Scan(&minAdv)
	rq.Select("MAX(date) AS v").Scan(&maxRec)
	var maxAdv struct{ V *string }
	aq.Select("MAX(date) AS v").Scan(&maxAdv)
	earliest, latest := "", ""
	for _, v := range []*string{minRec.V, minAdv.V} {
		if v != nil && (earliest == "" || *v < earliest) {
			earliest = *v
		}
	}
	for _, v := range []*string{maxRec.V, maxAdv.V} {
		if v != nil && *v > latest {
			latest = *v
		}
	}
	// 按项目结算时另算一笔「未指定工地」的借支，只用于前端提示，不参与本次抵扣。
	var looseCount, looseCents int64
	if projectID != nil {
		lq := db.Model(&Advance{}).Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL AND project_id IS NULL", uid, workerID)
		if start != "" {
			lq = lq.Where("date >= ?", start)
		}
		if end != "" {
			lq = lq.Where("date <= ?", end)
		}
		lq.Count(&looseCount)
		lq.Select("COALESCE(SUM(amount_cents),0)").Scan(&looseCents)
	}
	return &workerUnsettled{
		WorkerID: worker.ID, WorkerName: worker.Name,
		RecordCount: int(recordCount), WorkAmountCents: workTotal,
		AdvanceCount: int(advanceCount), AdvanceAmountCents: advanceTotal,
		PayableCents: workTotal - advanceTotal,
		EarliestDate: earliest, LatestDate: latest,
		LooseAdvanceCount: int(looseCount), LooseAdvanceCents: looseCents,
	}, nil
}

func unsettledList(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.canManage() {
			forbidden(c)
			return
		}
		uid = ac.scopeUser()
		start, end := c.Query("start"), c.Query("end")
		if start != "" && !validDate(start) || end != "" && !validDate(end) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		// 有未结算记工或借支的工人
		var workerIDs []uint
		db.Model(&WorkRecord{}).Select("DISTINCT worker_id").
			Where("user_id = ? AND settlement_id IS NULL AND status = ?", uid, RecordStatusApproved).Scan(&workerIDs)
		var advanceIDs []uint
		db.Model(&Advance{}).Select("DISTINCT worker_id").
			Where("user_id = ? AND settlement_id IS NULL", uid).Scan(&advanceIDs)
		idSet := map[uint]bool{}
		for _, id := range append(workerIDs, advanceIDs...) {
			idSet[id] = true
		}
		if len(idSet) == 0 {
			response.Success(c, []workerUnsettled{})
			return
		}
		ids := make([]uint, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		var workers []Worker
		db.Where("user_id = ? AND id IN ?", uid, ids).Order("name ASC").Find(&workers)
		result := make([]workerUnsettled, 0, len(workers))
		for _, w := range workers {
			s, err := unsettledSummary(db, uid, w.ID, start, end, nil)
			if err != nil {
				response.ErrorInternal(c, "汇总失败")
				return
			}
			if s.RecordCount == 0 && s.AdvanceCount == 0 {
				continue
			}
			result = append(result, *s)
		}
		response.Success(c, result)
	}
}

// unsettledProjectRow 按项目未结算汇总。
// 借支自 v0.6.0 起有了工地归属，这里同步扣掉「指定了本项目」的借支；
// 未指定工地的借支不在任何项目行里扣（由按人结算兜底），单列出来给前端提示。
type unsettledProjectRow struct {
	ProjectID         uint   `json:"project_id"`
	ProjectName       string `json:"project_name"`
	WorkerCount       int64  `json:"worker_count"`
	RecordCount       int64  `json:"record_count"`
	WorkAmountCents   int64  `json:"work_amount_cents"`
	AdvanceCents      int64  `json:"advance_cents"`
	AmountCents       int64  `json:"amount_cents"` // 应结 = 记工 − 本项目借支
	LooseAdvanceCents int64  `json:"loose_advance_cents"`
	LooseAdvanceCount int64  `json:"loose_advance_count"`
}

func unsettledProjects(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.canManage() {
			forbidden(c)
			return
		}
		dsUID := ac.scopeUser()
		type aggRow struct {
			ProjectID *uint
			Amount    int64
			Workers   int64
			Cnt       int64
		}
		var workRows, advRows []aggRow
		if err := db.Model(&WorkRecord{}).
			Select("project_id, COALESCE(SUM(amount_cents),0) AS amount, COUNT(DISTINCT worker_id) AS workers, COUNT(*) AS cnt").
			Where("user_id = ? AND settlement_id IS NULL AND project_id IS NOT NULL AND status = ?", dsUID, RecordStatusApproved).
			Group("project_id").Scan(&workRows).Error; err != nil {
			response.ErrorInternal(c, "汇总失败")
			return
		}
		// 本项目借支一次分组聚合；未指定工地的借支单独一行（ProjectID=nil）只做提示用。
		if err := db.Model(&Advance{}).
			Select("project_id, COALESCE(SUM(amount_cents),0) AS amount, COUNT(*) AS cnt").
			Where("user_id = ? AND settlement_id IS NULL", dsUID).
			Group("project_id").Scan(&advRows).Error; err != nil {
			response.ErrorInternal(c, "汇总失败")
			return
		}
		advByProject := map[uint]int64{}
		var looseCents, looseCount int64
		for _, a := range advRows {
			if a.ProjectID == nil {
				looseCents += a.Amount
				looseCount += a.Cnt
				continue
			}
			advByProject[*a.ProjectID] += a.Amount
		}
		// 工地名一次批量取，别在循环里逐条查（N+1）。
		ids := make([]uint, 0, len(workRows))
		for _, r := range workRows {
			if r.ProjectID != nil {
				ids = append(ids, *r.ProjectID)
			}
		}
		names := map[uint]string{}
		if len(ids) > 0 {
			var projects []Project
			db.Where("user_id = ? AND id IN ?", dsUID, ids).Find(&projects)
			for _, p := range projects {
				names[p.ID] = p.Name
			}
		}
		result := make([]unsettledProjectRow, 0, len(workRows))
		for _, r := range workRows {
			if r.ProjectID == nil {
				continue // 未分工地的记工走「按人结算」，不在项目行里展示
			}
			adv := advByProject[*r.ProjectID]
			result = append(result, unsettledProjectRow{
				ProjectID: *r.ProjectID, ProjectName: names[*r.ProjectID],
				WorkerCount: r.Workers, RecordCount: r.Cnt,
				WorkAmountCents: r.Amount, AdvanceCents: adv,
				AmountCents: r.Amount - adv,
				// 每行带同一份「未指定工地」借支合计，供前端提示「这些将在按人结算时抵扣」。
				LooseAdvanceCents: looseCents, LooseAdvanceCount: looseCount,
			})
		}
		response.Success(c, result)
	}
}

func settlementPreview(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		uid = ac.scopeUser()
		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, ok := ac.workerScope(); ok {
			if ac.workerUnbound() {
				forbidden(c)
				return
			}
			workerID = int(sw) // 员工只能预览自己
		}
		if workerID <= 0 {
			response.ErrorBadRequest(c, "请选择工人")
			return
		}
		start, end := c.Query("start"), c.Query("end")
		if start != "" && !validDate(start) || end != "" && !validDate(end) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		var projectID *uint
		if v := utils.Atoi(c.Query("project_id"), 0); v > 0 {
			projectID = new(uint)
			*projectID = uint(v)
		}
		summary, err := unsettledSummary(db, uid, uint(workerID), start, end, projectID)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		// 与 doSettle 同一套取数口径：预览看到什么，结算就挂走什么。
		rq, aq := settleScope(db, uid, uint(workerID), start, end, projectID)
		var records []WorkRecord
		rq.Order("date ASC, id ASC").Find(&records)
		fillRecordNames(db, records)
		var advances []Advance
		aq.Order("date ASC, id ASC").Find(&advances)
		fillAdvanceNames(db, advances)
		response.Success(c, gin.H{"summary": summary, "records": records, "advances": advances})
	}
}

// settleInput 一次结算的输入。PaidCents 用指针区分「没传」与「传了 0」：
//
//	nil → 本次全额付清（v0.5.x 的行为，老前端不传也不变）；
//	&0  → 全额挂欠薪（月底先对账、过几天再给钱的常见做法）；
//	其它 → 0 ≤ paid ≤ 应付，差额记为尚欠。
type settleInput struct {
	Start       string
	End         string
	Note        string
	ProjectID   *uint
	AdjustCents int64  // 调整项：抹零/罚款为负，额外奖励为正
	AdjustNote  string // 调整原因，工资条上要显示，否则工人对不上账
	PaidCents   *int64
}

// recalcSettlementPaid 用付款流水重算结算单的已付与付款状态。
//
// 故意用 SUM 重算而不是 ±累加：付款登记/删除都可能被重试或并发提交
// （SQLite 单写连接天然串行，但业务上仍然要求幂等），重算即使出错也能自愈，
// 不会越积越偏。必须在调用方事务内执行。
func recalcSettlementPaid(tx *gorm.DB, settlementID uint) error {
	var s Settlement
	if err := tx.Where("id = ?", settlementID).First(&s).Error; err != nil {
		return err
	}
	type sumRow struct {
		Total int64
		Cnt   int64
	}
	var row sumRow
	if err := tx.Model(&SettlementPayment{}).Where("settlement_id = ?", settlementID).
		Select("COALESCE(SUM(amount_cents),0) AS total, COUNT(*) AS cnt").Scan(&row).Error; err != nil {
		return err
	}
	if row.Total > s.PayableCents && s.PayableCents >= 0 {
		return fmt.Errorf("付款合计已大于应付，请核对")
	}
	if err := tx.Model(&Settlement{}).Where("id = ?", settlementID).Updates(map[string]interface{}{
		"paid_cents": row.Total,
		"pay_status": payStatusOf(s.PayableCents, row.Total),
	}).Error; err != nil {
		return err
	}
	_ = row.Cnt
	return nil
}

// fillSettlementDerived 填结算单的派生展示字段（尚欠、流水笔数）。
// paymentCount 传 0 表示「接口不返回流水明细」的列表场景，仍按已付推算笔数为 0 无意义，
// 所以列表场景各自查一次真实笔数后传进来。
func fillSettlementDerived(s *Settlement, paymentCount int) {
	if s.PayStatus == "" {
		s.PayStatus = payStatusOf(s.PayableCents, s.PaidCents)
	}
	if s.PaidCents == 0 && s.PayStatus == PayStatusPaid {
		// v0.6.0 之前的历史结算单没有流水，语义上就是「一次性付清」；
		// 回填脚本没跑时（如未注入版本号的开发构建）在这里兜底，
		// 否则整批历史工资条会显示成「全额欠薪」。
		s.PaidCents = s.PayableCents
	}
	s.DueCents = s.PayableCents - s.PaidCents
	if s.DueCents < 0 {
		s.DueCents = 0
	}
	s.PaymentCount = paymentCount
}

// doSettle 在事务中为一名工人生成结算单、挂单并按需登记第一笔付款。
//
// 应付 = 记工合计 − 借支合计 + 调整项（系统算出，人不能直接改成任意数）；
// 欠薪留在本单的 PaidCents/DueCents 上，**不会把记工退回未结算池** ——
// 记工已被 settlement_id 锁定，退回会让同一笔工被第二次结算（双付真钱）。
func doSettle(tx *gorm.DB, uid, workerID uint, in settleInput) (*Settlement, error) {
	summary, err := unsettledSummary(tx, uid, workerID, in.Start, in.End, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if summary.RecordCount == 0 && summary.AdvanceCount == 0 {
		return nil, fmt.Errorf("该时间段没有未结算的记录")
	}
	if in.AdjustCents < -maxUnitPriceCents || in.AdjustCents > maxUnitPriceCents {
		return nil, fmt.Errorf("调整金额过大，请核对")
	}
	payable := summary.PayableCents + in.AdjustCents
	// 本次实付：没传就是全额付清。分期只在应付为正时有意义（借支倒挂时
	// 「先付一部分」没有可付的基数，引导用调整项把账做平）。
	paid := payable
	if in.PaidCents != nil {
		paid = *in.PaidCents
		if paid < 0 {
			return nil, fmt.Errorf("本次实付不能为负数")
		}
		if payable <= 0 && paid != 0 {
			return nil, fmt.Errorf("本单没有应付金额，不能登记实付")
		}
		if paid > payable {
			return nil, fmt.Errorf("本次实付大于应付 %s，请核对；多给的请用借支记录", centsToYuanString(payable))
		}
	}

	rq, aq := settleScope(tx, uid, workerID, in.Start, in.End, in.ProjectID)
	var records []WorkRecord
	if err := rq.Order("date ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	var advances []Advance
	if err := aq.Order("date ASC, id ASC").Find(&advances).Error; err != nil {
		return nil, err
	}
	// 快照里的工地名按「当时」定格的写法与记工/借支名称快照一致（工地改名不影响历史工资条）。
	fillRecordNames(tx, records)
	fillAdvanceNames(tx, advances)

	detail := settlementDetail{Records: make([]detailRecord, 0, len(records)), Advances: make([]detailAdvance, 0, len(advances))}
	for _, r := range records {
		detail.Records = append(detail.Records, detailRecord{
			RecordID: r.ID, Date: r.Date, Type: r.Type, TypeLabel: typeLabels[r.Type],
			Quantity: r.Quantity, UnitPriceCents: r.UnitPriceCents,
			WageCents: r.WageCents, SubsidyCents: r.SubsidyCents, SubsidyNote: r.SubsidyNote, DeductCents: r.DeductCents,
			AmountCents: r.AmountCents, ProjectName: r.ProjectName,
			PieceName: r.PieceName, PieceUnit: r.PieceUnit, Note: r.Note,
		})
	}
	for _, a := range advances {
		detail.Advances = append(detail.Advances, detailAdvance{
			AdvanceID: a.ID, Date: a.Date, AmountCents: a.AmountCents,
			Method: a.Method, MethodLabel: methodLabels[a.Method],
			ProjectName: a.ProjectName, Note: a.Note,
		})
	}
	if in.AdjustCents != 0 {
		detail.AdjustNote = in.AdjustNote
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return nil, err
	}

	projName := ""
	if in.ProjectID != nil {
		var proj Project
		if err := tx.Where("user_id = ? AND id = ?", uid, *in.ProjectID).First(&proj).Error; err == nil {
			projName = proj.Name
		}
	}
	settlement := Settlement{
		UserID: uid, WorkerID: workerID, WorkerName: summary.WorkerName,
		ProjectID: in.ProjectID, ProjectName: projName,
		PeriodStart: summary.EarliestDate, PeriodEnd: summary.LatestDate,
		WorkAmountCents: summary.WorkAmountCents, AdvanceAmountCents: summary.AdvanceAmountCents,
		AdjustCents: in.AdjustCents, AdjustNote: limitRunes(strings.TrimSpace(in.AdjustNote), 60),
		PayableCents: payable, RecordCount: summary.RecordCount,
		Details: string(detailJSON), Note: in.Note,
		PaidCents: 0, PayStatus: payStatusOf(payable, 0),
		SettledAt: time.Now(),
	}
	if err := tx.Create(&settlement).Error; err != nil {
		return nil, err
	}

	recordIDs := make([]uint, 0, len(records))
	for _, r := range records {
		recordIDs = append(recordIDs, r.ID)
	}
	if len(recordIDs) > 0 {
		if err := tx.Model(&WorkRecord{}).Where("id IN ?", recordIDs).
			Update("settlement_id", settlement.ID).Error; err != nil {
			return nil, err
		}
	}
	// 借支挂单：按人结算抵扣全部、按项目结算只抵扣该项目的（口径已收在 settleScope 里），
	// 两种路径都靠 settlement_id 保证同一笔借支不会被扣两次。
	advanceIDs := make([]uint, 0, len(advances))
	for _, a := range advances {
		advanceIDs = append(advanceIDs, a.ID)
	}
	if len(advanceIDs) > 0 {
		if err := tx.Model(&Advance{}).Where("id IN ?", advanceIDs).
			Update("settlement_id", settlement.ID).Error; err != nil {
			return nil, err
		}
	}
	// 首笔实付直接落成一条流水，之后补付走付款接口，口径统一。
	if paid != 0 {
		pay := SettlementPayment{
			UserID: uid, SettlementID: settlement.ID, WorkerID: workerID, AmountCents: paid,
			Method: "cash", PaidOn: time.Now().Format("2006-01-02"),
			Note: "结算时支付", CreatedBy: uid,
		}
		if err := tx.Create(&pay).Error; err != nil {
			return nil, err
		}
		if err := recalcSettlementPaid(tx, settlement.ID); err != nil {
			return nil, err
		}
		if err := tx.Where("id = ?", settlement.ID).First(&settlement).Error; err != nil {
			return nil, err
		}
	}
	fillSettlementDerived(&settlement, 0)
	return &settlement, nil
}

// settlementInput 创建结算单的请求体（单人与一键月结共用一个结构，少一处口径分叉）。
//
// PaidCents 用指针：不传 = 本次全额付清（v0.5.x 行为，老前端不传也不变）；
// 传 0 = 全额挂欠薪；传正数 = 分期，差额留在本单「尚欠」里。
type settlementInput struct {
	WorkerID    uint   `json:"worker_id"`
	Start       string `json:"start"`
	End         string `json:"end"`
	Note        string `json:"note"`
	ProjectID   *uint  `json:"project_id"`
	AdjustCents int64  `json:"adjust_cents"`
	AdjustNote  string `json:"adjust_note"`
	PaidCents   *int64 `json:"paid_cents"`
}

// toSettle 转成 doSettle 入参。批量结算禁止带调整项与非 0 的实付：
// 一次月结会给每个工人各生成一张单，同一个金额对多人没有意义，
// 分期要按人算，只能进各自的结算单登记。
func (in settlementInput) toSettle(batch bool) (settleInput, error) {
	si := settleInput{
		Start: in.Start, End: in.End, Note: in.Note, ProjectID: in.ProjectID,
		AdjustCents: in.AdjustCents, AdjustNote: in.AdjustNote, PaidCents: in.PaidCents,
	}
	if !batch {
		return si, nil
	}
	if in.AdjustCents != 0 {
		return si, fmt.Errorf("一键月结不支持调整项，请逐人结算时填写")
	}
	if in.PaidCents != nil && *in.PaidCents != 0 {
		return si, fmt.Errorf("一键月结只支持全额付清或全额挂欠，分期请进各结算单登记")
	}
	return si, nil
}

func createSettlement(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() {
			forbidden(c)
			return
		}
		uid = ac.scopeUser()
		var in settlementInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if in.WorkerID <= 0 {
			response.ErrorBadRequest(c, "请选择工人")
			return
		}
		if (in.Start != "" && !validDate(in.Start)) || (in.End != "" && !validDate(in.End)) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		// 按项目结算时工地必须属于当前数据集，否则会结出一张抵扣不到任何借支的单。
		if err := validateProjectRef(db, uid, in.ProjectID); err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		si, err := in.toSettle(false)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		var settlement *Settlement
		err = db.Transaction(func(tx *gorm.DB) error {
			var e error
			settlement, e = doSettle(tx, uid, in.WorkerID, si)
			return e
		})
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		response.Success(c, settlement)
	}
}

func batchSettlement(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() {
			forbidden(c)
			return
		}
		uid = ac.scopeUser()
		var in settlementInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if (in.Start != "" && !validDate(in.Start)) || (in.End != "" && !validDate(in.End)) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		si, err := in.toSettle(true)
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		// 有未结算记录的工人（按项目结算时只取该项目的记录）
		idSet := map[uint]bool{}
		var ids []uint
		recQ := db.Model(&WorkRecord{}).Where("user_id = ? AND settlement_id IS NULL AND status = ?", uid, RecordStatusApproved)
		if in.ProjectID != nil {
			recQ = recQ.Where("project_id = ?", *in.ProjectID)
		}
		recQ.Select("DISTINCT worker_id").Scan(&ids)
		for _, id := range ids {
			idSet[id] = true
		}
		if in.ProjectID == nil {
			ids = nil
			db.Model(&Advance{}).Select("DISTINCT worker_id").
				Where("user_id = ? AND settlement_id IS NULL", uid).Scan(&ids)
			for _, id := range ids {
				idSet[id] = true
			}
		}
		if len(idSet) == 0 {
			response.ErrorBadRequest(c, "没有需要结算的记录")
			return
		}
		created := make([]Settlement, 0, len(idSet))
		var failed []string
		if err := db.Transaction(func(tx *gorm.DB) error {
			for workerID := range idSet {
				s, err := doSettle(tx, uid, workerID, si)
				if err != nil {
					failed = append(failed, fmt.Sprintf("工人#%d: %s", workerID, err.Error()))
					continue
				}
				created = append(created, *s)
			}
			return nil
		}); err != nil {
			response.ErrorInternal(c, "结算失败")
			return
		}
		if len(created) == 0 && len(failed) > 0 {
			response.ErrorBadRequest(c, failed[0])
			return
		}
		response.Success(c, gin.H{"settled": created, "failed": failed})
	}
}

func listSettlements(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		q := db.Model(&Settlement{}).Where("user_id = ?", ac.scopeUser())
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("worker_id = ?", sw)
		} else if v := c.Query("worker_id"); v != "" {
			q = q.Where("worker_id = ?", v)
		}
		if v := c.Query("start"); validDate(v) {
			q = q.Where("period_start >= ?", v)
		}
		if v := c.Query("end"); validDate(v) {
			q = q.Where("period_end <= ?", v)
		}
		// 付款状态筛选：全部 / unpaid / partial / paid（v0.6.0 欠薪看板用）。
		if v := c.Query("pay_status"); v == PayStatusUnpaid || v == PayStatusPartial || v == PayStatusPaid {
			q = q.Where("pay_status = ?", v)
		}
		var total int64
		q.Count(&total)
		page, pageSize := utils.NormalizePage(utils.Atoi(c.Query("page"), 1), utils.Atoi(c.Query("page_size"), 20))
		var items []Settlement
		if err := q.Omit("details").Order("settled_at DESC, id DESC").
			Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		ids := make([]uint, 0, len(items))
		for _, s := range items {
			ids = append(ids, s.ID)
		}
		stats := loadPaymentStats(db, ids)
		for i := range items {
			fillSettlementDerived(&items[i], int(stats[items[i].ID].Cnt))
		}
		response.SuccessPage(c, items, total, page, pageSize)
	}
}

func getSettlement(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		var item Settlement
		if err := db.Where("user_id = ? AND id = ?", ac.scopeUser(), c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "结算单不存在")
			return
		}
		if sw, ok := ac.workerScope(); ok && item.WorkerID != sw {
			forbidden(c)
			return
		}
		// 详情带全部付款流水：工资条要能逐笔说明「这 5000 是分几次给的」。
		var payments []SettlementPayment
		if err := db.Where("settlement_id = ?", item.ID).
			Order("paid_on ASC, id ASC").Find(&payments).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		fillPaymentNames(db, payments, item.WorkerName)
		item.Payments = payments
		fillSettlementDerived(&item, len(payments))
		response.Success(c, item)
	}
}

func updateSettlementNote(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		if !loadAccess(db, uid).isBoss() {
			forbidden(c)
			return
		}
		var item Settlement
		if err := db.Where("user_id = ? AND id = ?", uid, c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "结算单不存在")
			return
		}
		var in struct {
			Note string `json:"note"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if err := db.Model(&item).Update("note", in.Note).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// revertSettlement 撤销结算：记录解挂、付款流水连带删除、结算单删除，全在一个事务里。
//
// 流水必须跟着删：否则结算单没了、流水还在，之后再按 ID 关联就是一堆孤儿数据，
// 付款统计也会虚高。
func revertSettlement(db *gorm.DB) gin.HandlerFunc {
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
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&WorkRecord{}).Where("settlement_id = ?", item.ID).
				Update("settlement_id", nil).Error; err != nil {
				return err
			}
			if err := tx.Model(&Advance{}).Where("settlement_id = ?", item.ID).
				Update("settlement_id", nil).Error; err != nil {
				return err
			}
			if err := tx.Where("settlement_id = ?", item.ID).Delete(&SettlementPayment{}).Error; err != nil {
				return err
			}
			return tx.Delete(&item).Error
		})
		if err != nil {
			response.ErrorInternal(c, "撤销失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}
