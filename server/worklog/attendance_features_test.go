package worklog

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 本文件覆盖 2026-10-23 粉丝反馈的考勤/对工改造：
//   - 记工按工地过滤（GET /records?project_id=）
//   - 月度考勤按工地过滤（GET /attendance?project_id=）
//   - 自定义时间段考勤（GET /attendance/range）
//   - 一键对工（GET /attendance/reconcile）与对工单导出（/export/reconcile.csv）
//   - 考勤表按时间段导出（/export/attendance.csv?start=&end=）
//   - 员工角色的数据隔离（只能看自己）

// attendanceFixture 测试数据：
//
//	张三（日薪 300 元）：10-01 A 工地点工 1 天、10-02 B 工地点工 0.5 天、
//	                    10-05 A 工地休息、10-06 A 工地计时 2 小时（50 元/时）、
//	                    10-07 A 工地计件 100（5 元/件）
//	李四（日薪 200 元）：10-03、10-04 B 工地各点工 1 天
//	王五（日薪 250 元）：10-08 未分工地点工 1 天
type attendanceFixture struct {
	projectA, projectB uint
	zhang, li, wang    uint
}

func seedAttendanceFixture(t *testing.T, r *gin.Engine) attendanceFixture {
	t.Helper()
	f := attendanceFixture{}

	w, out := doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{"name": "A 工地"})
	f.projectA = uint(mustOK(t, w, out, "create project A")["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{"name": "B 工地"})
	f.projectB = uint(mustOK(t, w, out, "create project B")["id"].(float64))

	createWorker := func(name string, daily int64) uint {
		w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1,
			map[string]interface{}{"name": name, "daily_wage_cents": daily})
		return uint(mustOK(t, w, out, "create worker "+name)["id"].(float64))
	}
	f.zhang = createWorker("张三", 30000)
	f.li = createWorker("李四", 20000)
	f.wang = createWorker("王五", 25000)

	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1,
		map[string]interface{}{"name": "砌墙", "unit": "㎡", "unit_price_cents": 500})
	pieceID := uint(mustOK(t, w, out, "create piece item")["id"].(float64))

	records := []map[string]interface{}{
		{"worker_id": f.zhang, "date": "2026-10-01", "type": "day", "quantity": "1", "project_id": f.projectA},
		{"worker_id": f.zhang, "date": "2026-10-02", "type": "day", "quantity": "0.5", "project_id": f.projectB},
		{"worker_id": f.zhang, "date": "2026-10-05", "type": "rest", "project_id": f.projectA},
		{"worker_id": f.zhang, "date": "2026-10-06", "type": "hour", "quantity": "2", "unit_price_cents": 5000, "project_id": f.projectA},
		{"worker_id": f.zhang, "date": "2026-10-07", "type": "piece", "quantity": "100", "piece_item_id": pieceID, "project_id": f.projectA},
		{"worker_id": f.li, "date": "2026-10-03", "type": "day", "quantity": "1", "project_id": f.projectB},
		{"worker_id": f.li, "date": "2026-10-04", "type": "day", "quantity": "1", "project_id": f.projectB},
		{"worker_id": f.wang, "date": "2026-10-08", "type": "day", "quantity": "1"},
	}
	for i, rec := range records {
		postDayRecord(t, r, rec, "seed record "+itoa(i))
	}
	return f
}

// bindEmployee 建班组并把 userID 绑定到已有工人档案（模拟工人在班组里的账号）。
func bindEmployee(t *testing.T, db *gorm.DB, workerID, userID uint) {
	t.Helper()
	createTestUser(t, db, userID, "worker-"+itoa(int(userID)))
	team := Team{Name: "测试班组", OwnerUserID: 1, InviteCode: "test" + itoa(int(userID))}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	wid := workerID
	if err := db.Create(&TeamMember{
		TeamID: team.ID, UserID: userID, Role: RoleWorker, WorkerID: &wid, Status: "active",
	}).Error; err != nil {
		t.Fatalf("create member: %v", err)
	}
}

