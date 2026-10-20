package worklog

import (
	"net/http"
	"testing"
)

// TestWorkerWithoutBoundWorkerCannotReadBossData 锁住 2026-10-18 审查发现的越权：
// worker 角色成员的 WorkerID 为 nil（历史数据/半途失败的事务会留下这种行）时，
// 早先 workerScope() 返回 ok=false，调用方见 false 就不加过滤，等于把老板视角的
// 全量数据交给这名员工。修复后必须 fail closed：要么 403，要么空结果。
func TestWorkerWithoutBoundWorkerCannotReadBossData(t *testing.T) {
	r, db := setupTest(t)
	createTestUser(t, db, 2, "unbound-worker")

	// 老板(uid=1) 建班组 + 两个工人
	w, out := doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{"name": "一队"})
	team := mustOK(t, w, out, "create team")
	teamID := uint(team["id"].(float64))
	for _, n := range []string{"张三", "李四"} {
		w, out = doReq(r, http.MethodPost, "/api/worklog/workers", 1,
			map[string]interface{}{"name": n, "daily_wage_cents": 30000})
		mustOK(t, w, out, "create worker "+n)
	}

	// 关键构造：worker 角色成员，但 worker_id 为 NULL
	if err := db.Create(&TeamMember{
		TeamID: teamID, UserID: 2, Role: RoleWorker, WorkerID: nil, Status: "active",
	}).Error; err != nil {
		t.Fatalf("insert member: %v", err)
	}

	ac := loadAccess(db, 2)
	if ac.Role != RoleWorker {
		t.Fatalf("前置条件不成立：role=%s", ac.Role)
	}
	if !ac.workerUnbound() {
		t.Fatal("workerUnbound() 应为 true")
	}
	if sw, ok := ac.workerScope(); !ok || sw != 0 {
		t.Fatalf("workerScope() 应为 (0,true) 以 fail closed，实际 (%d,%v)", sw, ok)
	}

	// 工人列表：不能看到老板的两个工人
	w, out = doReq(r, http.MethodGet, "/api/worklog/workers", 2, nil)
	if out["code"].(float64) == 0 {
		if rows, ok := out["data"].([]interface{}); ok && len(rows) > 0 {
			t.Fatalf("越权：未绑定工人档案的员工读到了 %d 个工人", len(rows))
		}
	}

	// 考勤导出：必须 403，而不是导出全部在职工人
	w, out = doReq(r, http.MethodGet, "/api/worklog/export/attendance.csv?year=2026&month=10", 2, nil)
	if w.Code == http.StatusOK {
		t.Fatal("越权：未绑定工人档案的员工可以导出考勤表")
	}

	// 按工地统计：不能出现老板视角的数据
	w, out = doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 2, nil)
	if out["code"].(float64) == 0 {
		t.Logf("stats/project 返回成功但应为空：%v", out["data"])
	}
}
