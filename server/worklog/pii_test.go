package worklog

// pii_test.go — 工人敏感信息（身份证 / 银行卡）的加密、脱敏、权限与导出。
//
// 这三个字段都是选填；填了就必须：落库加密、接口只给掩码、完整值只给老板、
// 工资表导出要显式请求。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func piiWorker(t *testing.T, r *gin.Engine, uid int, body map[string]interface{}) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	return doReq(r, http.MethodPost, "/api/worklog/workers", uid, body)
}

// TestWorkerPIICreateAndMask 创建带敏感信息的工人：库里是密文，接口只回掩码。
func TestWorkerPIICreateAndMask(t *testing.T) {
	r, db := setupTest(t)
	w, out := piiWorker(t, r, 1, map[string]interface{}{
		"name": "张三", "worker_no": "W01", "daily_wage_cents": 30000,
		"id_card": "110101199003078888", "bank_card": "6222021234567890123", "bank_name": "工商银行",
	})
	data := mustOK(t, w, out, "create worker with pii")
	if data["worker_no"] != "W01" {
		t.Fatalf("工号未保存: %v", data["worker_no"])
	}
	if data["id_card_masked"] != "110101********8888" {
		t.Fatalf("身份证掩码错误: %v", data["id_card_masked"])
	}
	if data["bank_card_masked"] != "**** **** **** 0123" {
		t.Fatalf("银行卡掩码错误: %v", data["bank_card_masked"])
	}
	if data["id_card_set"] != true || data["bank_card_set"] != true {
		t.Fatal("已填标记未置位")
	}
	// 响应里绝不能出现明文
	if strings.Contains(w.Body.String(), "110101199003078888") || strings.Contains(w.Body.String(), "6222021234567890123") {
		t.Fatal("响应里泄露了明文敏感信息")
	}
	// 库里必须是密文
	var row Worker
	db.First(&row)
	if row.IDCardEnc == "" || row.BankCardEnc == "" {
		t.Fatal("敏感信息未加密落库")
	}
	if strings.Contains(row.IDCardEnc, "110101199003078888") || strings.Contains(row.BankCardEnc, "6222021234567890123") {
		t.Fatal("库里存的是明文")
	}
	// 解密应还原
	if p, err := piiDecrypt(db, row.BankCardEnc); err != nil || p != "6222021234567890123" {
		t.Fatalf("解密失败: %v %v", p, err)
	}
}

// TestWorkerPIIValidation 格式校验（选填，但填了要合法）。
func TestWorkerPIIValidation(t *testing.T) {
	r, _ := setupTest(t)
	if w, out := piiWorker(t, r, 1, map[string]interface{}{"name": "李四", "bank_card": "123"}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatal("过短银行卡号应被拒绝")
	}
	if w, out := piiWorker(t, r, 1, map[string]interface{}{"name": "李四", "id_card": "12345"}); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatal("非法身份证号应被拒绝")
	}
	// 不填也能建
	if w, out := piiWorker(t, r, 1, map[string]interface{}{"name": "王五"}); out["code"].(float64) != 0 {
		t.Fatalf("不填敏感信息应能创建: %s", w.Body.String())
	}
}

// TestWorkerPIIPermission 完整值只给老板；组长/员工一律 403。
func TestWorkerPIIPermission(t *testing.T) {
	r, db := setupTest(t)
	w, out := piiWorker(t, r, 1, map[string]interface{}{
		"name": "赵六", "id_card": "110101199003078888", "bank_card": "6222021234567890123",
	})
	wid := uint(mustOK(t, w, out, "create")["id"].(float64))

	// 老板能看明文
	w2, out2 := doReq(r, http.MethodGet, "/api/worklog/workers/"+itoa(int(wid))+"/pii", 1, nil)
	pii := mustOK(t, w2, out2, "boss read pii")
	if pii["id_card"] != "110101199003078888" || pii["bank_card"] != "6222021234567890123" {
		t.Fatalf("老板应能看到明文: %v", pii)
	}

	// 组长 403
	bindLeader(t, db, 31)
	if w3, out3 := doReq(r, http.MethodGet, "/api/worklog/workers/"+itoa(int(wid))+"/pii", 31, nil); w3.Code == http.StatusOK && out3["code"].(float64) == 0 {
		t.Fatalf("组长不该看到明文: %v", out3)
	}

	// 员工 403
	bindEmployee(t, db, wid, 32)
	if w4, out4 := doReq(r, http.MethodGet, "/api/worklog/workers/"+itoa(int(wid))+"/pii", 32, nil); w4.Code == http.StatusOK && out4["code"].(float64) == 0 {
		t.Fatalf("员工不该看到明文: %v", out4)
	}
}

// TestPayrollExportWithPII 工资表导出：显式 with_pii=1 才带明文卡号，且仅老板。
func TestPayrollExportWithPII(t *testing.T) {
	r, db := setupTest(t)
	w, out := piiWorker(t, r, 1, map[string]interface{}{
		"name": "钱七", "worker_no": "W09", "daily_wage_cents": 30000,
		"bank_card": "6222021234567890123", "bank_name": "建设银行",
	})
	wid := uint(mustOK(t, w, out, "create")["id"].(float64))
	v06Record(t, r, 1, map[string]interface{}{"worker_id": wid, "date": "2026-11-01", "type": "day", "quantity": "1"})
	v06Settle(t, r, map[string]interface{}{"worker_id": wid})

	// 普通导出：有工号与开户行，没有卡号明文
	res, _ := doReq(r, http.MethodGet, "/api/worklog/export/settlements.csv", 1, nil)
	body := res.Body.String()
	if !strings.Contains(body, "工号") || !strings.Contains(body, "W09") {
		t.Fatalf("普通导出应含工号: %s", body[:200])
	}
	if strings.Contains(body, "6222021234567890123") {
		t.Fatal("普通导出不应含银行卡明文")
	}

	// with_pii=1：老板可拿到明文
	res2, _ := doReq(r, http.MethodGet, "/api/worklog/export/settlements.csv?with_pii=1", 1, nil)
	body2 := res2.Body.String()
	if !strings.Contains(body2, "6222021234567890123") {
		t.Fatalf("工资表应含银行卡明文: %s", body2[:300])
	}

	// 组长带 with_pii=1 也拿不到
	bindLeader(t, db, 41)
	res3, _ := doReq(r, http.MethodGet, "/api/worklog/export/settlements.csv?with_pii=1", 41, nil)
	if strings.Contains(res3.Body.String(), "6222021234567890123") {
		t.Fatal("组长不该能导出银行卡明文")
	}
}

// TestWorkerPIIUpdateByBossOnly 组长编辑工人不会清空敏感信息。
func TestWorkerPIIUpdateByBossOnly(t *testing.T) {
	r, db := setupTest(t)
	w, out := piiWorker(t, r, 1, map[string]interface{}{
		"name": "孙八", "bank_card": "6222021234567890123", "daily_wage_cents": 30000,
	})
	wid := uint(mustOK(t, w, out, "create")["id"].(float64))

	bindLeader(t, db, 51)
	// 组长改薪资（不带敏感字段）
	if _, o := doReq(r, http.MethodPut, "/api/worklog/workers/"+itoa(int(wid)), 51, map[string]interface{}{
		"name": "孙八", "daily_wage_cents": 35000,
	}); o["code"].(float64) != 0 {
		t.Fatalf("组长改薪资失败: %v", o)
	}
	var row Worker
	db.First(&row, wid)
	if row.BankCardEnc == "" {
		t.Fatal("组长编辑不应清空银行卡")
	}
}