func rangeRowByName(t *testing.T, data map[string]interface{}, name string) map[string]interface{} {
	t.Helper()
	rows, ok := data["rows"].([]interface{})
	if !ok {
		t.Fatalf("rows 字段类型错误: %v", data["rows"])
	}
	for _, raw := range rows {
		row := raw.(map[string]interface{})
		if row["worker_name"] == name {
			return row
		}
	}
	t.Fatalf("rows 中找不到工人 %s: %v", name, rows)
	return nil
}

func assertNum(t *testing.T, row map[string]interface{}, field string, want float64, ctx string) {
	t.Helper()
	got, ok := row[field].(float64)
	if !ok || got != want {
		t.Fatalf("%s: %s = %v, want %v (row=%v)", ctx, field, row[field], want, row)
	}
}

func TestAttendanceRangeAggregates(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31", 1, nil)
	data := mustOK(t, w, out, "attendance range")

	rows := data["rows"].([]interface{})
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3（含零记录的工人在内）", len(rows))
	}
	zhang := rangeRowByName(t, data, "张三")
	assertNum(t, zhang, "attend_days", 4, "张三")
	assertNum(t, zhang, "days", 1.5, "张三")
	assertNum(t, zhang, "hours", 2, "张三")
	assertNum(t, zhang, "pieces", 100, "张三")
	assertNum(t, zhang, "rest_count", 1, "张三")
	assertNum(t, zhang, "amount_cents", 105000, "张三")
	assertNum(t, zhang, "record_count", 5, "张三")
	if uint(zhang["worker_id"].(float64)) != f.zhang {
		t.Fatalf("张三 worker_id = %v, want %d", zhang["worker_id"], f.zhang)
	}

	li := rangeRowByName(t, data, "李四")
	assertNum(t, li, "attend_days", 2, "李四")
	assertNum(t, li, "days", 2, "李四")
	assertNum(t, li, "amount_cents", 40000, "李四")

	wang := rangeRowByName(t, data, "王五")
	assertNum(t, wang, "attend_days", 1, "王五")
	assertNum(t, wang, "amount_cents", 25000, "王五")

	total := data["total"].(map[string]interface{})
	assertNum(t, total, "attend_days", 7, "合计")
	assertNum(t, total, "days", 4.5, "合计")
	assertNum(t, total, "hours", 2, "合计")
	assertNum(t, total, "pieces", 100, "合计")
	assertNum(t, total, "rest_count", 1, "合计")
	assertNum(t, total, "amount_cents", 170000, "合计")
	assertNum(t, total, "record_count", 8, "合计")
}

func TestAttendanceRangeProjectFilter(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	// A 工地：只有张三的 4 条记录
	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31&project_id="+itoa(int(f.projectA)), 1, nil)
	data := mustOK(t, w, out, "range project A")
	if uint(data["project_id"].(float64)) != f.projectA {
		t.Fatalf("project_id 回显 = %v, want %d", data["project_id"], f.projectA)
	}
	zhang := rangeRowByName(t, data, "张三")
	assertNum(t, zhang, "attend_days", 3, "张三 A 工地")
	assertNum(t, zhang, "days", 1, "张三 A 工地")
	assertNum(t, zhang, "hours", 2, "张三 A 工地")
	assertNum(t, zhang, "pieces", 100, "张三 A 工地")
	assertNum(t, zhang, "rest_count", 1, "张三 A 工地")
	assertNum(t, zhang, "amount_cents", 90000, "张三 A 工地")
	// 其他工人在 A 工地零记录但仍出行（0 值）
	assertNum(t, rangeRowByName(t, data, "李四"), "amount_cents", 0, "李四 A 工地")
	total := data["total"].(map[string]interface{})
	assertNum(t, total, "amount_cents", 90000, "A 工地合计")

	// 未分工地：只有王五
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31&project_id=none", 1, nil)
	data = mustOK(t, w, out, "range project none")
	if data["project_id"] != nil {
		t.Fatalf("project_id=none 回显应为 null，实际 %v", data["project_id"])
	}
	wang := rangeRowByName(t, data, "王五")
	assertNum(t, wang, "amount_cents", 25000, "王五 未分工地")
	assertNum(t, rangeRowByName(t, data, "张三"), "amount_cents", 0, "张三 未分工地")
}

