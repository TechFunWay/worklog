package worklog

import (
	"net/http"
	"strings"
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

	// 员工不能自带单价，点工按档案日薪计算；给班组工人设 300 元/天。
	w, out = doReq(r, http.MethodPut, "/api/worklog/workers/"+itoa(workerID), 1, map[string]interface{}{
		"name": "小王", "daily_wage_cents": 30000, "status": "active",
	})
	mustOK(t, w, out, "set worker daily wage")

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
		"worker_id": workerID, "date": "2026-09-01", "type": "day", "quantity": "1",
	})
	mustOK(t, w, out, "worker self record")

	// 员工给别人记工 → 被强制改成自己（不报错但落库是自己）
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": 9999, "date": "2026-09-02", "type": "day", "quantity": "1",
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

// TestLeaderCreatedArchivesBelongToTeamOwner 组长建的工人/计件项目必须归班组老板数据集，
// 否则组长建完自己都没法用它记工（记工按数据集老板校验工人归属）。
func TestLeaderCreatedArchivesBelongToTeamOwner(t *testing.T) {
	r, db := setupRoleTest(t)
	w, out := doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{"name": "归属测试班组"})
	team := mustOK(t, w, out, "create team")
	invite := team["invite_code"].(string)
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/join", 2, map[string]interface{}{
		"invite_code": invite, "worker_name": "组长小李",
	})
	mustOK(t, w, out, "join as leader")
	w, out = doReq(r, http.MethodGet, "/api/worklog/team", 1, nil)
	members := mustOK(t, w, out, "team detail")["members"].([]interface{})
	var memberID float64
	for _, m := range members {
		mm := m.(map[string]interface{})
		if mm["user_id"].(float64) == 2 {
			memberID = mm["id"].(float64)
		}
	}
	if memberID == 0 {
		t.Fatal("member not found")
	}
	w, out = doReq(r, http.MethodPut, "/api/worklog/team/members/"+itoa(int(memberID))+"/role", 1, map[string]interface{}{"role": "leader"})
	mustOK(t, w, out, "promote leader")

	// 组长建工人与计件项目
	w, out = doReq(r, http.MethodPost, "/api/worklog/workers", 2, map[string]interface{}{"name": "组长建的工人", "daily_wage_cents": 30000})
	worker := mustOK(t, w, out, "leader create worker")
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 2, map[string]interface{}{"name": "组长建的项目", "unit": "件", "unit_price_cents": 100})
	piece := mustOK(t, w, out, "leader create piece")
	wid := uint(worker["id"].(float64))
	pid := uint(piece["id"].(float64))

	var dbWorker Worker
	if err := db.First(&dbWorker, wid).Error; err != nil {
		t.Fatalf("load worker: %v", err)
	}
	if dbWorker.UserID != 1 {
		t.Fatalf("leader-created worker user_id = %d, want 1", dbWorker.UserID)
	}
	var dbPiece PieceItem
	if err := db.First(&dbPiece, pid).Error; err != nil {
		t.Fatalf("load piece: %v", err)
	}
	if dbPiece.UserID != 1 {
		t.Fatalf("leader-created piece user_id = %d, want 1", dbPiece.UserID)
	}

	// 老板用这个工人/项目记工应当成功（旧实现会提示工人不存在）
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{
		"worker_id": wid, "piece_item_id": pid, "date": "2026-11-13", "type": "piece", "quantity": "10",
	})
	mustOK(t, w, out, "boss record with leader-created archives")
}

// TestWorkerCannotSetUnitPrice 员工不能自带单价：关闭审核的班组里员工若可传
// unit_price_cents，就能给自己抬价，直接算进工资。
func TestWorkerCannotSetUnitPrice(t *testing.T) {
	r, db := setupRoleTest(t)
	w, out := doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{"name": "单价测试班组"})
	team := mustOK(t, w, out, "create team")
	invite := team["invite_code"].(string)
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/join", 2, map[string]interface{}{
		"invite_code": invite, "worker_name": "员工小王",
	})
	joined := mustOK(t, w, out, "join")
	workerID := int(joined["worker"].(map[string]interface{})["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{
		"name": "计件项目", "unit": "件", "unit_price_cents": 100,
	})
	piece := mustOK(t, w, out, "piece")
	pieceID := int(piece["id"].(float64))

	// 员工故意传一个天价单价，服务端必须忽略，按计件项目 100 分/件计算。
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": workerID, "piece_item_id": pieceID, "date": "2026-11-13",
		"type": "piece", "quantity": "5", "unit_price_cents": 888888,
	})
	rec := mustOK(t, w, out, "worker record")
	if rec["unit_price_cents"].(float64) != 100 || rec["amount_cents"].(float64) != 500 {
		t.Fatalf("worker-supplied price was not ignored: %v", rec)
	}
	if rec["status"] != RecordStatusApproved {
		t.Fatalf("review off team should approve directly, got %v", rec["status"])
	}

	// 点工没有设置日薪时，即便员工传天价也应报「请填写大于 0 的单价」而不是按天价落库。
	w, _ = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": workerID, "date": "2026-11-13", "type": "day", "quantity": "1", "unit_price_cents": 888888,
	})
	if w.Code == http.StatusOK {
		t.Fatalf("worker-supplied price on day work should not be accepted without wage")
	}
	var count int64
	db.Model(&WorkRecord{}).Where("type = ?", TypeDay).Count(&count)
	if count != 0 {
		t.Fatalf("worker-supplied day price created %d records", count)
	}
}

