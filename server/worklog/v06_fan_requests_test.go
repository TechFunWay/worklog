package worklog

// v06_fan_requests_test.go — v0.6.0 三项粉丝需求的回归测试。
//
// 来源：2026-11-17 抖音粉丝「一个字」私信（见 docs/06-结算分期借支工地补贴需求评估.md）。
// 覆盖：
//   A. 记工补贴/扣款：金额口径、四层默认值、半天不带、越权自填、上限与扣穿
//   B. 借支选工地：筛选、按项目抵扣口径、重复抵扣、删除守卫
//   C. 结算调整项与分期实付：欠薪不回未结算池、流水重算、状态推导、撤销连带清理
//   D. v0.6.0 数据回填脚本的幂等与历史工资条口径

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"smallgo/server/database"
)

// ---- 公共构造 ----

// v06Worker 建工人档案；subsidy 为该工人的默认补贴（分/出勤日）。
func v06Worker(t *testing.T, r *gin.Engine, name string, daily, subsidy int64) uint {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{
		"name": name, "daily_wage_cents": daily, "subsidy_cents": subsidy,
	})
	return uint(mustOK(t, w, out, "create worker "+name)["id"].(float64))
}

// v06Project 建工地，可带默认日价与默认补贴。
func v06Project(t *testing.T, r *gin.Engine, name string, dayPrice, subsidy int64) uint {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{
		"name": name, "day_price_cents": dayPrice, "subsidy_cents": subsidy,
	})
	return uint(mustOK(t, w, out, "create project "+name)["id"].(float64))
}

// v06Record 记一笔工并返回记录对象；期望成功。
func v06Record(t *testing.T, r *gin.Engine, uid int, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/records", uid, body)
	if w.Code != http.StatusOK || out["code"].(float64) != 0 {
		t.Fatalf("记工失败: http=%d body=%s", w.Code, w.Body.String())
	}
	d, ok := out["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("记工响应结构错误: %s", w.Body.String())
	}
	return d
}

// v06Advance 记一笔借支（projectID 传 0 表示不指定工地）。
func v06Advance(t *testing.T, r *gin.Engine, workerID, projectID uint, date string, cents int64) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{"worker_id": workerID, "date": date, "amount_cents": cents, "method": "cash"}
	if projectID > 0 {
		body["project_id"] = projectID
	}
	w, out := doReq(r, http.MethodPost, "/api/worklog/advances", 1, body)
	if w.Code != http.StatusOK || out["code"].(float64) != 0 {
		t.Fatalf("记借支失败: http=%d body=%s", w.Code, w.Body.String())
	}
	return mustOK(t, w, out, "create advance")
}

// v06Num 读响应里的数字字段，缺失/非数字返回 0。
func v06Num(row map[string]interface{}, field string) int64 {
	v, ok := row[field]
	if !ok {
		return 0
	}
	f64, ok := v.(float64)
	if !ok {
		return 0
	}
	return int64(f64)
}

// v06Effective 查生效价与默认补贴。
func v06Effective(t *testing.T, r *gin.Engine, worker, project uint, date string) map[string]interface{} {
	t.Helper()
	q := fmt.Sprintf("/api/worklog/effective-price?worker_id=%d&date=%s", worker, date)
	if project > 0 {
		q = fmt.Sprintf("/api/worklog/effective-price?worker_id=%d&project_id=%d&date=%s", worker, project, date)
	}
	w, out := doReq(r, http.MethodGet, q, 1, nil)
	return mustOK(t, w, out, "effective price")
}

// ---- A. 记工补贴与扣款 ----

// TestRecordAmountCentsEntry 金额唯一入口的算术与校验（单元级）。
func TestRecordAmountCentsEntry(t *testing.T) {
	if w, total, err := recordAmountCents(mustRat("1"), 30000, 1500, 0); err != nil || w != 30000 || total != 31500 {
		t.Fatalf("工钱+补贴: wage=%d total=%d err=%v", w, total, err)
	}
	if w, total, err := recordAmountCents(mustRat("0.5"), 30000, 0, 2000); err != nil || w != 15000 || total != 13000 {
		t.Fatalf("半天减扣款: wage=%d total=%d err=%v", w, total, err)
	}
	// 扣穿必须报错，不能写负数（记工金额恒 ≥ 0）
	if _, _, err := recordAmountCents(mustRat("0.5"), 30000, 0, 20000); err == nil {
		t.Fatal("扣款大于工钱应报错")
	}
	if _, _, err := recordAmountCents(mustRat("1"), 30000, maxSubsidyCents+1, 0); err == nil {
		t.Fatal("补贴超上限应报错")
	}
	if _, _, err := recordAmountCents(mustRat("1"), 30000, 0, maxDeductCents+1); err == nil {
		t.Fatal("扣款超上限应报错")
	}
	if _, _, err := recordAmountCents(mustRat("1"), 30000, -1, 0); err == nil {
		t.Fatal("负补贴应报错")
	}
}

