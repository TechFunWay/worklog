package worklog

// review_extra_test.go — v0.5.0「扫码报工 + 班组审核 + 批量报工」发布前审查补测。
//
// 本文件只补测试，不改业务代码。覆盖重点：
//  1. 历史数据升级口径：v0.4.1 的 work_records 没有 status 列，AutoMigrate 补列后
//     旧记录必须读成 approved，否则升级当天历史工资全部消失（静默数据丢失）。
//  2. 已结算记录不能被审核；审核批次任一条不满足前置条件时整批回滚。
//  3. 审核并发：同一条记录被并发、重复点击审核时只能生效一次。
//  4. 公开报工：停用/过期/删除/换令牌/错误产品/错误工人/绑定优先级/日期与数量边界，
//     以及公开响应是否泄露手机号与单价。
//  5. 批量报工：非法行、重复工人行、越权工人/项目、金额计算、重复提交。
//  6. 权限与数据集归属：员工不能审核/管理报工码/改删已审记录；组长按老板数据集取数，
//     但组长创建的工人/计件项目落到自己账号下（归属 bug，见对应用例内注释）。
//
// 约定：对「当前实现确实如此、但审查认为有风险」的行为，用例断言现状并在 t.Logf 标注
// 建议口径，保证 CI 是绿的；问题清单里单独列为风险项 / 建议修复项。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/database"
)

// ---- 本地辅助 ----

func xtCreateWorker(t *testing.T, r *gin.Engine, uid int, name string) uint {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", uid, map[string]interface{}{"name": name})
	data := mustOK(t, w, out, "create worker "+name)
	return uint(data["id"].(float64))
}

func xtCreatePiece(t *testing.T, r *gin.Engine, uid int, name, unit string, price int64) uint {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/piece-items", uid, map[string]interface{}{
		"name": name, "unit": unit, "unit_price_cents": price,
	})
	data := mustOK(t, w, out, "create piece "+name)
	return uint(data["id"].(float64))
}

func xtCreateCode(t *testing.T, r *gin.Engine, uid int, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/entry-codes", uid, body)
	return mustOK(t, w, out, "create entry code")
}

func xtSubmit(t *testing.T, r *gin.Engine, token string, body map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	return doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, body)
}

func xtPublicInfo(t *testing.T, r *gin.Engine, token string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	return doReq(r, http.MethodGet, "/api/worklog/public/entry/"+token, 0, nil)
}

func xtApprove(t *testing.T, r *gin.Engine, uid int, ids []uint) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	return doReq(r, http.MethodPost, "/api/worklog/review/approve", uid, map[string]interface{}{"ids": ids})
}

func xtReject(t *testing.T, r *gin.Engine, uid int, ids []uint, note string) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	body := map[string]interface{}{"ids": ids}
	if note != "" {
		body["note"] = note
	}
	return doReq(r, http.MethodPost, "/api/worklog/review/reject", uid, body)
}

// xtNewTeam 直接落库建班组（测试里不走邀请码流程，方便给任意账号发指定角色）。
func xtNewTeam(t *testing.T, db *gorm.DB, owner uint, name string, reviewRequired bool) Team {
	t.Helper()
	team := Team{
		Name: name, OwnerUserID: owner,
		InviteCode:     fmt.Sprintf("xt%06d", time.Now().UnixNano()%1000000),
		ReviewRequired: reviewRequired,
	}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team %s: %v", name, err)
	}
	return team
}

// xtJoinAs 建一条归属班组老板数据集的工人档案 + 成员行（模拟真实的加入班组）。
func xtJoinAs(t *testing.T, db *gorm.DB, team Team, uid uint, role, workerName string) Worker {
	t.Helper()
	if workerName == "" {
		workerName = fmt.Sprintf("成员%d", uid)
	}
	worker := Worker{UserID: team.OwnerUserID, Name: workerName, Status: "active"}
	if err := db.Create(&worker).Error; err != nil {
		t.Fatalf("create worker for member %d: %v", uid, err)
	}
	member := TeamMember{TeamID: team.ID, UserID: uid, Role: role, WorkerID: &worker.ID, Status: "active"}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create member %d: %v", uid, err)
	}
	return worker
}

// xtPending 直接落一条待审核记工（避开扫码/员工提交路径，精确构造前置条件）。
func xtPending(t *testing.T, db *gorm.DB, userID, workerID, pieceID uint) WorkRecord {
	t.Helper()
	rec := WorkRecord{
		UserID: userID, WorkerID: workerID,
		Date: time.Now().Format("2006-01-02"), Type: TypePiece,
		Quantity: "1", UnitPriceCents: 100, AmountCents: 100,
		Status: RecordStatusPending, Source: RecordSourceQR,
	}
	if pieceID > 0 {
		rec.PieceItemID = &pieceID
	}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("create pending record: %v", err)
	}
	return rec
}

func xtRecord(t *testing.T, db *gorm.DB, id uint) WorkRecord {
	t.Helper()
	var rec WorkRecord
	if err := db.First(&rec, id).Error; err != nil {
		t.Fatalf("load record %d: %v", id, err)
	}
	return rec
}

func xtWantCode(t *testing.T, w *httptest.ResponseRecorder, want int, ctx string) {
	t.Helper()
	if w.Code != want {
		t.Fatalf("%s: http=%d want %d body=%s", ctx, w.Code, want, w.Body.String())
	}
}

func xtWantFail(t *testing.T, w *httptest.ResponseRecorder, out map[string]interface{}, ctx string) {
	t.Helper()
	if w.Code == http.StatusOK && out["code"] == float64(0) {
		t.Fatalf("%s 应当失败，实际成功 body=%s", ctx, w.Body.String())
	}
}

func xtWantMsg(t *testing.T, out map[string]interface{}, sub, ctx string) {
	t.Helper()
	msg, _ := out["message"].(string)
	if !strings.Contains(msg, sub) {
		t.Fatalf("%s: message=%q 不含 %q", ctx, msg, sub)
	}
}

func xtBodyHasKey(body, key string) bool {
	return strings.Contains(body, "\""+key+"\"")
}

// ---- 1. 历史数据升级口径 ----

