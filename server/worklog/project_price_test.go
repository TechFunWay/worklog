package worklog

// project_price_test.go — 2026-10-23 第 2 轮粉丝反馈：工地单价。
//
// 覆盖：
//   - 工地默认日工/小时单价的增改与校验
//   - 价格优先级：手动单价 > 工人在该工地的单独单价 > 工地默认单价 > 工人月薪折算/日薪/时薪
//   - 一键记工（POST /records 不传单价）自动按工地单价算钱
//   - 工地单价管理接口的权限（组长可管、员工 403、越权 404/400）
//   - 删除工地/工人时清理单独单价、JSON 全量导出包含 project_prices

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// bindLeader 建一个归 uid=1 的班组，把 userID 设为组长（不绑工人档案）。
func bindLeader(t *testing.T, db *gorm.DB, userID uint) {
	t.Helper()
	createTestUser(t, db, userID, "leader-"+itoa(int(userID)))
	team := Team{Name: "组长班组", OwnerUserID: 1, InviteCode: "lead" + itoa(int(userID))}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create leader team: %v", err)
	}
	if err := db.Create(&TeamMember{
		TeamID: team.ID, UserID: userID, Role: RoleLeader, Status: "active",
	}).Error; err != nil {
		t.Fatalf("create leader member: %v", err)
	}
}

// priceWorker 建工人工档，返回工人 ID。
func priceWorker(t *testing.T, r *gin.Engine, uid int, name string, daily, hourly, monthly int64) uint {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", uid, map[string]interface{}{
		"name": name, "daily_wage_cents": daily, "hourly_wage_cents": hourly, "monthly_wage_cents": monthly,
	})
	return uint(mustOK(t, w, out, "create worker "+name)["id"].(float64))
}

// priceProject 建工地，返回工地 ID。
func priceProject(t *testing.T, r *gin.Engine, uid int, name string, day, hour int64) uint {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/projects", uid, map[string]interface{}{
		"name": name, "day_price_cents": day, "hour_price_cents": hour,
	})
	return uint(mustOK(t, w, out, "create project "+name)["id"].(float64))
}

// savePrices 批量保存某工地的工人单独单价，返回响应。
func savePrices(t *testing.T, r *gin.Engine, uid int, projectID uint, rows []map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	return doReq(r, http.MethodPut, "/api/worklog/projects/"+itoa(int(projectID))+"/prices", uid, map[string]interface{}{"rows": rows})
}

// effectivePriceData 调生效价接口并返回 data。
func effectivePriceData(t *testing.T, r *gin.Engine, uid int, query string) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodGet, "/api/worklog/effective-price?"+query, uid, nil)
	return mustOK(t, w, out, "effective price "+query)
}

// postRecordData 记一笔工并返回记录数据。
func postRecordData(t *testing.T, r *gin.Engine, uid int, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/records", uid, body)
	return mustOK(t, w, out, "create record")
}

func asInt64(v interface{}) int64 {
	f, _ := v.(float64)
	return int64(f)
}

func TestProjectPriceFieldsCRUD(t *testing.T) {
	r, _ := setupTest(t)

	w, out := doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{
		"name": "滨江工地", "day_price_cents": 30000, "hour_price_cents": 3000,
	})
	data := mustOK(t, w, out, "create project with prices")
	if asInt64(data["day_price_cents"]) != 30000 || asInt64(data["hour_price_cents"]) != 3000 {
		t.Fatalf("created project prices = %v/%v, want 30000/3000", data["day_price_cents"], data["hour_price_cents"])
	}
	pid := uint(asInt64(data["id"]))

	// 更新工地默认单价（小时价清 0 = 不设置）
	w, out = doReq(r, http.MethodPut, "/api/worklog/projects/"+itoa(int(pid)), 1, map[string]interface{}{
		"name": "滨江工地", "day_price_cents": 35000, "hour_price_cents": 0,
	})
	data = mustOK(t, w, out, "update project prices")
	if asInt64(data["day_price_cents"]) != 35000 || asInt64(data["hour_price_cents"]) != 0 {
		t.Fatalf("updated project prices = %v/%v, want 35000/0", data["day_price_cents"], data["hour_price_cents"])
	}

	// 负数单价必须 400
	w, out = doReq(r, http.MethodPut, "/api/worklog/projects/"+itoa(int(pid)), 1, map[string]interface{}{
		"name": "滨江工地", "day_price_cents": -1,
	})
	if w.Code != http.StatusBadRequest || asInt64(out["code"]) == 0 {
		t.Fatalf("negative price should 400, got http=%d body=%s", w.Code, w.Body.String())
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{
		"name": "负价工地", "hour_price_cents": -5,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("negative price create should 400, got %d", w.Code)
	}

	// 列表回显两个单价
	w, out = doReq(r, http.MethodGet, "/api/worklog/projects", 1, nil)
	items := mustOKList(t, w, out, "list projects")
	if len(items) != 1 || asInt64(items[0].(map[string]interface{})["day_price_cents"]) != 35000 {
		t.Fatalf("list project prices mismatch: %v", items)
	}
}

