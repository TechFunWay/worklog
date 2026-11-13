package worklog

import (
	"net/http"
	"testing"
	"time"
)

func TestPublicEntryCodeFlow(t *testing.T) {
	r, db := setupTest(t)
	now := time.Now()
	today := now.Format("2006-01-02")

	// 老板：工人 + 计件项目
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "张三"})
	worker := mustOK(t, w, out, "create worker")
	workerID := int(worker["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{
		"name": "打包装箱", "unit": "件", "unit_price_cents": 250,
	})
	piece := mustOK(t, w, out, "create piece item")
	pieceID := int(piece["id"].(float64))

	// 产品 + 工人双绑定：工人扫码后只剩数量一个输入框
	w, out = doReq(r, http.MethodPost, "/api/worklog/entry-codes", 1, map[string]interface{}{
		"name": "1号产线报工", "piece_item_id": pieceID, "worker_id": workerID,
	})
	code := mustOK(t, w, out, "create entry code")
	token, _ := code["token"].(string)
	if token == "" {
		t.Fatalf("entry code token missing")
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/public/entry/"+token, 0, nil)
	info := mustOK(t, w, out, "public entry info")
	prod, _ := info["product"].(map[string]interface{})
	if prod == nil || prod["name"] != "打包装箱" {
		t.Fatalf("public product wrong: %v", info["product"])
	}
	wk, _ := info["worker"].(map[string]interface{})
	if wk == nil || wk["name"] != "张三" {
		t.Fatalf("public worker wrong: %v", info["worker"])
	}

	// 匿名提交 → 待审核
	w, out = doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, map[string]interface{}{
		"quantity": "12",
	})
	sub := mustOK(t, w, out, "public submit")
	if sub["status"] != RecordStatusPending {
		t.Fatalf("public submit status = %v, want pending", sub["status"])
	}
	if sub["today_count"].(float64) != 1 {
		t.Fatalf("today count = %v", sub["today_count"])
	}
	recID := int(sub["record"].(map[string]interface{})["id"].(float64))
	// 公开响应不回显单价/金额：持有二维码的人不能反推工价。
	publicRecord := sub["record"].(map[string]interface{})
	if _, ok := publicRecord["unit_price_cents"]; ok {
		t.Fatalf("public submit leaked unit price: %v", publicRecord)
	}
	if _, ok := publicRecord["amount_cents"]; ok {
		t.Fatalf("public submit leaked amount: %v", publicRecord)
	}

	// 15 秒内重复提交按重复处理，不落第二条
	w, out = doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, map[string]interface{}{
		"quantity": "12",
	})
	sub2 := mustOK(t, w, out, "duplicate submit")
	if sub2["duplicate"] != true {
		t.Fatalf("second submit should be duplicate: %v", sub2)
	}
	var count int64
	db.Model(&WorkRecord{}).Count(&count)
	if count != 1 {
		t.Fatalf("duplicate submit created %d records", count)
	}

	// 校验收到的数量/单价/来源快照
	var rec WorkRecord
	if err := db.First(&rec, recID).Error; err != nil {
		t.Fatalf("load record: %v", err)
	}
	if rec.Quantity != "12" || rec.UnitPriceCents != 250 || rec.AmountCents != 3000 {
		t.Fatalf("record amount wrong: %+v", rec)
	}
	if rec.Source != RecordSourceQR || rec.EntryCodeName != "1号产线报工" || rec.SubmitterName != "张三" {
		t.Fatalf("record source snapshot wrong: %+v", rec)
	}

	// 班组长通过后计入工资
	w, out = doReq(r, http.MethodPost, "/api/worklog/review/approve", 1, map[string]interface{}{"ids": []int{recID}})
	mustOK(t, w, out, "approve public record")
	w, out = doReq(r, http.MethodGet, "/api/worklog/records/list?status=approved", 1, nil)
	page := mustOKPage(t, w, out, "approved list")
	if page["total"].(float64) != 1 {
		t.Fatalf("approved total = %v", page["total"])
	}

	// 非整数 / 超大数量被拒绝
	w, _ = doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, map[string]interface{}{"quantity": "1.5"})
	if w.Code == http.StatusOK {
		t.Fatalf("fractional piece quantity should fail")
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, map[string]interface{}{"quantity": "1000001"})
	if w.Code == http.StatusOK {
		t.Fatalf("oversized quantity should fail")
	}

	// 停用后公开页不可用
	w, out = doReq(r, http.MethodPut, "/api/worklog/entry-codes/"+itoa(int(code["id"].(float64))), 1, map[string]interface{}{"enabled": false})
	mustOK(t, w, out, "disable entry code")
	w, _ = doReq(r, http.MethodGet, "/api/worklog/public/entry/"+token, 0, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("disabled code should 404, got %d", w.Code)
	}

	_ = today
}