// TestTeamExportsRespectScope 组长/员工的导出必须按班组老板数据集取数，
// 否则导出的 CSV/JSON 只有表头或空数据（历史 bug）。
func TestTeamExportsRespectScope(t *testing.T) {
	r, _ := setupRoleTest(t)
	w, out := doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{"name": "导出测试班组"})
	team := mustOK(t, w, out, "create team")
	invite := team["invite_code"].(string)
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/join", 2, map[string]interface{}{"invite_code": invite, "worker_name": "导出工人"})
	joined := mustOK(t, w, out, "join worker")
	workerID := int(joined["worker"].(map[string]interface{})["id"].(float64))
	w, out = doReq(r, http.MethodPut, "/api/worklog/workers/"+itoa(workerID), 1, map[string]interface{}{"name": "导出工人", "daily_wage_cents": 30000, "status": "active"})
	mustOK(t, w, out, "set wage")
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{"worker_id": workerID, "date": "2026-11-13", "type": "day", "quantity": "1"})
	mustOK(t, w, out, "boss record")

	// 组长账号 3（先加入并提升）
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/join", 3, map[string]interface{}{"invite_code": invite, "worker_name": "导出组长"})
	mustOK(t, w, out, "join leader")
	w, out = doReq(r, http.MethodGet, "/api/worklog/team", 1, nil)
	members := mustOK(t, w, out, "team detail")["members"].([]interface{})
	var memberID float64
	for _, m := range members {
		mm := m.(map[string]interface{})
		if mm["user_id"].(float64) == 3 {
			memberID = mm["id"].(float64)
		}
	}
	if memberID == 0 {
		t.Fatal("leader member not found")
	}
	w, out = doReq(r, http.MethodPut, "/api/worklog/team/members/"+itoa(int(memberID))+"/role", 1, map[string]interface{}{"role": "leader"})
	mustOK(t, w, out, "promote leader")

	for _, uid := range []int{2, 3} {
		w, _ = doReq(r, http.MethodGet, "/api/worklog/export/records.csv", uid, nil)
		if w.Code != http.StatusOK {
			t.Fatalf("uid=%d export records http=%d", uid, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, "导出工人") || !strings.Contains(body, "300.00") {
			t.Fatalf("uid=%d 记工导出没有班组老板数据: %s", uid, body)
		}
	}
	// 已通过记录的 JSON 备份也应包含该记录（组长视角）。
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/json", 3, nil)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "导出工人") {
		t.Fatalf("leader JSON export missing record: http=%d body=%s", w.Code, w.Body.String())
	}
}

// TestPriceCapAndDeleteGuards 单价上限与档案引用保护。
func TestPriceCapAndDeleteGuards(t *testing.T) {
	r, _ := setupRoleTest(t)
	// 超上限单价/工资一律拒绝
	w, _ := doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{"name": "天价项目", "unit": "件", "unit_price_cents": maxUnitPriceCents + 1})
	if w.Code == http.StatusOK {
		t.Fatal("piece item over price cap should fail")
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "天价工人", "daily_wage_cents": maxUnitPriceCents + 1})
	if w.Code == http.StatusOK {
		t.Fatal("worker over wage cap should fail")
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{"name": "天价工地", "day_price_cents": maxUnitPriceCents + 1})
	if w.Code == http.StatusOK {
		t.Fatal("project over price cap should fail")
	}

	// 正常档案；被记工/报工码引用后不允许删除
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "引用工人", "daily_wage_cents": 30000})
	worker := mustOK(t, w, out, "worker")
	wid := int(worker["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{"name": "引用项目", "unit": "件", "unit_price_cents": 100})
	piece := mustOK(t, w, out, "piece")
	pid := int(piece["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/entry-codes", 1, map[string]interface{}{"name": "引用码", "piece_item_id": pid, "worker_id": wid})
	mustOK(t, w, out, "code")
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/workers/"+itoa(wid), 1, nil)
	if w.Code == http.StatusOK {
		t.Fatal("worker bound to entry code should not be deletable")
	}
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/piece-items/"+itoa(pid), 1, nil)
	if w.Code == http.StatusOK {
		t.Fatal("piece bound to entry code should not be deletable")
	}
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{"worker_id": wid, "piece_item_id": pid, "date": "2026-11-13", "type": "piece", "quantity": "1"})
	mustOK(t, w, out, "record")
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/piece-items/"+itoa(pid), 1, nil)
	if w.Code == http.StatusOK {
		t.Fatal("piece with records should not be deletable")
	}

	// 工地被记工引用后不允许删除
	w, out = doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{"name": "引用工地"})
	project := mustOK(t, w, out, "project")
	prid := int(project["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 1, map[string]interface{}{"worker_id": wid, "project_id": prid, "date": "2026-11-13", "type": "day", "quantity": "1"})
	mustOK(t, w, out, "project record")
	w, _ = doReq(r, http.MethodDelete, "/api/worklog/projects/"+itoa(prid), 1, nil)
	if w.Code == http.StatusOK {
		t.Fatal("project with records should not be deletable")
	}
}
