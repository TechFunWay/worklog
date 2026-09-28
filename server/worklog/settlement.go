package worklog

import (
	"encoding/json"
	"fmt"
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
}

type detailRecord struct {
	RecordID       uint   `json:"record_id"`
	Date           string `json:"date"`
	Type           string `json:"type"`
	TypeLabel      string `json:"type_label"`
	Quantity       string `json:"quantity"`
	UnitPriceCents int64  `json:"unit_price_cents"`
	AmountCents    int64  `json:"amount_cents"`
	ProjectName    string `json:"project_name,omitempty"`
	PieceName      string `json:"piece_name,omitempty"`
	PieceUnit      string `json:"piece_unit,omitempty"`
	Note           string `json:"note,omitempty"`
}

type detailAdvance struct {
	AdvanceID   uint   `json:"advance_id"`
	Date        string `json:"date"`
	AmountCents int64  `json:"amount_cents"`
	Method      string `json:"method"`
	MethodLabel string `json:"method_label"`
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
}

// unsettledSummary 计算某工人指定时间段内未结算的记工与借支汇总。
// projectID 非空时只汇总该工地的记工（借支仍按人全量展示）。
func unsettledSummary(db *gorm.DB, uid, workerID uint, start, end string, projectID *uint) (*workerUnsettled, error) {
	var worker Worker
	if err := db.Where("user_id = ? AND id = ?", uid, workerID).First(&worker).Error; err != nil {
		return nil, fmt.Errorf("工人不存在")
	}
	rq := db.Model(&WorkRecord{}).Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL", uid, workerID)
	aq := db.Model(&Advance{}).Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL", uid, workerID)
	if start != "" {
		rq = rq.Where("date >= ?", start)
		aq = aq.Where("date >= ?", start)
	}
	if end != "" {
		rq = rq.Where("date <= ?", end)
		aq = aq.Where("date <= ?", end)
	}
	if projectID != nil {
		rq = rq.Where("project_id = ?", *projectID)
	}
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
	return &workerUnsettled{
		WorkerID: worker.ID, WorkerName: worker.Name,
		RecordCount: int(recordCount), WorkAmountCents: workTotal,
		AdvanceCount: int(advanceCount), AdvanceAmountCents: advanceTotal,
		PayableCents: workTotal - advanceTotal,
		EarliestDate: earliest, LatestDate: latest,
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
			Where("user_id = ? AND settlement_id IS NULL", uid).Scan(&workerIDs)
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
type unsettledProjectRow struct {
	ProjectID   uint   `json:"project_id"`
	ProjectName string `json:"project_name"`
	WorkerCount int64  `json:"worker_count"`
	RecordCount int64  `json:"record_count"`
	AmountCents int64  `json:"amount_cents"`
}

func unsettledProjects(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.canManage() {
			forbidden(c)
			return
		}
		type row struct {
			ProjectID uint
			Amount    int64
			Workers   int64
			Cnt       int64
		}
		var rows []row
		if err := db.Model(&WorkRecord{}).
			Select("project_id, COALESCE(SUM(amount_cents),0) AS amount, COUNT(DISTINCT worker_id) AS workers, COUNT(*) AS cnt").
			Where("user_id = ? AND settlement_id IS NULL AND project_id IS NOT NULL", ac.scopeUser()).
			Group("project_id").Scan(&rows).Error; err != nil {
			response.ErrorInternal(c, "汇总失败")
			return
		}
		result := make([]unsettledProjectRow, 0, len(rows))
		for _, r := range rows {
			var proj Project
			name := ""
			if err := db.Where("user_id = ? AND id = ?", ac.scopeUser(), r.ProjectID).First(&proj).Error; err == nil {
				name = proj.Name
			}
			result = append(result, unsettledProjectRow{
				ProjectID: r.ProjectID, ProjectName: name,
				WorkerCount: r.Workers, RecordCount: r.Cnt, AmountCents: r.Amount,
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
		rq := db.Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL", uid, workerID)
		aq := db.Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL", uid, workerID)
		if start != "" {
			rq = rq.Where("date >= ?", start)
			aq = aq.Where("date >= ?", start)
		}
		if end != "" {
			rq = rq.Where("date <= ?", end)
			aq = aq.Where("date <= ?", end)
		}
		if projectID != nil {
			rq = rq.Where("project_id = ?", *projectID)
		}
		var records []WorkRecord
		rq.Order("date ASC, id ASC").Find(&records)
		fillRecordNames(db, records)
		var advances []Advance
		aq.Order("date ASC, id ASC").Find(&advances)
		fillAdvanceNames(db, advances)
		response.Success(c, gin.H{"summary": summary, "records": records, "advances": advances})
	}
}

// doSettle 在事务中为一名工人生成结算单并挂单。
// projectID 非空时按项目结算：只挂该工地的记工，借支不抵扣（留给按人结算）。
func doSettle(tx *gorm.DB, uid, workerID uint, start, end, note string, projectID *uint) (*Settlement, error) {
	summary, err := unsettledSummary(tx, uid, workerID, start, end, projectID)
	if err != nil {
		return nil, err
	}
	if summary.RecordCount == 0 && summary.AdvanceCount == 0 {
		return nil, fmt.Errorf("该时间段没有未结算的记录")
	}

	rq := tx.Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL", uid, workerID)
	aq := tx.Where("user_id = ? AND worker_id = ? AND settlement_id IS NULL", uid, workerID)
	if start != "" {
		rq = rq.Where("date >= ?", start)
		aq = aq.Where("date >= ?", start)
	}
	if end != "" {
		rq = rq.Where("date <= ?", end)
		aq = aq.Where("date <= ?", end)
	}
	if projectID != nil {
		rq = rq.Where("project_id = ?", *projectID)
	}
	var records []WorkRecord
	if err := rq.Order("date ASC, id ASC").Find(&records).Error; err != nil {
		return nil, err
	}
	var advances []Advance
	if projectID == nil {
		if err := aq.Order("date ASC, id ASC").Find(&advances).Error; err != nil {
			return nil, err
		}
	}

	detail := settlementDetail{Records: make([]detailRecord, 0, len(records)), Advances: make([]detailAdvance, 0, len(advances))}
	for _, r := range records {
		detail.Records = append(detail.Records, detailRecord{
			RecordID: r.ID, Date: r.Date, Type: r.Type, TypeLabel: typeLabels[r.Type],
			Quantity: r.Quantity, UnitPriceCents: r.UnitPriceCents, AmountCents: r.AmountCents,
			Note: r.Note,
		})
	}
	for _, a := range advances {
		detail.Advances = append(detail.Advances, detailAdvance{
			AdvanceID: a.ID, Date: a.Date, AmountCents: a.AmountCents,
			Method: a.Method, MethodLabel: methodLabels[a.Method], Note: a.Note,
		})
	}
	detailJSON, err := json.Marshal(detail)
	if err != nil {
		return nil, err
	}

	projName := ""
	if projectID != nil {
		var proj Project
		if err := tx.Where("user_id = ? AND id = ?", uid, *projectID).First(&proj).Error; err == nil {
			projName = proj.Name
		}
	}
	settlement := Settlement{
		UserID: uid, WorkerID: workerID, WorkerName: summary.WorkerName,
		ProjectID: projectID, ProjectName: projName,
		PeriodStart: summary.EarliestDate, PeriodEnd: summary.LatestDate,
		WorkAmountCents: summary.WorkAmountCents, AdvanceAmountCents: summary.AdvanceAmountCents,
		PayableCents: summary.PayableCents, RecordCount: summary.RecordCount,
		Details: string(detailJSON), Note: note, SettledAt: time.Now(),
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
	advanceIDs := make([]uint, 0, len(advances))
	for _, a := range advances {
		advanceIDs = append(advanceIDs, a.ID)
	}
	if len(advanceIDs) > 0 && projectID == nil {
		if err := tx.Model(&Advance{}).Where("id IN ?", advanceIDs).
			Update("settlement_id", settlement.ID).Error; err != nil {
			return nil, err
		}
	}
	return &settlement, nil
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
		var in struct {
			WorkerID    uint   `json:"worker_id"`
			Start       string `json:"start"`
			End         string `json:"end"`
			Note        string `json:"note"`
			ProjectID   *uint  `json:"project_id"`
		}
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
		var settlement *Settlement
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			settlement, err = doSettle(tx, uid, in.WorkerID, in.Start, in.End, in.Note, in.ProjectID)
			return err
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
		var in struct {
			Start     string `json:"start"`
			End       string `json:"end"`
			Note      string `json:"note"`
			ProjectID *uint  `json:"project_id"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if (in.Start != "" && !validDate(in.Start)) || (in.End != "" && !validDate(in.End)) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		// 有未结算记录的工人（按项目结算时只取该项目的记录）
		idSet := map[uint]bool{}
		var ids []uint
		recQ := db.Model(&WorkRecord{}).Where("user_id = ? AND settlement_id IS NULL", uid)
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
		err := db.Transaction(func(tx *gorm.DB) error {
			for workerID := range idSet {
				s, err := doSettle(tx, uid, workerID, in.Start, in.End, in.Note, in.ProjectID)
				if err != nil {
					failed = append(failed, fmt.Sprintf("工人#%d: %s", workerID, err.Error()))
					continue
				}
				created = append(created, *s)
			}
			return nil
		})
		if err != nil {
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
		var total int64
		q.Count(&total)
		page, pageSize := utils.NormalizePage(utils.Atoi(c.Query("page"), 1), utils.Atoi(c.Query("page_size"), 20))
		var items []Settlement
		if err := q.Omit("details").Order("settled_at DESC, id DESC").
			Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
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

// revertSettlement 撤销结算：记录解挂并删除结算单。
func revertSettlement(db *gorm.DB) gin.HandlerFunc {
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
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&WorkRecord{}).Where("settlement_id = ?", item.ID).
				Update("settlement_id", nil).Error; err != nil {
				return err
			}
			if err := tx.Model(&Advance{}).Where("settlement_id = ?", item.ID).
				Update("settlement_id", nil).Error; err != nil {
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