// TestLegacyRecordStatusSurvivesUpgrade 锁住 v0.5.0 的历史数据口径：
// v0.4.1 的 work_records 没有 status 列，AutoMigrate 用 ALTER TABLE ADD COLUMN 补列后，
// 既有行必须读成 approved（SQLite 用列默认值回填）。否则升级当天所有历史记工被工资/
// 考勤/统计/结算口径静默排除——这是本次改动最危险的回归点。
func TestLegacyRecordStatusSurvivesUpgrade(t *testing.T) {
	legacy, err := database.InitDB(filepath.Join(t.TempDir(), "legacy-v041.db"))
	if err != nil {
		t.Fatalf("init legacy db: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB(legacy) })

	// v0.4.1 的 work_records：没有 status / source / entry_code_* / submitter_* / reviewed_*。
	const legacyDDL = "CREATE TABLE `work_records` (" +
		"`id` integer PRIMARY KEY AUTOINCREMENT," +
		"`user_id` integer NOT NULL," +
		"`worker_id` integer NOT NULL," +
		"`project_id` integer," +
		"`piece_item_id` integer," +
		"`date` text NOT NULL," +
		"`type` text NOT NULL," +
		"`quantity` text NOT NULL," +
		"`unit_price_cents` integer," +
		"`amount_cents` integer," +
		"`note` text," +
		"`settlement_id` integer," +
		"`created_by` integer," +
		"`created_at` datetime," +
		"`updated_at` datetime" +
		")"
	if err := legacy.Exec(legacyDDL).Error; err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	if err := legacy.Exec("INSERT INTO work_records (user_id, worker_id, date, type, quantity, unit_price_cents, amount_cents, created_at, updated_at) VALUES (1, 7, '2026-09-01', 'day', '1', 30000, 30000, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)").Error; err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}
	var ddl string
	if err := legacy.Raw("SELECT COALESCE(sql, '') FROM sqlite_master WHERE type = 'table' AND name = 'work_records'").Row().Scan(&ddl); err != nil {
		t.Fatalf("read legacy ddl: %v", err)
	}
	if strings.Contains(ddl, "status") {
		t.Fatalf("前置条件失败：旧库不该有 status 列: %s", ddl)
	}

	// 与真实启动一致：AutoMigrate 给旧表补列。
	if err := database.AutoMigrate(legacy); err != nil {
		t.Fatalf("migrate legacy db: %v", err)
	}
	if !legacy.Migrator().HasColumn(&WorkRecord{}, "status") {
		t.Fatal("升级后 work_records 仍没有 status 列")
	}

	var old WorkRecord
	if err := legacy.Where("worker_id = ?", 7).First(&old).Error; err != nil {
		t.Fatalf("旧记录在升级后丢失: %v", err)
	}
	if old.Status != RecordStatusApproved {
		t.Fatalf("历史记录 status = %q，期望 %q（否则升级当天历史工资全部消失）", old.Status, RecordStatusApproved)
	}

	var nullCount int64
	if err := legacy.Model(&WorkRecord{}).Where("status IS NULL OR status = ''").Count(&nullCount).Error; err != nil {
		t.Fatalf("count null status: %v", err)
	}
	if nullCount != 0 {
		t.Fatalf("升级后仍有 %d 条记录 status 为 NULL/空，会被工资口径静默排除", nullCount)
	}

	// 历史工资必须仍计入（统计/结算共用的口径）。
	var sum int64
	if err := legacy.Model(&WorkRecord{}).
		Where("user_id = ? AND date BETWEEN ? AND ?", 1, "2026-09-01", "2026-09-30").
		Where("status = ?", RecordStatusApproved).
		Select("COALESCE(SUM(amount_cents), 0)").Scan(&sum).Error; err != nil {
		t.Fatalf("sum legacy amount: %v", err)
	}
	if sum != 30000 {
		t.Fatalf("历史工资合计 = %d，期望 30000", sum)
	}

	// 不走模型默认值、直接写 SQL 的路径（旧 JSON 备份恢复等）也要落 approved。
	if err := legacy.Exec("INSERT INTO work_records (user_id, worker_id, date, type, quantity, unit_price_cents, amount_cents) VALUES (1, 8, '2026-09-02', 'day', '1', 100, 100)").Error; err != nil {
		t.Fatalf("raw insert without status: %v", err)
	}
	var raw WorkRecord
	if err := legacy.Where("worker_id = ?", 8).First(&raw).Error; err != nil {
		t.Fatalf("load raw row: %v", err)
	}
	if raw.Status != RecordStatusApproved {
		t.Fatalf("不带 status 的 raw insert 落库为 %q，期望 %q", raw.Status, RecordStatusApproved)
	}

	// 模型零值 Create 同样必须落 approved（GORM default 语义）。
	zero := WorkRecord{UserID: 1, WorkerID: 9, Date: "2026-09-03", Type: TypeDay, Quantity: "1", AmountCents: 123}
	if err := legacy.Create(&zero).Error; err != nil {
		t.Fatalf("create zero-value record: %v", err)
	}
	var zeroCheck WorkRecord
	if err := legacy.First(&zeroCheck, zero.ID).Error; err != nil {
		t.Fatalf("load zero-value record: %v", err)
	}
	if zeroCheck.Status != RecordStatusApproved {
		t.Fatalf("零值 Create 的 status = %q，期望 %q", zeroCheck.Status, RecordStatusApproved)
	}
}

// ---- 2. 报工码生命周期 ----

func TestEntryCodeLifecycleGuards(t *testing.T) {
	r, db := setupTest(t)
	wid := xtCreateWorker(t, r, 1, "张三")
	pid := xtCreatePiece(t, r, 1, "打包装箱", "件", 250)
	code := xtCreateCode(t, r, 1, map[string]interface{}{
		"name": "1号产线报工", "piece_item_id": pid, "worker_id": wid,
	})
	token, _ := code["token"].(string)
	codeID := uint(code["id"].(float64))
	if token == "" {
		t.Fatal("报工码令牌为空")
	}

	if _, out := xtPublicInfo(t, r, token); out["code"] != float64(0) {
		t.Fatalf("启用中的报工码应可访问: %v", out)
	}

	// 停用 → 读与写都必须 404（不能只挡 GET）。
	w, _ := doReq(r, http.MethodPut, "/api/worklog/entry-codes/"+itoa(int(codeID)), 1, map[string]interface{}{"enabled": false})
	xtWantCode(t, w, http.StatusOK, "disable code")
	w, _ = xtPublicInfo(t, r, token)
	xtWantCode(t, w, http.StatusNotFound, "disabled code info")
	w, out := xtSubmit(t, r, token, map[string]interface{}{"quantity": "1"})
	xtWantCode(t, w, http.StatusNotFound, "disabled code submit")
	xtWantMsg(t, out, "停用", "disabled submit message")

	// 重新启用。
	w, _ = doReq(r, http.MethodPut, "/api/worklog/entry-codes/"+itoa(int(codeID)), 1, map[string]interface{}{"enabled": true})
	xtWantCode(t, w, http.StatusOK, "enable code")
	w, _ = xtPublicInfo(t, r, token)
	xtWantCode(t, w, http.StatusOK, "re-enabled code info")

	// 过期（截止日期为昨天）→ 读写都 404，且文案区分「过期」。
	past := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if err := db.Model(&EntryCode{}).Where("id = ?", codeID).Update("expires_at", past).Error; err != nil {
		t.Fatalf("set expired: %v", err)
	}
	w, out = xtPublicInfo(t, r, token)
	xtWantCode(t, w, http.StatusNotFound, "expired code info")
	xtWantMsg(t, out, "过期", "expired info message")
	w, _ = xtSubmit(t, r, token, map[string]interface{}{"quantity": "1"})
	xtWantCode(t, w, http.StatusNotFound, "expired code submit")

	// 截止日期当天仍然有效（边界：过期判定用「今天 > 截止日」）。
	today := time.Now().Format("2006-01-02")
	if err := db.Model(&EntryCode{}).Where("id = ?", codeID).Update("expires_at", today).Error; err != nil {
		t.Fatalf("set expiring today: %v", err)
	}
	w, _ = xtPublicInfo(t, r, token)
	xtWantCode(t, w, http.StatusOK, "code expiring today should still work")

	// 换令牌：旧令牌立即失效，新令牌可用。
	w, out = doReq(r, http.MethodPost, "/api/worklog/entry-codes/"+itoa(int(codeID))+"/rotate-token", 1, nil)
	rotated := mustOK(t, w, out, "rotate token")
	newToken, _ := rotated["token"].(string)
	if newToken == "" || newToken == token {
		t.Fatalf("换令牌后 token 未变化: old=%q new=%q", token, newToken)
	}
	w, _ = xtPublicInfo(t, r, token)
	xtWantCode(t, w, http.StatusNotFound, "old token after rotate")
	w, _ = xtPublicInfo(t, r, newToken)
	xtWantCode(t, w, http.StatusOK, "new token after rotate")

	// 删除报工码 → 令牌立即失效（历史记录保留 entry_code_id 快照）。
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/entry-codes/"+itoa(int(codeID)), 1, nil)
	xtWantCode(t, w, http.StatusOK, "delete code")
	w, _ = xtPublicInfo(t, r, newToken)
	xtWantCode(t, w, http.StatusNotFound, "deleted code info")
	w, _ = xtSubmit(t, r, newToken, map[string]interface{}{"quantity": "1"})
	xtWantCode(t, w, http.StatusNotFound, "deleted code submit")

	// 不可枚举：随机/超长令牌都只给 404，不回显任何数据。
	for _, bad := range []string{"0123456789abcdef0123456789abcdef", "abc", strings.Repeat("a", 80), "admin"} {
		w, _ = xtPublicInfo(t, r, bad)
		xtWantCode(t, w, http.StatusNotFound, "invalid token "+bad[:minInt(len(bad), 8)])
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ---- 3. 公开接口信息泄露 ----

func TestPublicEntryInfoDoesNotLeakSensitiveFields(t *testing.T) {
	r, _ := setupTest(t)
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{
		"name": "张三", "phone": "13800000000", "daily_wage_cents": 30000, "hourly_wage_cents": 3000,
	})
	worker := mustOK(t, w, out, "create worker with phone")
	wid := uint(worker["id"].(float64))
	pid := xtCreatePiece(t, r, 1, "打包装箱", "件", 250)

	code := xtCreateCode(t, r, 1, map[string]interface{}{"name": "隐私码", "piece_item_id": pid, "worker_id": wid})
	token := code["token"].(string)
	w, out = xtPublicInfo(t, r, token)
	mustOK(t, w, out, "public info")
	body := w.Body.String()
	for _, forbidden := range []string{"phone", "13800000000", "unit_price_cents", "daily_wage_cents", "hourly_wage_cents", "amount_cents", token} {
		if xtBodyHasKey(body, forbidden) || strings.Contains(body, forbidden) {
			t.Fatalf("公开报工上下文泄露 %q: %s", forbidden, body)
		}
	}

	// 不绑定产品时返回候选列表，每项只允许 id/name/unit（产品）与 id/name（工人）。
	w, out = xtPublicInfo(t, r, code2Token(t, r, 1, "候选码"))
	info := mustOK(t, w, out, "public info candidates")
	products, _ := info["products"].([]interface{})
	if len(products) == 0 {
		t.Fatalf("产品码应返回候选产品: %v", info)
	}
	for _, p := range products {
		item, _ := p.(map[string]interface{})
		if len(item) != 3 {
			t.Fatalf("公开产品项多余字段: %v", item)
		}
		for _, k := range []string{"id", "name", "unit"} {
			if _, ok := item[k]; !ok {
				t.Fatalf("公开产品项缺字段 %q: %v", k, item)
			}
		}
	}
	workers, _ := info["workers"].([]interface{})
	if len(workers) == 0 {
		t.Fatalf("产品码应返回候选工人: %v", info)
	}
	for _, p := range workers {
		item, _ := p.(map[string]interface{})
		if len(item) != 2 {
			t.Fatalf("公开工人项多余字段（可能带出手机号/工资）: %v", item)
		}
	}

	// 提交响应只回显工人需要确认的字段：不能带出单价/金额（持有报工码即可反推工价）。
	ptok := code2Token(t, r, 1, "回显码")
	w, out = xtSubmit(t, r, ptok, map[string]interface{}{"worker_id": wid, "piece_item_id": pid, "quantity": "2"})
	sub := mustOK(t, w, out, "submit response shape")
	rec, ok := sub["record"].(map[string]interface{})
	if !ok {
		t.Fatalf("提交响应缺少 record: %v", sub)
	}
	wantKeys := map[string]bool{"id": true, "status": true, "quantity": true, "date": true}
	for k := range rec {
		if !wantKeys[k] {
			t.Fatalf("提交响应 record 含多余字段 %q: %v", k, rec)
		}
	}
	for k := range wantKeys {
		if _, exists := rec[k]; !exists {
			t.Fatalf("提交响应 record 缺字段 %q: %v", k, rec)
		}
	}
	for _, leaked := range []string{"unit_price_cents", "amount_cents", "worker_id", "piece_item_id", "settlement_id", "reviewed_by"} {
		if _, exists := rec[leaked]; exists {
			t.Fatalf("提交响应泄露 %q: %v", leaked, rec)
		}
	}
	if sub["today_count"].(float64) != 1 {
		t.Fatalf("today_count = %v，期望 1", sub["today_count"])
	}
	if body := w.Body.String(); strings.Contains(body, "unit_price_cents") || strings.Contains(body, "amount_cents") {
		t.Fatalf("提交响应正文仍含单价/金额字段: %s", body)
	}
}

// code2Token 建一个不绑定产品/工人的报工码并返回令牌（公开候选列表场景）。
func code2Token(t *testing.T, r *gin.Engine, uid int, name string) string {
	t.Helper()
	code := xtCreateCode(t, r, uid, map[string]interface{}{"name": name})
	tok, _ := code["token"].(string)
	if tok == "" {
		t.Fatalf("create code %s: 令牌为空", name)
	}
	return tok
}

// ---- 4. 公开报工校验与绑定优先级 ----

func TestPublicEntryValidationAndBindingPriority(t *testing.T) {
	r, db := setupTest(t)
	w1 := xtCreateWorker(t, r, 1, "王五")
	w2 := xtCreateWorker(t, r, 1, "赵六")
	p1 := xtCreatePiece(t, r, 1, "组装", "件", 250)
	// 无单价的计件项目与归档档案（API 不允许，直接落库模拟历史/异常数据）。
	zeroPiece := PieceItem{UserID: 1, Name: "零价产品", Unit: "件", Status: "active"}
	if err := db.Create(&zeroPiece).Error; err != nil {
		t.Fatalf("create zero-price piece: %v", err)
	}
	archivedPiece := PieceItem{UserID: 1, Name: "停用产品", Unit: "件", UnitPriceCents: 100, Status: "archived"}
	if err := db.Create(&archivedPiece).Error; err != nil {
		t.Fatalf("create archived piece: %v", err)
	}
	archivedWorker := Worker{UserID: 1, Name: "归档工人", Status: "archived"}
	if err := db.Create(&archivedWorker).Error; err != nil {
		t.Fatalf("create archived worker: %v", err)
	}
	// 另一个数据集（uid=2 是独立老板）。
	otherWorker := xtCreateWorker(t, r, 2, "别家工人")
	otherPiece := xtCreatePiece(t, r, 2, "别家产品", "件", 999)

	// 产品 + 工人双绑定：请求参数不得覆盖绑定（否则匿名者能给任意工人/产品报工）。
	code := xtCreateCode(t, r, 1, map[string]interface{}{"name": "双绑定码", "piece_item_id": p1, "worker_id": w1})
	codeID := uint(code["id"].(float64))
	token := code["token"].(string)
	w, out := xtSubmit(t, r, token, map[string]interface{}{
		"worker_id": otherWorker, "piece_item_id": otherPiece, "quantity": "3",
	})
	sub := mustOK(t, w, out, "bound priority submit")
	rec := xtRecord(t, db, uint(sub["record"].(map[string]interface{})["id"].(float64)))
	if rec.WorkerID != w1 {
		t.Fatalf("绑定工人被请求参数覆盖: worker_id=%d want %d", rec.WorkerID, w1)
	}
	if rec.PieceItemID == nil || *rec.PieceItemID != p1 {
		t.Fatalf("绑定产品被请求参数覆盖: piece_item_id=%v want %d", rec.PieceItemID, p1)
	}
	if rec.UnitPriceCents != 250 || rec.AmountCents != 750 {
		t.Fatalf("金额按错误单价计算: %+v", rec)
	}
	if rec.Source != RecordSourceQR || rec.EntryCodeID == nil || *rec.EntryCodeID != codeID {
		t.Fatalf("报工来源快照错误: %+v", rec)
	}
	if rec.Status != RecordStatusPending {
		t.Fatalf("默认应落待审核，实际 %q", rec.Status)
	}
	if rec.SubmitterName != "王五" || rec.EntryCodeName != "双绑定码" {
		t.Fatalf("提交人/报工码名称快照错误: %+v", rec)
	}

	// 只绑产品：工人由请求指定，必须属于同一数据集且在职。
	ptok := code2Token(t, r, 1, "只绑定产品")
	if err := db.Model(&EntryCode{}).Where("token = ?", ptok).Update("piece_item_id", p1).Error; err != nil {
		t.Fatalf("bind product: %v", err)
	}
	w, out = xtSubmit(t, r, ptok, map[string]interface{}{"worker_id": otherWorker, "quantity": "1"})
	xtWantFail(t, w, out, "other dataset worker")
	xtWantMsg(t, out, "工人不存在", "other dataset worker")
	w, out = xtSubmit(t, r, ptok, map[string]interface{}{"worker_id": archivedWorker.ID, "quantity": "1"})
	xtWantFail(t, w, out, "archived worker")
	xtWantMsg(t, out, "已归档", "archived worker")
	w, out = xtSubmit(t, r, ptok, map[string]interface{}{"quantity": "1"})
	xtWantFail(t, w, out, "missing worker")
	xtWantMsg(t, out, "请选择姓名", "missing worker")
	w, out = xtSubmit(t, r, ptok, map[string]interface{}{"worker_id": w2, "quantity": "1"})
	mustOK(t, w, out, "valid product-only submit")

	// 只绑工人：产品由请求指定。
	wtok := code2Token(t, r, 1, "只绑定工人")
	if err := db.Model(&EntryCode{}).Where("token = ?", wtok).Update("worker_id", w1).Error; err != nil {
		t.Fatalf("bind worker: %v", err)
	}
	w, out = xtSubmit(t, r, wtok, map[string]interface{}{"piece_item_id": otherPiece, "quantity": "1"})
	xtWantFail(t, w, out, "other dataset piece")
	xtWantMsg(t, out, "产品不存在", "other dataset piece")
	w, out = xtSubmit(t, r, wtok, map[string]interface{}{"piece_item_id": archivedPiece.ID, "quantity": "1"})
	xtWantFail(t, w, out, "archived piece")
	xtWantMsg(t, out, "已停用", "archived piece")
	w, out = xtSubmit(t, r, wtok, map[string]interface{}{"piece_item_id": zeroPiece.ID, "quantity": "1"})
	xtWantFail(t, w, out, "zero price piece")
	xtWantMsg(t, out, "单价", "zero price piece")
	w, out = xtSubmit(t, r, wtok, map[string]interface{}{"quantity": "1"})
	xtWantFail(t, w, out, "missing piece")
	xtWantMsg(t, out, "请选择产品", "missing piece")

	// 报工码绑定对象在建码后被归档/停用：公开上下文 404，提交 400。
	latePiece := xtCreatePiece(t, r, 1, "建码后停用", "件", 100)
	lateCode := xtCreateCode(t, r, 1, map[string]interface{}{"name": "建码后停用", "piece_item_id": latePiece})
	if err := db.Model(&PieceItem{}).Where("id = ?", latePiece).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive late piece: %v", err)
	}
	w, out = xtPublicInfo(t, r, lateCode["token"].(string))
	xtWantCode(t, w, http.StatusNotFound, "code bound to archived piece")
	xtWantMsg(t, out, "产品已停用", "code bound to archived piece")
	w, out = xtSubmit(t, r, lateCode["token"].(string), map[string]interface{}{"quantity": "1"})
	xtWantFail(t, w, out, "submit to archived bound piece")

	lateWorker := xtCreateWorker(t, r, 1, "建码后归档")
	lateWCode := xtCreateCode(t, r, 1, map[string]interface{}{"name": "建码后归档", "worker_id": lateWorker})
	if err := db.Model(&Worker{}).Where("id = ?", lateWorker).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive late worker: %v", err)
	}
	w, out = xtPublicInfo(t, r, lateWCode["token"].(string))
	xtWantCode(t, w, http.StatusNotFound, "code bound to archived worker")
	xtWantMsg(t, out, "工人已停用", "code bound to archived worker")
	w, out = xtSubmit(t, r, lateWCode["token"].(string), map[string]interface{}{"quantity": "1"})
	xtWantFail(t, w, out, "submit to archived bound worker")

	// 数量边界（只绑产品的码，单价 250）。
	quantityCases := []struct {
		qty  string
		want int
	}{
		{"0", http.StatusBadRequest},
		{"-2", http.StatusBadRequest},
		{"1.5", http.StatusBadRequest},
		{"", http.StatusBadRequest},
		{"abc", http.StatusBadRequest},
		{"1000001", http.StatusBadRequest},
		{"1", http.StatusOK},
		{"1000000", http.StatusOK},
	}
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	for _, tc := range quantityCases {
		w, out = xtSubmit(t, r, ptok, map[string]interface{}{"worker_id": w2, "quantity": tc.qty, "date": yesterday})
		if tc.want == http.StatusOK {
			mustOK(t, w, out, "quantity "+tc.qty)
		} else {
			xtWantCode(t, w, tc.want, "quantity "+tc.qty)
		}
	}
	// 上限正好 1000000 件 × 250 分 = 250000000 分。
	var maxAmount int64
	db.Model(&WorkRecord{}).Where("quantity = ? AND date = ?", "1000000", yesterday).
		Select("COALESCE(MAX(amount_cents), 0)").Scan(&maxAmount)
	if maxAmount != 250000000 {
		t.Fatalf("1000000 件的金额 = %d，期望 250000000", maxAmount)
	}

	// 日期边界：只允许今天/昨天。
	today := time.Now().Format("2006-01-02")
	dateCases := []struct {
		date string
		want int
	}{
		{"", http.StatusOK},
		{today, http.StatusOK},
		{yesterday, http.StatusOK},
		{time.Now().AddDate(0, 0, -2).Format("2006-01-02"), http.StatusBadRequest},
		{"2026-02-30", http.StatusBadRequest},
		{"abc", http.StatusBadRequest},
	}
	for i, tc := range dateCases {
		body := map[string]interface{}{"worker_id": w2, "quantity": fmt.Sprintf("%d", 10+i)}
		if tc.date != "" {
			body["date"] = tc.date
		}
		w, out = xtSubmit(t, r, ptok, body)
		if tc.want == http.StatusOK {
			mustOK(t, w, out, "date "+tc.date)
		} else {
			xtWantCode(t, w, tc.want, "date "+tc.date)
		}
	}

	// 自动通过：直接进已通过，且立刻计入当月工资。
	autoCode := xtCreateCode(t, r, 1, map[string]interface{}{
		"name": "自动通过", "piece_item_id": p1, "worker_id": w1, "auto_approve": true,
	})
	w, out = xtSubmit(t, r, autoCode["token"].(string), map[string]interface{}{"quantity": "2"})
	autoSub := mustOK(t, w, out, "auto approve submit")
	if autoSub["status"] != RecordStatusApproved {
		t.Fatalf("自动通过码提交状态 = %v，期望 approved", autoSub["status"])
	}
	autoRec := xtRecord(t, db, uint(autoSub["record"].(map[string]interface{})["id"].(float64)))
	if autoRec.ReviewedAt == nil || autoRec.ReviewNote == "" {
		t.Fatalf("自动通过缺少审核时间/备注快照: %+v", autoRec)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/stats/dashboard", 1, nil)
	dash := mustOK(t, w, out, "dashboard after auto approve")
	if dash["month_work_cents"].(float64) != 500 {
		t.Fatalf("自动通过的记工应计入当月工资 500，实际 %v", dash["month_work_cents"])
	}
}

// ---- 5. 批量报工边界 ----

func TestBatchEntryPartialInvalidRows(t *testing.T) {
	r, db := setupTest(t)
	a := xtCreateWorker(t, r, 1, "甲")
	b := xtCreateWorker(t, r, 1, "乙")
	c := xtCreateWorker(t, r, 1, "丙")
	piece := xtCreatePiece(t, r, 1, "组装", "件", 200)

	batchWith := func(uid int, pieceID uint, date string, projectID interface{}, rows []map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
		body := map[string]interface{}{"date": date, "piece_item_id": pieceID, "rows": rows}
		if projectID != nil {
			body["project_id"] = projectID
		}
		return doReq(r, http.MethodPost, "/api/worklog/records/batch", uid, body)
	}
	row := func(worker uint, qty interface{}) map[string]interface{} {
		return map[string]interface{}{"worker_id": worker, "quantity": qty}
	}
	countOn := func(date string) int64 {
		var n int64
		db.Model(&WorkRecord{}).Where("date = ?", date).Count(&n)
		return n
	}

	// 空数量（"0" / 空白）视为本次不报：跳过但不报错，其余行正常落库。
	w, out := batchWith(1, piece, "2026-09-10", nil, []map[string]interface{}{row(a, "10"), row(b, "0"), row(c, "")})
	data := mustOK(t, w, out, "blank quantity rows")
	if data["created"].(float64) != 1 || int64(data["amount_cents"].(float64)) != 2000 {
		t.Fatalf("空白数量行的批量结果 = %v，期望 created=1 amount=2000", data)
	}
	if got := countOn("2026-09-10"); got != 1 {
		t.Fatalf("空白数量行落库 %d 条，期望 1", got)
	}
	// total_quantity 是 trimFloat 出来的 decimal 文本。
	if qty, _ := data["total_quantity"].(string); qty != "10" {
		t.Fatalf("批量报工 total_quantity = %v，期望 \"10\"", data["total_quantity"])
	}

	// 非法数量（非数字/小数/负数/超限）：整批拒绝并给出行号，不静默跳过。
	for _, tc := range []struct {
		qty string
	}{
		{"abc"},
		{"1.5"},
		{"-3"},
		{"1000001"},
	} {
		w, out = batchWith(1, piece, "2026-09-11", nil, []map[string]interface{}{row(a, "1"), row(b, tc.qty)})
		xtWantFail(t, w, out, "invalid quantity "+tc.qty)
		xtWantMsg(t, out, "第 2 行", "invalid quantity "+tc.qty)
	}
	if got := countOn("2026-09-11"); got != 0 {
		t.Fatalf("整批拒绝时不应落库，实际 %d 条", got)
	}

	// 同一工人重复填写：整批拒绝（不丢后一条）。
	w, out = batchWith(1, piece, "2026-09-11", nil, []map[string]interface{}{row(a, "5"), row(a, "7")})
	xtWantFail(t, w, out, "duplicate worker rows")
	xtWantMsg(t, out, "重复", "duplicate worker rows")

	// 缺工人行：整批拒绝并给出行号。
	w, out = batchWith(1, piece, "2026-09-11", nil, []map[string]interface{}{row(a, "1"), map[string]interface{}{"worker_id": 0, "quantity": "1"}})
	xtWantFail(t, w, out, "missing worker row")
	xtWantMsg(t, out, "第 2 行", "missing worker row")
	t.Logf("批量报工对非法行/重复工人的口径：整批 400 并标明行号（空白或 0 视为本次不报）；这是「不静默少记」的正确取舍，已作为回归用例锁住")

	// 未知工人 / 他数据集工人 / 归档工人：整批拒绝（这里不静默丢）。
	w, out = batchWith(1, piece, "2026-09-13", nil, []map[string]interface{}{row(a, "1"), row(9999, "1")})
	xtWantFail(t, w, out, "unknown worker")
	xtWantMsg(t, out, "工人不存在", "unknown worker")
	otherWorker := xtCreateWorker(t, r, 2, "别家工人")
	w, out = batchWith(1, piece, "2026-09-13", nil, []map[string]interface{}{row(otherWorker, "1")})
	xtWantFail(t, w, out, "other dataset worker")
	if err := db.Model(&Worker{}).Where("id = ?", c).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive worker: %v", err)
	}
	w, out = batchWith(1, piece, "2026-09-13", nil, []map[string]interface{}{row(c, "1")})
	xtWantFail(t, w, out, "archived worker")

	// 计件项目归档 / 单价为 0。
	w, _ = doReq(r, http.MethodPut, "/api/worklog/piece-items/"+itoa(int(piece)), 1, map[string]interface{}{
		"name": "组装", "unit": "件", "unit_price_cents": 200, "status": "archived",
	})
	xtWantCode(t, w, http.StatusOK, "archive piece")
	w, out = batchWith(1, piece, "2026-09-14", nil, []map[string]interface{}{row(a, "1")})
	xtWantFail(t, w, out, "archived piece")
	xtWantMsg(t, out, "已归档", "archived piece")
	w, _ = doReq(r, http.MethodPut, "/api/worklog/piece-items/"+itoa(int(piece)), 1, map[string]interface{}{
		"name": "组装", "unit": "件", "unit_price_cents": 200, "status": "active",
	})
	xtWantCode(t, w, http.StatusOK, "restore piece")
	zeroPiece := PieceItem{UserID: 1, Name: "零价", Unit: "件", Status: "active"}
	if err := db.Create(&zeroPiece).Error; err != nil {
		t.Fatalf("create zero-price piece: %v", err)
	}
	w, out = batchWith(1, zeroPiece.ID, "2026-09-14", nil, []map[string]interface{}{row(a, "1")})
	xtWantFail(t, w, out, "zero price piece")
	xtWantMsg(t, out, "单价", "zero price piece")

	// 工地归属：别人工地 400；project_id=0 视为不带工地。
	w, out = doReq(r, http.MethodPost, "/api/worklog/projects", 2, map[string]interface{}{"name": "别家工地"})
	otherProject := mustOK(t, w, out, "create other project")
	w, out = batchWith(1, piece, "2026-09-15", otherProject["id"], []map[string]interface{}{row(a, "1")})
	xtWantFail(t, w, out, "other dataset project")
	xtWantMsg(t, out, "工地不存在", "other dataset project")
	w, out = batchWith(1, piece, "2026-09-15", 0, []map[string]interface{}{row(a, "1")})
	mustOK(t, w, out, "project id 0")
	var noProject WorkRecord
	if err := db.Where("date = ? AND worker_id = ?", "2026-09-15", a).First(&noProject).Error; err != nil {
		t.Fatalf("load project-null record: %v", err)
	}
	if noProject.ProjectID != nil {
		t.Fatalf("project_id=0 应视为不带工地，实际 %v", *noProject.ProjectID)
	}

	// 空行 / 超过 200 行。
	w, out = batchWith(1, piece, "2026-09-16", nil, nil)
	xtWantFail(t, w, out, "empty rows")
	xtWantMsg(t, out, "至少", "empty rows")
	tooMany := make([]map[string]interface{}, 0, 201)
	for i := 0; i < 201; i++ {
		tooMany = append(tooMany, row(a, "1"))
	}
	w, out = batchWith(1, piece, "2026-09-16", nil, tooMany)
	xtWantFail(t, w, out, "too many rows")
	xtWantMsg(t, out, "200", "too many rows")

	// 金额 = 数量 × 计件单价；批量报工直接 approved。
	w, out = batchWith(1, piece, "2026-09-17", nil, []map[string]interface{}{row(a, "3")})
	data = mustOK(t, w, out, "amount batch")
	if int64(data["amount_cents"].(float64)) != 600 {
		t.Fatalf("批量报工金额 = %v，期望 600", data["amount_cents"])
	}
	var batchRec WorkRecord
	if err := db.Where("date = ? AND worker_id = ?", "2026-09-17", a).First(&batchRec).Error; err != nil {
		t.Fatalf("load batch record: %v", err)
	}
	if batchRec.Status != RecordStatusApproved || batchRec.Source != RecordSourceBatch ||
		batchRec.AmountCents != 600 || batchRec.UnitPriceCents != 200 {
		t.Fatalf("批量报工落库字段错误: %+v", batchRec)
	}

	// 防重复：15 秒内整批都重复时拒绝，工资不翻倍。
	w, out = batchWith(1, piece, "2026-09-18", nil, []map[string]interface{}{row(b, "2")})
	mustOK(t, w, out, "first batch")
	w, out = batchWith(1, piece, "2026-09-18", nil, []map[string]interface{}{row(b, "2")})
	xtWantFail(t, w, out, "duplicate batch")
	xtWantMsg(t, out, "重复", "duplicate batch")
	if got := countOn("2026-09-18"); got != 1 {
		t.Fatalf("重复批量提交落库 %d 条，期望 1", got)
	}
	// 换数量不算重复：允许同一天分多次补报。
	w, out = batchWith(1, piece, "2026-09-18", nil, []map[string]interface{}{row(b, "3")})
	mustOK(t, w, out, "batch with new quantity")
	if got := countOn("2026-09-18"); got != 2 {
		t.Fatalf("同一天不同数量的补报应落第 2 条，实际 %d 条", got)
	}
	t.Logf("批量报工已加 15 秒重复提交拦截：整批全重复才拒绝，部分重复时其余行照常落库（同一天改数量补报不受影响）")
}