func TestEffectivePriceFallbackChain(t *testing.T) {
	r, _ := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 28000, 2500, 0)
	projA := priceProject(t, r, 1, "A 工地", 30000, 3000)
	projB := priceProject(t, r, 1, "B 工地", 0, 0)

	// 无工地：工人自身单价
	data := effectivePriceData(t, r, 1, "worker_id="+itoa(int(zhang))+"&date=2026-10-23")
	if asInt64(data["daily_wage_cents"]) != 28000 || data["daily_source"] != priceSourceWorker {
		t.Fatalf("no project daily = %v/%v, want 28000/worker", data["daily_wage_cents"], data["daily_source"])
	}
	if asInt64(data["hourly_wage_cents"]) != 2500 || data["hourly_source"] != priceSourceWorker {
		t.Fatalf("no project hourly = %v/%v, want 2500/worker", data["hourly_wage_cents"], data["hourly_source"])
	}

	// A 工地设了默认价：工地默认优先于工人单价
	data = effectivePriceData(t, r, 1, "worker_id="+itoa(int(zhang))+"&project_id="+itoa(int(projA))+"&date=2026-10-23")
	if asInt64(data["daily_wage_cents"]) != 30000 || data["daily_source"] != priceSourceProject {
		t.Fatalf("project daily = %v/%v, want 30000/project", data["daily_wage_cents"], data["daily_source"])
	}
	if asInt64(data["hourly_wage_cents"]) != 3000 || data["hourly_source"] != priceSourceProject {
		t.Fatalf("project hourly = %v/%v, want 3000/project", data["hourly_wage_cents"], data["hourly_source"])
	}

	// B 工地没有默认价：回落到工人单价
	data = effectivePriceData(t, r, 1, "worker_id="+itoa(int(zhang))+"&project_id="+itoa(int(projB))+"&date=2026-10-23")
	if asInt64(data["daily_wage_cents"]) != 28000 || data["daily_source"] != priceSourceWorker {
		t.Fatalf("project B daily = %v/%v, want 28000/worker", data["daily_wage_cents"], data["daily_source"])
	}

	// 给张三在 A 工地单独定价：覆盖价最高（没填的小时价回落到工地默认）
	w, out := savePrices(t, r, 1, projA, []map[string]interface{}{
		{"worker_id": zhang, "daily_wage_cents": 32000, "hourly_wage_cents": 0},
	})
	mustOK(t, w, out, "save worker-project price")
	data = effectivePriceData(t, r, 1, "worker_id="+itoa(int(zhang))+"&project_id="+itoa(int(projA))+"&date=2026-10-23")
	if asInt64(data["daily_wage_cents"]) != 32000 || data["daily_source"] != priceSourceProjectWorker {
		t.Fatalf("override daily = %v/%v, want 32000/project_worker", data["daily_wage_cents"], data["daily_source"])
	}
	if asInt64(data["hourly_wage_cents"]) != 3000 || data["hourly_source"] != priceSourceProject {
		t.Fatalf("override hourly = %v/%v, want 3000/project", data["hourly_wage_cents"], data["hourly_source"])
	}

	// 管理页列表回显覆盖价与生效价
	w, out = doReq(r, http.MethodGet, "/api/worklog/projects/"+itoa(int(projA))+"/prices", 1, nil)
	list := mustOK(t, w, out, "get project prices")
	if asInt64(list["project_daily_price_cents"]) != 30000 {
		t.Fatalf("project default daily = %v, want 30000", list["project_daily_price_cents"])
	}
	rows := list["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0].(map[string]interface{})
	if asInt64(row["daily_wage_cents"]) != 32000 || asInt64(row["effective_daily_wage_cents"]) != 32000 ||
		row["daily_source"] != priceSourceProjectWorker {
		t.Fatalf("price row mismatch: %v", row)
	}

	// 清空覆盖（两个 0）→ 回落到工地默认价
	w, out = savePrices(t, r, 1, projA, []map[string]interface{}{
		{"worker_id": zhang, "daily_wage_cents": 0, "hourly_wage_cents": 0},
	})
	mustOK(t, w, out, "clear worker-project price")
	data = effectivePriceData(t, r, 1, "worker_id="+itoa(int(zhang))+"&project_id="+itoa(int(projA))+"&date=2026-10-23")
	if asInt64(data["daily_wage_cents"]) != 30000 || data["daily_source"] != priceSourceProject {
		t.Fatalf("after clear daily = %v/%v, want 30000/project", data["daily_wage_cents"], data["daily_source"])
	}
}