// TestRecordSubsidyAndDeduct 记工接口的补贴/扣款与金额口径。
func TestRecordSubsidyAndDeduct(t *testing.T) {
	r, _ := setupTest(t)
	worker := v06Worker(t, r, "张三", 30000, 0)

	rec := v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-01", "type": "day", "quantity": "1",
		"subsidy_cents": 1500, "subsidy_note": "饭补",
	})
	if v06Num(rec, "wage_cents") != 30000 || v06Num(rec, "subsidy_cents") != 1500 || v06Num(rec, "amount_cents") != 31500 {
		t.Fatalf("补贴未并进金额: %v", rec)
	}
	if rec["subsidy_note"] != "饭补" {
		t.Fatalf("补贴说明未落库: %v", rec["subsidy_note"])
	}

	rec = v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-02", "type": "day", "quantity": "1",
		"subsidy_cents": 2000, "deduct_cents": 500,
	})
	if v06Num(rec, "amount_cents") != 31500 {
		t.Fatalf("补贴-扣款合计错误: %v", rec["amount_cents"])
	}

	w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-03", "type": "day", "quantity": "0.5", "deduct_cents": 99900,
	})
	if w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("扣穿应被拒绝: %s", w.Body.String())
	}

	rec = v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-04", "type": "rest",
		"subsidy_cents": 1500, "subsidy_note": "不该生效",
	})
	if v06Num(rec, "amount_cents") != 0 || v06Num(rec, "subsidy_cents") != 0 || v06Num(rec, "wage_cents") != 0 {
		t.Fatalf("休息日不应带任何金额: %v", rec)
	}

	rec = v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-05", "type": "day", "quantity": "1", "subsidy_cents": 0,
	})
	if v06Num(rec, "subsidy_cents") != 0 || v06Num(rec, "wage_cents") != v06Num(rec, "amount_cents") {
		t.Fatalf("显式 0 补贴未生效: %v", rec)
	}
}

// TestSubsidyDefaultChain 补贴默认值的四层链与「半天/计件不自动带」。
func TestSubsidyDefaultChain(t *testing.T) {
	r, db := setupTest(t)
	worker := v06Worker(t, r, "李四", 30000, 800)      // 档案级默认补贴 8 元
	project := v06Project(t, r, "A 工地", 35000, 1500) // 工地默认补贴 15 元

	data := v06Effective(t, r, worker, project, "2026-11-01")
	if v06Num(data, "subsidy_cents") != 1500 || data["subsidy_source"] != "project" {
		t.Fatalf("默认补贴应为工地 1500/project，实际 %v/%v", data["subsidy_cents"], data["subsidy_source"])
	}

	data = v06Effective(t, r, worker, 0, "2026-11-01")
	if v06Num(data, "subsidy_cents") != 800 || data["subsidy_source"] != "worker" {
		t.Fatalf("默认补贴应回落到工人 800/worker，实际 %v/%v", data["subsidy_cents"], data["subsidy_source"])
	}

	if _, out := savePrices(t, r, 1, project, []map[string]interface{}{{"worker_id": worker, "subsidy_cents": 2000}}); out["code"].(float64) != 0 {
		t.Fatalf("保存单独补贴失败: %v", out)
	}
	data = v06Effective(t, r, worker, project, "2026-11-01")
	if v06Num(data, "subsidy_cents") != 2000 || data["subsidy_source"] != "project_worker" {
		t.Fatalf("单独补贴覆盖未生效: %v/%v", data["subsidy_cents"], data["subsidy_source"])
	}

	// 不传 subsidy_cents：整工自动带出 2000
	rec := v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-06", "type": "day", "quantity": "1", "project_id": project,
	})
	if v06Num(rec, "wage_cents") != 35000 || v06Num(rec, "subsidy_cents") != 2000 || v06Num(rec, "amount_cents") != 37000 {
		t.Fatalf("整工未自动带默认补贴: %v", rec)
	}

	// 半天：不自动带（不替老板猜「半天管不管饭」）
	rec = v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-07", "type": "day", "quantity": "0.5", "project_id": project,
	})
	if v06Num(rec, "subsidy_cents") != 0 || v06Num(rec, "amount_cents") != 17500 {
		t.Fatalf("半天不应自动带补贴: %v", rec)
	}

	// 半天 + 手填：尊重手填
	rec = v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-08", "type": "day", "quantity": "0.5", "subsidy_cents": 2000, "project_id": project,
	})
	if v06Num(rec, "amount_cents") != 19500 {
		t.Fatalf("半天手填补贴未生效: %v", rec)
	}

	// 计件不自动带按出勤日的补贴
	w, out := doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{"name": "砌墙", "unit": "㎡", "unit_price_cents": 500})
	pieceID := uint(mustOK(t, w, out, "create piece")["id"].(float64))
	rec = v06Record(t, r, 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-09", "type": "piece", "quantity": "10",
		"piece_item_id": pieceID, "project_id": project,
	})
	if v06Num(rec, "subsidy_cents") != 0 || v06Num(rec, "amount_cents") != 5000 {
		t.Fatalf("计件不该自动带按出勤日的补贴: %v", rec)
	}

	// 三项全 0 → 删除覆盖行，回落上层
	if _, out = savePrices(t, r, 1, project, []map[string]interface{}{{"worker_id": worker}}); out["code"].(float64) != 0 {
		t.Fatalf("清零单独补贴失败: %v", out)
	}
	var n int64
	db.Model(&ProjectWorkerPrice{}).Where("project_id = ? AND worker_id = ?", project, worker).Count(&n)
	if n != 0 {
		t.Fatalf("三项全 0 应删除覆盖行，实际剩 %d 行", n)
	}
}