// ---- 6. 审核：已结算 / 批次原子性 / 并发 ----

func TestReviewSettledRecordGuard(t *testing.T) {
	r, db := setupTest(t)
	wid := xtCreateWorker(t, r, 1, "王五")

	var ids []uint
	for i, date := range []string{"2026-09-01", "2026-09-02"} {
		w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
			"worker_id": wid, "date": date, "type": "day", "quantity": "1", "unit_price_cents": 30000,
		})
		rec := mustOK(t, w, out, fmt.Sprintf("create approved record %d", i))
		ids = append(ids, uint(rec["id"].(float64)))
	}
	w, out := doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": wid, "start": "2026-09-01", "end": "2026-09-30",
	})
	mustOK(t, w, out, "settle records")

	// 已结算（此时 approved）再审核必须失败，且状态/结算绑定不变。
	w, out = xtApprove(t, r, 1, ids[:1])
	xtWantFail(t, w, out, "approve settled record")
	w, out = xtReject(t, r, 1, ids[:1], "改错了")
	xtWantFail(t, w, out, "reject settled record")
	rec := xtRecord(t, db, ids[0])
	if rec.Status != RecordStatusApproved || rec.SettlementID == nil {
		t.Fatalf("被拒绝的审核仍改动了已结算记录: %+v", rec)
	}

	// 伪造「待审核 + 已结算」（历史脏数据/并发窗口）：审核必须被 SettlementID 拦住。
	forged := xtPending(t, db, 1, wid, 0)
	if err := db.Model(&WorkRecord{}).Where("id = ?", forged.ID).Update("settlement_id", 999).Error; err != nil {
		t.Fatalf("forge settled pending: %v", err)
	}
	w, out = xtApprove(t, r, 1, []uint{forged.ID})
	xtWantFail(t, w, out, "approve forged settled pending")
	xtWantMsg(t, out, "已结算", "approve forged settled pending")
	if got := xtRecord(t, db, forged.ID); got.Status != RecordStatusPending || got.ReviewedAt != nil {
		t.Fatalf("被拦下的审核改动了记录: %+v", got)
	}

	// 混合批次：一条正常待审 + 一条已结算待审 → 整批失败，正常那条保持待审。
	pending := xtPending(t, db, 1, wid, 0)
	w, out = xtApprove(t, r, 1, []uint{pending.ID, forged.ID})
	xtWantFail(t, w, out, "mixed batch with settled")
	if got := xtRecord(t, db, pending.ID); got.Status != RecordStatusPending {
		t.Fatalf("整批失败时正常记录被部分提交: %+v", got)
	}
}

