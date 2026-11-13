package worklog

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/response"
)

// entryCodeInput 新建/修改报工码的入参。修改时字段用指针区分「没传」与「清空」：
// 关联 id 传 0 表示清空。
type entryCodeInput struct {
	Name        *string `json:"name"`
	PieceItemID *uint   `json:"piece_item_id"`
	WorkerID    *uint   `json:"worker_id"`
	ProjectID   *uint   `json:"project_id"`
	BaseURL     *string `json:"base_url"`
	AutoApprove *bool   `json:"auto_approve"`
	Enabled     *bool   `json:"enabled"`
	ExpiresAt   *string `json:"expires_at"`
	Note        *string `json:"note"`
}

const (
	maxEntryCodesPerUser = 200
	maxEntryNoteRunes    = 120
	maxBatchRows         = 200
	maxReviewBatch       = 200
)

// genEntryToken 生成 32 位十六进制报工码令牌（不可枚举）。
func genEntryToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeBaseURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	if len(s) > 200 {
		return "", fmt.Errorf("报工地址过长")
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		return "", fmt.Errorf("报工地址需以 http:// 或 https:// 开头")
	}
	return strings.TrimRight(s, "/"), nil
}

func parseEntryExpiry(raw string) (*string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil, nil
	}
	if !validDate(s) {
		return nil, fmt.Errorf("截止日期格式应为 YYYY-MM-DD")
	}
	if s < time.Now().Format("2006-01-02") {
		return nil, fmt.Errorf("截止日期不能早于今天")
	}
	return &s, nil
}

func entryCodeExpired(code *EntryCode) bool {
	if code.ExpiresAt == nil || *code.ExpiresAt == "" {
		return false
	}
	return time.Now().Format("2006-01-02") > *code.ExpiresAt
}

// validateEntryRefs 校验报工码关联的产品/工人/工地属于当前数据集且可用。
func validateEntryRefs(db *gorm.DB, owner uint, pieceID, workerID, projectID *uint) error {
	if pieceID != nil && *pieceID > 0 {
		var p PieceItem
		if err := db.Where("user_id = ? AND id = ?", owner, *pieceID).First(&p).Error; err != nil {
			return fmt.Errorf("计件项目不存在")
		}
		if p.Status != "active" {
			return fmt.Errorf("计件项目已归档，请先恢复")
		}
	}
	if workerID != nil && *workerID > 0 {
		var w Worker
		if err := db.Where("user_id = ? AND id = ?", owner, *workerID).First(&w).Error; err != nil {
			return fmt.Errorf("工人不存在")
		}
		if w.Status != "active" {
			return fmt.Errorf("工人已归档，请先恢复")
		}
	}
	if projectID != nil && *projectID > 0 {
		var p Project
		if err := db.Where("user_id = ? AND id = ?", owner, *projectID).First(&p).Error; err != nil {
			return fmt.Errorf("工地不存在")
		}
	}
	return nil
}

func fillEntryCodeNames(db *gorm.DB, codes []EntryCode) {
	if len(codes) == 0 {
		return
	}
	pieceIDs := make([]uint, 0, len(codes))
	workerIDs := make([]uint, 0, len(codes))
	projectIDs := make([]uint, 0, len(codes))
	for _, c := range codes {
		if c.PieceItemID != nil {
			pieceIDs = append(pieceIDs, *c.PieceItemID)
		}
		if c.WorkerID != nil {
			workerIDs = append(workerIDs, *c.WorkerID)
		}
		if c.ProjectID != nil {
			projectIDs = append(projectIDs, *c.ProjectID)
		}
	}
	pieces := map[uint]*PieceItem{}
	if len(pieceIDs) > 0 {
		var rows []PieceItem
		db.Where("id IN ?", pieceIDs).Find(&rows)
		for i := range rows {
			pieces[rows[i].ID] = &rows[i]
		}
	}
	workers := map[uint]string{}
	if len(workerIDs) > 0 {
		var rows []Worker
		db.Where("id IN ?", workerIDs).Find(&rows)
		for _, w := range rows {
			workers[w.ID] = w.Name
		}
	}
	projects := map[uint]string{}
	if len(projectIDs) > 0 {
		var rows []Project
		db.Where("id IN ?", projectIDs).Find(&rows)
		for _, p := range rows {
			projects[p.ID] = p.Name
		}
	}
	for i := range codes {
		if codes[i].PieceItemID != nil {
			if p := pieces[*codes[i].PieceItemID]; p != nil {
				codes[i].PieceName = p.Name
				codes[i].PieceUnit = p.Unit
			}
		}
		if codes[i].WorkerID != nil {
			codes[i].WorkerName = workers[*codes[i].WorkerID]
		}
		if codes[i].ProjectID != nil {
			codes[i].ProjectName = projects[*codes[i].ProjectID]
		}
	}
}

