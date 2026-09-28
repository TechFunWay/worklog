package worklog

import (
	"bytes"
	"encoding/json"
	"strconv"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/database"
)

// setupTest 每个用例独立临时库 + 假认证中间件（X-Test-User 指定 userID）。
func setupTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("init db: %v", err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	r := gin.New()
	api := r.Group("/api")
	api.Use(func(c *gin.Context) {
		uid := uint(1)
		if v := c.GetHeader("X-Test-User"); v != "" {
			if n, err := strconv.ParseUint(v, 10, 32); err == nil {
				uid = uint(n)
			}
		}
		c.Set("userID", uid)
		c.Next()
	})
	setupRoutes(api, db)
	return r, db
}

func doReq(r *gin.Engine, method, path string, uid int, body interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	var buf io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		buf = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if uid > 0 {
		req.Header.Set("X-Test-User", itoa(uid))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		out = map[string]interface{}{}
	}
	return w, out
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}

func mustOK(t *testing.T, w *httptest.ResponseRecorder, out map[string]interface{}, ctx string) map[string]interface{} {
	t.Helper()
	if w.Code != http.StatusOK || out["code"].(float64) != 0 {
		t.Fatalf("%s: http=%d body=%s", ctx, w.Code, w.Body.String())
	}
	return out["data"].(map[string]interface{})
}


func mustOKList(t *testing.T, w *httptest.ResponseRecorder, out map[string]interface{}, ctx string) []interface{} {
	t.Helper()
	if w.Code != http.StatusOK || out["code"].(float64) != 0 {
		t.Fatalf("%s: http=%d body=%s", ctx, w.Code, w.Body.String())
	}
	return out["data"].([]interface{})
}

// ---- 纯函数 ----

func TestParseQuantity(t *testing.T) {
	if _, err := parseQuantity("0.5"); err != nil {
		t.Fatalf("0.5 should parse: %v", err)
	}
	if _, err := parseQuantity("0.005"); err == nil {
		t.Fatalf("3-decimal should fail")
	}
	if _, err := parseQuantity("-2"); err == nil {
		t.Fatalf("negative should fail")
	}
	if _, err := parseQuantity("abc"); err == nil {
		t.Fatalf("non-numeric should fail")
	}
	if got := computeAmountCents(mustRat("0.5"), 30000); got != 15000 {
		t.Fatalf("half day amount = %d, want 15000", got)
	}
	if got := computeAmountCents(mustRat("8.25"), 2400); got != 19800 {
		t.Fatalf("8.25h amount = %d, want 19800", got)
	}
	if !quantityMultipleOK(TypeDay, mustRat("1.5")) || quantityMultipleOK(TypeDay, mustRat("1.25")) {
		t.Fatalf("day step 0.5 check failed")
	}
	if !quantityMultipleOK(TypeHour, mustRat("8.25")) || quantityMultipleOK(TypeHour, mustRat("8.3")) {
		t.Fatalf("hour step 0.25 check failed")
	}
	if !quantityMultipleOK(TypePiece, mustRat("120")) || quantityMultipleOK(TypePiece, mustRat("1.5")) {
		t.Fatalf("piece integer check failed")
	}
}

func mustRat(s string) *big.Rat {
	r, _ := new(big.Rat).SetString(s)
	return r
}

// ---- 业务流 ----

func seedWorker(t *testing.T, r *gin.Engine, name string, daily, hourly int64) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{
		"name": name, "daily_wage_cents": daily, "hourly_wage_cents": hourly,
	})
	return mustOK(t, w, out, "create worker "+name)
}

func TestWorkerCRUDAndGuard(t *testing.T) {
	r, _ := setupTest(t)
	worker := seedWorker(t, r, "张三", 30000, 3000)

	// 列表
	w, out := doReq(r, http.MethodGet, "/api/worklog/workers", 1, nil)
	data := mustOKList(t, w, out, "list workers")
	if len(data) != 1 {
		t.Fatalf("expected 1 worker, got %v", data)
	}

	// 更新
	wid := int(worker["id"].(float64))
	w, out = doReq(r, http.MethodPut, "/api/worklog/workers/"+itoa(wid), 1, map[string]interface{}{
		"name": "张三丰", "phone": "13800000000", "daily_wage_cents": 35000, "hourly_wage_cents": 3500, "status": "active",
	})
	mustOK(t, w, out, "update worker")

	// 有记工记录时禁止删除
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": wid, "date": "2026-09-01", "type": "day", "quantity": "1",
	})
	mustOK(t, w, out, "create record")
	w, out = doReq(r, http.MethodDelete, "/api/worklog/workers/"+itoa(wid), 1, nil)
	if w.Code == http.StatusOK {
		t.Fatalf("worker delete should be blocked when records exist")
	}

	// user_id 隔离：用户 2 看不到
	w, out = doReq(r, http.MethodGet, "/api/worklog/workers", 2, nil)
	data = mustOKList(t, w, out, "list workers as user2")
	if len(data) != 0 {
		t.Fatalf("user2 should see no workers")
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/workers/"+itoa(wid), 2, nil)
	if w.Code == http.StatusOK {
		t.Fatalf("user2 should not access user1's worker")
	}
}