func TestAttendanceRangeWorkerFilterAndValidation(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31&worker_id="+itoa(int(f.zhang)), 1, nil)
	data := mustOK(t, w, out, "range single worker")
	if rows := data["rows"].([]interface{}); len(rows) != 1 {
		t.Fatalf("指定工人时 rows = %d, want 1", len(rows))
	}
	if worker, ok := data["worker"].(map[string]interface{}); !ok || worker["name"] != "张三" {
		t.Fatalf("worker 回显错误: %v", data["worker"])
	}

	bad := []string{
		"/api/worklog/attendance/range?start=2026-10-31&end=2026-10-01",                  // 起止颠倒
		"/api/worklog/attendance/range?start=2026-10-01",                                 // 缺 end
		"/api/worklog/attendance/range?start=2026-01-01&end=2028-02-01",                  // 超 731 天
		"/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31&project_id=abc",   // 工地参数非法
		"/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31&worker_id=999999", // 工人不存在
	}
	for _, path := range bad {
		w, _ := doReq(r, http.MethodGet, path, 1, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: http=%d, want 400", path, w.Code)
		}
	}
}

func TestAttendanceRangeEmployeeScope(t *testing.T) {
	r, db := setupTest(t)
	f := seedAttendanceFixture(t, r)
	bindEmployee(t, db, f.li, 2)

	// 员工只能看到自己（李四）：传别人的 worker_id 也强制改回自己
	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31&worker_id="+itoa(int(f.zhang)), 2, nil)
	data := mustOK(t, w, out, "employee range")
	rows := data["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("员工 rows = %d, want 1", len(rows))
	}
	row := rows[0].(map[string]interface{})
	if row["worker_name"] != "李四" {
		t.Fatalf("员工越权看到 %v，应为李四", row["worker_name"])
	}
	assertNum(t, row, "amount_cents", 40000, "员工李四")

	// 对工单同样强制自己
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance/reconcile?worker_id="+itoa(int(f.zhang))+
		"&start=2026-10-01&end=2026-10-31", 2, nil)
	data = mustOK(t, w, out, "employee reconcile")
	worker := data["worker"].(map[string]interface{})
	if worker["name"] != "李四" {
		t.Fatalf("员工对工单越权：worker=%v", worker["name"])
	}

	// 未绑定工人档案的员工：新接口必须 403，而不是退回全量数据
	createTestUser(t, db, 3, "unbound")
	team2 := Team{Name: "二队", OwnerUserID: 1, InviteCode: "unbound"}
	if err := db.Create(&team2).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	if err := db.Create(&TeamMember{TeamID: team2.ID, UserID: 3, Role: RoleWorker, WorkerID: nil, Status: "active"}).Error; err != nil {
		t.Fatalf("create unbound member: %v", err)
	}
	for _, path := range []string{
		"/api/worklog/attendance/range?start=2026-10-01&end=2026-10-31",
		"/api/worklog/attendance/reconcile?worker_id=" + itoa(int(f.li)) + "&start=2026-10-01&end=2026-10-31",
		"/api/worklog/export/reconcile.csv?worker_id=" + itoa(int(f.li)) + "&start=2026-10-01&end=2026-10-31",
	} {
		w, _ := doReq(r, http.MethodGet, path, 3, nil)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s: 未绑定员工的请求 http=%d, want 403（body=%s）", path, w.Code, w.Body.String())
		}
	}
}

func TestRecordsByDateProjectFilter(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	w, out := doReq(r, http.MethodGet, "/api/worklog/records?date=2026-10-01&project_id="+itoa(int(f.projectA)), 1, nil)
	data := mustOK(t, w, out, "records project A")
	if uint(data["project_id"].(float64)) != f.projectA {
		t.Fatalf("project_id 回显错误: %v", data["project_id"])
	}
	row := dayRowByName(t, data, "张三")
	assertNum(t, row, "amount_cents", 30000, "张三 A 工地 10-01")
	assertNum(t, row, "record_count", 1, "张三 A 工地 10-01")

	// 同一日期切到 B 工地：张三当天在 B 没有记录
	w, out = doReq(r, http.MethodGet, "/api/worklog/records?date=2026-10-01&project_id="+itoa(int(f.projectB)), 1, nil)
	data = mustOK(t, w, out, "records project B")
	row = dayRowByName(t, data, "张三")
	assertNum(t, row, "record_count", 0, "张三 B 工地 10-01")
	assertNum(t, row, "amount_cents", 0, "张三 B 工地 10-01")

	// 未分工地：10-08 王五那笔只在 none 下出现
	w, out = doReq(r, http.MethodGet, "/api/worklog/records?date=2026-10-08&project_id=none", 1, nil)
	data = mustOK(t, w, out, "records project none")
	assertNum(t, dayRowByName(t, data, "王五"), "amount_cents", 25000, "王五 未分工地 10-08")

	// 非法参数
	w, _ = doReq(r, http.MethodGet, "/api/worklog/records?date=2026-10-01&project_id=abc", 1, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("非法 project_id 应 400，实际 %d", w.Code)
	}
}

