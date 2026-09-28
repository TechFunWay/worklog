package worklog

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/utils"
)

func currentUID(c *gin.Context) uint {
	return c.MustGet("userID").(uint)
}

func forbidden(c *gin.Context) {
	response.Error(c, 403, response.CodeForbidden, "没有权限")
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

func validDate(s string) bool { return datePattern.MatchString(s) }

// ---- 工人 / 工地 / 计件项目（同构 CRUD） ----

func listWorkers(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		q := db.Where("user_id = ?", ac.scopeUser())
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("id = ?", sw)
		}
		switch c.Query("status") {
		case "active":
			q = q.Where("status = ?", "active")
		case "archived":
			q = q.Where("status = ?", "archived")
		}
		if kw := strings.TrimSpace(c.Query("keyword")); kw != "" {
			like := "%" + kw + "%"
			q = q.Where("name LIKE ? OR phone LIKE ?", like, like)
		}
		var items []Worker
		if err := q.Order("status ASC, name ASC").Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		response.Success(c, items)
	}
}

func createWorker(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !requireManage(ac) {
			forbidden(c)
			return
		}
		var in Worker
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		if in.Name == "" {
			response.ErrorBadRequest(c, "请填写工人姓名")
			return
		}
		if in.Status == "" {
			in.Status = "active"
		}
		in.ID = 0
		in.UserID = uid
		if err := db.Create(&in).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, in)
	}
}

func updateWorker(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !requireManage(ac) {
			forbidden(c)
			return
		}
		var item Worker
		if err := db.Where("user_id = ? AND id = ?", ac.scopeUser(), c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "工人不存在")
			return
		}
		var in Worker
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if strings.TrimSpace(in.Name) == "" {
			response.ErrorBadRequest(c, "请填写工人姓名")
			return
		}
		item.Name = strings.TrimSpace(in.Name)
		item.Phone = in.Phone
		item.Note = in.Note
		item.DailyWageCents = in.DailyWageCents
		item.HourlyWageCents = in.HourlyWageCents
		if in.Status == "active" || in.Status == "archived" {
			item.Status = in.Status
		}
		if err := db.Save(&item).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, item)
	}
}

func deleteWorker(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !ac.isBoss() {
			forbidden(c)
			return
		}
		var item Worker
		if err := db.Where("user_id = ? AND id = ?", ac.scopeUser(), c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "工人不存在")
			return
		}
		var nRecords, nAdvances int64
		db.Model(&WorkRecord{}).Where("user_id = ? AND worker_id = ?", uid, item.ID).Count(&nRecords)
		db.Model(&Advance{}).Where("user_id = ? AND worker_id = ?", uid, item.ID).Count(&nAdvances)
		if nRecords+nAdvances > 0 {
			response.ErrorBadRequest(c, "该工人已有记工或借支记录，不能删除，请改为归档")
			return
		}
		if err := db.Delete(&item).Error; err != nil {
			response.ErrorInternal(c, "删除失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// ---- 工地 ----

func listProjects(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var items []Project
		ac := loadAccess(db, currentUID(c))
		q := db.Where("user_id = ?", ac.scopeUser())
		if c.Query("status") == "active" {
			q = q.Where("status = ?", "active")
		}
		if err := q.Order("status ASC, name ASC").Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		response.Success(c, items)
	}
}

func createProject(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireManage(loadAccess(db, currentUID(c))) {
			forbidden(c)
			return
		}
		var in Project
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			response.ErrorBadRequest(c, "请填写名称")
			return
		}
		in.ID = 0
		in.UserID = loadAccess(db, currentUID(c)).scopeUser()
		in.Name = strings.TrimSpace(in.Name)
		if in.Status == "" {
			in.Status = "active"
		}
		if err := db.Create(&in).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, in)
	}
}

func updateProject(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		if !requireManage(loadAccess(db, uid)) {
			forbidden(c)
			return
		}
		uid = loadAccess(db, uid).scopeUser()
		var item Project
		if err := db.Where("user_id = ? AND id = ?", uid, c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "工地不存在")
			return
		}
		var in Project
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			response.ErrorBadRequest(c, "请填写名称")
			return
		}
		item.Name = strings.TrimSpace(in.Name)
		item.Note = in.Note
		if in.Status == "active" || in.Status == "archived" {
			item.Status = in.Status
		}
		if err := db.Save(&item).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, item)
	}
}