func TestRecordTypesAndAmounts(t *testing.T) {
	r, _ := setupTest(t)
	worker := seedWorker(t, r, "李四", 30000, 2500)
	wid := int(worker["id"].(float64))

	cases := []struct {
		typ       string
		quantity  string
		price     interface{}
		wantCents int64
	}{
		{"day", "1", nil, 30000},
		{"day", "0.5", nil, 15000},
		{"hour", "8.5", nil, 21250},
		{"hour", "8.5", 2400, 20400}, // 单价覆盖
	}
	for i, tc := range cases {
		w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
			"worker_id": wid, "date": "2026-09-01", "type": tc.typ, "quantity": tc.quantity, "unit_price_cents": tc.price,
		})
		data := mustOK(t, w, out, "create record case "+itoa(i))
		if int64(data["amount_cents"].(float64)) != tc.wantCents {
			t.Fatalf("case %d amount = %v, want %d", i, data["amount_cents"], tc.wantCents)
		}
	}

	// 步进校验
	w, _ := doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": wid, "date": "2026-09-02", "type": "day", "quantity": "0.3",
	})
	if w.Code == http.StatusOK {
		t.Fatalf("day 0.3 should be rejected")
	}
	// 计件必须选项目
	w, _ = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": wid, "date": "2026-09-02", "type": "piece", "quantity": "120",
	})
	if w.Code == http.StatusOK {
		t.Fatalf("piece without item should be rejected")
	}
}

func TestSettlementFlow(t *testing.T) {
	r, db := setupTest(t)
	worker := seedWorker(t, r, "王五", 30000, 2500)
	wid := int(worker["id"].(float64))

	// 记工：1天 + 0.5天 + 8小时 = 30000 + 15000 + 20000 = 65000 分
	for _, rec := range []map[string]interface{}{
		{"worker_id": wid, "date": "2026-09-01", "type": "day", "quantity": "1"},
		{"worker_id": wid, "date": "2026-09-02", "type": "day", "quantity": "0.5"},
		{"worker_id": wid, "date": "2026-09-03", "type": "hour", "quantity": "8"},
	} {
		w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, rec)
		mustOK(t, w, out, "seed record")
	}
	// 借支 100000 分
	w, out := doReq(r, http.MethodPost, "/api/worklog/advances", 1, map[string]interface{}{
		"worker_id": wid, "date": "2026-09-02", "amount_cents": 100000, "method": "wechat",
	})
	mustOK(t, w, out, "create advance")

	// 未结算汇总
	w, out = doReq(r, http.MethodGet, "/api/worklog/settlements/unsettled", 1, nil)
	list := mustOKList(t, w, out, "unsettled list")
	if len(list) != 1 {
		t.Fatalf("expected 1 unsettled worker, got %d", len(list))
	}
	s := list[0].(map[string]interface{})
	if int64(s["work_amount_cents"].(float64)) != 65000 || int64(s["advance_amount_cents"].(float64)) != 100000 {
		t.Fatalf("unsettled summary wrong: %v", s)
	}
	if int64(s["payable_cents"].(float64)) != -35000 {
		t.Fatalf("payable should be -35000 (借支超发), got %v", s["payable_cents"])
	}

	// 已结算前可改删
	var recCount int64
	db.Model(&WorkRecord{}).Where("user_id = 1").Count(&recCount)
	if recCount != 3 {
		t.Fatalf("expected 3 records, got %d", recCount)
	}

	// 结算
	w, out = doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": wid, "start": "2026-09-01", "end": "2026-09-30", "note": "9月工资",
	})
	settleData := mustOK(t, w, out, "settle")
	sid := int(settleData["id"].(float64))
	if int64(settleData["work_amount_cents"].(float64)) != 65000 || int64(settleData["payable_cents"].(float64)) != -35000 {
		t.Fatalf("settlement totals wrong: %v", settleData)
	}
	var detail settlementDetail
	if err := json.Unmarshal([]byte(settleData["details"].(string)), &detail); err != nil {
		t.Fatalf("details unmarshal: %v", err)
	}
	if len(detail.Records) != 3 || len(detail.Advances) != 1 {
		t.Fatalf("details should have 3 records + 1 advance")
	}

	// 记录挂单后不可改删
	w, out = doReq(r, http.MethodGet, "/api/worklog/records/list", 1, nil)
	listData := mustOK(t, w, out, "records list")
	first := listData["items"].([]interface{})[0].(map[string]interface{})
	rid := int(first["id"].(float64))
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/records/"+itoa(rid), 1, nil)
	if w.Code == http.StatusOK {
		t.Fatalf("settled record delete should be blocked")
	}

	// 再查未结算：为空
	w, out = doReq(r, http.MethodGet, "/api/worklog/settlements/unsettled", 1, nil)
	remain := mustOKList(t, w, out, "unsettled after settle")
	if len(remain) != 0 {
		t.Fatalf("no unsettled should remain")
	}

	// 撤销结算
	w, out = doReq(r, http.MethodDelete, "/api/worklog/settlements/"+itoa(sid), 1, nil)
	mustOK(t, w, out, "revert settlement")
	var locked int64
	db.Model(&WorkRecord{}).Where("settlement_id IS NOT NULL").Count(&locked)
	if locked != 0 {
		t.Fatalf("records should be unlocked after revert")
	}
}