func dayRowByName(t *testing.T, data map[string]interface{}, name string) map[string]interface{} {
	t.Helper()
	for _, raw := range data["workers"].([]interface{}) {
		row := raw.(map[string]interface{})
		if row["name"] == name {
			return row
		}
	}
	t.Fatalf("workers 中找不到 %s", name)
	return nil
}

func TestAttendanceMonthlyProjectFilter(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance?year=2026&month=10&worker_id="+itoa(int(f.zhang))+
		"&project_id="+itoa(int(f.projectA)), 1, nil)
	data := mustOK(t, w, out, "monthly attendance project A")
	assertNum(t, data, "days", 1, "张三 A 工地")
	assertNum(t, data, "hours", 2, "张三 A 工地")
	assertNum(t, data, "pieces", 100, "张三 A 工地")
	assertNum(t, data, "rest_count", 1, "张三 A 工地")
	assertNum(t, data, "amount_cents", 90000, "张三 A 工地")
	if got := len(data["cells"].([]interface{})); got != 4 {
		t.Fatalf("cells = %d, want 4（A 工地 4 天有记录）", got)
	}

	// 不传工地：整月 5 条记录、5 天
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance?year=2026&month=10&worker_id="+itoa(int(f.zhang)), 1, nil)
	data = mustOK(t, w, out, "monthly attendance all projects")
	assertNum(t, data, "amount_cents", 105000, "张三 全部工地")
	if got := len(data["cells"].([]interface{})); got != 5 {
		t.Fatalf("cells = %d, want 5", got)
	}
}

func TestReconcile(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/reconcile?worker_id="+itoa(int(f.zhang))+
		"&start=2026-10-01&end=2026-10-31", 1, nil)
	data := mustOK(t, w, out, "reconcile")
	worker := data["worker"].(map[string]interface{})
	if worker["name"] != "张三" {
		t.Fatalf("worker = %v", worker["name"])
	}
	assertNum(t, data, "attend_days", 4, "对工单")
	assertNum(t, data, "days", 1.5, "对工单")
	assertNum(t, data, "amount_cents", 105000, "对工单")
	assertNum(t, data, "record_count", 5, "对工单")
	if data["project_name"] != "全部工地" {
		t.Fatalf("project_name = %v, want 全部工地", data["project_name"])
	}
	if data["generated_at"] == "" {
		t.Fatal("generated_at 不应为空")
	}

	items := data["items"].([]interface{})
	if len(items) != 5 {
		t.Fatalf("items = %d, want 5", len(items))
	}
	first := items[0].(map[string]interface{})
	if first["date"] != "2026-10-01" || first["type_label"] != "点工" || first["project_name"] != "A 工地" {
		t.Fatalf("首条明细错误: %v", first)
	}
	restFound := false
	for _, raw := range items {
		it := raw.(map[string]interface{})
		if it["rest"].(bool) {
			restFound = true
			if it["type_label"] != "休息" || it["date"] != "2026-10-05" {
				t.Fatalf("休息明细错误: %v", it)
			}
		}
	}
	if !restFound {
		t.Fatal("对工单缺少休息明细")
	}

	// 按工地过滤 + 工地名回显
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance/reconcile?worker_id="+itoa(int(f.zhang))+
		"&start=2026-10-01&end=2026-10-31&project_id="+itoa(int(f.projectA)), 1, nil)
	data = mustOK(t, w, out, "reconcile project A")
	if data["project_name"] != "A 工地" {
		t.Fatalf("project_name = %v, want A 工地", data["project_name"])
	}
	if items := data["items"].([]interface{}); len(items) != 4 {
		t.Fatalf("A 工地 items = %d, want 4", len(items))
	}

	// 未分工地：王五那一笔，工地名回显「未分工地」
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance/reconcile?worker_id="+itoa(int(f.wang))+
		"&start=2026-10-01&end=2026-10-31&project_id=none", 1, nil)
	data = mustOK(t, w, out, "reconcile project none")
	if data["project_name"] != "未分工地" {
		t.Fatalf("project_name = %v, want 未分工地", data["project_name"])
	}

	// 非法参数
	bad := []string{
		"/api/worklog/attendance/reconcile?start=2026-10-01&end=2026-10-31",                                                        // 缺工人
		"/api/worklog/attendance/reconcile?worker_id=999999&start=2026-10-01&end=2026-10-31",                                       // 工人不存在
		"/api/worklog/attendance/reconcile?worker_id=" + itoa(int(f.zhang)) + "&start=2026-10-31&end=2026-10-01",                   // 起止颠倒
		"/api/worklog/attendance/reconcile?worker_id=" + itoa(int(f.zhang)) + "&start=2026-10-01&end=2026-10-31&project_id=999999", // 工地不存在
	}
	for _, path := range bad {
		w, _ := doReq(r, http.MethodGet, path, 1, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: http=%d, want 400（body=%s）", path, w.Code, w.Body.String())
		}
	}
}