func deleteProject(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		if !requireManage(loadAccess(db, uid)) {
			forbidden(c)
			return
		}
		uid = loadAccess(db, uid).scopeUser()
		res := db.Where("user_id = ? AND id = ?", uid, c.Param("id")).Delete(&Project{})
		if res.Error != nil {
			response.ErrorInternal(c, "删除失败")
			return
		}
		if res.RowsAffected == 0 {
			notFound(c, "工地不存在")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// ---- 计件项目 ----

func listPieceItems(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		var items []PieceItem
		ac := loadAccess(db, currentUID(c))
		q := db.Where("user_id = ?", ac.scopeUser())
		if c.Query("status") == "active" {
			q = q.Where("status = ?", "active")
		}
		if err := q.Order("status ASC, name ASC").Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		response.Success(c, items)
	}
}

func createPieceItem(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireManage(loadAccess(db, currentUID(c))) {
			forbidden(c)
			return
		}
		var in PieceItem
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			response.ErrorBadRequest(c, "请填写名称")
			return
		}
		in.ID = 0
		in.UserID = currentUID(c)
		in.Name = strings.TrimSpace(in.Name)
		if in.Unit == "" {
			in.Unit = "个"
		}
		if in.UnitPriceCents <= 0 {
			response.ErrorBadRequest(c, "请填写大于 0 的单价")
			return
		}
		if in.Status == "" {
			in.Status = "active"
		}
		if err := db.Create(&in).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, in)
	}
}

func updatePieceItem(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		if !requireManage(loadAccess(db, uid)) {
			forbidden(c)
			return
		}
		uid = loadAccess(db, uid).scopeUser()
		var item PieceItem
		if err := db.Where("user_id = ? AND id = ?", uid, c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "计件项目不存在")
			return
		}
		var in PieceItem
		if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Name) == "" {
			response.ErrorBadRequest(c, "请填写名称")
			return
		}
		item.Name = strings.TrimSpace(in.Name)
		item.Unit = in.Unit
		if item.Unit == "" {
			item.Unit = "个"
		}
		if in.UnitPriceCents <= 0 {
			response.ErrorBadRequest(c, "请填写大于 0 的单价")
			return
		}
		item.UnitPriceCents = in.UnitPriceCents
		item.Note = in.Note
		if in.Status == "active" || in.Status == "archived" {
			item.Status = in.Status
		}
		if err := db.Save(&item).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, item)
	}
}

