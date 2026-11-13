package worklog

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
	"smallgo/server/sysconfig"
	"smallgo/server/utils"
)

// setupPublicRoutes 注册免登录报工接口（public 组，仅靠报工码令牌识别）。
func setupPublicRoutes(api *gin.RouterGroup, db *gorm.DB) {
	g := api.Group("/worklog/public")
	g.GET("/entry/:token", publicEntryInfo(db))
	g.POST("/entry/:token/records", publicEntrySubmit(db))
}

// ---- 公开报工（免登录，工人扫码用）----

type publicProduct struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Unit string `json:"unit"`
}

type publicWorker struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

// publicEntryInfo 扫码后拿到的报工上下文。只暴露姓名/产品，不暴露工资与单价。
func publicEntryInfo(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		code, err := loadPublicEntryCode(db, c.Param("token"))
		if err != nil {
			response.Error(c, 404, response.CodeNotFound, err.Error())
			return
		}
		owner := code.UserID

		products := make([]publicProduct, 0)
		var boundProduct *publicProduct
		if code.PieceItemID != nil {
			var p PieceItem
			if err := db.Where("user_id = ? AND id = ?", owner, *code.PieceItemID).First(&p).Error; err != nil || p.Status != "active" {
				response.Error(c, 404, response.CodeNotFound, "报工码对应的产品已停用，请联系班组长")
				return
			}
			item := publicProduct{ID: p.ID, Name: p.Name, Unit: p.Unit}
			boundProduct = &item
		} else {
			var rows []PieceItem
			db.Where("user_id = ? AND status = ?", owner, "active").Order("name ASC, id ASC").Limit(100).Find(&rows)
			for _, p := range rows {
				products = append(products, publicProduct{ID: p.ID, Name: p.Name, Unit: p.Unit})
			}
			if len(products) == 0 {
				response.Error(c, 400, response.CodeBadRequest, "还没有可报工的计件项目，请联系班组长")
				return
			}
		}

		workers := make([]publicWorker, 0)
		var boundWorker *publicWorker
		if code.WorkerID != nil {
			var w Worker
			if err := db.Where("user_id = ? AND id = ?", owner, *code.WorkerID).First(&w).Error; err != nil || w.Status != "active" {
				response.Error(c, 404, response.CodeNotFound, "报工码对应的工人已停用，请联系班组长")
				return
			}
			item := publicWorker{ID: w.ID, Name: w.Name}
			boundWorker = &item
		} else {
			var rows []Worker
			db.Where("user_id = ? AND status = ?", owner, "active").Order("name ASC, id ASC").Limit(200).Find(&rows)
			for _, w := range rows {
				workers = append(workers, publicWorker{ID: w.ID, Name: w.Name})
			}
			if len(workers) == 0 {
				response.Error(c, 400, response.CodeBadRequest, "还没有工人，请联系班组长")
				return
			}
		}

		// 工人专属码链接带 ?w=工人ID：产品码 + 工人预选，扫码后只剩数量一个输入框。
		selected := boundWorker
		if selected == nil {
			if raw := strings.TrimSpace(c.Query("w")); raw != "" {
				wid := uint(atoiDefault(raw, 0))
				if wid > 0 {
					for i := range workers {
						if workers[i].ID == wid {
							w := workers[i]
							selected = &w
							break
						}
					}
				}
			}
		}

		siteTitle, _ := sysconfig.GetConfig(db, "site_title", 0)
		today := time.Now().Format("2006-01-02")
		response.Success(c, gin.H{
			"site_title":      siteTitle,
			"code":            gin.H{"name": code.Name, "auto_approve": code.AutoApprove, "note": code.Note},
			"product":         boundProduct,
			"products":        products,
			"worker":          selected,
			"worker_bound":    code.WorkerID != nil,
			"workers":         workers,
			"today":           today,
			"yesterday":       addDateString(today, -1),
			"allow_yesterday": true,
		})
	}
}