func TestReviewBatchAtomicAndScope(t *testing.T) {
	r, db := setupTest(t)
	team := xtNewTeam(t, db, 1, "审核班组", true)
	_ = xtJoinAs(t, db, team, 2, RoleLeader, "组长李")
	_ = xtJoinAs(t, db, team, 3, RoleWorker, "员工王")
	bossWorker := xtCreateWorker(t, r, 1, "老板工人")

	pendings := make([]WorkRecord, 0, 6)
	for i := 0; i < 6; i++ {
		pendings = append(pendings, xtPending(t, db, 1, bossWorker, 0))
	}

	// 批内去重：重复 id 只算一次。
	w, out := xtApprove(t, r, 1, []uint{pendings[0].ID, pendings[1].ID, pendings[0].ID})
	data := mustOK(t, w, out, "approve with duplicate ids")
	if data["updated"].(float64) != 2 {
		t.Fatalf("批内重复 id 的 updated = %v，期望 2", data["updated"])
	}

	// 重复点击：第二次必须失败（不能把 approved 再改一遍）。
	w, out = xtApprove(t, r, 1, []uint{pendings[0].ID, pendings[1].ID})
	xtWantFail(t, w, out, "approve twice")

	// 部分已处理 → 整批回滚，未处理那条仍待审。
	w, out = xtApprove(t, r, 1, []uint{pendings[2].ID, pendings[0].ID})
	xtWantFail(t, w, out, "partial processed batch")
	if got := xtRecord(t, db, pendings[2].ID); got.Status != RecordStatusPending {
		t.Fatalf("整批失败时未处理记录被改动: %+v", got)
	}

	// 组长可以审核老板数据集；员工不行。
	w, out = xtApprove(t, r, 2, []uint{pendings[3].ID})
	mustOK(t, w, out, "leader approve")
	w, out = xtApprove(t, r, 3, []uint{pendings[2].ID})
	xtWantCode(t, w, http.StatusForbidden, "worker approve")

	// 跨数据集：别人的记录既查不到也审不了。
	other := xtPending(t, db, 7, 700, 0)
	w, out = xtApprove(t, r, 1, []uint{other.ID})
	xtWantFail(t, w, out, "approve other dataset record")
	xtWantMsg(t, out, "不存在", "approve other dataset record")
	w, out = xtApprove(t, r, 7, []uint{pendings[2].ID})
	xtWantFail(t, w, out, "outsider approve boss record")

	// 参数边界：空 ids / 全 0 / 超过 200 条。
	w, out = xtApprove(t, r, 1, nil)
	xtWantFail(t, w, out, "empty ids")
	w, out = xtApprove(t, r, 1, []uint{0})
	xtWantFail(t, w, out, "zero ids")
	tooMany := make([]uint, 0, 201)
	for i := 0; i < 201; i++ {
		tooMany = append(tooMany, uint(i+1))
	}
	w, out = xtApprove(t, r, 1, tooMany)
	xtWantFail(t, w, out, "too many ids")
	xtWantMsg(t, out, "200", "too many ids")

	// 驳回：默认原因 + 备注截断到 120 字。
	w, out = xtReject(t, r, 1, []uint{pendings[4].ID}, "")
	mustOK(t, w, out, "reject with default note")
	got := xtRecord(t, db, pendings[4].ID)
	if got.Status != RecordStatusRejected || got.ReviewNote != "不符合报工要求" ||
		got.ReviewedAt == nil || got.ReviewedBy == nil {
		t.Fatalf("驳回记录字段错误: %+v", got)
	}
	w, out = xtReject(t, r, 1, []uint{pendings[5].ID}, strings.Repeat("很", 200))
	mustOK(t, w, out, "reject with long note")
	got = xtRecord(t, db, pendings[5].ID)
	if n := len([]rune(got.ReviewNote)); n != 120 {
		t.Fatalf("驳回原因应截断到 120 字，实际 %d", n)
	}

	// 审核列表带审核人姓名字段也不能 panic（users 表里可能没有该账号行）。
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status=approved", 1, nil)
	mustOKPage(t, w, out, "review list approved")
}