// TestWorkerCannotSelfSetSubsidy 员工不能自填补贴/扣款（与「员工不能自带单价」同一口径）。
func TestWorkerCannotSelfSetSubsidy(t *testing.T) {
	r, db := setupTest(t)
	worker := v06Worker(t, r, "王五", 20000, 0)
	bindEmployee(t, db, worker, 7)

	w, out := doReq(r, http.MethodPost, "/api/worklog/records", 7, map[string]interface{}{
		"worker_id": 999, // 员工只能给自己记，服务端会改写
		"date":      "2026-11-01", "type": "day", "quantity": "1",
		"unit_price_cents": 99999900, "subsidy_cents": 500000, "subsidy_note": "自己给自己发",
	})
	if w.Code != http.StatusOK || out["code"].(float64) != 0 {
		t.Fatalf("员工记工应成功: %s", w.Body.String())
	}
	rec := mustOK(t, w, out, "worker self record")
	if v06Num(rec, "subsidy_cents") != 0 {
		t.Fatalf("员工自填补贴未被服务端清零: %v", rec)
	}
	if v06Num(rec, "amount_cents") != 20000 {
		t.Fatalf("员工靠自填单价/补贴抬价成功了，金额=%v", rec["amount_cents"])
	}
	if rec["subsidy_note"] != "" {
		t.Fatalf("员工的补贴说明应被清空: %v", rec["subsidy_note"])
	}
}

// ---- B. 借支选工地 ----