func TestBatchSettlementAndMonthlyStats(t *testing.T) {
	r, _ := setupTest(t)
	w1 := seedWorker(t, r, "赵六", 30000, 0)
	w2 := seedWorker(t, r, "钱七", 20000, 0)
	for _, rec := range []map[string]interface{}{
		{"worker_id": w1["id"], "date": "2026-09-01", "type": "day", "quantity": "2"},
		{"worker_id": w2["id"], "date": "2026-09-01", "type": "day", "quantity": "1"},
	} {
		w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, rec)
		mustOK(t, w, out, "seed record")
	}

	// 月度统计
	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/monthly?year=2026&month=9", 1, nil)
	data := mustOK(t, w, out, "monthly stats")
	rows := data["rows"].([]interface{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 stat rows, got %d", len(rows))
	}

	// 一键月结
	w, out = doReq(r, http.MethodPost, "/api/worklog/settlements/batch", 1, map[string]interface{}{
		"start": "2026-09-01", "end": "2026-09-30",
	})
	data = mustOK(t, w, out, "batch settle")
	if len(data["settled"].([]interface{})) != 2 {
		t.Fatalf("expected 2 settlements, got %v", data)
	}

	// 已结账列表
	w, out = doReq(r, http.MethodGet, "/api/worklog/settlements", 1, nil)
	data = mustOK(t, w, out, "settlement list")
	if data["total"].(float64) != 2 {
		t.Fatalf("expected 2 settlements, got %v", data["total"])
	}

	// 看板
	w, out = doReq(r, http.MethodGet, "/api/worklog/stats/dashboard", 1, nil)
	data = mustOK(t, w, out, "dashboard")
	if data["settled_cents"].(float64) != 80000 {
		t.Fatalf("settled_cents should be 80000, got %v", data["settled_cents"])
	}
}

func TestExportsAndBackup(t *testing.T) {
	r, db := setupTest(t)
	worker := seedWorker(t, r, "孙八", 30000, 0)
	w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": worker["id"], "date": "2026-09-01", "type": "day", "quantity": "1",
	})
	mustOK(t, w, out, "create record")

	for _, path := range []string{
		"/api/worklog/export/records.csv",
		"/api/worklog/export/monthly.csv?year=2026&month=9",
		"/api/worklog/export/advances.csv",
		"/api/worklog/export/settlements.csv",
		"/api/worklog/export/json",
	} {
		w, _ := doReq(r, http.MethodGet, path, 1, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("%s export failed: %d %s", path, w.Code, w.Body.String())
		}
	}
	// CSV 需带 UTF-8 BOM
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/records.csv", 1, nil)
	if !strings.HasPrefix(w.Body.String(), "\ufeff") {
		t.Fatalf("csv should start with UTF-8 BOM")
	}

	// db 备份（挂到独立引擎验证 handler）
	g := gin.New()
	admin := g.Group("/api")
	admin.Use(func(c *gin.Context) { c.Set("userID", uint(1)); c.Next() })
	setupAdminRoutes(admin, db)
	req := httptest.NewRequest(http.MethodGet, "/api/worklog/export/db", nil)
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("db backup failed: %d", rec.Code)
	}
}