func TestBatchRecords(t *testing.T) {
	r, _ := setupTest(t)
	today := time.Now().Format("2006-01-02")
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "甲"})
	wa := mustOK(t, w, out, "worker a")
	w, out = doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "乙"})
	wb := mustOK(t, w, out, "worker b")
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{
		"name": "组装", "unit": "件", "unit_price_cents": 200,
	})
	piece := mustOK(t, w, out, "piece item")
	pieceID := int(piece["id"].(float64))
	aID := int(wa["id"].(float64))
	bID := int(wb["id"].(float64))

	w, out = doReq(r, http.MethodPost, "/api/worklog/records/batch", 1, map[string]interface{}{
		"date": today, "piece_item_id": pieceID,
		"rows": []map[string]interface{}{
			{"worker_id": aID, "quantity": "10"},
			{"worker_id": bID, "quantity": "20"},
		},
	})
	res := mustOK(t, w, out, "batch create")
	if res["created"].(float64) != 2 {
		t.Fatalf("batch created = %v", res["created"])
	}
	if res["amount_cents"].(float64) != 6000 {
		t.Fatalf("batch amount = %v", res["amount_cents"])
	}

	w, out = doReq(r, http.MethodGet, "/api/worklog/settlements/preview?worker_id="+itoa(aID)+"&start="+today+"&end="+today, 1, nil)
	preview := mustOK(t, w, out, "preview a")
	summary := preview["summary"].(map[string]interface{})
	if summary["work_amount_cents"].(float64) != 2000 {
		t.Fatalf("preview A amount = %v", summary["work_amount_cents"])
	}
	if len(preview["records"].([]interface{})) != 1 {
		t.Fatalf("preview A records = %v", preview["records"])
	}

	// 班组长（非老板）也能批量代录，但员工不能
	w, _ = doReq(r, http.MethodPost, "/api/worklog/records/batch", 2, map[string]interface{}{
		"date": today, "piece_item_id": pieceID,
		"rows": []map[string]interface{}{{"worker_id": aID, "quantity": "1"}},
	})
	if w.Code == http.StatusOK {
		t.Fatalf("plain worker should not batch record")
	}
}

func TestPublicEntryAutoApprove(t *testing.T) {
	r, _ := setupTest(t)
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "钱七"})
	worker := mustOK(t, w, out, "worker")
	wid := int(worker["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{
		"name": "贴标", "unit": "件", "unit_price_cents": 100,
	})
	piece := mustOK(t, w, out, "piece")
	pid := int(piece["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/entry-codes", 1, map[string]interface{}{
		"name": "自动通过码", "piece_item_id": pid, "worker_id": wid, "auto_approve": true,
	})
	code := mustOK(t, w, out, "code")
	token := code["token"].(string)
	w, out = doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, map[string]interface{}{"quantity": "5"})
	sub := mustOK(t, w, out, "auto submit")
	if sub["status"] != RecordStatusApproved {
		t.Fatalf("auto approve status = %v, want approved", sub["status"])
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/review/summary", 1, nil)
	summary := mustOK(t, w, out, "summary")
	if summary["pending_count"].(float64) != 0 {
		t.Fatalf("auto approved record should not be pending: %v", summary)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/records/list?status=approved", 1, nil)
	page := mustOKPage(t, w, out, "approved list")
	if page["total"].(float64) != 1 {
		t.Fatalf("auto approved total = %v", page["total"])
	}
}

func TestPublicEntryYesterdayFeedback(t *testing.T) {
	r, _ := setupTest(t)
	now := time.Now()
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "夜班工人"})
	worker := mustOK(t, w, out, "worker")
	wid := int(worker["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{
		"name": "夜班包装", "unit": "件", "unit_price_cents": 100,
	})
	piece := mustOK(t, w, out, "piece")
	pid := int(piece["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/entry-codes", 1, map[string]interface{}{
		"name": "夜班报工", "piece_item_id": pid, "worker_id": wid,
	})
	code := mustOK(t, w, out, "code")
	token := code["token"].(string)
	w, out = doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, map[string]interface{}{
		"quantity": "7", "date": yesterday,
	})
	sub := mustOK(t, w, out, "submit yesterday")
	if sub["date"] != yesterday {
		t.Fatalf("submit date = %v, want %s", sub["date"], yesterday)
	}
	if sub["today_count"].(float64) != 1 {
		t.Fatalf("yesterday count = %v, want 1", sub["today_count"])
	}
	items := sub["today_items"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("yesterday items = %v", items)
	}
	if items[0].(map[string]interface{})["quantity"] != "7" {
		t.Fatalf("yesterday item quantity = %v", items[0])
	}
}