// v06URL 用 fmt.Sprintf 拼带 id 的请求路径（避免测试里到处字符串相加）。
func v06URL(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// TestAdvanceProjectDimension 借支可选工地、可按工地筛选、回显工地名。
func TestAdvanceProjectDimension(t *testing.T) {
	r, _ := setupTest(t)
	worker := v06Worker(t, r, "赵一", 30000, 0)
	projA := v06Project(t, r, "A 工地", 30000, 0)
	projB := v06Project(t, r, "B 工地", 30000, 0)

	v06Advance(t, r, worker, projA, "2026-11-01", 10000)
	v06Advance(t, r, worker, projB, "2026-11-02", 5000)
	v06Advance(t, r, worker, 0, "2026-11-03", 8000) // 未指定工地

	// 按 A 工地筛选
	w, out := doReq(r, http.MethodGet, v06URL("/api/worklog/advances?project_id=%d", projA), 1, nil)
	items := mustOK(t, w, out, "list advances of A")["items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("A 工地应只有 1 笔借支，实际 %d", len(items))
	}
	row := items[0].(map[string]interface{})
	if v06Num(row, "amount_cents") != 10000 || row["project_name"] != "A 工地" {
		t.Fatalf("A 工地借支回显错误: %v", row)
	}

	// none = 未分工地
	w, out = doReq(r, http.MethodGet, "/api/worklog/advances?project_id=none", 1, nil)
	items = mustOK(t, w, out, "list loose advances")["items"].([]interface{})
	if len(items) != 1 || v06Num(items[0].(map[string]interface{}), "amount_cents") != 8000 {
		t.Fatalf("未分工地借支筛选错误: %v", items)
	}

	// 全部 3 笔
	w, out = doReq(r, http.MethodGet, "/api/worklog/advances", 1, nil)
	if total := v06Num(mustOK(t, w, out, "list all advances"), "total"); total != 3 {
		t.Fatalf("借支总数应为 3，实际 %d", total)
	}

	// 工地必须属于当前数据集
	if w, out = doReq(r, http.MethodPost, "/api/worklog/advances", 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-11-04", "amount_cents": 100, "project_id": 99999,
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("挂到不存在的工地应被拒绝: %v", out)
	}
}

// TestAdvanceProjectSettlementScope 借支抵扣口径：按项目只抵本项目、未分工地的不被任何
// 项目结算扣走、也不会被两张项目单重复扣（粉丝反馈「借支向谁借的分不清」的正解）。
func TestAdvanceProjectSettlementScope(t *testing.T) {
	r, db := setupTest(t)
	worker := v06Worker(t, r, "钱二", 30000, 0)
	projA := v06Project(t, r, "A 工地", 30000, 0)
	projB := v06Project(t, r, "B 工地", 30000, 0)

	for _, d := range []string{"2026-11-01", "2026-11-02"} {
		v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": d, "type": "day", "quantity": "1", "project_id": projA})
	}
	v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": "2026-11-03", "type": "day", "quantity": "1", "project_id": projB})
	v06Advance(t, r, worker, projA, "2026-11-01", 10000)
	v06Advance(t, r, worker, projB, "2026-11-02", 5000)
	v06Advance(t, r, worker, 0, "2026-11-03", 8000)

	// 预览 A 工地：只应看到 A 的记工与 A 的借支，并提示另有未分工地的借支
	w, out := doReq(r, http.MethodGet, v06URL("/api/worklog/settlements/preview?worker_id=%d&project_id=%d", worker, projA), 1, nil)
	data := mustOK(t, w, out, "preview A")
	sum := data["summary"].(map[string]interface{})
	if v06Num(sum, "work_amount_cents") != 60000 || v06Num(sum, "advance_amount_cents") != 10000 || v06Num(sum, "payable_cents") != 50000 {
		t.Fatalf("A 工地预览金额错误: %v", sum)
	}
	if v06Num(sum, "loose_advance_count") != 1 || v06Num(sum, "loose_advance_cents") != 8000 {
		t.Fatalf("未提示未分工地的借支: %v", sum)
	}

	// 结算 A 工地：A 的 2 笔记工 + A 的 1 笔借支挂单；B 与未分工地的借支保持未结
	if w, out = doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": worker, "project_id": projA, "note": "A 工地结清",
	}); out["code"].(float64) != 0 {
		t.Fatalf("结算 A 失败: %v", out)
	}
	sA := mustOK(t, w, out, "settle A")
	if v06Num(sA, "payable_cents") != 50000 {
		t.Fatalf("A 结算单应付错误: %v", sA)
	}

	var advs []Advance
	db.Where("user_id = ?", 1).Order("date ASC").Find(&advs)
	if advs[0].SettlementID == nil {
		t.Fatal("A 工地借支应已挂单")
	}
	if advs[1].SettlementID != nil || advs[2].SettlementID != nil {
		t.Fatal("B 工地借支与未指定工地的借支都不该被 A 项目结算扣走")
	}

	// 再结算 B 工地：只抵 B 的借支，A 的不会被重复抵扣
	if w, out = doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": worker, "project_id": projB,
	}); out["code"].(float64) != 0 {
		t.Fatalf("结算 B 失败: %v", out)
	}
	sB := mustOK(t, w, out, "settle B")
	if v06Num(sB, "work_amount_cents") != 30000 || v06Num(sB, "advance_amount_cents") != 5000 || v06Num(sB, "payable_cents") != 25000 {
		t.Fatalf("B 结算单金额错误（疑似重复抵扣）: %v", sB)
	}

	// 最后按人结算：只剩未指定工地的那笔借支（记工已全部结清）
	if w, out = doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{"worker_id": worker}); out["code"].(float64) != 0 {
		t.Fatalf("按人结算失败: %v", out)
	}
	sAll := mustOK(t, w, out, "settle loose")
	if v06Num(sAll, "advance_amount_cents") != 8000 || v06Num(sAll, "record_count") != 0 {
		t.Fatalf("按人结算应只兜底抵扣未分工地的借支: %v", sAll)
	}

	// 三笔借支各自只挂了一张结算单，且互不相同
	db.Where("user_id = ?", 1).Order("date ASC").Find(&advs)
	if advs[0].SettlementID == nil || advs[1].SettlementID == nil || advs[2].SettlementID == nil {
		t.Fatal("存在未挂单的借支")
	}
	if *advs[0].SettlementID == *advs[1].SettlementID || *advs[1].SettlementID == *advs[2].SettlementID {
		t.Fatal("同一笔记工/借支被多张结算单重复挂走")
	}
}

// TestDeleteProjectGuardedByAdvance 工地被借支引用后不能删除（否则项目归属悬空）。
func TestDeleteProjectGuardedByAdvance(t *testing.T) {
	r, _ := setupTest(t)
	worker := v06Worker(t, r, "孙三", 30000, 0)
	proj := v06Project(t, r, "只此一家", 30000, 0)
	v06Advance(t, r, worker, proj, "2026-11-01", 2000)

	if w, out := doReq(r, http.MethodDelete, v06URL("/api/worklog/projects/%d", proj), 1, nil); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("有借支的工地应禁止删除: %v", out)
	}
	// 归档不受影响
	if _, out := doReq(r, http.MethodPut, v06URL("/api/worklog/projects/%d", proj), 1, map[string]interface{}{
		"name": "只此一家", "status": "archived",
	}); out["code"].(float64) != 0 {
		t.Fatalf("归档工地失败: %v", out)
	}
}

// ---- E. 日期真值校验（端到端跑出来的真 bug）----

// TestValidDateRejectsImpossibleDays 日期不能只看形状：2026-13-99 / 2026-02-30 这类
// 不存在的日子一旦落库，就永远落在月度区间之外 —— 工记上了、统计里没有，属于静默丢账。
func TestValidDateRejectsImpossibleDays(t *testing.T) {
	good := []string{"2026-11-17", "2024-02-29", "2026-12-31", "2026-01-01"}
	bad := []string{"2026-13-99", "2026-02-30", "2026-02-29", "2026-1-05", "2026-11-5", "20260115", "", "2026-11-17 ", "0000-00-00"}
	for _, s := range good {
		if !validDate(s) {
			t.Errorf("合法日期被拒: %q", s)
		}
	}
	for _, s := range bad {
		if validDate(s) {
			t.Errorf("不存在的日期被放过: %q", s)
		}
	}
}

