package worklog

import (
	"net/http"
	"testing"

	"gorm.io/gorm"
	"smallgo/server/database"
)

// createTestUser 在测试库里补一个账号，用于「直接加成员」场景。
func createTestUser(t *testing.T, db *gorm.DB, id uint, username string) database.User {
	t.Helper()
	u := database.User{ID: id, Username: username, Password: "x", Role: "user", Status: 1, AuthVersion: 1}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("create test user %s: %v", username, err)
	}
	return u
}

// TestMultiTeamsAndDirectAddMember 覆盖粉丝需求 #2/#5：
// 老板可建多个班组、在用户管理里直接把已有用户加进指定班组、给成员设单价，
// 以及越权与重复添加等边界。
func TestMultiTeamsAndDirectAddMember(t *testing.T) {
	r, db := setupTest(t)
	memberUser := createTestUser(t, db, 2, "worker1")
	bossUser := createTestUser(t, db, 3, "other-boss")

	// 老板（uid=1）建两个班组
	w, out := doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{"name": "一队"})
	teamA := mustOK(t, w, out, "create team A")
	w, out = doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{"name": "二队"})
	teamB := mustOK(t, w, out, "create team B")
	teamAID := uint(teamA["id"].(float64))
	teamBID := uint(teamB["id"].(float64))

	// 班组列表返回两个自有班组
	w, out = doReq(r, http.MethodGet, "/api/worklog/teams", 1, nil)
	teamsData := mustOK(t, w, out, "list teams")
	if got := len(teamsData["teams"].([]interface{})); got != 2 {
		t.Fatalf("owned teams = %d, want 2", got)
	}

	// 直接把 worker1 加到「二队」，并带日薪/时薪
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/members", 1, map[string]interface{}{
		"team_id": teamBID, "user_id": memberUser.ID, "role": RoleWorker,
		"worker_name": "小李", "daily_wage_cents": 20000, "hourly_wage_cents": 2000,
	})
	added := mustOK(t, w, out, "add member directly")
	memberID := uint(added["member"].(map[string]interface{})["id"].(float64))

	// 「二队」成员里有小李且单价回填；「一队」没有成员
	w, out = doReq(r, http.MethodGet, "/api/worklog/team?team_id="+itoa(int(teamBID)), 1, nil)
	detail := mustOK(t, w, out, "team B detail")
	members := detail["members"].([]interface{})
	if len(members) != 1 {
		t.Fatalf("team B members = %d, want 1", len(members))
	}
	m := members[0].(map[string]interface{})
	if m["worker_name"] != "小李" || int64(m["daily_wage_cents"].(float64)) != 20000 {
		t.Fatalf("member display/wage wrong: %v", m)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/team?team_id="+itoa(int(teamAID)), 1, nil)
	if got := len(mustOK(t, w, out, "team A detail")["members"].([]interface{})); got != 0 {
		t.Fatalf("team A members = %d, want 0", got)
	}

	// 给成员改单价，写回绑定的工人档案
	w, out = doReq(r, http.MethodPut, "/api/worklog/team/members/"+itoa(int(memberID))+"/wage", 1, map[string]interface{}{
		"daily_wage_cents": 25000, "hourly_wage_cents": 2500,
	})
	mustOK(t, w, out, "update member wage")
	w, out = doReq(r, http.MethodGet, "/api/worklog/team?team_id="+itoa(int(teamBID)), 1, nil)
	m = mustOK(t, w, out, "team B detail again")["members"].([]interface{})[0].(map[string]interface{})
	if int64(m["daily_wage_cents"].(float64)) != 25000 {
		t.Fatalf("updated daily wage = %v, want 25000", m["daily_wage_cents"])
	}

	// 被直接加入的账号以员工身份看到所在班组与绑定工人
	w, out = doReq(r, http.MethodGet, "/api/worklog/me", int(memberUser.ID), nil)
	me := mustOK(t, w, out, "member me")
	if me["role"] != RoleWorker || me["self_worker_name"] != "小李" || !me["in_team"].(bool) {
		t.Fatalf("member me wrong: %v", me)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/teams", int(memberUser.ID), nil)
	joined := mustOK(t, w, out, "member teams")["joined"]
	if joined == nil || uint(joined.(map[string]interface{})["id"].(float64)) != teamBID {
		t.Fatalf("member joined team wrong: %v", joined)
	}

	// 边界：成员不能加人；老板不能加自己 / 重复加 / 加另一个老板
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/members", int(memberUser.ID), map[string]interface{}{
		"team_id": teamBID, "user_id": bossUser.ID, "role": RoleWorker,
	})
	if w.Code == http.StatusOK {
		t.Fatalf("member must not add others: %d %v", w.Code, out)
	}
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/members", 1, map[string]interface{}{
		"team_id": teamBID, "user_id": 1, "role": RoleWorker,
	})
	if w.Code == http.StatusOK {
		t.Fatalf("boss must not add self")
	}
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/members", 1, map[string]interface{}{
		"team_id": teamBID, "user_id": memberUser.ID, "role": RoleWorker,
	})
	if w.Code == http.StatusOK {
		t.Fatalf("duplicate add must fail")
	}
	// bossUser 自己拥有班组后不能被别人加为成员
	w, out = doReq(r, http.MethodPost, "/api/worklog/teams", int(bossUser.ID), map[string]interface{}{"name": "别人的队"})
	mustOK(t, w, out, "other boss create team")
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/members", 1, map[string]interface{}{
		"team_id": teamBID, "user_id": bossUser.ID, "role": RoleWorker,
	})
	if w.Code == http.StatusOK {
		t.Fatalf("adding another team owner must fail")
	}

	// 只解散指定班组：一队还在，worker1 恢复个人模式
	w, out = doReq(r, http.MethodDelete, "/api/worklog/teams/"+itoa(int(teamBID)), 1, nil)
	mustOK(t, w, out, "delete team B")
	w, out = doReq(r, http.MethodGet, "/api/worklog/teams", 1, nil)
	if got := len(mustOK(t, w, out, "teams after delete")["teams"].([]interface{})); got != 1 {
		t.Fatalf("owned teams after delete = %d, want 1", got)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/me", int(memberUser.ID), nil)
	if role := mustOK(t, w, out, "member me after disband")["role"]; role != RoleBoss {
		t.Fatalf("member role after disband = %v, want boss", role)
	}
}