func TestMonthlyWorkerEffectiveDaily(t *testing.T) {
	r, _ := setupTest(t)
	// 2026-10 有 31 天：月薪 31000 元 → 日薪 1000 元。
	li := priceWorker(t, r, 1, "李四", 0, 0, 3100000)
	proj := priceProject(t, r, 1, "月薪工地", 0, 0)

	data := effectivePriceData(t, r, 1, "worker_id="+itoa(int(li))+"&date=2026-10-23")
	if asInt64(data["daily_wage_cents"]) != 100000 || data["daily_source"] != priceSourceMonthly {
		t.Fatalf("monthly daily = %v/%v, want 100000/monthly", data["daily_wage_cents"], data["daily_source"])
	}

	// 工地默认价优先于月薪折算
	proj2 := priceProject(t, r, 1, "有价工地", 35000, 0)
	data = effectivePriceData(t, r, 1, "worker_id="+itoa(int(li))+"&project_id="+itoa(int(proj2))+"&date=2026-10-23")
	if asInt64(data["daily_wage_cents"]) != 35000 || data["daily_source"] != priceSourceProject {
		t.Fatalf("monthly project daily = %v/%v, want 35000/project", data["daily_wage_cents"], data["daily_source"])
	}
	_ = proj
}

func TestRecordPriceUsesProjectSetting(t *testing.T) {
	r, _ := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 28000, 2500, 0)
	projA := priceProject(t, r, 1, "A 工地", 30000, 3000)
	projB := priceProject(t, r, 1, "B 工地", 35000, 0)
	w, out := doReq(r, http.MethodPost, "/api/worklog/piece-items", 1,
		map[string]interface{}{"name": "砌墙", "unit": "㎡", "unit_price_cents": 500})
	pieceID := uint(mustOK(t, w, out, "create piece item")["id"].(float64))

	// 一键记工（不传单价）→ 按工地默认价
	rec := postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-23", "type": "day", "quantity": "1", "project_id": projA,
	})
	if asInt64(rec["unit_price_cents"]) != 30000 || asInt64(rec["amount_cents"]) != 30000 {
		t.Fatalf("project A day price = %v/%v, want 30000/30000", rec["unit_price_cents"], rec["amount_cents"])
	}

	// 同一天换到 B 工地，金额随工地价变化（不同工地不同价）
	rec = postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-23", "type": "day", "quantity": "1", "project_id": projB,
	})
	if asInt64(rec["unit_price_cents"]) != 35000 {
		t.Fatalf("project B day price = %v, want 35000", rec["unit_price_cents"])
	}

	// 不分工地 → 工人日薪
	rec = postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-24", "type": "day", "quantity": "1",
	})
	if asInt64(rec["unit_price_cents"]) != 28000 {
		t.Fatalf("no project day price = %v, want 28000", rec["unit_price_cents"])
	}

	// 手动填单价最高优先
	rec = postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-25", "type": "day", "quantity": "1",
		"project_id": projA, "unit_price_cents": 99900,
	})
	if asInt64(rec["unit_price_cents"]) != 99900 {
		t.Fatalf("manual price = %v, want 99900", rec["unit_price_cents"])
	}

	// 工人在 A 工地的单独单价（日工覆盖，小时价仍走工地默认；一键记工也要用上）
	w, out = savePrices(t, r, 1, projA, []map[string]interface{}{
		{"worker_id": zhang, "daily_wage_cents": 32000, "hourly_wage_cents": 0},
	})
	mustOK(t, w, out, "save override")
	rec = postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-26", "type": "day", "quantity": "1", "project_id": projA,
	})
	if asInt64(rec["unit_price_cents"]) != 32000 {
		t.Fatalf("override day price = %v, want 32000", rec["unit_price_cents"])
	}

	// 点时：工地默认小时价生效
	rec = postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-27", "type": "hour", "quantity": "2", "project_id": projA,
	})
	if asInt64(rec["unit_price_cents"]) != 3000 || asInt64(rec["amount_cents"]) != 6000 {
		t.Fatalf("project hour price = %v/%v, want 3000/6000", rec["unit_price_cents"], rec["amount_cents"])
	}

	// 计件仍按计件项目单价，不受工地默认价影响
	rec = postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-28", "type": "piece", "quantity": "10",
		"piece_item_id": pieceID, "project_id": projA,
	})
	if asInt64(rec["unit_price_cents"]) != 500 || asInt64(rec["amount_cents"]) != 5000 {
		t.Fatalf("piece price = %v/%v, want 500/5000", rec["unit_price_cents"], rec["amount_cents"])
	}

	// 休息记录也带工地
	rec = postRecordData(t, r, 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-29", "type": "rest", "project_id": projB,
	})
	if asInt64(rec["amount_cents"]) != 0 || asInt64(rec["project_id"]) != int64(projB) {
		t.Fatalf("rest record project/amount = %v/%v", rec["project_id"], rec["amount_cents"])
	}

	// 不存在的工地 → 400
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-30", "type": "day", "quantity": "1", "project_id": 999999,
	})
	if w.Code != http.StatusBadRequest || asInt64(out["code"]) == 0 {
		t.Fatalf("unknown project should 400, got http=%d body=%s", w.Code, w.Body.String())
	}
}

