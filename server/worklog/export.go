package worklog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

func exportRecordsCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		uid := ac.scopeUser()
		q := db.Where("user_id = ? AND status = ?", uid, RecordStatusApproved)
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("worker_id = ?", sw)
		} else if v := c.Query("worker_id"); v != "" {
			q = q.Where("worker_id = ?", v)
		}
		if v := c.Query("start"); validDate(v) {
			q = q.Where("date >= ?", v)
		}
		if v := c.Query("end"); validDate(v) {
			q = q.Where("date <= ?", v)
		}
		var records []WorkRecord
		if err := q.Order("date ASC, id ASC").Limit(50000).Find(&records).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		fillRecordNames(db, records)
		// 列序约定（v0.6.0）：既有列一律不动，「工钱 / 补贴 / 扣款」追加在末尾，
		// 免得下游按列位取数的 Excel 模板错位。「金额(元)」仍是该笔应得合计 = 工钱 + 补贴 − 扣款。
		rows := [][]string{{"日期", "工人", "类型", "数量", "单价(元)", "金额(元)", "工地", "计件项目", "状态", "备注", "工钱(元)", "补贴(元)", "扣款(元)"}}
		for _, r := range records {
			status := "未结算"
			if r.SettlementID != nil {
				status = "已结算"
			}
			rows = append(rows, []string{
				r.Date, r.WorkerName, typeLabels[r.Type], r.Quantity,
				centsToYuanString(r.UnitPriceCents), centsToYuanString(r.AmountCents),
				r.ProjectName, pieceLabel(r), status, r.Note,
				centsToYuanString(r.WageCents), centsToYuanString(r.SubsidyCents), centsToYuanString(r.DeductCents),
			})
		}
		writeCSV(c, utils.ExportFilename("记工明细", "csv"), rows)
	}
}

func pieceLabel(r WorkRecord) string {
	return pieceLabelFromName(r.PieceName, r.PieceUnit)
}

func exportMonthlyCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		uid := ac.scopeUser()
		now := time.Now()
		year := atoiDefault(c.Query("year"), now.Year())
		month := atoiDefault(c.Query("month"), int(now.Month()))
		start, end := monthRange(year, month)
		q := db.Where("user_id = ? AND date BETWEEN ? AND ? AND status = ?", uid, start, end, RecordStatusApproved)
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("worker_id = ?", sw)
		}
		var records []WorkRecord
		if err := q.Order("worker_id ASC, date ASC").Find(&records).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		fillRecordNames(db, records)
		// 「金额(元)」= 该笔应得合计（已含补贴、已减扣款）；「工钱(元)」才是 数量 × 单价。
		// 拆出来的三列一律追加在末尾，老模板按列位取数不受影响。
		rows := [][]string{{"工人", "日期", "类型", "数量", "单价(元)", "金额(元)", "工地", "计件项目", "备注", "工钱(元)", "补贴(元)", "扣款(元)"}}
		for _, r := range records {
			rows = append(rows, []string{
				r.WorkerName, r.Date, typeLabels[r.Type], r.Quantity,
				centsToYuanString(r.UnitPriceCents), centsToYuanString(r.AmountCents),
				r.ProjectName, pieceLabel(r), r.Note,
				centsToYuanString(r.WageCents), centsToYuanString(r.SubsidyCents), centsToYuanString(r.DeductCents),
			})
		}
		writeCSV(c, utils.ExportFilename(fmt.Sprintf("工资月报-%d-%02d", year, month), "csv"), rows)
	}
}