// loadPublicEntryCode 按令牌取启用中的报工码；不存在/停用/过期都返回统一的 404 文案。
func loadPublicEntryCode(db *gorm.DB, token string) (*EntryCode, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 64 {
		return nil, fmt.Errorf("报工码无效")
	}
	var code EntryCode
	if err := db.Where("token = ?", token).First(&code).Error; err != nil {
		return nil, fmt.Errorf("报工码无效或已删除")
	}
	if !code.Enabled {
		return nil, fmt.Errorf("报工码已停用，请联系班组长")
	}
	if entryCodeExpired(&code) {
		return nil, fmt.Errorf("报工码已过期，请联系班组长")
	}
	return &code, nil
}

func addDateString(date string, days int) string {
	t, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		return date
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

type publicSubmitInput struct {
	WorkerID    *uint  `json:"worker_id"`
	PieceItemID *uint  `json:"piece_item_id"`
	Quantity    string `json:"quantity"`
	Date        string `json:"date"`
	Note        string `json:"note"`
}

// publicEntrySubmit 匿名提交一条计件报工。默认进待审核；报工码勾选「直接生效」时直接通过。
func publicEntrySubmit(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		code, err := loadPublicEntryCode(db, c.Param("token"))
		if err != nil {
			response.Error(c, 404, response.CodeNotFound, err.Error())
			return
		}
		var in publicSubmitInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		owner := code.UserID

		workerID := uint(0)
		if code.WorkerID != nil {
			workerID = *code.WorkerID
		} else if in.WorkerID != nil {
			workerID = *in.WorkerID
		}
		var worker Worker
		if workerID == 0 {
			response.ErrorBadRequest(c, "请选择姓名")
			return
		}
		if err := db.Where("user_id = ? AND id = ?", owner, workerID).First(&worker).Error; err != nil {
			response.ErrorBadRequest(c, "工人不存在")
			return
		}
		if worker.Status != "active" {
			response.ErrorBadRequest(c, "该工人已归档，请联系班组长")
			return
		}

		pieceID := uint(0)
		if code.PieceItemID != nil {
			pieceID = *code.PieceItemID
		} else if in.PieceItemID != nil {
			pieceID = *in.PieceItemID
		}
		var piece PieceItem
		if pieceID == 0 {
			response.ErrorBadRequest(c, "请选择产品")
			return
		}
		if err := db.Where("user_id = ? AND id = ?", owner, pieceID).First(&piece).Error; err != nil {
			response.ErrorBadRequest(c, "产品不存在")
			return
		}
		if piece.Status != "active" {
			response.ErrorBadRequest(c, "该产品已停用，请联系班组长")
			return
		}

		date := strings.TrimSpace(in.Date)
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		today := time.Now().Format("2006-01-02")
		if date != today && date != addDateString(today, -1) {
			response.ErrorBadRequest(c, "只能补报今天或昨天的数量")
			return
		}

		q, err := parseQuantity(strings.TrimSpace(in.Quantity))
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		if !quantityMultipleOK(TypePiece, q) {
			response.ErrorBadRequest(c, "计件数量需为整数")
			return
		}
		if q.Sign() <= 0 {
			response.ErrorBadRequest(c, "请填写数量")
			return
		}
		if q.Cmp(bigRatFromInt(1000000)) > 0 {
			response.ErrorBadRequest(c, "数量过大，请核对后重填")
			return
		}
		qty := normalizeQuantity(q)
		price := piece.UnitPriceCents
		if price <= 0 {
			response.ErrorBadRequest(c, "该产品还没有设置单价，请联系班组长")
			return
		}
		if price > maxUnitPriceCents {
			response.ErrorBadRequest(c, "该产品单价异常，请联系班组长")
			return
		}

		// 防连点：15 秒内同码、同人、同产品、同数量的待审记录直接视为重复提交。
		var dup WorkRecord
		err = db.Where("user_id = ? AND entry_code_id = ? AND worker_id = ? AND piece_item_id = ? AND date = ? AND quantity = ? AND status <> ? AND created_at >= ?",
			owner, code.ID, workerID, pieceID, date, qty, RecordStatusRejected, time.Now().Add(-15*time.Second)).First(&dup).Error
		if err == nil {
			fillRecordNames(db, []WorkRecord{dup})
			response.Success(c, publicSubmitResult(db, &dup, true, "刚才已提交过，请勿重复点击", dup.Date))
			return
		}

		status := RecordStatusPending
		reviewNote := ""
		var reviewedAt *time.Time
		if code.AutoApprove {
			status = RecordStatusApproved
			reviewNote = "报工码自动通过"
			now := time.Now()
			reviewedAt = &now
		}
		rec := WorkRecord{
			UserID: owner, WorkerID: workerID, PieceItemID: &pieceID, ProjectID: code.ProjectID,
			Date: date, Type: TypePiece, Quantity: qty, UnitPriceCents: price,
			AmountCents: computeAmountCents(q, price), Note: limitRunes(strings.TrimSpace(in.Note), 80),
			Status: status, Source: RecordSourceQR, EntryCodeID: &code.ID, EntryCodeName: code.Name,
			SubmitterName: worker.Name, ReviewedAt: reviewedAt, ReviewNote: reviewNote,
		}
		if err := db.Create(&rec).Error; err != nil {
			response.ErrorInternal(c, "提交失败，请稍后重试")
			return
		}
		fillRecordNames(db, []WorkRecord{rec})
		msg := "已提交，等班组长审核"
		if status == RecordStatusApproved {
			msg = "已记录"
		}
		response.Success(c, publicSubmitResult(db, &rec, false, msg, date))
	}
}