// ---- 报工码管理（老板/组长）----

func listEntryCodes(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		owner := ac.scopeUser()
		q := db.Where("user_id = ?", owner)
		switch c.Query("status") {
		case "enabled":
			q = q.Where("enabled = ?", true)
		case "disabled":
			q = q.Where("enabled = ?", false)
		}
		var codes []EntryCode
		if err := q.Order("created_at DESC, id DESC").Limit(maxEntryCodesPerUser).Find(&codes).Error; err != nil {
			response.ErrorInternal(c, "查询失败")
			return
		}
		fillEntryCodeNames(db, codes)
		today := time.Now().Format("2006-01-02")
		type statRow struct {
			EntryCodeID uint
			Cnt         int64
		}
		loadCounts := func(where string, args ...interface{}) map[uint]int64 {
			var rows []statRow
			db.Model(&WorkRecord{}).
				Select("entry_code_id, COUNT(*) AS cnt").
				Where(where, args...).
				Group("entry_code_id").Scan(&rows)
			m := make(map[uint]int64, len(rows))
			for _, r := range rows {
				m[r.EntryCodeID] = r.Cnt
			}
			return m
		}
		todayCounts := loadCounts("user_id = ? AND entry_code_id IS NOT NULL AND date = ? AND status <> ?", owner, today, RecordStatusRejected)
		pendingCounts := loadCounts("user_id = ? AND entry_code_id IS NOT NULL AND status = ?", owner, RecordStatusPending)
		totalCounts := loadCounts("user_id = ? AND entry_code_id IS NOT NULL", owner)
		for i := range codes {
			codes[i].TodayCount = todayCounts[codes[i].ID]
			codes[i].PendingCount = pendingCounts[codes[i].ID]
			codes[i].TotalCount = totalCounts[codes[i].ID]
		}
		response.Success(c, gin.H{"items": codes, "total": len(codes)})
	}
}

func createEntryCode(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		owner := ac.scopeUser()
		var in entryCodeInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		name := strings.TrimSpace(valueOrEmpty(in.Name))
		if len([]rune(name)) < 2 || len([]rune(name)) > 40 {
			response.ErrorBadRequest(c, "报工码名称请填 2-40 个字")
			return
		}
		baseURL, err := normalizeBaseURL(valueOrEmpty(in.BaseURL))
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		expiry, err := parseEntryExpiry(valueOrEmpty(in.ExpiresAt))
		if err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		note := strings.TrimSpace(valueOrEmpty(in.Note))
		if len([]rune(note)) > maxEntryNoteRunes {
			response.ErrorBadRequest(c, "备注请控制在 120 个字以内")
			return
		}
		pieceID := ptrUintOrNil(in.PieceItemID)
		workerID := ptrUintOrNil(in.WorkerID)
		projectID := ptrUintOrNil(in.ProjectID)
		if err := validateEntryRefs(db, owner, pieceID, workerID, projectID); err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		var n int64
		if err := db.Model(&EntryCode{}).Where("user_id = ?", owner).Count(&n).Error; err != nil {
			response.ErrorInternal(c, "创建失败")
			return
		}
		if n >= maxEntryCodesPerUser {
			response.ErrorBadRequest(c, "最多创建 200 个报工码")
			return
		}
		token := genEntryToken()
		for i := 0; i < 5; i++ {
			var dup int64
			db.Model(&EntryCode{}).Where("token = ?", token).Count(&dup)
			if dup == 0 {
				break
			}
			token = genEntryToken()
		}
		enabled := in.Enabled == nil || *in.Enabled
		code := EntryCode{
			UserID: owner, Name: name, Token: token, Type: TypePiece,
			PieceItemID: pieceID, WorkerID: workerID, ProjectID: projectID,
			BaseURL: baseURL, AutoApprove: in.AutoApprove != nil && *in.AutoApprove,
			Enabled: enabled, ExpiresAt: expiry,
			Note: note, CreatedBy: currentUID(c),
		}
		if err := db.Create(&code).Error; err != nil {
			response.ErrorInternal(c, "创建失败")
			return
		}
		// GORM 对带数据库默认值的 bool 字段会省略 false，创建停用码时要补一次更新。
		if !enabled {
			if err := db.Model(&code).Update("enabled", false).Error; err != nil {
				response.ErrorInternal(c, "创建失败")
				return
			}
			code.Enabled = false
		}
		out := []EntryCode{code}
		fillEntryCodeNames(db, out)
		response.Success(c, out[0])
	}
}