// exportAttendanceCSV 导出「考勤表」：按工人逐日列出点工/点时/计件/休息
// 与当天工资，每人末尾带一行合计。支持两种时间维度：
//   - year+month（默认当前月）：月度考勤表；
//   - start+end：自定义时间段考勤表（粉丝反馈要按不同时间段查考勤）。
//
// 另支持 project_id 按工地过滤。不传 worker_id 时导出全部在职工人（含期间内
// 有记录的已归档工人），传 worker_id 时只导出该工人；员工角色强制只导出自己。
func exportAttendanceCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()

		// 时间维度：优先 start/end（两者都合法），否则按 year/month。
		start, end := c.Query("start"), c.Query("end")
		var name string
		rangeMode := validDate(start) && validDate(end)
		if rangeMode {
			st, err1 := time.Parse("2006-01-02", start)
			et, err2 := time.Parse("2006-01-02", end)
			if err1 != nil || err2 != nil || st.After(et) {
				response.ErrorBadRequest(c, "时间段不正确：请提供 start/end（YYYY-MM-DD）且起止顺序正确")
				return
			}
			if int(et.Sub(st).Hours()/24)+1 > maxAttendanceRangeDays {
				response.ErrorBadRequest(c, "查询时间段不能超过 731 天")
				return
			}
			name = fmt.Sprintf("考勤表-%s至%s", start, end)
		} else {
			if start != "" || end != "" {
				response.ErrorBadRequest(c, "时间段不正确：请同时提供 start/end（YYYY-MM-DD）")
				return
			}
			now := time.Now()
			year := atoiDefault(c.Query("year"), now.Year())
			month := atoiDefault(c.Query("month"), int(now.Month()))
			if month < 1 || month > 12 {
				response.ErrorBadRequest(c, "月份不正确")
				return
			}
			start, end = monthRange(year, month)
			name = fmt.Sprintf("考勤表-%d-%02d", year, month)
		}

		pf, ok := parseProjectFilter(c)
		if !ok {
			response.ErrorBadRequest(c, "project_id 参数不正确")
			return
		}

		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, ok := ac.workerScope(); ok {
			// 员工只能导出自己；成员行没绑工人档案时直接拒绝，
			// 不能退回下面的 else 分支去导出全部在职工人。
			if ac.workerUnbound() {
				forbidden(c)
				return
			}
			workerID = int(sw)
		}

		var workers []Worker
		qw := db.Where("user_id = ?", dsUID)
		if workerID > 0 {
			qw = qw.Where("id = ?", workerID)
		} else {
			qw = qw.Where("status = ?", "active")
		}
		if err := qw.Order("name ASC, id ASC").Find(&workers).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		if workerID > 0 && len(workers) == 0 {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}

		var records []WorkRecord
		q := db.Where("user_id = ? AND date BETWEEN ? AND ? AND status = ?", dsUID, start, end, RecordStatusApproved)
		q = pf.apply(q)
		if workerID > 0 {
			q = q.Where("worker_id = ?", workerID)
		}
		if err := q.Order("worker_id ASC, date ASC, id ASC").Find(&records).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}

		// 当月有记录、但已归档或不在职工人列表里的工人也要出现在考勤表里，
		// 否则「上月记了工、这月归档」的人会整月从考勤表消失。
		known := make(map[uint]bool, len(workers))
		for _, w := range workers {
			known[w.ID] = true
		}
		var extraIDs []uint
		for _, r := range records {
			if !known[r.WorkerID] {
				known[r.WorkerID] = true
				extraIDs = append(extraIDs, r.WorkerID)
			}
		}
		if len(extraIDs) > 0 {
			var extra []Worker
			if err := db.Where("user_id = ? AND id IN ?", dsUID, extraIDs).Find(&extra).Error; err != nil {
				response.ErrorInternal(c, "导出失败")
				return
			}
			workers = append(workers, extra...)
			sort.SliceStable(workers, func(i, j int) bool {
				if workers[i].Name == workers[j].Name {
					return workers[i].ID < workers[j].ID
				}
				return workers[i].Name < workers[j].Name
			})
		}

		// 工人 + 日期 聚合
		// amountCents 是当天应得合计（补贴已并入、扣款已减掉），subsidyCents / deductCents
		// 只用来解释「当天工资为什么和 数量×单价 对不上」，不参与重算。
		type dayAgg struct {
			days, hours, pieces float64
			rest                bool
			amountCents         int64
			subsidyCents        int64
			deductCents         int64
			notes               []string
		}
		agg := make(map[uint]map[string]*dayAgg, len(workers))
		for _, r := range records {
			byDate := agg[r.WorkerID]
			if byDate == nil {
				byDate = map[string]*dayAgg{}
				agg[r.WorkerID] = byDate
			}
			cell := byDate[r.Date]
			if cell == nil {
				cell = &dayAgg{}
				byDate[r.Date] = cell
			}
			cell.amountCents += r.AmountCents
			cell.subsidyCents += r.SubsidyCents
			cell.deductCents += r.DeductCents
			if r.Type == TypeRest {
				cell.rest = true
			} else {
				qty := parseFloat(r.Quantity)
				switch r.Type {
				case TypeDay:
					cell.days += qty
				case TypeHour:
					cell.hours += qty
				case TypePiece:
					cell.pieces += qty
				}
			}
			if r.Note != "" {
				cell.notes = append(cell.notes, r.Note)
			}
		}

		// 「金额(元)」= 当天应得合计（已含补贴、已减扣款）；补贴/扣款两列追加在末尾，
		// 让工人能自己把「数量 × 单价 ≠ 金额」这件事对上账。既有列序不变。
		rows := [][]string{{"工人", "日期", "点工(天)", "点时(小时)", "计件(数量)", "休息", "金额(元)", "备注", "补贴(元)", "扣款(元)"}}
		for _, w := range workers {
			byDate := agg[w.ID]
			dates := make([]string, 0, len(byDate))
			for d := range byDate {
				dates = append(dates, d)
			}
			sort.Strings(dates)

			var totalDays, totalHours, totalPieces float64
			var totalRest int
			var totalAmount, totalSubsidy, totalDeduct int64
			for _, d := range dates {
				cell := byDate[d]
				restMark := ""
				if cell.rest {
					restMark = "休"
					totalRest++
				}
				rows = append(rows, []string{
					w.Name, d, trimFloat(cell.days), trimFloat(cell.hours), trimFloat(cell.pieces),
					restMark, centsToYuanString(cell.amountCents), strings.Join(cell.notes, "；"),
					centsToYuanString(cell.subsidyCents), centsToYuanString(cell.deductCents),
				})
				totalDays += cell.days
				totalHours += cell.hours
				totalPieces += cell.pieces
				totalAmount += cell.amountCents
				totalSubsidy += cell.subsidyCents
				totalDeduct += cell.deductCents
			}
			// 每人一行合计，没有记录的人也列出 0，考勤表才能覆盖全部在职工人。
			// 合计行的补贴/扣款同样只作拆项展示，金额列已经是含补贴的应得合计。
			rows = append(rows, []string{
				w.Name, "合计", trimFloat(totalDays), trimFloat(totalHours),
				trimFloat(totalPieces), fmt.Sprintf("%d", totalRest),
				centsToYuanString(totalAmount), "",
				centsToYuanString(totalSubsidy), centsToYuanString(totalDeduct),
			})
		}
		writeCSV(c, utils.ExportFilename(name, "csv"), rows)
	}
}