func TestReviewConcurrentApprove(t *testing.T) {
	r, db := setupTest(t)
	wid := xtCreateWorker(t, r, 1, "并发工人")
	rec := xtPending(t, db, 1, wid, 0)

	const attempts = 8
	codes := make([]int, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w, _ := doReq(r, http.MethodPost, "/api/worklog/review/approve", 1, map[string]interface{}{"ids": []uint{rec.ID}})
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()

	success := 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			success++
		case http.StatusBadRequest:
		default:
			t.Fatalf("并发审核出现意外状态码 %d", code)
		}
	}
	if success != 1 {
		t.Fatalf("并发 %d 次审核同一条记录，生效次数 = %d，期望恰好 1", attempts, success)
	}
	got := xtRecord(t, db, rec.ID)
	if got.Status != RecordStatusApproved || got.ReviewedAt == nil {
		t.Fatalf("并发审核后记录状态错误: %+v", got)
	}
	t.Logf("并发 %d 次审核同一记录：仅 1 次生效（事务内先读 + WHERE status=pending + RowsAffected 校验）", attempts)

	// 通过/驳回混跑：只能有一个动作生效，最终状态与胜者一致。
	rec2 := xtPending(t, db, 1, wid, 0)
	approveCodes := make([]int, 4)
	rejectCodes := make([]int, 4)
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			w, _ := doReq(r, http.MethodPost, "/api/worklog/review/approve", 1, map[string]interface{}{"ids": []uint{rec2.ID}})
			approveCodes[i] = w.Code
		}(i)
		go func(i int) {
			defer wg.Done()
			w, _ := doReq(r, http.MethodPost, "/api/worklog/review/reject", 1, map[string]interface{}{"ids": []uint{rec2.ID}, "note": "并发驳回"})
			rejectCodes[i] = w.Code
		}(i)
	}
	wg.Wait()

	countOK := func(list []int) int {
		n := 0
		for _, c := range list {
			if c == http.StatusOK {
				n++
			}
		}
		return n
	}
	okApprove, okReject := countOK(approveCodes), countOK(rejectCodes)
	if okApprove+okReject != 1 {
		t.Fatalf("通过/驳回混跑生效次数 = %d，期望恰好 1", okApprove+okReject)
	}
	final := xtRecord(t, db, rec2.ID)
	want := RecordStatusRejected
	if okApprove == 1 {
		want = RecordStatusApproved
	}
	if final.Status != want {
		t.Fatalf("混跑后状态 = %q，与胜出动作 %q 不一致", final.Status, want)
	}
}

