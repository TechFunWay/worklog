package worklog

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupRoleTest 复用主测试文件的独立临时库 + 假认证。
func setupRoleTest(t *testing.T) (*gin.Engine, *gorm.DB) {
	return setupTest(t)
}

func TestTeamLifecycleAndRoles(t *testing.T) {
	r, db := setupRoleTest(t)

	// 老板（用户1）创建班组
	w, out := doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{
		"name": "张记工程队", "note": "测试班组",
	})
	data := mustOK(t, w, out, "create team")
	inviteCode, _ := data["invite_code"].(string)
	if inviteCode == "" {
		t.Fatalf("invite code should be returned")
	}

	// 用户2（员工）凭邀请码加入
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/join", 2, map[string]interface{}{
		"invite_code": inviteCode, "worker_name": "小王",
	})
	data = mustOK(t, w, out, "join team")
	workerID := int(data["worker"].(map[string]interface{})["id"].(float64))

	// 员工 me 信息
	w, out = doReq(r, http.MethodGet, "/api/worklog/me", 2, nil)
	me := mustOK(t, w, out, "me as worker")
	if me["role"] != RoleWorker || !me["in_team"].(bool) {
		t.Fatalf("worker me info wrong: %v", me)
	}
	if int(me["self_worker_id"].(float64)) != workerID {
		t.Fatalf("self worker id mismatch")
	}

	// 员工给自己记工
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": workerID, "date": "2026-09-01", "type": "day", "quantity": "1", "unit_price_cents": 30000,
	})
	mustOK(t, w, out, "worker self record")

	// 员工给别人记工 → 被强制改成自己（不报错但落库是自己）
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": 9999, "date": "2026-09-02", "type": "day", "quantity": "1", "unit_price_cents": 30000,
	})
	data = mustOK(t, w, out, "worker forced self record")
	if int(data["worker_id"].(float64)) != workerID {
		t.Fatalf("worker_id should be forced to self")
	}

	// 员工创建工人 → 拒绝
	w, _ = doReq(r, http.MethodPost, "/api/worklog/workers", 2, map[string]interface{}{"name": " intrusion"})
	if w.Code == http.StatusOK {
		t.Fatalf("worker should not create workers")
	}

	// 员工休息记录
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": workerID, "date": "2026-09-03", "type": "rest",
	})
	data = mustOK(t, w, out, "rest record")
	if data["amount_cents"].(float64) != 0 {
		t.Fatalf("rest should be 0 amount")
	}

	// 员工列表只能看到自己
	w, out = doReq(r, http.MethodGet, "/api/worklog/workers", 2, nil)
	list := mustOKList(t, w, out, "worker list as worker")
	if len(list) != 1 {
		t.Fatalf("worker should only see self, got %d", len(list))
	}

	// 老板（用户1）创建第二个账号为组长前的检查：用户3加入
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/join", 3, map[string]interface{}{
		"invite_code": inviteCode, "worker_name": "小李",
	})
	data = mustOK(t, w, out, "join team as member3")
	member3Worker := int(data["worker"].(map[string]interface{})["id"].(float64))

	// 老板查看成员列表，把成员3提升为组长
	w, out = doReq(r, http.MethodGet, "/api/worklog/team", 1, nil)
	data = mustOK(t, w, out, "team detail")
	members := data["members"].([]interface{})
	var member3ID float64
	for _, m := range members {
		mm := m.(map[string]interface{})
		if mm["role"] != RoleBoss && mm["user_id"].(float64) == 3 {
			member3ID = mm["id"].(float64)
		}
	}
	if member3ID == 0 {
		t.Fatalf("member3 not found in team")
	}
	w, out = doReq(r, http.MethodPut, "/api/worklog/team/members/"+itoa(int(member3ID))+"/role", 1, map[string]interface{}{"role": "leader"})
	mustOK(t, w, out, "promote leader")

	// 组长（用户3）可以给员工的工人记工（workerID 属于小王的档案？不——组长给自己建了工人档案小李）
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 3, map[string]interface{}{
		"worker_id": member3Worker, "date": "2026-09-01", "type": "day", "quantity": "1", "unit_price_cents": 20000,
	})
	mustOK(t, w, out, "leader records")

	// 组长不能结算
	w, _ = doReq(r, http.MethodPost, "/api/worklog/settlements", 3, map[string]interface{}{
		"worker_id": member3Worker, "start": "2026-09-01", "end": "2026-09-30",
	})
	if w.Code == http.StatusOK {
		t.Fatalf("leader should not settle")
	}
	// 员工不能结算
	w, _ = doReq(r, http.MethodPost, "/api/worklog/settlements", 2, map[string]interface{}{
		"worker_id": workerID, "start": "2026-09-01", "end": "2026-09-30",
	})
	if w.Code == http.StatusOK {
		t.Fatalf("worker should not settle")
	}

	// 老板可以结算（用户2工人的记录在用户1数据集中）
	w, out = doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": workerID, "start": "2026-09-01", "end": "2026-09-30",
	})
	data = mustOK(t, w, out, "boss settles")
	// 2 笔点工 × 30000 = 60000
	if data["work_amount_cents"].(float64) != 60000 {
		t.Fatalf("settle amount wrong: %v", data["work_amount_cents"])
	}

	// 考勤日历（老板查看小王）
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance?worker_id="+itoa(workerID)+"&year=2026&month=9", 1, nil)
	data = mustOK(t, w, out, "attendance")
	if data["days"].(float64) != 2 {
		t.Fatalf("attendance days should be 2 (settled records still count), got %v", data["days"])
	}
	cells := data["cells"].([]interface{})
	restFound := false
	for _, cell := range cells {
		if cell.(map[string]interface{})["rest"].(bool) {
			restFound = true
		}
	}
	if !restFound {
		t.Fatalf("attendance should contain rest day")
	}

	// 员工查看考勤（只能自己）
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance?year=2026&month=9", 2, nil)
	data = mustOK(t, w, out, "attendance as worker")
	wk := data["worker"].(map[string]interface{})
	if int(wk["id"].(float64)) != workerID {
		t.Fatalf("worker attendance should be self")
	}

	// 按项目结算
	w, out = doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{"name": "城东项目"})
	proj := mustOK(t, w, out, "create project")
	pid := int(proj["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": workerID, "date": "2026-09-04", "type": "day", "quantity": "1", "unit_price_cents": 30000, "project_id": pid,
	})
	mustOK(t, w, out, "project record")
	w, out = doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": workerID, "start": "2026-09-01", "end": "2026-09-30", "project_id": pid,
	})
	data = mustOK(t, w, out, "project settle")
	if data["project_name"].(string) != "城东项目" {
		t.Fatalf("project name snapshot wrong: %v", data)
	}
	if data["work_amount_cents"].(float64) != 30000 {
		t.Fatalf("project settle amount should be 30000, got %v", data["work_amount_cents"])
	}

	// 解散班组：数据留在老板名下
	w, out = doReq(r, http.MethodDelete, "/api/worklog/team", 1, nil)
	mustOK(t, w, out, "disband team")
	var memberCount int64
	db.Model(&TeamMember{}).Count(&memberCount)
	if memberCount != 0 {
		t.Fatalf("members should be removed after disband")
	}
	var recCount int64
	db.Model(&WorkRecord{}).Where("user_id = 1").Count(&recCount)
	if recCount == 0 {
		t.Fatalf("records should remain with boss after disband")
	}
}