func TestBatchRecordsRejectsInvalidRows(t *testing.T) {
	r, db := setupTest(t)
	today := time.Now().Format("2006-01-02")
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "甲"})
	a := mustOK(t, w, out, "worker a")
	w, out = doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "乙"})
	b := mustOK(t, w, out, "worker b")
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{"name": "组装", "unit": "件", "unit_price_cents": 200})
	piece := mustOK(t, w, out, "piece")
	aid := int(a["id"].(float64))
	bid := int(b["id"].(float64))
	pid := int(piece["id"].(float64))
	post := func(rows []map[string]interface{}) int {
		w, _ := doReq(r, http.MethodPost, "/api/worklog/records/batch", 1, map[string]interface{}{
			"date": today, "piece_item_id": pid, "rows": rows,
		})
		return w.Code
	}
	if code := post([]map[string]interface{}{{"worker_id": aid, "quantity": "10"}, {"worker_id": bid, "quantity": "1.5"}}); code != http.StatusBadRequest {
		t.Fatalf("fractional batch row should 400, got %d", code)
	}
	if code := post([]map[string]interface{}{{"worker_id": aid, "quantity": "10"}, {"worker_id": aid, "quantity": "20"}}); code != http.StatusBadRequest {
		t.Fatalf("duplicate worker batch row should 400, got %d", code)
	}
	if code := post([]map[string]interface{}{{"worker_id": 0, "quantity": "10"}}); code != http.StatusBadRequest {
		t.Fatalf("missing worker batch row should 400, got %d", code)
	}
	var count int64
	db.Model(&WorkRecord{}).Count(&count)
	if count != 0 {
		t.Fatalf("invalid batch should not create records, got %d", count)
	}
	w, out = doReq(r, http.MethodPost, "/api/worklog/records/batch", 1, map[string]interface{}{
		"date": today, "piece_item_id": pid, "rows": []map[string]interface{}{
			{"worker_id": aid, "quantity": "10"}, {"worker_id": bid, "quantity": "20"},
		},
	})
	res := mustOK(t, w, out, "valid batch")
	if res["created"].(float64) != 2 || res["total_quantity"] != "30" || res["amount_cents"].(float64) != 6000 {
		t.Fatalf("valid batch result = %v", res)
	}

	// 同一批 15 秒内重复提交（双击/网络重试）应整批拒绝，工资不能翻倍。
	w, out = doReq(r, http.MethodPost, "/api/worklog/records/batch", 1, map[string]interface{}{
		"date": today, "piece_item_id": pid, "rows": []map[string]interface{}{
			{"worker_id": aid, "quantity": "10"}, {"worker_id": bid, "quantity": "20"},
		},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("duplicate batch should 400, got %d body=%s", w.Code, w.Body.String())
	}
	if err := db.Model(&WorkRecord{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("duplicate batch created extra records, total=%d want 2", count)
	}
}

func TestReviewCannotTouchSettledOrDuplicate(t *testing.T) {
	r, _ := setupTest(t)
	today := time.Now().Format("2006-01-02")
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{"name": "张三"})
	worker := mustOK(t, w, out, "worker")
	wid := int(worker["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/piece-items", 1, map[string]interface{}{"name": "组装", "unit": "件", "unit_price_cents": 100})
	piece := mustOK(t, w, out, "piece")
	pid := int(piece["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/entry-codes", 1, map[string]interface{}{"name": "测试码", "piece_item_id": pid, "worker_id": wid})
	code := mustOK(t, w, out, "code")
	token := code["token"].(string)
	w, out = doReq(r, http.MethodPost, "/api/worklog/public/entry/"+token+"/records", 0, map[string]interface{}{"quantity": "5"})
	sub := mustOK(t, w, out, "submit")
	recID := int(sub["record"].(map[string]interface{})["id"].(float64))
	w, out = doReq(r, http.MethodPost, "/api/worklog/review/approve", 1, map[string]interface{}{"ids": []int{recID}})
	mustOK(t, w, out, "approve first")
	w, _ = doReq(r, http.MethodPost, "/api/worklog/review/approve", 1, map[string]interface{}{"ids": []int{recID}})
	if w.Code == http.StatusOK {
		t.Fatalf("second approve should fail")
	}
	w, out = doReq(r, http.MethodPost, "/api/worklog/settlements", 1, map[string]interface{}{
		"worker_id": wid, "start": today, "end": today,
	})
	mustOK(t, w, out, "settle")
	w, _ = doReq(r, http.MethodPost, "/api/worklog/review/reject", 1, map[string]interface{}{"ids": []int{recID}, "note": "试试"})
	if w.Code == http.StatusOK {
		t.Fatalf("settled record should not be rejected")
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/review/approve", 1, map[string]interface{}{"ids": []int{recID}})
	if w.Code == http.StatusOK {
		t.Fatalf("settled record should not be approved again")
	}
}