// ---- 7. 报工码权限与数据集 ----

func TestEntryCodePermissionsAndDataset(t *testing.T) {
	r, db := setupTest(t)
	team := xtNewTeam(t, db, 1, "报工班组", false)
	_ = xtJoinAs(t, db, team, 2, RoleLeader, "组长")
	_ = xtJoinAs(t, db, team, 3, RoleWorker, "员工王")
	wid := xtCreateWorker(t, r, 1, "张三")
	pid := xtCreatePiece(t, r, 1, "打包", "件", 250)
	code := xtCreateCode(t, r, 1, map[string]interface{}{"name": "老板的码", "piece_item_id": pid})
	codeID := int(code["id"].(float64))
	token := code["token"].(string)

	// 员工（worker 角色）对报工码管理、审核、批量报工全部 403。
	today := time.Now().Format("2006-01-02")
	batchBody := map[string]interface{}{
		"date": today, "piece_item_id": pid,
		"rows": []map[string]interface{}{{"worker_id": wid, "quantity": "1"}},
	}
	workerCases := []struct {
		method string
		path   string
		body   interface{}
	}{
		{http.MethodGet, "/api/worklog/entry-codes", nil},
		{http.MethodPost, "/api/worklog/entry-codes", map[string]interface{}{"name": "偷偷建码", "piece_item_id": pid}},
		{http.MethodPut, "/api/worklog/entry-codes/" + itoa(codeID), map[string]interface{}{"enabled": false}},
		{http.MethodDelete, "/api/worklog/entry-codes/" + itoa(codeID), nil},
		{http.MethodPost, "/api/worklog/entry-codes/" + itoa(codeID) + "/rotate-token", nil},
		{http.MethodGet, "/api/worklog/review/summary", nil},
		{http.MethodGet, "/api/worklog/review/records", nil},
		{http.MethodPost, "/api/worklog/review/approve", map[string]interface{}{"ids": []uint{1}}},
		{http.MethodPost, "/api/worklog/review/reject", map[string]interface{}{"ids": []uint{1}}},
		{http.MethodPost, "/api/worklog/records/batch", batchBody},
	}
	for _, c := range workerCases {
		w, _ := doReq(r, c.method, c.path, 3, c.body)
		xtWantCode(t, w, http.StatusForbidden, "worker "+c.method+" "+c.path)
	}

	// 组长可以管理报工码，但数据归属必须是班组老板（user_id=1）。
	leaderCode := xtCreateCode(t, r, 2, map[string]interface{}{"name": "组长的码", "piece_item_id": pid})
	leaderCodeID := int(leaderCode["id"].(float64))
	var stored EntryCode
	if err := db.First(&stored, uint(leaderCodeID)).Error; err != nil {
		t.Fatalf("load leader code: %v", err)
	}
	if stored.UserID != 1 {
		t.Fatalf("组长创建的报工码 user_id=%d，期望归属班组老板 1", stored.UserID)
	}

	// 老板能看到组长建的码；按启用状态过滤。
	w, out := doReq(r, http.MethodGet, "/api/worklog/entry-codes", 1, nil)
	list := mustOK(t, w, out, "boss list codes")["items"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("老板应看到 2 个报工码，实际 %d", len(list))
	}
	w, _ = doReq(r, http.MethodPut, "/api/worklog/entry-codes/"+itoa(leaderCodeID), 1, map[string]interface{}{"enabled": false})
	xtWantCode(t, w, http.StatusOK, "boss disables leader code")
	w, out = doReq(r, http.MethodGet, "/api/worklog/entry-codes?status=disabled", 1, nil)
	if n := len(mustOK(t, w, out, "disabled list")["items"].([]interface{})); n != 1 {
		t.Fatalf("停用列表 = %d，期望 1", n)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/entry-codes?status=enabled", 1, nil)
	if n := len(mustOK(t, w, out, "enabled list")["items"].([]interface{})); n != 1 {
		t.Fatalf("启用列表 = %d，期望 1", n)
	}

	// 公开报工后的计数：today/total 1、pending 1；审核通过后 pending 归零、total 仍 1。
	w, out = xtSubmit(t, r, token, map[string]interface{}{"worker_id": wid, "quantity": "2"})
	mustOK(t, w, out, "public submit for counts")
	findStat := func() map[string]interface{} {
		w, out := doReq(r, http.MethodGet, "/api/worklog/entry-codes", 1, nil)
		for _, it := range mustOK(t, w, out, "list for counts")["items"].([]interface{}) {
			m := it.(map[string]interface{})
			if int(m["id"].(float64)) == codeID {
				return m
			}
		}
		return nil
	}
	stat := findStat()
	if stat == nil {
		t.Fatal("找不到报工码统计")
	}
	if stat["pending_count"].(float64) != 1 || stat["today_count"].(float64) != 1 || stat["total_count"].(float64) != 1 {
		t.Fatalf("报工码计数错误: %v", stat)
	}
	var rec WorkRecord
	if err := db.Where("entry_code_id = ?", codeID).First(&rec).Error; err != nil {
		t.Fatalf("load submitted record: %v", err)
	}
	w, out = xtApprove(t, r, 1, []uint{rec.ID})
	mustOK(t, w, out, "approve submitted")
	stat = findStat()
	if stat["pending_count"].(float64) != 0 || stat["total_count"].(float64) != 1 || stat["today_count"].(float64) != 1 {
		t.Fatalf("审核后报工码计数错误: %v", stat)
	}

	// 外部账号（同实例但不在该数据集）看不到、改不到别人的报工码。
	createTestUser(t, db, 5, "outsider")
	w, out = doReq(r, http.MethodGet, "/api/worklog/entry-codes", 5, nil)
	if n := len(mustOK(t, w, out, "outsider list")["items"].([]interface{})); n != 0 {
		t.Fatalf("外部账号看到了 %d 个报工码", n)
	}
	for _, method := range []string{http.MethodPut, http.MethodDelete} {
		body := map[string]interface{}{"enabled": false}
		if method == http.MethodDelete {
			body = nil
		}
		w, _ = doReq(r, method, "/api/worklog/entry-codes/"+itoa(codeID), 5, body)
		xtWantCode(t, w, http.StatusNotFound, "outsider "+method)
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/entry-codes/"+itoa(codeID)+"/rotate-token", 5, nil)
	xtWantCode(t, w, http.StatusNotFound, "outsider rotate")
}

// ---- 8. 报工码编辑校验 ----

func TestEntryCodeUpdateValidation(t *testing.T) {
	r, db := setupTest(t)
	wid := xtCreateWorker(t, r, 1, "张三")
	otherWid := xtCreateWorker(t, r, 1, "李四")
	pid := xtCreatePiece(t, r, 1, "打包", "件", 250)
	archivedPiece := xtCreatePiece(t, r, 1, "停用产品", "件", 100)
	w, _ := doReq(r, http.MethodPut, "/api/worklog/piece-items/"+itoa(int(archivedPiece)), 1, map[string]interface{}{
		"name": "停用产品", "unit": "件", "unit_price_cents": 100, "status": "archived",
	})
	xtWantCode(t, w, http.StatusOK, "archive piece")
	archivedWorker := xtCreateWorker(t, r, 1, "归档工人")
	if err := db.Model(&Worker{}).Where("id = ?", archivedWorker).Update("status", "archived").Error; err != nil {
		t.Fatalf("archive worker: %v", err)
	}

	code := xtCreateCode(t, r, 1, map[string]interface{}{
		"name": "可编辑码", "piece_item_id": pid, "worker_id": wid,
		"base_url": "https://work.example.com/", "expires_at": time.Now().AddDate(0, 0, 1).Format("2006-01-02"), "note": "备注",
	})
	codeID := uint(code["id"].(float64))
	put := func(body map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
		return doReq(r, http.MethodPut, "/api/worklog/entry-codes/"+itoa(int(codeID)), 1, body)
	}

	// 关联 id 传 0 = 清空（前端「解除绑定」）。
	w, out := put(map[string]interface{}{"piece_item_id": 0, "worker_id": 0})
	mustOK(t, w, out, "clear refs")
	var stored EntryCode
	if err := db.First(&stored, codeID).Error; err != nil {
		t.Fatalf("load cleared code: %v", err)
	}
	if stored.PieceItemID != nil || stored.WorkerID != nil {
		t.Fatalf("关联未清空: piece=%v worker=%v", stored.PieceItemID, stored.WorkerID)
	}
	w, out = xtPublicInfo(t, r, stored.Token)
	info := mustOK(t, w, out, "public info after clear")
	if info["product"] != nil || info["worker"] != nil {
		t.Fatalf("清空绑定后公开页仍有预选项: %v", info)
	}

	// 参数校验。
	w, out = put(map[string]interface{}{"base_url": "ftp://x"})
	xtWantFail(t, w, out, "bad base url")
	xtWantMsg(t, out, "http", "bad base url")
	w, out = put(map[string]interface{}{"expires_at": "2000-01-01"})
	xtWantFail(t, w, out, "expired date")
	xtWantMsg(t, out, "早于", "expired date")
	w, out = put(map[string]interface{}{"name": "a"})
	xtWantFail(t, w, out, "short name")
	w, out = put(map[string]interface{}{})
	xtWantFail(t, w, out, "empty update")
	w, out = put(map[string]interface{}{"note": strings.Repeat("备", 121)})
	xtWantFail(t, w, out, "long note")

	// 归属校验：别人的工地 / 归档的产品与工人。
	w, out = doReq(r, http.MethodPost, "/api/worklog/projects", 2, map[string]interface{}{"name": "别家工地"})
	otherProject := mustOK(t, w, out, "create other project")
	w, out = put(map[string]interface{}{"project_id": otherProject["id"]})
	xtWantFail(t, w, out, "other dataset project")
	xtWantMsg(t, out, "工地不存在", "other dataset project")
	w, out = put(map[string]interface{}{"piece_item_id": archivedPiece})
	xtWantFail(t, w, out, "archived piece")
	xtWantMsg(t, out, "已归档", "archived piece")
	w, out = put(map[string]interface{}{"worker_id": archivedWorker})
	xtWantFail(t, w, out, "archived worker")
	xtWantMsg(t, out, "已归档", "archived worker")

	// 合法更新：重新绑定 + 清空有效期。
	w, out = put(map[string]interface{}{"piece_item_id": pid, "worker_id": otherWid, "note": "改过", "expires_at": ""})
	mustOK(t, w, out, "valid update")
	if err := db.First(&stored, codeID).Error; err != nil {
		t.Fatalf("reload code: %v", err)
	}
	if stored.PieceItemID == nil || *stored.PieceItemID != pid || stored.WorkerID == nil || *stored.WorkerID != otherWid {
		t.Fatalf("合法更新未生效: %+v", stored)
	}
	if stored.ExpiresAt != nil || stored.Note != "改过" {
		t.Fatalf("清空有效期/备注更新失败: %+v", stored)
	}
}

// ---- 9. 员工经济权限 ----

func TestWorkerReviewEconomics(t *testing.T) {
	r, db := setupTest(t)
	team := xtNewTeam(t, db, 1, "审核班组", true)
	self := xtJoinAs(t, db, team, 2, RoleWorker, "小王")
	other := xtJoinAs(t, db, team, 3, RoleWorker, "小李")
	// 员工工资口径来自工人档案（createRecord 会忽略员工自带的 unit_price_cents）。
	if err := db.Model(&Worker{}).Where("id = ?", self.ID).Update("daily_wage_cents", 30000).Error; err != nil {
		t.Fatalf("set worker daily wage: %v", err)
	}
	today := time.Now().Format("2006-01-02")
	dashWork := func(uid int) int64 {
		w, out := doReq(r, http.MethodGet, "/api/worklog/stats/dashboard", uid, nil)
		return int64(mustOK(t, w, out, "dashboard")["month_work_cents"].(float64))
	}

	// 员工自带 unit_price_cents 会被忽略：按工人档案日薪 30000 记，不能自抬价。
	w, out := doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": self.ID, "date": today, "type": "day", "quantity": "1", "unit_price_cents": 999999,
	})
	rec := mustOK(t, w, out, "worker self record with price")
	if rec["status"] != RecordStatusPending {
		t.Fatalf("员工提交应落待审核，实际 %v", rec["status"])
	}
	if int64(rec["unit_price_cents"].(float64)) != 30000 {
		t.Fatalf("员工自带单价必须被忽略、按档案日薪 30000 记，实际 %v", rec["unit_price_cents"])
	}
	recID := uint(rec["id"].(float64))
	if got := dashWork(1); got != 0 {
		t.Fatalf("待审核记录不应计入工资，实际 %d", got)
	}

	// 计件：员工自带单价同样被忽略，按计件项目单价 100 算（5 × 100 = 500）。
	pid := xtCreatePiece(t, r, 1, "计件组装", "件", 100)
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": self.ID, "piece_item_id": pid, "date": today,
		"type": "piece", "quantity": "5", "unit_price_cents": 888888,
	})
	pieceRec := mustOK(t, w, out, "worker piece record")
	if int64(pieceRec["unit_price_cents"].(float64)) != 100 || int64(pieceRec["amount_cents"].(float64)) != 500 {
		t.Fatalf("计件员工自带单价应被忽略（项目价 100 × 5 = 500），实际 %v / %v",
			pieceRec["unit_price_cents"], pieceRec["amount_cents"])
	}

	// 员工不能自审，审核通过前工资为 0（审核是唯一闸门）。
	w, _ = xtApprove(t, r, 2, []uint{recID})
	xtWantCode(t, w, http.StatusForbidden, "worker self approve")
	w, _ = xtReject(t, r, 2, []uint{recID}, "")
	xtWantCode(t, w, http.StatusForbidden, "worker self reject")
	w, out = xtApprove(t, r, 1, []uint{recID})
	mustOK(t, w, out, "boss approve")
	if got := dashWork(1); got != 30000 {
		t.Fatalf("审核通过后应计入档案日薪 30000，实际 %d", got)
	}

	// 员工没有工资档案时，自带单价也救不了：必须报「请填写大于 0 的单价」。
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 3, map[string]interface{}{
		"worker_id": other.ID, "date": today, "type": "day", "quantity": "1", "unit_price_cents": 888888,
	})
	xtWantFail(t, w, out, "worker without wage")
	xtWantMsg(t, out, "单价", "worker without wage")

	// 员工不能改/删自己已审核通过的记录。
	w, _ = doReq(r, http.MethodPut, "/api/worklog/records/"+itoa(int(recID)), 2, map[string]interface{}{
		"worker_id": self.ID, "date": today, "type": "day", "quantity": "2",
	})
	xtWantCode(t, w, http.StatusBadRequest, "worker update approved")
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/records/"+itoa(int(recID)), 2, nil)
	xtWantCode(t, w, http.StatusBadRequest, "worker delete approved")

	// 员工不能改/删别人的记录。
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": other.ID, "date": today, "type": "day", "quantity": "1", "unit_price_cents": 30000,
	})
	otherRec := mustOK(t, w, out, "boss records other worker")
	otherID := uint(otherRec["id"].(float64))
	w, _ = doReq(r, http.MethodPut, "/api/worklog/records/"+itoa(int(otherID)), 2, map[string]interface{}{
		"worker_id": other.ID, "date": today, "type": "day", "quantity": "2",
	})
	xtWantCode(t, w, http.StatusForbidden, "worker update others record")
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/records/"+itoa(int(otherID)), 2, nil)
	xtWantCode(t, w, http.StatusForbidden, "worker delete others record")

	// 员工可以删除自己被驳回的记录（不能删除已通过的）。
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": self.ID, "date": today, "type": "day", "quantity": "1", "unit_price_cents": 30000,
	})
	rejected := mustOK(t, w, out, "worker second record")
	rejectedID := uint(rejected["id"].(float64))
	w, out = xtReject(t, r, 1, []uint{rejectedID}, "数量不对")
	mustOK(t, w, out, "boss rejects")
	w, out = doReq(r, http.MethodPut, "/api/worklog/records/"+itoa(int(rejectedID)), 2, map[string]interface{}{
		"worker_id": self.ID, "date": today, "type": "day", "quantity": "1", "unit_price_cents": 30000,
	})
	xtWantFail(t, w, out, "worker update rejected record")
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/records/"+itoa(int(rejectedID)), 2, nil)
	xtWantCode(t, w, http.StatusOK, "worker delete rejected record")

	// 关闭班组审核开关：员工提交直接生效，且单价仍由员工自己给（经济风险）。
	w, _ = doReq(r, http.MethodPut, "/api/worklog/teams/"+itoa(int(team.ID)), 1, map[string]interface{}{"review_required": false})
	xtWantCode(t, w, http.StatusOK, "disable review required")
	before := dashWork(1)
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": self.ID, "date": today, "type": "day", "quantity": "1", "unit_price_cents": 888888,
	})
	direct := mustOK(t, w, out, "worker direct record after review off")
	if direct["status"] != RecordStatusApproved {
		t.Fatalf("关闭审核后员工提交应直接生效，实际 %v", direct["status"])
	}
	if int64(direct["unit_price_cents"].(float64)) != 30000 {
		t.Fatalf("关闭审核后员工自带单价仍应被忽略，实际 %v", direct["unit_price_cents"])
	}
	if delta := dashWork(1) - before; delta != 30000 {
		t.Fatalf("员工提交按档案日薪计入 30000，实际增量 %d", delta)
	}
	t.Logf("员工自带单价已被忽略（createRecord/updateRecord 对 worker 角色清空 unit_price_cents）；" +
		"副作用：未配置日薪/时薪的员工提交点工/点时会被拒（提示请填写大于 0 的单价），需先由班组长设单价")
}