// publicSubmitResult 提交结果 + 该工人当日已报笔数/明细，供工人页面即时回显。
func publicSubmitResult(db *gorm.DB, rec *WorkRecord, duplicate bool, message, date string) gin.H {
	// 只回显同一报工码、同一工人当天的记录：不把工人其他产品/其他码的产量暴露给持码人。
	q := func() *gorm.DB {
		x := db.Model(&WorkRecord{}).Where("user_id = ? AND worker_id = ? AND date = ? AND status <> ?", rec.UserID, rec.WorkerID, date, RecordStatusRejected)
		if rec.EntryCodeID != nil {
			x = x.Where("entry_code_id = ?", *rec.EntryCodeID)
		}
		return x
	}
	var todayCount int64
	q().Count(&todayCount)
	var rows []WorkRecord
	q().Order("created_at DESC, id DESC").Limit(10).Find(&rows)
	fillRecordNames(db, rows)
	items := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		items = append(items, gin.H{
			"id": r.ID, "piece_name": r.PieceName, "piece_unit": r.PieceUnit,
			"quantity": r.Quantity, "status": r.Status, "created_at": r.CreatedAt,
		})
	}
	return gin.H{
		// 只回显工人需要确认的字段：不返回单价/金额，避免持有报工码的人反推工价。
		"record":    gin.H{"id": rec.ID, "status": rec.Status, "quantity": rec.Quantity, "date": rec.Date},
		"duplicate": duplicate, "message": message,
		"status": rec.Status, "date": date, "today_count": todayCount, "today_items": items,
	}
}

func limitRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// bigRatFromInt 把整数转成 big.Rat，复用统一的金额计算/数量上限判断。
func bigRatFromInt(n int64) *big.Rat {
	return new(big.Rat).SetInt64(n)
}

// ---- 审核 ----

