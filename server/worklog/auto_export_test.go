package worklog

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFSSandbox 目录浏览只在允许根内，越界一律拒绝（防路径穿越）。
func TestFSSandbox(t *testing.T) {
	r, db := setupTest(t)
	// 根列表
	w, out := doReq(r, http.MethodGet, "/api/worklog/fs/list", 1, nil)
	data := mustOK(t, w, out, "fs roots")
	roots, _ := data["roots"].([]interface{})
	if len(roots) == 0 {
		t.Fatal("应至少有一个允许根目录")
	}
	root := roots[0].(string)

	// 越界：/etc 不在允许根内
	if w, out := doReq(r, http.MethodGet, "/api/worklog/fs/list?path=/etc", 1, nil); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatal("/etc 不该可浏览")
	}
	// 穿越：root/../../etc
	if w, out := doReq(r, http.MethodGet, "/api/worklog/fs/list?path="+root+"/../../../../etc", 1, nil); w.Code == http.StatusOK && out["code"].(float64) == 0 {
		t.Fatal(".. 穿越不该成功")
	}
	// 合法：在根下建子目录再列出来
	if w, out := doReq(r, http.MethodPost, "/api/worklog/fs/mkdir", 1, map[string]interface{}{"path": root, "name": "工资表"}); out["code"].(float64) != 0 {
		t.Fatalf("建目录失败: %s", w.Body.String())
	}
	w2, out2 := doReq(r, http.MethodGet, "/api/worklog/fs/list?path="+root, 1, nil)
	d2 := mustOK(t, w2, out2, "list root")
	found := false
	for _, d := range d2["dirs"].([]interface{}) {
		if d.(map[string]interface{})["name"] == "工资表" {
			found = true
		}
	}
	if !found {
		t.Fatal("新建目录没出现在列表里")
	}
	// 组长无权限
	bindLeader(t, db, 61)
	if w3, out3 := doReq(r, http.MethodGet, "/api/worklog/fs/list", 61, nil); w3.Code == http.StatusOK && out3["code"].(float64) == 0 {
		t.Fatal("组长不该能浏览目录")
	}
}

// TestAutoExportRun 手动触发定时导出的那次生成（考勤 + 结算两种）。
func TestAutoExportRun(t *testing.T) {
	r, db := setupTest(t)
	wk := v06Worker(t, r, "张三", 30000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": wk, "date": "2026-11-02", "type": "day", "quantity": "1"})
	v06Settle(t, r, map[string]interface{}{"worker_id": wk})

	w, out := doReq(r, http.MethodPost, "/api/worklog/auto-export/run?scope=both", 1, nil)
	data := mustOK(t, w, out, "auto export run")
	files, _ := data["files"].([]interface{})
	if len(files) != 2 {
		t.Fatalf("both 应生成 2 个文件，实际 %d: %v", len(files), files)
	}
	for _, f := range files {
		p := f.(string)
		if !strings.Contains(p, "auto") {
			t.Fatalf("文件应落在 auto 子目录: %s", p)
		}
		st, err := os.Stat(p)
		if err != nil || st.Size() == 0 {
			t.Fatalf("文件未生成: %s %v", p, err)
		}
	}
	// 结算那份应该是合法的 xlsx（zip 头 PK）
	for _, f := range files {
		p := f.(string)
		if strings.Contains(p, "结算") {
			b, _ := os.ReadFile(p)
			if len(b) < 4 || string(b[:2]) != "PK" {
				t.Fatal("结算 Excel 不是合法 xlsx")
			}
		}
	}
	_ = filepath.Dir(files[0].(string))
	_ = db
}

// TestAutoExportScope 只导考勤时不出结算文件。
func TestAutoExportScope(t *testing.T) {
	r, _ := setupTest(t)
	wk := v06Worker(t, r, "李四", 30000, 0)
	v06Record(t, r, 1, map[string]interface{}{"worker_id": wk, "date": "2026-11-03", "type": "day", "quantity": "1"})
	w, out := doReq(r, http.MethodPost, "/api/worklog/auto-export/run?scope=attendance", 1, nil)
	data := mustOK(t, w, out, "auto export attendance")
	files := data["files"].([]interface{})
	if len(files) != 1 || !strings.Contains(files[0].(string), "考勤") {
		t.Fatalf("只导考勤应为 1 个考勤文件: %v", files)
	}
}