func TestProjectPricePermissions(t *testing.T) {
	r, db := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 28000, 0, 0)
	proj := priceProject(t, r, 1, "A 工地", 30000, 0)
	url := "/api/worklog/projects/" + itoa(int(proj)) + "/prices"

	// 员工：不能看/改工地单价
	bindEmployee(t, db, zhang, 2)
	w, _ := doReq(r, http.MethodGet, url, 2, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee get prices = %d, want 403", w.Code)
	}
	w, _ = savePrices(t, r, 2, proj, []map[string]interface{}{{"worker_id": zhang, "daily_wage_cents": 1}})
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee put prices = %d, want 403", w.Code)
	}

	// 组长：可以看/改，也可以按工地价记工
	bindLeader(t, db, 3)
	w, out := doReq(r, http.MethodGet, url, 3, nil)
	mustOK(t, w, out, "leader get prices")
	w, out = savePrices(t, r, 3, proj, []map[string]interface{}{
		{"worker_id": zhang, "daily_wage_cents": 31000, "hourly_wage_cents": 0},
	})
	mustOK(t, w, out, "leader put prices")
	rec := postRecordData(t, r, 3, map[string]interface{}{
		"worker_id": zhang, "date": "2026-10-23", "type": "day", "quantity": "1", "project_id": proj,
	})
	if asInt64(rec["unit_price_cents"]) != 31000 {
		t.Fatalf("leader record price = %v, want 31000", rec["unit_price_cents"])
	}

	// 其它老板的工人不能塞进我的工地单价
	otherWorker := priceWorker(t, r, 4, "别人的工人", 20000, 0, 0)
	w, out = savePrices(t, r, 1, proj, []map[string]interface{}{
		{"worker_id": otherWorker, "daily_wage_cents": 30000},
	})
	if w.Code != http.StatusBadRequest || asInt64(out["code"]) == 0 {
		t.Fatalf("other worker should 400, got http=%d body=%s", w.Code, w.Body.String())
	}

	// 其它老板的工地不能改
	otherProj := priceProject(t, r, 4, "别人的工地", 0, 0)
	w, _ = savePrices(t, r, 1, otherProj, []map[string]interface{}{{"worker_id": zhang, "daily_wage_cents": 1}})
	if w.Code != http.StatusNotFound {
		t.Fatalf("other project should 404, got %d", w.Code)
	}
	w, _ = doReq(r, http.MethodGet, "/api/worklog/projects/"+itoa(int(otherProj))+"/prices", 1, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("other project get should 404, got %d", w.Code)
	}

	// 参数校验：负数、重复工人、缺少工人
	w, _ = savePrices(t, r, 1, proj, []map[string]interface{}{{"worker_id": zhang, "daily_wage_cents": -1}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("negative price should 400, got %d", w.Code)
	}
	w, _ = savePrices(t, r, 1, proj, []map[string]interface{}{
		{"worker_id": zhang, "daily_wage_cents": 1},
		{"worker_id": zhang, "daily_wage_cents": 2},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("duplicate worker should 400, got %d", w.Code)
	}
	w, _ = savePrices(t, r, 1, proj, []map[string]interface{}{{"daily_wage_cents": 1}})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing worker id should 400, got %d", w.Code)
	}
}