func reviewSummary(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		owner := ac.scopeUser()
		today := time.Now().Format("2006-01-02")
		var out struct {
			PendingCount       int64 `json:"pending_count"`
			PendingAmountCents int64 `json:"pending_amount_cents"`
			TodayPendingCount  int64 `json:"today_pending_count"`
			TodayPendingCents  int64 `json:"today_pending_amount_cents"`
			TodayApprovedCount int64 `json:"today_approved_count"`
			TodayApprovedCents int64 `json:"today_approved_amount_cents"`
			RejectedCount      int64 `json:"rejected_count"`
		}
		db.Model(&WorkRecord{}).Where("user_id = ? AND status = ?", owner, RecordStatusPending).Count(&out.PendingCount)
		db.Model(&WorkRecord{}).Where("user_id = ? AND status = ?", owner, RecordStatusPending).Select("COALESCE(SUM(amount_cents),0)").Scan(&out.PendingAmountCents)
		db.Model(&WorkRecord{}).Where("user_id = ? AND status = ? AND date = ?", owner, RecordStatusPending, today).Count(&out.TodayPendingCount)
		db.Model(&WorkRecord{}).Where("user_id = ? AND status = ? AND date = ?", owner, RecordStatusPending, today).Select("COALESCE(SUM(amount_cents),0)").Scan(&out.TodayPendingCents)
		db.Model(&WorkRecord{}).Where("user_id = ? AND status = ? AND date = ?", owner, RecordStatusApproved, today).Count(&out.TodayApprovedCount)
		db.Model(&WorkRecord{}).Where("user_id = ? AND status = ? AND date = ?", owner, RecordStatusApproved, today).Select("COALESCE(SUM(amount_cents),0)").Scan(&out.TodayApprovedCents)
		db.Model(&WorkRecord{}).Where("user_id = ? AND status = ?", owner, RecordStatusRejected).Count(&out.RejectedCount)
		response.Success(c, out)
	}
}

func listReviewRecords(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		owner := ac.scopeUser()
		status := c.DefaultQuery("status", RecordStatusPending)
		if status != RecordStatusPending && status != RecordStatusApproved && status != RecordStatusRejected && status != "all" {
			response.ErrorBadRequest(c, "status 参数不正确")
			return
		}
		q := db.Model(&WorkRecord{}).Where("work_records.user_id = ?", owner)
		if status != "all" {
			q = q.Where("work_records.status = ?", status)
		}
		if v := c.Query("date"); validDate(v) {
			q = q.Where("date = ?", v)
		}
		if v := c.Query("start"); validDate(v) {
			q = q.Where("date >= ?", v)
		}
		if v := c.Query("end"); validDate(v) {
			q = q.Where("date <= ?", v)
		}
		if v := c.Query("worker_id"); v != "" {
			q = q.Where("worker_id = ?", v)
		}
		if v := c.Query("project_id"); v != "" {
			q = q.Where("project_id = ?", v)
		}
		if v := c.Query("entry_code_id"); v != "" {
			q = q.Where("entry_code_id = ?", v)
		}
		var total int64
		q.Count(&total)
		page, pageSize := utils.NormalizePage(utils.Atoi(c.Query("page"), 1), utils.Atoi(c.Query("page_size"), 20))
		var items []WorkRecord
		if err := q.Order("date DESC, created_at DESC, id DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&items).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		fillRecordNames(db, items)
		fillReviewerNames(db, items)
		response.SuccessPage(c, items, total, page, pageSize)
	}
}

func fillReviewerNames(db *gorm.DB, records []WorkRecord) {
	ids := make([]uint, 0, len(records))
	for _, r := range records {
		if r.ReviewedBy != nil {
			ids = append(ids, *r.ReviewedBy)
		}
	}
	if len(ids) == 0 {
		return
	}
	var users []struct {
		ID       uint
		Username string
	}
	db.Table("users").Select("id, username").Where("id IN ?", ids).Scan(&users)
	names := make(map[uint]string, len(users))
	for _, u := range users {
		names[u.ID] = u.Username
	}
	for i := range records {
		if records[i].ReviewedBy != nil {
			records[i].ReviewerName = names[*records[i].ReviewedBy]
		}
	}
}

type reviewIDsInput struct {
	IDs  []uint `json:"ids"`
	Note string `json:"note"`
}

func approveRecords(db *gorm.DB) gin.HandlerFunc {
	return updateReviewStatus(db, RecordStatusApproved)
}

func rejectRecords(db *gorm.DB) gin.HandlerFunc {
	return updateReviewStatus(db, RecordStatusRejected)
}