func deletePieceItem(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		if !requireManage(loadAccess(db, uid)) {
			forbidden(c)
			return
		}
		uid = loadAccess(db, uid).scopeUser()
		res := db.Where("user_id = ? AND id = ?", uid, c.Param("id")).Delete(&PieceItem{})
		if res.Error != nil {
			response.ErrorInternal(c, "删除失败")
			return
		}
		if res.RowsAffected == 0 {
			notFound(c, "计件项目不存在")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// ---- 记工 ----

type recordInput struct {
	WorkerID       uint    `json:"worker_id"`
	ProjectID      *uint   `json:"project_id"`
	PieceItemID    *uint   `json:"piece_item_id"`
	Date           string  `json:"date"`
	Type           string  `json:"type"`
	Quantity       string  `json:"quantity"`
	UnitPriceCents *int64  `json:"unit_price_cents"`
	Note           string  `json:"note"`
}

// validateRecordInput 校验并填充记录字段（不含 UserID/ID）。
func validateRecordInput(db *gorm.DB, uid uint, in *recordInput, rec *WorkRecord) error {
	if !validDate(in.Date) {
		return fmt.Errorf("日期格式应为 YYYY-MM-DD")
	}
	if in.Type != TypeDay && in.Type != TypeHour && in.Type != TypePiece && in.Type != TypeRest {
		return fmt.Errorf("记工类型不正确")
	}
	var worker Worker
	if err := db.Where("user_id = ? AND id = ?", uid, in.WorkerID).First(&worker).Error; err != nil {
		return fmt.Errorf("工人不存在")
	}
	if worker.Status != "active" {
		return fmt.Errorf("工人已归档，请先恢复")
	}

	rec.WorkerID = worker.ID
	rec.Date = in.Date
	rec.Type = in.Type
	rec.Note = in.Note
	rec.ProjectID = nil
	rec.PieceItemID = nil

	// 休息：不计钱
	if in.Type == TypeRest {
		rec.Quantity = "0"
		rec.UnitPriceCents = 0
		rec.AmountCents = 0
		if in.ProjectID != nil && *in.ProjectID > 0 {
			var n int64
			db.Model(&Project{}).Where("user_id = ? AND id = ?", uid, *in.ProjectID).Count(&n)
			if n == 0 {
				return fmt.Errorf("工地不存在")
			}
			rec.ProjectID = in.ProjectID
		}
		return nil
	}

	q, err := parseQuantity(in.Quantity)
	if err != nil {
		return err
	}
	if !quantityMultipleOK(in.Type, q) {
		switch in.Type {
		case TypeDay:
			return fmt.Errorf("点工数量需为 0.5 的整数倍")
		case TypeHour:
			return fmt.Errorf("点时数量需为 0.25 的整数倍")
		default:
			return fmt.Errorf("计件数量需为整数")
		}
	}

	// 计件项目先解析：既做存在性校验，也是计件单价的默认来源。
	var pieceItem PieceItem
	if in.Type == TypePiece {
		if in.PieceItemID == nil || *in.PieceItemID <= 0 {
			return fmt.Errorf("计件记录需选择计件项目")
		}
		if err := db.Where("user_id = ? AND id = ?", uid, *in.PieceItemID).First(&pieceItem).Error; err != nil {
			return fmt.Errorf("计件项目不存在")
		}
	}

	// 单价：未传则带默认值（工人日薪/时薪或计件项目单价）
	price := int64(0)
	if in.UnitPriceCents != nil {
		price = *in.UnitPriceCents
	} else {
		switch in.Type {
		case TypeDay:
			price = worker.DailyWageCents
		case TypeHour:
			price = worker.HourlyWageCents
		case TypePiece:
			price = pieceItem.UnitPriceCents
		}
	}
	if price <= 0 {
		return fmt.Errorf("请填写大于 0 的单价")
	}

	rec.Quantity = normalizeQuantity(q)
	rec.UnitPriceCents = price
	rec.AmountCents = computeAmountCents(q, price)

	if in.ProjectID != nil && *in.ProjectID > 0 {
		var n int64
		db.Model(&Project{}).Where("user_id = ? AND id = ?", uid, *in.ProjectID).Count(&n)
		if n == 0 {
			return fmt.Errorf("工地不存在")
		}
		rec.ProjectID = in.ProjectID
	}
	if in.Type == TypePiece {
		rec.PieceItemID = in.PieceItemID
	}
	return nil
}

// dayWorkerRow 出勤日视图的每一行：工人 + 当日记录与小计。
type dayWorkerRow struct {
	Worker
	Records      []WorkRecord `json:"records"`
	AmountCents  int64        `json:"amount_cents"`
	Days         string       `json:"days"`
	Hours        string       `json:"hours"`
	Pieces       string       `json:"pieces"`
	RecordCount  int          `json:"record_count"`
}

func recordsByDate(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()
		date := c.Query("date")
		if !validDate(date) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		// 当日出过工的已归档工人也要展示出来
		var touchedIDs []uint
		db.Model(&WorkRecord{}).Select("DISTINCT worker_id").
			Where("user_id = ? AND date = ?", dsUID, date).Scan(&touchedIDs)

		workerQ := db.Where("user_id = ? AND (status = ? OR id IN ?)", dsUID, "active", touchedIDs)
		if sw, ok := ac.workerScope(); ok {
			workerQ = db.Where("id = ?", sw)
		}
		var workers []Worker
		if err := workerQ.Order("status DESC, name ASC").Find(&workers).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		var records []WorkRecord
		db.Where("user_id = ? AND date = ?", dsUID, date).Order("created_at ASC").Find(&records)
		fillRecordNames(db, records)

		byWorker := map[uint][]WorkRecord{}
		for _, r := range records {
			byWorker[r.WorkerID] = append(byWorker[r.WorkerID], r)
		}
		rows := make([]dayWorkerRow, 0, len(workers))
		for _, w := range workers {
			recs := byWorker[w.ID]
			if recs == nil {
				recs = []WorkRecord{}
			}
			row := dayWorkerRow{Worker: w, Records: recs}
			var days, hours, pieces float64
			for _, r := range row.Records {
				row.AmountCents += r.AmountCents
				row.RecordCount++
				switch r.Type {
				case TypeDay:
					days += parseFloat(r.Quantity)
				case TypeHour:
					hours += parseFloat(r.Quantity)
				case TypePiece:
					pieces += parseFloat(r.Quantity)
				}
			}
			row.Days = trimFloat(days)
			row.Hours = trimFloat(hours)
			row.Pieces = trimFloat(pieces)
			rows = append(rows, row)
		}
		response.Success(c, gin.H{"date": date, "workers": rows})
	}
}

func listRecords(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		q := db.Model(&WorkRecord{}).Where("work_records.user_id = ?", ac.scopeUser())
		if sw, ok := ac.workerScope(); ok {
			q = q.Where("worker_id = ?", sw)
		} else if v := c.Query("worker_id"); v != "" {
			q = q.Where("worker_id = ?", v)
		}
		if v := c.Query("project_id"); v != "" {
			q = q.Where("project_id = ?", v)
		}
		if v := c.Query("type"); v != "" {
			q = q.Where("type = ?", v)
		}
		if v := c.Query("settled"); v == "0" {
			q = q.Where("settlement_id IS NULL")
		} else if v == "1" {
			q = q.Where("settlement_id IS NOT NULL")
		}
		if v := c.Query("start"); validDate(v) {
			q = q.Where("date >= ?", v)
		}
		if v := c.Query("end"); validDate(v) {
			q = q.Where("date <= ?", v)
		}
		var total int64
		q.Count(&total)
		page, pageSize := utils.NormalizePage(utils.Atoi(c.Query("page"), 1), utils.Atoi(c.Query("page_size"), 20))
		var items []WorkRecord
		if err := q.Order("date DESC, id DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		fillRecordNames(db, items)
		response.SuccessPage(c, items, total, page, pageSize)
	}
}

func fillRecordNames(db *gorm.DB, records []WorkRecord) {
	if len(records) == 0 {
		return
	}
	var workerIDs, projectIDs, pieceIDs []uint
	for _, r := range records {
		workerIDs = append(workerIDs, r.WorkerID)
		if r.ProjectID != nil {
			projectIDs = append(projectIDs, *r.ProjectID)
		}
		if r.PieceItemID != nil {
			pieceIDs = append(pieceIDs, *r.PieceItemID)
		}
	}
	workerNames := map[uint]string{}
	var workers []Worker
	db.Where("id IN ?", workerIDs).Find(&workers)
	for _, w := range workers {
		workerNames[w.ID] = w.Name
	}
	projectNames := map[uint]string{}
	if len(projectIDs) > 0 {
		var projects []Project
		db.Where("id IN ?", projectIDs).Find(&projects)
		for _, p := range projects {
			projectNames[p.ID] = p.Name
		}
	}
	pieceInfo := map[uint]*PieceItem{}
	if len(pieceIDs) > 0 {
		var pieces []PieceItem
		db.Where("id IN ?", pieceIDs).Find(&pieces)
		for i := range pieces {
			pieceInfo[pieces[i].ID] = &pieces[i]
		}
	}
	for i := range records {
		records[i].WorkerName = workerNames[records[i].WorkerID]
		if records[i].ProjectID != nil {
			records[i].ProjectName = projectNames[*records[i].ProjectID]
		}
		if records[i].PieceItemID != nil {
			if p := pieceInfo[*records[i].PieceItemID]; p != nil {
				records[i].PieceName = p.Name
				records[i].PieceUnit = p.Unit
			}
		}
	}
}

func createRecord(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		var in recordInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if sw, ok := ac.workerScope(); ok {
			in.WorkerID = sw // 员工只能给自己记工
		}
		dsUID := ac.scopeUser()
		var rec WorkRecord
		if err := validateRecordInput(db, dsUID, &in, &rec); err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		rec.UserID = dsUID
		rec.CreatedBy = uid
		if err := db.Create(&rec).Error; err != nil {
			response.ErrorInternal(c, "保存失败: "+err.Error())
			return
		}
		fillRecordNames(db, []WorkRecord{rec})
		response.Success(c, rec)
	}
}