// ---- 10. 组长建档案的数据集归属 ----

// TestLeaderCreatedWorkerAndPieceDataset 防回归：组长建的工人/计件项目必须归班组老板
// 数据集（createWorker/createPieceItem 曾用 currentUID，导致组长建完自己都用不了）。
func TestLeaderCreatedWorkerAndPieceDataset(t *testing.T) {
	r, db := setupTest(t)
	bindLeader(t, db, 2) // user2 是 uid1 班组的组长

	// 组长建工人：requireManage 放行，但 createWorker 用 currentUID 落 UserID。
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 2, map[string]interface{}{"name": "组长建的人"})
	workerData := mustOK(t, w, out, "leader create worker")
	var storedWorker Worker
	if err := db.First(&storedWorker, uint(workerData["id"].(float64))).Error; err != nil {
		t.Fatalf("load leader worker: %v", err)
	}
	if storedWorker.UserID != 1 {
		t.Fatalf("组长创建的工人 user_id=%d，期望归属班组老板 1（否则组长建完自己也无法用于记工）", storedWorker.UserID)
	}
	for _, uid := range []int{1, 2} {
		w, out = doReq(r, http.MethodGet, "/api/worklog/workers", uid, nil)
		found := false
		for _, it := range mustOKList(t, w, out, "list workers") {
			if it.(map[string]interface{})["name"] == "组长建的人" {
				found = true
			}
		}
		if !found {
			t.Fatalf("uid=%d 看不到组长建的工人（应归班组老板数据集）", uid)
		}
	}

	// 组长建计件项目同理（归属错），且建完自己也用不了。
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 2, map[string]interface{}{
		"name": "组长建的产品", "unit": "件", "unit_price_cents": 100,
	})
	pieceData := mustOK(t, w, out, "leader create piece item")
	var storedPiece PieceItem
	if err := db.First(&storedPiece, uint(pieceData["id"].(float64))).Error; err != nil {
		t.Fatalf("load leader piece: %v", err)
	}
	if storedPiece.UserID != 1 {
		t.Fatalf("组长创建的计件项目 user_id=%d，期望归属班组老板 1", storedPiece.UserID)
	}
	batchDate := time.Now().Format("2006-01-02")
	w, out = doReq(r, http.MethodPost, "/api/worklog/records/batch", 2, map[string]interface{}{
		"date": batchDate, "piece_item_id": pieceData["id"],
		"rows": []map[string]interface{}{{"worker_id": storedWorker.ID, "quantity": "1"}},
	})
	mustOK(t, w, out, "leader batch with team archives")
	w, out = doReq(r, http.MethodPost, "/api/worklog/records/batch", 1, map[string]interface{}{
		"date": batchDate, "piece_item_id": pieceData["id"],
		"rows": []map[string]interface{}{{"worker_id": storedWorker.ID, "quantity": "2"}},
	})
	mustOK(t, w, out, "boss batch with leader archives")

	// 对照：工地的创建用的是 scopeUser，归属正确（说明工人/计件项目是漏改，不是设计）。
	w, out = doReq(r, http.MethodPost, "/api/worklog/projects", 2, map[string]interface{}{"name": "组长建的工地"})
	projectData := mustOK(t, w, out, "leader create project")
	var storedProject Project
	if err := db.First(&storedProject, uint(projectData["id"].(float64))).Error; err != nil {
		t.Fatalf("load leader project: %v", err)
	}
	if storedProject.UserID != 1 {
		t.Fatalf("组长创建的工地 user_id=%d，期望归属班组老板 1", storedProject.UserID)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/projects", 1, nil)
	found := false
	for _, it := range mustOKList(t, w, out, "boss list projects") {
		if it.(map[string]interface{})["name"] == "组长建的工地" {
			found = true
		}
	}
	if !found {
		t.Fatal("老板看不到组长创建的工地")
	}
}