func updateReviewStatus(db *gorm.DB, target string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		owner := ac.scopeUser()
		var in reviewIDsInput
		if err := c.ShouldBindJSON(&in); err != nil || len(in.IDs) == 0 {
			response.ErrorBadRequest(c, "请选择要审核的记工")
			return
		}
		if len(in.IDs) > maxReviewBatch {
			response.ErrorBadRequest(c, "一次最多审核 200 条")
			return
		}
		seen := make(map[uint]bool, len(in.IDs))
		ids := make([]uint, 0, len(in.IDs))
		for _, id := range in.IDs {
			if id > 0 && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
		if len(ids) == 0 {
			response.ErrorBadRequest(c, "请选择要审核的记工")
			return
		}
		note := strings.TrimSpace(in.Note)
		if target == RecordStatusRejected && note == "" {
			note = "不符合报工要求"
		}
		note = limitRunes(note, 120)
		updated := int64(0)
		err := db.Transaction(func(tx *gorm.DB) error {
			var recs []WorkRecord
			if err := tx.Where("user_id = ? AND id IN ?", owner, ids).Find(&recs).Error; err != nil {
				return err
			}
			if len(recs) != len(ids) {
				return fmt.Errorf("部分记录不存在，请刷新后重试")
			}
			for _, r := range recs {
				if r.Status != RecordStatusPending {
					return fmt.Errorf("部分记录已被处理，请刷新后重试")
				}
				if r.SettlementID != nil {
					return fmt.Errorf("部分记录已结算，不能审核")
				}
			}
			res := tx.Model(&WorkRecord{}).Where("user_id = ? AND id IN ? AND status = ?", owner, ids, RecordStatusPending).
				Updates(map[string]interface{}{
					"status": target, "reviewed_by": currentUID(c), "reviewed_at": time.Now(), "review_note": note,
				})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != int64(len(ids)) {
				return fmt.Errorf("部分记录已被处理，请刷新后重试")
			}
			updated = res.RowsAffected
			return nil
		})
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		response.Success(c, gin.H{"updated": updated, "status": target})
	}
}

// ---- 批量报工（班组长代录整班产量）----

type batchRecordRow struct {
	WorkerID uint   `json:"worker_id"`
	Quantity string `json:"quantity"`
}

type batchRecordInput struct {
	Date        string           `json:"date"`
	PieceItemID uint             `json:"piece_item_id"`
	ProjectID   *uint            `json:"project_id"`
	Note        string           `json:"note"`
	Rows        []batchRecordRow `json:"rows"`
}