func updateRecord(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()
		var rec WorkRecord
		if err := db.Where("user_id = ? AND id = ?", dsUID, c.Param("id")).First(&rec).Error; err != nil {
			notFound(c, "记录不存在")
			return
		}
		if sw, ok := ac.workerScope(); ok && rec.WorkerID != sw {
			forbidden(c)
			return
		}
		if rec.SettlementID != nil {
			response.ErrorBadRequest(c, "该记录已结算，不能修改")
			return
		}
		var in recordInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if sw, ok := ac.workerScope(); ok {
			in.WorkerID = sw
		}
		if err := validateRecordInput(db, dsUID, &in, &rec); err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		if err := db.Save(&rec).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		fillRecordNames(db, []WorkRecord{rec})
		response.Success(c, rec)
	}
}

func deleteRecord(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		dsUID := ac.scopeUser()
		var rec WorkRecord
		if err := db.Where("user_id = ? AND id = ?", dsUID, c.Param("id")).First(&rec).Error; err != nil {
			notFound(c, "记录不存在")
			return
		}
		if sw, ok := ac.workerScope(); ok && rec.WorkerID != sw {
			forbidden(c)
			return
		}
		if rec.SettlementID != nil {
			response.ErrorBadRequest(c, "该记录已结算，不能删除；如需调整请先撤销结算")
			return
		}
		if err := db.Delete(&rec).Error; err != nil {
			response.ErrorInternal(c, "删除失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

// ---- 借支 ----

type advanceInput struct {
	WorkerID    uint   `json:"worker_id"`
	Date        string `json:"date"`
	AmountCents int64  `json:"amount_cents"`
	Method      string `json:"method"`
	Note        string `json:"note"`
}

func listAdvances(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		q := db.Model(&Advance{}).Where("advances.user_id = ?", ac.scopeUser())
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
		var total int64
		q.Count(&total)
		page, pageSize := utils.NormalizePage(utils.Atoi(c.Query("page"), 1), utils.Atoi(c.Query("page_size"), 20))
		var items []Advance
		if err := q.Order("date DESC, id DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		fillAdvanceNames(db, items)
		response.SuccessPage(c, items, total, page, pageSize)
	}
}

func fillAdvanceNames(db *gorm.DB, items []Advance) {
	if len(items) == 0 {
		return
	}
	ids := make([]uint, 0, len(items))
	for _, a := range items {
		ids = append(ids, a.WorkerID)
	}
	var workers []Worker
	db.Where("id IN ?", ids).Find(&workers)
	names := map[uint]string{}
	for _, w := range workers {
		names[w.ID] = w.Name
	}
	for i := range items {
		items[i].WorkerName = names[items[i].WorkerID]
	}
}

func createAdvance(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !requireManage(ac) {
			forbidden(c)
			return
		}
		uid = ac.scopeUser()
		var in advanceInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if !validDate(in.Date) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		if in.AmountCents <= 0 {
			response.ErrorBadRequest(c, "借支金额需大于 0")
			return
		}
		var worker Worker
		if err := db.Where("user_id = ? AND id = ?", uid, in.WorkerID).First(&worker).Error; err != nil {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}
		if in.Method == "" {
			in.Method = "cash"
		}
		item := Advance{
			UserID: uid, WorkerID: in.WorkerID, Date: in.Date,
			AmountCents: in.AmountCents, Method: in.Method, Note: in.Note,
			CreatedBy: currentUID(c),
		}
		if err := db.Create(&item).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		fillAdvanceNames(db, []Advance{item})
		response.Success(c, item)
	}
}

func deleteAdvance(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := currentUID(c)
		ac := loadAccess(db, uid)
		if !requireManage(ac) {
			forbidden(c)
			return
		}
		uid = ac.scopeUser()
		var item Advance
		if err := db.Where("user_id = ? AND id = ?", uid, c.Param("id")).First(&item).Error; err != nil {
			notFound(c, "借支记录不存在")
			return
		}
		if item.SettlementID != nil {
			response.ErrorBadRequest(c, "该借支已结算，不能删除；如需调整请先撤销结算")
			return
		}
		if err := db.Delete(&item).Error; err != nil {
			response.ErrorInternal(c, "删除失败")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}