func TestProjectPriceCleanup(t *testing.T) {
	r, db := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 28000, 0, 0)
	proj := priceProject(t, r, 1, "A 工地", 30000, 0)
	if w, out := savePrices(t, r, 1, proj, []map[string]interface{}{
		{"worker_id": zhang, "daily_wage_cents": 32000},
	}); w.Code != http.StatusOK {
		t.Fatalf("save price failed: %s", w.Body.String())
	} else {
		_ = out
	}

	// 删除工地：单价行一并清理
	w, out := doReq(r, http.MethodDelete, "/api/worklog/projects/"+itoa(int(proj)), 1, nil)
	mustOK(t, w, out, "delete project")
	var n int64
	db.Model(&ProjectWorkerPrice{}).Where("project_id = ?", proj).Count(&n)
	if n != 0 {
		t.Fatalf("project prices after project delete = %d, want 0", n)
	}

	// 删除工人：单价行一并清理
	proj2 := priceProject(t, r, 1, "B 工地", 0, 0)
	if w, _ := savePrices(t, r, 1, proj2, []map[string]interface{}{
		{"worker_id": zhang, "daily_wage_cents": 33000},
	}); w.Code != http.StatusOK {
		t.Fatalf("save price for worker delete failed: %s", w.Body.String())
	}
	w, out = doReq(r, http.MethodDelete, "/api/worklog/workers/"+itoa(int(zhang)), 1, nil)
	mustOK(t, w, out, "delete worker")
	db.Model(&ProjectWorkerPrice{}).Where("worker_id = ?", zhang).Count(&n)
	if n != 0 {
		t.Fatalf("project prices after worker delete = %d, want 0", n)
	}
}

func TestJSONExportIncludesProjectPrices(t *testing.T) {
	r, _ := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 28000, 0, 0)
	proj := priceProject(t, r, 1, "A 工地", 30000, 0)
	mustOKT(t, r, 1, proj, zhang)

	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/json", 1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("export json http = %d", w.Code)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("export json body 不是 JSON: %v, body=%s", err, w.Body.String())
	}
	prices, ok := payload["project_prices"].([]interface{})
	if !ok || len(prices) != 1 {
		t.Fatalf("project_prices = %v, want 1 条", payload["project_prices"])
	}
	row := prices[0].(map[string]interface{})
	if asInt64(row["worker_id"]) != int64(zhang) || asInt64(row["daily_wage_cents"]) != 32000 {
		t.Fatalf("exported project price row = %v", row)
	}
}

// mustOKT 给 JSON 导出用例造一条工人-工地单价。
func mustOKT(t *testing.T, r *gin.Engine, uid int, proj, worker uint) {
	t.Helper()
	w, out := savePrices(t, r, uid, proj, []map[string]interface{}{
		{"worker_id": worker, "daily_wage_cents": 32000, "hourly_wage_cents": 0},
	})
	mustOK(t, w, out, "save price for export")
}