// exportReconcileCSV 导出「对工单」：一段人类可读的表头信息（工人/期间/工地）+
// 逐条记工明细 + 合计，方便组长直接转发给工人核对。数据与
// GET /worklog/attendance/reconcile 同源（buildReconcile），两处数字不会不一致。
func exportReconcileCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()

		start, end, ok := parseDateRange(c)
		if !ok {
			response.ErrorBadRequest(c, "时间段不正确：请提供 start/end（YYYY-MM-DD），起止顺序正确且不超过 731 天")
			return
		}
		pf, ok := parseProjectFilter(c)
		if !ok {
			response.ErrorBadRequest(c, "project_id 参数不正确")
			return
		}
		workerID := utils.Atoi(c.Query("worker_id"), 0)
		if sw, ok := ac.workerScope(); ok {
			if ac.workerUnbound() {
				forbidden(c)
				return
			}
			workerID = int(sw)
		}
		if workerID <= 0 {
			response.ErrorBadRequest(c, "请选择要核对的工人")
			return
		}
		res, err := buildReconcile(db, dsUID, uint(workerID), start, end, pf)
		if err == errWorkerNotFound {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}
		if err == errProjectNotFound {
			response.ErrorBadRequest(c, "工地不存在")
			return
		}
		if err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}

		// 表头与既有列序不动；「工钱 / 补贴 / 扣款 / 补贴说明」追加在末尾。
		// 金额(元) 是应得合计（含补贴、已减扣款），工人拿「数量 × 单价」乘不出来时
		// 看这四列就能对上账 —— 契约里「明细行含补贴要说清楚」的导出侧落点。
		rows := [][]string{
			{"对工单"},
			{"工人", res.Worker.Name},
			{"期间", res.Start + " 至 " + res.End},
			{"工地", res.ProjectName},
			{},
			{"日期", "类型", "数量", "单价(元)", "金额(元)", "工地", "计件项目", "备注", "工钱(元)", "补贴(元)", "扣款(元)", "补贴说明"},
		}
		for _, it := range res.Items {
			quantity := it.Quantity
			if it.Rest {
				quantity = ""
			}
			piece := pieceLabelFromName(it.PieceName, it.PieceUnit)
			rows = append(rows, []string{
				it.Date, it.TypeLabel, quantity,
				centsToYuanString(it.UnitPriceCents), centsToYuanString(it.AmountCents),
				it.ProjectName, piece, it.Note,
				centsToYuanString(it.WageCents), centsToYuanString(it.SubsidyCents),
				centsToYuanString(it.DeductCents), it.SubsidyNote,
			})
		}
		summary := fmt.Sprintf("点工 %s 天；计时 %s 小时；计件 %s；休息 %d 天",
			trimFloat(res.Days), trimFloat(res.Hours), trimFloat(res.Pieces), res.RestCount)
		// 有补贴/扣款时在合计行里写明白，别让工人以为金额算错了（金额为 0 的行不写，避免噪音）。
		if res.SubsidyCents != 0 || res.DeductCents != 0 {
			summary += fmt.Sprintf("；其中补贴 %s 元、扣款 %s 元（金额 = 工钱 + 补贴 − 扣款）",
				centsToYuanString(res.SubsidyCents), centsToYuanString(res.DeductCents))
		}
		rows = append(rows, []string{"合计", "", summary, "", centsToYuanString(res.AmountCents), "", "", "",
			centsToYuanString(res.WageCents), centsToYuanString(res.SubsidyCents), centsToYuanString(res.DeductCents), ""})
		writeCSV(c, utils.ExportFilename(fmt.Sprintf("对工单-%s-%s至%s", res.Worker.Name, start, end), "csv"), rows)
	}
}