// TestImpossibleDateRejectedOnWrite 记工 / 借支 / 付款三条写入口都要挡住假日期。
func TestImpossibleDateRejectedOnWrite(t *testing.T) {
	r, _ := setupTest(t)
	worker := v06Worker(t, r, "日期校验", 30000, 0)

	if w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-13-99", "type": "day", "quantity": "1",
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("假日期的记工被写入: %v", out)
	}
	if w, out := doReq(r, http.MethodPost, "/api/worklog/advances", 1, map[string]interface{}{
		"worker_id": worker, "date": "2026-02-30", "amount_cents": 100,
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("假日期的借支被写入: %v", out)
	}

	v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": "2026-11-01", "type": "day", "quantity": "1"})
	s := v06Settle(t, r, map[string]interface{}{"worker_id": worker, "paid_cents": 0})
	if w, out := doReq(r, http.MethodPost, v06URL("/api/worklog/settlements/%d/payments", uint(v06Num(s, "id"))), 1, map[string]interface{}{
		"amount_cents": 100, "paid_on": "2026-02-30",
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("假日期的付款被写入: %v", out)
	}
	// 正常日期仍然可以
	if _, out := doReq(r, http.MethodPost, v06URL("/api/worklog/settlements/%d/payments", uint(v06Num(s, "id"))), 1, map[string]interface{}{
		"amount_cents": 100, "paid_on": "2026-11-10",
	}); out["code"].(float64) != 0 {
		t.Fatalf("合法付款被拒: %v", out)
	}
}

// ---- C. 结算调整项与分期实付 ----

// v06Settle 创建结算单，返回结算单对象。
func v06Settle(t *testing.T, r *gin.Engine, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/settlements", 1, body)
	if out["code"] == nil || out["code"].(float64) != 0 {
		t.Fatalf("结算失败: http=%d body=%s", w.Code, w.Body.String())
	}
	return mustOK(t, w, out, "settle")
}

// v06Pay 登记一笔付款。
func v06Pay(t *testing.T, r *gin.Engine, settlementID uint, body map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	return doReq(r, http.MethodPost, v06URL("/api/worklog/settlements/%d/payments", settlementID), 1, body)
}

// v06GetSettlement 取结算单详情。
func v06GetSettlement(t *testing.T, r *gin.Engine, id uint) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodGet, v06URL("/api/worklog/settlements/%d", id), 1, nil)
	return mustOK(t, w, out, "get settlement")
}

