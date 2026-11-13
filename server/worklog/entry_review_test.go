package worklog

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestWorkerReviewFlow 员工记工 → 待审核 → 班组长通过/驳回，工资口径全过程。
func TestWorkerReviewFlow(t *testing.T) {
	r, _ := setupTest(t)
	today := time.Now().Format("2006-01-02")

	// 老板建班组并开启「员工提交需审核」
	w, out := doReq(r, http.MethodPost, "/api/worklog/teams", 1, map[string]interface{}{
		"name": "小厂班组", "review_required": true,
	})
	team := mustOK(t, w, out, "create team")
	invite, _ := team["invite_code"].(string)
	if invite == "" {
		t.Fatalf("missing invite code")
	}
	teamID := int(team["id"].(float64))

	// 员工加入
	w, out = doReq(r, http.MethodPost, "/api/worklog/team/join", 2, map[string]interface{}{
		"invite_code": invite, "worker_name": "小王",
	})
	joined := mustOK(t, w, out, "join team")
	workerID := int(joined["worker"].(map[string]interface{})["id"].(float64))
	// 员工不能自带单价，点工按档案日薪计算；给班组工人设 300 元/天。
	w, out = doReq(r, http.MethodPut, "/api/worklog/workers/"+itoa(workerID), 1, map[string]interface{}{
		"name": "小王", "daily_wage_cents": 30000, "status": "active",
	})
	mustOK(t, w, out, "set worker daily wage")
	w, out = doReq(r, http.MethodGet, "/api/worklog/me", 2, nil)
	me := mustOK(t, w, out, "me as worker")
	if me["review_required"] != true {
		t.Fatalf("worker me should expose review_required: %v", me)
	}

	// 员工提交一笔点工：应落 pending，且不计工资
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": workerID, "date": today, "type": "day", "quantity": "1",
	})
	rec := mustOK(t, w, out, "worker submits record")
	recID := int(rec["id"].(float64))
	if rec["status"] != RecordStatusPending {
		t.Fatalf("worker record status = %v, want pending", rec["status"])
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/stats/dashboard", 1, nil)
	dash := mustOK(t, w, out, "dashboard before approve")
	if dash["month_work_cents"].(float64) != 0 || dash["month_record_count"].(float64) != 0 {
		t.Fatalf("pending record must not count: %v", dash)
	}

	// 记工页：待审单独计数，工资合计为 0
	w, out = doReq(r, http.MethodGet, "/api/worklog/records?date="+today, 1, nil)
	day := mustOK(t, w, out, "records by date")
	rows := day["workers"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("expected 1 worker row, got %d", len(rows))
	}
	row := rows[0].(map[string]interface{})
	if row["pending_count"].(float64) != 1 || row["record_count"].(float64) != 0 || row["amount_cents"].(float64) != 0 {
		t.Fatalf("pending row counters wrong: %v", row)
	}

	// 待审不进结算与考勤：审核通过前「未结工资」和当月出勤都应为 0
	w, out = doReq(r, http.MethodGet, "/api/worklog/settlements/preview?worker_id="+itoa(workerID)+"&start="+today+"&end="+today, 1, nil)
	preview := mustOK(t, w, out, "preview before approve")
	previewSummary := preview["summary"].(map[string]interface{})
	if previewSummary["work_amount_cents"].(float64) != 0 || previewSummary["record_count"].(float64) != 0 {
		t.Fatalf("pending record should not be settleable: %v", previewSummary)
	}
	now := time.Now()
	w, out = doReq(r, http.MethodGet, fmt.Sprintf("/api/worklog/attendance?worker_id=%d&year=%d&month=%d", workerID, now.Year(), int(now.Month())), 1, nil)
	att := mustOK(t, w, out, "attendance before approve")
	if att["days"].(float64) != 0 {
		t.Fatalf("pending record should not count in attendance: %v", att["days"])
	}

	// 待审汇总 + 列表
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/summary", 1, nil)
	summary := mustOK(t, w, out, "review summary")
	if summary["pending_count"].(float64) != 1 {
		t.Fatalf("pending summary = %v", summary)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/records?status=pending", 1, nil)
	review := mustOKPage(t, w, out, "review list")
	if review["total"].(float64) != 1 {
		t.Fatalf("review total = %v", review["total"])
	}

	// 员工不能自己审核
	w, _ = doReq(r, http.MethodPost, "/api/worklog/review/approve", 2, map[string]interface{}{"ids": []int{recID}})
	if w.Code == http.StatusOK {
		t.Fatalf("worker should not approve records")
	}

	// 老板审核通过后计入工资
	w, out = doReq(r, http.MethodPost, "/api/worklog/review/approve", 1, map[string]interface{}{"ids": []int{recID}})
	mustOK(t, w, out, "approve record")
	w, out = doReq(r, http.MethodGet, "/api/worklog/stats/dashboard", 1, nil)
	dash = mustOK(t, w, out, "dashboard after approve")
	if dash["month_work_cents"].(float64) != 30000 || dash["month_record_count"].(float64) != 1 {
		t.Fatalf("approved record should count: %v", dash)
	}

	// 审核通过后结算与考勤才认这笔工
	w, out = doReq(r, http.MethodGet, "/api/worklog/settlements/preview?worker_id="+itoa(workerID)+"&start="+today+"&end="+today, 1, nil)
	preview = mustOK(t, w, out, "preview after approve")
	previewSummary = preview["summary"].(map[string]interface{})
	if previewSummary["work_amount_cents"].(float64) != 30000 || previewSummary["record_count"].(float64) != 1 {
		t.Fatalf("approved record should be settleable: %v", previewSummary)
	}
	w, out = doReq(r, http.MethodGet, fmt.Sprintf("/api/worklog/attendance?worker_id=%d&year=%d&month=%d", workerID, now.Year(), int(now.Month())), 1, nil)
	att = mustOK(t, w, out, "attendance after approve")
	if att["days"].(float64) != 1 {
		t.Fatalf("approved record should count in attendance: %v", att["days"])
	}

	// 已审核记录员工不能再改/删
	w, _ = doReq(r, http.MethodPut, "/api/worklog/records/"+itoa(recID), 2, map[string]interface{}{
		"worker_id": workerID, "date": today, "type": "day", "quantity": "1",
	})
	if w.Code == http.StatusOK {
		t.Fatalf("approved record should not be editable by worker")
	}

	// 再来一笔并驳回：不计工资且带驳回原因
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": workerID, "date": today, "type": "day", "quantity": "1",
	})
	rec2 := mustOK(t, w, out, "second worker record")
	rec2ID := int(rec2["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/review/reject", 1, map[string]interface{}{
		"ids": []int{rec2ID}, "note": "数量不对",
	})
	mustOK(t, w, out, "reject record")
	w, out = doReq(r, http.MethodGet, "/api/worklog/records?date="+today, 1, nil)
	day = mustOK(t, w, out, "records after reject")
	row = day["workers"].([]interface{})[0].(map[string]interface{})
	if row["rejected_count"].(float64) != 1 || row["record_count"].(float64) != 1 || row["amount_cents"].(float64) != 30000 {
		t.Fatalf("rejected row counters wrong: %v", row)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/records/list?status=rejected", 1, nil)
	list := mustOKPage(t, w, out, "rejected list")
	items := list["items"].([]interface{})
	if len(items) != 1 || items[0].(map[string]interface{})["review_note"] != "数量不对" {
		t.Fatalf("rejected note missing: %v", items)
	}

	// 关闭班组审核开关后，员工再提交直接生效（老班组默认关闭，保持原行为）
	w, out = doReq(r, http.MethodPut, "/api/worklog/teams/"+itoa(teamID), 1, map[string]interface{}{"review_required": false})
	mustOK(t, w, out, "disable review required")
	w, out = doReq(r, http.MethodPost, "/api/worklog/records", 2, map[string]interface{}{
		"worker_id": workerID, "date": today, "type": "day", "quantity": "1",
	})
	direct := mustOK(t, w, out, "worker record after review off")
	if direct["status"] != RecordStatusApproved {
		t.Fatalf("worker record after review off = %v, want approved", direct["status"])
	}
}

// mustOKPage 校验分页响应（SuccessPage 的 data 是 {items,total,page,pageSize}）。
func mustOKPage(t *testing.T, w *httptest.ResponseRecorder, out map[string]interface{}, ctx string) map[string]interface{} {
	t.Helper()
	if w.Code != http.StatusOK || out["code"].(float64) != 0 {
		t.Fatalf("%s: http=%d body=%s", ctx, w.Code, w.Body.String())
	}
	data, ok := out["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: data is not object: %v", ctx, out["data"])
	}
	return data
}