// pieceLabelFromName 计件项目展示：名称 + 单位（无单位时只显示名称）。
func pieceLabelFromName(name, unit string) string {
	if name == "" {
		return ""
	}
	if unit == "" {
		return name
	}
	return fmt.Sprintf("%s(%s)", name, unit)
}

func exportAdvancesCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		uid := ac.scopeUser()
		q := db.Where("user_id = ?", uid)
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("worker_id = ?", sw)
		}
		var items []Advance
		if err := q.Order("date ASC, id ASC").Find(&items).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		fillAdvanceNames(db, items)
		// 「工地」列追加在末尾（v0.6.0 借支可选工地）：未指定工地时留空，
		// 这类散借支只在按人结算时抵扣，不参与按项目结算（docs/06 §4.2）。
		// 注意工地名是用户录入内容，必须和其他文本列一样过公式注入防护。
		rows := [][]string{{"日期", "工人", "金额(元)", "方式", "状态", "备注", "工地"}}
		for _, a := range items {
			status := "未结算"
			if a.SettlementID != nil {
				status = "已结算"
			}
			rows = append(rows, []string{a.Date, a.WorkerName, centsToYuanString(a.AmountCents), methodLabels[a.Method], status, a.Note, a.ProjectName})
		}
		writeCSV(c, utils.ExportFilename("借支记录", "csv"), rows)
	}
}

func exportSettlementsCSV(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		uid := ac.scopeUser()
		q := db.Where("user_id = ?", uid)
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("worker_id = ?", sw)
		}
		var items []Settlement
		if err := q.Order("settled_at ASC, id ASC").Limit(20000).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		// 调整项 / 已付 / 尚欠 / 付款状态四列一律追加在末尾（既有列序不动）。
		// 口径：实发(元) = 应发 − 借支 + 调整项；尚欠 = max(实发 − 已付, 0)，
		// 与 settlement.go 里 DueCents 的派生规则逐字一致（多付不倒挂成负欠薪，
		// 打超了多少由「已付」列自己大于「实发」体现）。付款状态出中文标签。
		rows := [][]string{
			{"结算单号", "工人", "期间起", "期间止", "应发(元)", "借支(元)", "实发(元)", "记工条数", "结算时间", "备注", "调整项(元)", "调整原因", "已付(元)", "尚欠(元)", "付款状态"},
		}
		for _, s := range items {
			rows = append(rows, []string{
				fmt.Sprintf("JS%06d", s.ID), s.WorkerName, s.PeriodStart, s.PeriodEnd,
				centsToYuanString(s.WorkAmountCents), centsToYuanString(s.AdvanceAmountCents),
				centsToYuanString(s.PayableCents), fmt.Sprintf("%d", s.RecordCount),
				s.SettledAt.Format("2006-01-02 15:04"), s.Note,
				centsToYuanString(s.AdjustCents), s.AdjustNote,
				centsToYuanString(s.PaidCents), centsToYuanString(settlementDueCents(s.PayableCents, s.PaidCents)),
				csvPayStatusLabel(s.PayStatus, s.PayableCents, s.PaidCents),
			})
		}
		writeCSV(c, utils.ExportFilename("结算记录", "csv"), rows)
	}
}