func updateEntryCode(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		owner := ac.scopeUser()
		var code EntryCode
		if err := db.Where("user_id = ? AND id = ?", owner, c.Param("id")).First(&code).Error; err != nil {
			notFound(c, "报工码不存在")
			return
		}
		var in entryCodeInput
		if err := c.ShouldBindJSON(&in); err != nil {
			response.ErrorBadRequest(c, "参数错误")
			return
		}
		updates := map[string]interface{}{}
		if in.Name != nil {
			name := strings.TrimSpace(*in.Name)
			if len([]rune(name)) < 2 || len([]rune(name)) > 40 {
				response.ErrorBadRequest(c, "报工码名称请填 2-40 个字")
				return
			}
			updates["name"] = name
		}
		if in.BaseURL != nil {
			baseURL, err := normalizeBaseURL(*in.BaseURL)
			if err != nil {
				response.ErrorBadRequest(c, err.Error())
				return
			}
			updates["base_url"] = baseURL
		}
		if in.ExpiresAt != nil {
			expiry, err := parseEntryExpiry(*in.ExpiresAt)
			if err != nil {
				response.ErrorBadRequest(c, err.Error())
				return
			}
			updates["expires_at"] = expiry
		}
		if in.PieceItemID != nil {
			updates["piece_item_id"] = ptrUintOrNil(in.PieceItemID)
		}
		if in.WorkerID != nil {
			updates["worker_id"] = ptrUintOrNil(in.WorkerID)
		}
		if in.ProjectID != nil {
			updates["project_id"] = ptrUintOrNil(in.ProjectID)
		}
		if in.AutoApprove != nil {
			updates["auto_approve"] = *in.AutoApprove
		}
		if in.Enabled != nil {
			updates["enabled"] = *in.Enabled
		}
		if in.Note != nil {
			note := strings.TrimSpace(*in.Note)
			if len([]rune(note)) > maxEntryNoteRunes {
				response.ErrorBadRequest(c, "备注请控制在 120 个字以内")
				return
			}
			updates["note"] = note
		}
		// 关联变更后重新校验（产品/工人/工地仍须属于当前数据集）。
		pieceID, workerID, projectID := code.PieceItemID, code.WorkerID, code.ProjectID
		if v, ok := updates["piece_item_id"]; ok {
			pieceID = v.(*uint)
		}
		if v, ok := updates["worker_id"]; ok {
			workerID = v.(*uint)
		}
		if v, ok := updates["project_id"]; ok {
			projectID = v.(*uint)
		}
		if err := validateEntryRefs(db, owner, pieceID, workerID, projectID); err != nil {
			response.ErrorBadRequest(c, err.Error())
			return
		}
		if len(updates) == 0 {
			response.ErrorBadRequest(c, "没有需要修改的内容")
			return
		}
		if err := db.Model(&code).Updates(updates).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		if err := db.Where("id = ?", code.ID).First(&code).Error; err != nil {
			response.ErrorInternal(c, "保存失败")
			return
		}
		out := []EntryCode{code}
		fillEntryCodeNames(db, out)
		response.Success(c, out[0])
	}
}

func deleteEntryCode(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		res := db.Where("user_id = ? AND id = ?", ac.scopeUser(), c.Param("id")).Delete(&EntryCode{})
		if res.Error != nil {
			response.ErrorInternal(c, "删除失败")
			return
		}
		if res.RowsAffected == 0 {
			notFound(c, "报工码不存在")
			return
		}
		response.Success(c, gin.H{"ok": true})
	}
}

func rotateEntryCodeToken(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		ac := loadAccess(db, currentUID(c))
		if !ac.canManage() {
			forbidden(c)
			return
		}
		var code EntryCode
		if err := db.Where("user_id = ? AND id = ?", ac.scopeUser(), c.Param("id")).First(&code).Error; err != nil {
			notFound(c, "报工码不存在")
			return
		}
		token := genEntryToken()
		for i := 0; i < 5; i++ {
			var dup int64
			db.Model(&EntryCode{}).Where("token = ?", token).Count(&dup)
			if dup == 0 {
				break
			}
			token = genEntryToken()
		}
		if err := db.Model(&code).Update("token", token).Error; err != nil {
			response.ErrorInternal(c, "重置失败")
			return
		}
		code.Token = token
		response.Success(c, code)
	}
}

func valueOrEmpty(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func ptrUintOrNil(p *uint) *uint {
	if p == nil || *p == 0 {
		return nil
	}
	v := *p
	return &v
}