func TestExportAttendanceRangeCSV(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	// 时间段 + 工地 + 单工人导出
	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/attendance.csv?start=2026-10-01&end=2026-10-31&worker_id="+
		itoa(int(f.zhang))+"&project_id="+itoa(int(f.projectA)), 1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("attendance range csv http=%d body=%s", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, "2026-10-01") {
		t.Fatalf("导出文件名未体现时间段: %q", cd)
	}
	body := w.Body.String()
	for _, want := range []string{"工人,日期", "张三,2026-10-01", "合计", "900.00"} {
		if !strings.Contains(body, want) {
			t.Fatalf("考勤表 CSV 缺 %q，内容：\n%s", want, body)
		}
	}
	if strings.Contains(body, "李四") {
		t.Fatalf("指定工人导出不应包含其他工人，内容：\n%s", body)
	}

	// 月度导出保持兼容
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/attendance.csv?year=2026&month=10", 1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("monthly attendance csv http=%d", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "张三") || !strings.Contains(body, "李四") {
		t.Fatalf("月度考勤表应覆盖全部在职工人，内容：\n%s", body)
	}

	// 只有 start 或只有 end 时明确 400
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/attendance.csv?start=2026-10-01", 1, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("只传 start 应 400，实际 %d", w.Code)
	}
}

func TestExportReconcileCSV(t *testing.T) {
	r, _ := setupTest(t)
	f := seedAttendanceFixture(t, r)

	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/reconcile.csv?worker_id="+itoa(int(f.zhang))+
		"&start=2026-10-01&end=2026-10-31", 1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("reconcile csv http=%d body=%s", w.Code, w.Body.String())
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Fatalf("对工单 CSV 缺少下载头: %q", cd)
	}
	body := w.Body.String()
	for _, want := range []string{
		"对工单", "工人,张三", "期间,2026-10-01 至 2026-10-31", "工地,全部工地",
		"日期,类型,数量,单价(元),金额(元),工地,计件项目,备注",
		"2026-10-01,点工,1,300.00,300.00,A 工地,,",
		"点工 1.5 天；计时 2 小时；计件 100；休息 1 天",
		"合计", "1050.00",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("对工单 CSV 缺 %q，内容：\n%s", want, body)
		}
	}

	// 按工地导出时表头工地名跟着变
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/reconcile.csv?worker_id="+itoa(int(f.zhang))+
		"&start=2026-10-01&end=2026-10-31&project_id="+itoa(int(f.projectA)), 1, nil)
	if body := w.Body.String(); !strings.Contains(body, "工地,A 工地") {
		t.Fatalf("指定工地导出的对工单工地行错误：\n%s", body)
	}

	// 缺工人参数 400
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/reconcile.csv?start=2026-10-01&end=2026-10-31", 1, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("缺 worker_id 应 400，实际 %d", w.Code)
	}
}