func batchCreateRecords(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		owner := ac.scopeUser()
		var in batchRecordInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		if !validDate(in.Date) {
			response.ErrorBadRequest(c, "日期格式应为 YYYY-MM-DD")
			return
		}
		if in.PieceItemID == 0 {
			response.ErrorBadRequest(c, "请选择计件项目")
			return
		}
		var piece PieceItem
		if err := db.Where("user_id = ? AND id = ?", owner, in.PieceItemID).First(&piece).Error; err != nil {
			response.ErrorBadRequest(c, "计件项目不存在")
			return
		}
		if piece.Status != "active" {
			response.ErrorBadRequest(c, "计件项目已归档，请先恢复")
			return
		}
		if piece.UnitPriceCents <= 0 {
			response.ErrorBadRequest(c, "该计件项目还没有设置单价")
			return
		}
		if piece.UnitPriceCents > maxUnitPriceCents {
			response.ErrorBadRequest(c, "该计件项目单价异常，请先修正")
			return
		}
		projectID := in.ProjectID
		if projectID != nil && *projectID == 0 {
			projectID = nil
		}
		if projectID != nil {
			var p Project
			if err := db.Where("user_id = ? AND id = ?", owner, *projectID).First(&p).Error; err != nil {
				response.ErrorBadRequest(c, "工地不存在")
				return
			}
		}
		if len(in.Rows) == 0 {
			response.ErrorBadRequest(c, "请填写至少一名工人的数量")
			return
		}
		if len(in.Rows) > maxBatchRows {
			response.ErrorBadRequest(c, "一次最多 200 人")
			return
		}
		type prepared struct {
			workerID uint
			qty      string
			amount   int64
		}
		preparedRows := make([]prepared, 0, len(in.Rows))
		seen := make(map[uint]bool, len(in.Rows))
		for i, row := range in.Rows {
			if row.WorkerID == 0 {
				response.ErrorBadRequest(c, fmt.Sprintf("第 %d 行缺少工人，请刷新后重试", i+1))
				return
			}
			if seen[row.WorkerID] {
				response.ErrorBadRequest(c, "同一工人重复填写，请合并为一条")
				return
			}
			seen[row.WorkerID] = true
			raw := strings.TrimSpace(row.Quantity)
			// 空白或 0 视为本次不报；有内容但不是合法正整数时整批拒绝，
			// 避免静默跳过导致「提示报了 N 人、实际少记」。
			if raw == "" || raw == "0" {
				continue
			}
			q, err := parseQuantity(raw)
			if err != nil || !quantityMultipleOK(TypePiece, q) || q.Sign() <= 0 {
				response.ErrorBadRequest(c, fmt.Sprintf("第 %d 行数量需为大于 0 的整数", i+1))
				return
			}
			if q.Cmp(bigRatFromInt(1000000)) > 0 {
				response.ErrorBadRequest(c, fmt.Sprintf("第 %d 行数量过大，请核对", i+1))
				return
			}
			preparedRows = append(preparedRows, prepared{workerID: row.WorkerID, qty: normalizeQuantity(q), amount: computeAmountCents(q, piece.UnitPriceCents)})
		}
		if len(preparedRows) == 0 {
			response.ErrorBadRequest(c, "请填写至少一名工人的数量（计件需为整数）")
			return
		}
		ids := make([]uint, 0, len(preparedRows))
		for _, r := range preparedRows {
			ids = append(ids, r.workerID)
		}
		var workers []Worker
		if err := db.Where("user_id = ? AND id IN ? AND status = ?", owner, ids, "active").Find(&workers).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		if len(workers) != len(ids) {
			response.ErrorBadRequest(c, "有工人不存在或已归档，请刷新后重试")
			return
		}
		// 防双击/网络重试：15 秒内每个工人都有同产品、同日期、同数量的批量记录时，
		// 认定是同一批重复提交，整批拒绝，避免工资翻倍。
		windowStart := time.Now().Add(-15 * time.Second)
		duplicateRows := 0
		for _, r := range preparedRows {
			q := db.Model(&WorkRecord{}).
				Where("user_id = ? AND worker_id = ? AND date = ? AND piece_item_id = ? AND source = ? AND quantity = ? AND created_at >= ?",
					owner, r.workerID, in.Date, in.PieceItemID, RecordSourceBatch, r.qty, windowStart)
			if projectID != nil {
				q = q.Where("project_id = ?", *projectID)
			} else {
				q = q.Where("project_id IS NULL")
			}
			var n int64
			if err := q.Count(&n).Error; err != nil {
				response.ErrorInternal(c, "保存失败")
				return
			}
			if n > 0 {
				duplicateRows++
			}
		}
		if duplicateRows == len(preparedRows) {
			response.ErrorBadRequest(c, "刚才已提交过同一批报工，请勿重复点击")
			return
		}
		note := limitRunes(strings.TrimSpace(in.Note), 120)
		records := make([]WorkRecord, 0, len(preparedRows))
		var totalCents int64
		totalQty := 0.0
		for _, r := range preparedRows {
			records = append(records, WorkRecord{
				UserID: owner, WorkerID: r.workerID, PieceItemID: &in.PieceItemID, ProjectID: projectID,
				Date: in.Date, Type: TypePiece, Quantity: r.qty, UnitPriceCents: piece.UnitPriceCents,
				AmountCents: r.amount, Note: note, Status: RecordStatusApproved, Source: RecordSourceBatch,
				CreatedBy: currentUID(c),
			})
			totalCents += r.amount
			totalQty += parseFloat(r.qty)
		}
		if err := db.Transaction(func(tx *gorm.DB) error { return tx.Create(&records).Error }); err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		response.Success(c, gin.H{"created": len(records), "total_quantity": trimFloat(totalQty), "amount_cents": totalCents, "piece_item_id": piece.ID})
	}
}