// TestSettlementPartialPayment 分期实付：欠薪留在结算单上，绝不退回未结算池。
func TestSettlementPartialPayment(t *testing.T) {
	r, db := setupTest(t)
	worker := v06Worker(t, r, "周七", 30000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": "2026-11-01", "type": "day", "quantity": "1"})

	s := v06Settle(t, r, map[string]interface{}{"worker_id": worker, "paid_cents": 12000, "note": "先给一半"})
	sid := uint(v06Num(s, "id"))
	if v06Num(s, "payable_cents") != 30000 || v06Num(s, "paid_cents") != 12000 || v06Num(s, "due_cents") != 18000 {
		t.Fatalf("分期金额错误: %v", s)
	}
	if s["pay_status"] != PayStatusPartial {
		t.Fatalf("付款状态应为 partial: %v", s["pay_status"])
	}

	// 关键不变式：欠薪不能让记工回到未结算池，否则同一笔工会被第二次结算（双付真钱）。
	w, out := doReq(r, http.MethodGet, "/api/worklog/settlements/unsettled", 1, nil)
	if rows := mustOKList(t, w, out, "unsettled"); len(rows) != 0 {
		t.Fatalf("已结算的欠薪不应回到未结算列表: %v", rows)
	}
	var locked int64
	db.Model(&WorkRecord{}).Where("worker_id = ? AND settlement_id IS NOT NULL", worker).Count(&locked)
	if locked != 1 {
		t.Fatalf("记工应仍处于已结算锁定状态，实际 %d 条", locked)
	}

	// 待付看板能看到这 18000
	w, out = doReq(r, http.MethodGet, "/api/worklog/settlements/due", 1, nil)
	due := mustOK(t, w, out, "due")
	if v06Num(due, "total_due_cents") != 18000 || v06Num(due, "count") != 1 {
		t.Fatalf("待付汇总错误: %v", due)
	}

	// 补付一笔：状态仍是部分支付，流水两笔
	if w, out = v06Pay(t, r, sid, map[string]interface{}{"amount_cents": 8000, "method": "wechat", "paid_on": "2026-11-10", "note": "微信转"}); out["code"].(float64) != 0 {
		t.Fatalf("登记付款失败: %v", out)
	}
	got := v06GetSettlement(t, r, sid)
	if v06Num(got, "paid_cents") != 20000 || v06Num(got, "due_cents") != 10000 || got["pay_status"] != PayStatusPartial {
		t.Fatalf("补付后状态错误: %v", got)
	}
	if v06Num(got, "payment_count") != 2 {
		t.Fatalf("付款流水应为 2 笔: %v", got["payment_count"])
	}

	// 超出剩余应付必须拒绝
	if w, out = v06Pay(t, r, sid, map[string]interface{}{"amount_cents": 10001, "paid_on": "2026-11-11"}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("超出剩余应付应被拒绝: %v", out)
	}
	// 金额必须为正
	if w, out = v06Pay(t, r, sid, map[string]interface{}{"amount_cents": 0}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("0 元付款应被拒绝: %v", out)
	}
	// 非法付款方式与日期
	if w, out = v06Pay(t, r, sid, map[string]interface{}{"amount_cents": 100, "method": "bitcoin"}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("未知付款方式应被拒绝: %v", out)
	}
	if w, out = v06Pay(t, r, sid, map[string]interface{}{"amount_cents": 100, "paid_on": "2026/11/11"}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("非法日期应被拒绝: %v", out)
	}

	// 最后一笔付完 → 已付清
	if _, out = v06Pay(t, r, sid, map[string]interface{}{"amount_cents": 10000, "paid_on": "2026-11-12"}); out["code"].(float64) != 0 {
		t.Fatalf("结清付款失败: %v", out)
	}
	got = v06GetSettlement(t, r, sid)
	if got["pay_status"] != PayStatusPaid || v06Num(got, "due_cents") != 0 {
		t.Fatalf("结清后状态错误: %v", got)
	}

	// 删掉某一笔付款 → 已付按剩余流水 SUM 重算（不是减法累加）。
	// 流水按 paid_on 升序返回，所以按金额定位，不依赖顺序。
	payments := got["payments"].([]interface{})
	if len(payments) != 3 {
		t.Fatalf("应有 3 笔流水（结算首笔 + 两笔补付），实际 %d", len(payments))
	}
	var lastID uint
	for _, p := range payments {
		m := p.(map[string]interface{})
		if v06Num(m, "amount_cents") == 10000 {
			lastID = uint(v06Num(m, "id"))
		}
	}
	if lastID == 0 {
		t.Fatal("没找到 10000 元那笔流水")
	}
	if w, out = doReq(r, http.MethodDelete, v06URL("/api/worklog/settlements/%d/payments/%d", sid, lastID), 1, nil); out["code"].(float64) != 0 {
		t.Fatalf("删除付款失败: %v", out)
	}
	got = v06GetSettlement(t, r, sid)
	if v06Num(got, "paid_cents") != 20000 || got["pay_status"] != PayStatusPartial || v06Num(got, "due_cents") != 10000 {
		t.Fatalf("删除付款后重算错误: %v", got)
	}
}

// TestSettlementDefaultFullPaid 不传 paid_cents 时与 v0.5.x 行为一致（向后兼容）。
func TestSettlementDefaultFullPaid(t *testing.T) {
	r, _ := setupTest(t)
	worker := v06Worker(t, r, "吴八", 20000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": "2026-11-01", "type": "day", "quantity": "1"})
	s := v06Settle(t, r, map[string]interface{}{"worker_id": worker})
	if v06Num(s, "paid_cents") != 20000 || s["pay_status"] != PayStatusPaid || v06Num(s, "due_cents") != 0 {
		t.Fatalf("默认应为全额已付: %v", s)
	}

	// 显式 0 = 全额挂欠（月底先对账、过几天再给钱）
	w2 := v06Worker(t, r, "郑九", 20000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": w2, "date": "2026-11-01", "type": "day", "quantity": "1"})
	s2 := v06Settle(t, r, map[string]interface{}{"worker_id": w2, "paid_cents": 0})
	if s2["pay_status"] != PayStatusUnpaid || v06Num(s2, "due_cents") != 20000 {
		t.Fatalf("全额挂欠状态错误: %v", s2)
	}

	// 实付大于应付必须拒绝
	w3 := v06Worker(t, r, "王十", 10000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": w3, "date": "2026-11-01", "type": "day", "quantity": "1"})
	if w, out := doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": w3, "paid_cents": 999999, "note": "多给",
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("实付大于应付应被拒绝: %v", out)
	}
}

// TestSettlementAdjust 调整项（抹零/奖惩）：工资条仍可解释，且已付不得超过新应付。
func TestSettlementAdjust(t *testing.T) {
	r, _ := setupTest(t)
	worker := v06Worker(t, r, "冯一", 30000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": "2026-11-01", "type": "day", "quantity": "1"})
	v06Advance(t, r, worker, 0, "2026-11-01", 5000)

	// 抹零 3 元 + 本次只给 1000：应付 30000 - 5000 - 300 = 24700，尚欠 23700
	s := v06Settle(t, r, map[string]interface{}{
		"worker_id": worker, "adjust_cents": -300, "adjust_note": "抹零", "paid_cents": 1000,
	})
	sid := uint(v06Num(s, "id"))
	if v06Num(s, "payable_cents") != 24700 || v06Num(s, "adjust_cents") != -300 {
		t.Fatalf("调整项未计入应付: %v", s)
	}
	if s["adjust_note"] != "抹零" {
		t.Fatalf("调整原因未落库: %v", s)
	}
	// 工资条自洽：应发 - 借支 + 调整 = 实发
	if v06Num(s, "work_amount_cents")-v06Num(s, "advance_amount_cents")+v06Num(s, "adjust_cents") != v06Num(s, "payable_cents") {
		t.Fatalf("工资条金额不自洽: %v", s)
	}

	// 事后改调整项：应付重算
	if _, out := doReq(r, http.MethodPut, v06URL("/api/worklog/settlements/%d/adjust", sid), 1, map[string]interface{}{
		"adjust_cents": -1000, "adjust_note": "迟到扣款",
	}); out["code"].(float64) != 0 {
		t.Fatalf("修改调整项失败: %v", out)
	}
	got := v06GetSettlement(t, r, sid)
	if v06Num(got, "payable_cents") != 24000 || v06Num(got, "paid_cents") != 1000 || v06Num(got, "due_cents") != 23000 {
		t.Fatalf("改调整后应付/尚欠未重算: %v", got)
	}

	// 已先付一部分时，不允许把应付调到低于已付
	w2 := v06Worker(t, r, "陈二", 30000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": w2, "date": "2026-11-01", "type": "day", "quantity": "1"})
	s2 := v06Settle(t, r, map[string]interface{}{"worker_id": w2, "paid_cents": 20000})
	if w, out := doReq(r, http.MethodPut, v06URL("/api/worklog/settlements/%d/adjust", uint(v06Num(s2, "id"))), 1, map[string]interface{}{
		"adjust_cents": -20000, "adjust_note": "调到低于已付",
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("调整后应付低于已付应被拒绝: %v", out)
	}

	// 一键月结不接受调整项与非 0 实付（同一金额对多个工人没有意义）
	if w, out := doReq(r, http.MethodPost, "/api/worklog/settlements/batch", 1, map[string]interface{}{
		"adjust_cents": -100,
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("批量结算应拒绝调整项: %v", out)
	}
	if w, out := doReq(r, http.MethodPost, "/api/worklog/settlements/batch", 1, map[string]interface{}{
		"paid_cents": 100,
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("批量结算应拒绝分期金额: %v", out)
	}
}

// TestRevertSettlementClearsPayments 撤销结算必须连带清掉付款流水，否则留下孤儿数据。
func TestRevertSettlementClearsPayments(t *testing.T) {
	r, db := setupTest(t)
	worker := v06Worker(t, r, "卫三", 30000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": "2026-11-01", "type": "day", "quantity": "1"})
	s := v06Settle(t, r, map[string]interface{}{"worker_id": worker, "paid_cents": 10000})
	sid := uint(v06Num(s, "id"))
	if _, out := v06Pay(t, r, sid, map[string]interface{}{"amount_cents": 5000, "paid_on": "2026-11-05"}); out["code"].(float64) != 0 {
		t.Fatalf("登记付款失败: %v", out)
	}

	if _, out := doReq(r, http.MethodDelete, v06URL("/api/worklog/settlements/%d", sid), 1, nil); out["code"].(float64) != 0 {
		t.Fatalf("撤销结算失败: %v", out)
	}
	var nPay, nSettle int64
	db.Model(&SettlementPayment{}).Where("settlement_id = ?", sid).Count(&nPay)
	db.Model(&Settlement{}).Count(&nSettle)
	if nPay != 0 || nSettle != 0 {
		t.Fatalf("撤销后仍残留流水 %d 笔 / 结算单 %d 张", nPay, nSettle)
	}
	// 记工解挂，可以重新结算
	var unlocked int64
	db.Model(&WorkRecord{}).Where("settlement_id IS NULL").Count(&unlocked)
	if unlocked != 1 {
		t.Fatalf("撤销后记工应解挂")
	}
}

// TestPaymentPermission 付款只允许老板登记（组长与员工一律 403）。
func TestPaymentPermission(t *testing.T) {
	r, db := setupTest(t)
	worker := v06Worker(t, r, "蒋四", 30000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": worker, "date": "2026-11-01", "type": "day", "quantity": "1"})
	s := v06Settle(t, r, map[string]interface{}{"worker_id": worker, "paid_cents": 0})
	sid := uint(v06Num(s, "id"))

	bindLeader(t, db, 21)
	if w, out := doReq(r, http.MethodPost, v06URL("/api/worklog/settlements/%d/payments", sid), 21, map[string]interface{}{
		"amount_cents": 100, "paid_on": "2026-11-05",
	}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("组长不应能登记付款: %v", out)
	}

	employee := v06Worker(t, r, "员工五", 30000, 0)
	bindEmployee(t, db, employee, 22)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": employee, "date": "2026-11-01", "type": "day", "quantity": "1"})
	if w, out := doReq(r, http.MethodGet, v06URL("/api/worklog/settlements/%d/payments", sid), 22, nil); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatalf("员工不该看到别人的付款流水: %v", out)
	}
	// 员工看自己那张单的流水是允许的（只读）
	s2 := v06Settle(t, r, map[string]interface{}{"worker_id": employee, "paid_cents": 100})
	if _, out := doReq(r, http.MethodGet, v06URL("/api/worklog/settlements/%d", uint(v06Num(s2, "id"))), 22, nil); out["code"].(float64) != 0 {
		t.Fatalf("员工应能查看自己的结算单: %v", out)
	}
}

// ---- D. v0.6.0 数据回填 ----

// runV06Upgrade 找到并执行 v0.6.0 回填脚本。
func runV06Upgrade(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, u := range database.Upgrades {
		if u.Version == "0.6.0" {
			if err := u.Upgrade(db); err != nil {
				t.Fatalf("执行 v0.6.0 回填失败: %v", err)
			}
			return
		}
	}
	t.Fatal("没有注册 v0.6.0 回填脚本")
}

// TestV06MigrationBackfill 老库升级：记工补 wage_cents、历史结算单视为已付清；必须幂等。
func TestV06MigrationBackfill(t *testing.T) {
	_, db := setupTest(t)

	// 造 v0.5.x 的数据：只有 amount_cents，没有工钱拆分；结算单没有已付
	if err := db.Create(&WorkRecord{UserID: 1, WorkerID: 1, Date: "2026-10-01", Type: TypeDay,
		Quantity: "1", UnitPriceCents: 30000, AmountCents: 30000, Status: RecordStatusApproved}).Error; err != nil {
		t.Fatalf("seed legacy record: %v", err)
	}
	if err := db.Create(&WorkRecord{UserID: 1, WorkerID: 1, Date: "2026-10-02", Type: TypeRest,
		Quantity: "0", Status: RecordStatusApproved}).Error; err != nil {
		t.Fatalf("seed legacy rest: %v", err)
	}
	legacy := Settlement{UserID: 1, WorkerID: 1, WorkerName: "老数据",
		WorkAmountCents: 8000, AdvanceAmountCents: 500, PayableCents: 7500, PayStatus: PayStatusPaid}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy settlement: %v", err)
	}
	// 升级后新机制产生的未付单不能被误改成已付清
	unpaid := Settlement{UserID: 1, WorkerID: 1, WorkerName: "欠薪",
		WorkAmountCents: 9000, PayableCents: 9000, PaidCents: 3000, PayStatus: PayStatusPartial}
	if err := db.Create(&unpaid).Error; err != nil {
		t.Fatalf("seed partial settlement: %v", err)
	}

	runV06Upgrade(t, db)
	var rec WorkRecord
	db.Where("type = ?", TypeDay).First(&rec)
	if rec.WageCents != 30000 {
		t.Fatalf("记工工钱未回填: %+v", rec)
	}
	var rest WorkRecord
	db.Where("type = ?", TypeRest).First(&rest)
	if rest.WageCents != 0 || rest.AmountCents != 0 {
		t.Fatalf("休息日不该回填工钱: %+v", rest)
	}

	var s1, s2 Settlement
	db.First(&s1, legacy.ID)
	if s1.PaidCents != 7500 || s1.PayStatus != PayStatusPaid {
		t.Fatalf("历史结算单未回填为已付清: %+v", s1)
	}
	db.First(&s2, unpaid.ID)
	if s2.PaidCents != 3000 || s2.PayStatus != PayStatusPartial {
		t.Fatalf("回填误伤了新机制的欠薪单: %+v", s2)
	}

	// 幂等：再跑一次结果不变（升级脚本可能被重复触发，也可能与恢复旧备份同时发生）
	runV06Upgrade(t, db)
	db.First(&s1, legacy.ID)
	db.First(&s2, unpaid.ID)
	if s1.PaidCents != 7500 || s2.PaidCents != 3000 || s2.PayStatus != PayStatusPartial {
		t.Fatalf("回填脚本不幂等: legacy=%+v partial=%+v", s1, s2)
	}
}

// TestLegacySettlementShowsPaid 兜底：即使回填脚本没跑（例如未注入版本号的开发构建，
// version=dev 会让 RunUpgrades 跳过 0.6.0），历史工资条也不能显示成「全额欠薪」。
func TestLegacySettlementShowsPaid(t *testing.T) {
	r, db := setupTest(t)
	s := Settlement{UserID: 1, WorkerID: 1, WorkerName: "没回填的老单",
		WorkAmountCents: 5000, PayableCents: 5000, PaidCents: 0, PayStatus: PayStatusPaid}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	got := v06GetSettlement(t, r, s.ID)
	if v06Num(got, "paid_cents") != 5000 || v06Num(got, "due_cents") != 0 {
		t.Fatalf("老结算单被显示成欠薪: %v", got)
	}
}

// TestSettlementDerivedIgnoresNegativeDue 已付意外超过应付时，尚欠按 0 展示，不出现负欠款。
func TestSettlementDerivedIgnoresNegativeDue(t *testing.T) {
	s := Settlement{PayableCents: 1000, PaidCents: 1200, PayStatus: PayStatusPaid}
	fillSettlementDerived(&s, 1)
	if s.DueCents != 0 {
		t.Fatalf("尚欠不该为负: %d", s.DueCents)
	}
}