// ---- 11. 审核列表与汇总口径 ----

func TestReviewListFiltersAndSummary(t *testing.T) {
	r, db := setupTest(t)
	wid := xtCreateWorker(t, r, 1, "甲")
	today := time.Now().Format("2006-01-02")

	pending := xtPending(t, db, 1, wid, 0) // amount 100，日期今天
	w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": wid, "date": today, "type": "day", "quantity": "1", "unit_price_cents": 30000,
	})
	mustOK(t, w, out, "create approved record")

	w, out = doReq(r, http.MethodGet, "/api/worklog/review/summary", 1, nil)
	s := mustOK(t, w, out, "review summary")
	checks := map[string]float64{
		"pending_count":               1,
		"pending_amount_cents":        100,
		"today_pending_count":         1,
		"today_pending_amount_cents":  100,
		"today_approved_count":        1,
		"today_approved_amount_cents": 30000,
		"rejected_count":              0,
	}
	for k, want := range checks {
		if got := s[k].(float64); got != want {
			t.Fatalf("review/summary %s = %v，期望 %v", k, got, want)
		}
	}

	// 驳回一条待审记录：pending 仍 1，rejected 变 1。
	rej := xtPending(t, db, 1, wid, 0)
	w, out = xtReject(t, r, 1, []uint{rej.ID}, "不符合")
	mustOK(t, w, out, "reject for summary")
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/summary", 1, nil)
	s = mustOK(t, w, out, "review summary after reject")
	if s["pending_count"].(float64) != 1 || s["rejected_count"].(float64) != 1 {
		t.Fatalf("驳回后汇总错误: %v", s)
	}

	// 列表状态过滤。
	for _, tc := range []struct {
		status string
		want   float64
	}{
		{RecordStatusPending, 1},
		{RecordStatusApproved, 1},
		{RecordStatusRejected, 1},
		{"all", 3},
	} {
		w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status="+tc.status, 1, nil)
		page := mustOKPage(t, w, out, "review list "+tc.status)
		if page["total"].(float64) != tc.want {
			t.Fatalf("review list status=%s total=%v，期望 %v", tc.status, page["total"], tc.want)
		}
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status=bogus", 1, nil)
	xtWantFail(t, w, out, "invalid status")

	// 日期 / 工人 / 报工码过滤。
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status=pending&worker_id="+itoa(int(wid))+"&date="+today, 1, nil)
	if page := mustOKPage(t, w, out, "filtered review list"); page["total"].(float64) != 1 {
		t.Fatalf("工人+日期过滤 total=%v，期望 1", page["total"])
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status=pending&date=1999-01-01", 1, nil)
	if page := mustOKPage(t, w, out, "empty date review list"); page["total"].(float64) != 0 {
		t.Fatalf("空日期过滤 total=%v，期望 0", page["total"])
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status=pending&entry_code_id=9999", 1, nil)
	if page := mustOKPage(t, w, out, "entry code filter"); page["total"].(float64) != 0 {
		t.Fatalf("报工码过滤 total=%v，期望 0", page["total"])
	}
	// 待审记录带报工码 id 时应能被过滤到。
	pid := xtCreatePiece(t, r, 1, "筛选用产品", "件", 100)
	code := xtCreateCode(t, r, 1, map[string]interface{}{"name": "筛选码", "piece_item_id": pid})
	codeID := uint(code["id"].(float64))
	if err := db.Model(&WorkRecord{}).Where("id = ?", pending.ID).Update("entry_code_id", codeID).Error; err != nil {
		t.Fatalf("bind entry code: %v", err)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status=pending&entry_code_id="+itoa(int(codeID)), 1, nil)
	if page := mustOKPage(t, w, out, "bound entry code filter"); page["total"].(float64) != 1 {
		t.Fatalf("按报工码过滤 total=%v，期望 1", page["total"])
	}

	// 外部账号看不到这些待审/已审数据。
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/summary", 9, nil)
	s = mustOK(t, w, out, "outsider summary")
	if s["pending_count"].(float64) != 0 || s["rejected_count"].(float64) != 0 {
		t.Fatalf("外部账号看到了别人的审核数据: %v", s)
	}
}