// settlementDueCents 结算单尚欠（分）：应付 − 已付，只取正数部分。
// 与 settlement.go 给 DueCents 赋值的规则一致：多付不倒挂成「负欠薪」，
// 打超多少由「已付」列自己大于「实发」体现。导出与网页必须同一套数，否则对不上账。
func settlementDueCents(payableCents, paidCents int64) int64 {
	if payableCents <= paidCents {
		return 0
	}
	return payableCents - paidCents
}

// exportJSON 导出当前用户全部业务数据（JSON 包）。
func exportJSON(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		uid := ac.scopeUser()
		sw, isWorker := ac.workerScope()
		var workers []Worker
		var projects []Project
		var pieces []PieceItem
		var projectPrices []ProjectWorkerPrice
		var records []WorkRecord
		var advances []Advance
		var settlements []Settlement
		// payments（结算单付款流水，v0.6.0）必须一起导出：
		// 少这一张表，用户「导出备份 → 换机恢复」后结算单上的已付金额就没了来源，
		// 工资条会从「已付清」倒退回「欠薪」，属于会把人坑到真金白银的缺数据。
		// 初始化成空切片：没有流水时导出 "payments": []，与既有各键的形态一致，
		// 免得下游把 null 当「没导出」而不敢覆盖。
		payments := make([]SettlementPayment, 0)

		wq := db.Where("user_id = ?", uid)
		if isWorker {
			wq = wq.Where("id = ?", sw)
		}
		if err := wq.Order("id ASC").Find(&workers).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		if err := db.Where("user_id = ?", uid).Order("id ASC").Find(&projects).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		if err := db.Where("user_id = ?", uid).Order("id ASC").Find(&pieces).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		pq := db.Where("user_id = ?", uid)
		if isWorker {
			pq = pq.Where("worker_id = ?", sw)
		}
		if err := pq.Order("id ASC").Find(&projectPrices).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		rq := db.Where("user_id = ?", uid)
		if isWorker {
			rq = rq.Where("worker_id = ?", sw)
		}
		if err := rq.Order("date ASC, id ASC").Find(&records).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		// 借支与记工共用同一套工人过滤，但查询链单独建，避免 GORM 复用条件。
		aq := db.Where("user_id = ?", uid)
		if isWorker {
			aq = aq.Where("worker_id = ?", sw)
		}
		if err := aq.Order("date ASC, id ASC").Find(&advances).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		sq := db.Where("user_id = ?", uid)
		if isWorker {
			sq = sq.Where("worker_id = ?", sw)
		}
		if err := sq.Order("settled_at ASC, id ASC").Find(&settlements).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		// 付款流水同样按数据集归属过滤；员工只看自己那几笔（SettlementPayment 冗余了 worker_id）。
		payq := db.Where("user_id = ?", uid)
		if isWorker {
			payq = payq.Where("worker_id = ?", sw)
		}
		if err := payq.Order("paid_on ASC, id ASC").Find(&payments).Error; err != nil {
			response.ErrorInternal(c, "导出失败")
			return
		}
		data := gin.H{
			"app":            "worklog",
			"exported_at":    time.Now().Format(time.RFC3339),
			"workers":        workers,
			"projects":       projects,
			"project_prices": projectPrices,
			"piece_items":    pieces,
			"records":        records,
			"advances":       advances,
			"settlements":    settlements,
			"payments":       payments,
		}
		if err := utils.WriteJSONFile(c, utils.ExportFilename("工记数据备份", "json"), data); err != nil {
			response.ErrorInternal(c, "导出失败")
		}
	}
}

// exportDB 管理员下载 SQLite 数据库快照（VACUUM INTO 临时文件后流式下载）。
func exportDB(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil {
			response.ErrorInternal(c, "备份失败")
			return
		}
		tmpDir, err := os.MkdirTemp("", "worklog-backup-*")
		if err != nil {
			response.ErrorInternal(c, "备份失败")
			return
		}
		defer os.RemoveAll(tmpDir)
		backupPath := filepath.Join(tmpDir, fmt.Sprintf("worklog-backup-%s.db", time.Now().Format("20060102-150405")))
		if _, err := sqlDB.Exec("VACUUM INTO ?", backupPath); err != nil {
			response.ErrorInternal(c, "备份失败: "+err.Error())
			return
		}
		c.Header("Content-Type", "application/octet-stream")
		c.FileAttachment(backupPath, filepath.Base(backupPath))
	}
}
