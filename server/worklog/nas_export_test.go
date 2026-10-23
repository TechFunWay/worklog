// nas_export_test.go — 2026-10-23 第 3 轮粉丝反馈：考勤表导出 Excel/图片到 NAS。
//
// 覆盖：
//   - 考勤网格 /attendance/grid：月度/区间、按工地筛选只留该工地有记录的工人、
//     员工只看自己、合计口径（考勤天数、休息、金额）
//   - 本机 Excel 下载 /export/attendance.xlsx（xlsx 结构与工作表）
//   - NAS 导出流程：存 Excel、上传图片、列表、信息、下载、删除
//   - 权限：员工可用网格与本机 Excel，但 NAS 保存/列表/下载/删除仅老板/班组长
//   - 参数与导出目录校验、xlsx 文本转义
package worklog

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"smallgo/server/sysconfig"
)

// seedRecord 记一笔工并返回记录数据。
func seedRecord(t *testing.T, r *gin.Engine, uid int, workerID uint, date, typ, qty string, projectID uint) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{
		"worker_id": workerID, "date": date, "type": typ, "quantity": qty,
	}
	if projectID > 0 {
		body["project_id"] = projectID
	}
	w, out := doReq(r, http.MethodPost, "/api/worklog/records", uid, body)
	return mustOK(t, w, out, "create record "+date+" "+typ)
}

// postMultipartFile 以 multipart 上传一个文件（模拟浏览器存图片）。
func postMultipartFile(t *testing.T, r *gin.Engine, path string, uid int, fields map[string]string, fileName string, content []byte) (*httptest.ResponseRecorder, map[string]interface{}) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatalf("write field %s: %v", k, err)
		}
	}
	fw, err := mw.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if uid > 0 {
		req.Header.Set("X-Test-User", itoa(uid))
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		out = map[string]interface{}{}
	}
	return w, out
}

// readZipEntry 读 xlsx（zip）里的一个文件内容。
func readZipEntry(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open xlsx zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open zip entry %s: %v", name, err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read zip entry %s: %v", name, err)
		}
		return string(b)
	}
	t.Fatalf("zip entry %s not found", name)
	return ""
}

// testPNG 一张只有 PNG 文件头的最小图片（服务端只校验文件类型头，不解码）。
func testPNG() []byte {
	return append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0}, 64)...)
}

func TestAttendanceGridDataAndProjectFilter(t *testing.T) {
	r, db := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 30000, 3000, 0)
	li := priceWorker(t, r, 1, "李四", 20000, 0, 0)
	projA := priceProject(t, r, 1, "A 工地", 0, 0)
	projB := priceProject(t, r, 1, "B 工地", 0, 0)

	seedRecord(t, r, 1, zhang, "2026-09-01", "day", "1", projA)
	seedRecord(t, r, 1, zhang, "2026-09-01", "hour", "2", projA)
	seedRecord(t, r, 1, li, "2026-09-02", "rest", "0", 0)
	seedRecord(t, r, 1, li, "2026-09-03", "day", "1", projB)

	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/grid?year=2026&month=9", 1, nil)
	data := mustOK(t, w, out, "grid month")
	if data["mode"] != "month" || asInt64(data["year"]) != 2026 || asInt64(data["month"]) != 9 {
		t.Fatalf("grid mode/year/month = %v/%v/%v", data["mode"], data["year"], data["month"])
	}
	if data["project_name"] != "全部工地" {
		t.Fatalf("grid project_name = %v, want 全部工地", data["project_name"])
	}
	dates, _ := data["dates"].([]interface{})
	if len(dates) != 30 || dates[0] != "2026-09-01" || dates[29] != "2026-09-30" {
		t.Fatalf("grid dates = len %d, first %v, last %v", len(dates), dates[0], dates[len(dates)-1])
	}
	workers, _ := data["workers"].([]interface{})
	if len(workers) != 2 {
		t.Fatalf("grid workers len = %d, want 2", len(workers))
	}
	zrow := workers[0].(map[string]interface{})
	if zrow["worker_name"] != "张三" {
		t.Fatalf("workers[0] = %v, want 张三（按姓名排序）", zrow["worker_name"])
	}
	zcells, _ := zrow["cells"].([]interface{})
	if len(zcells) != 1 {
		t.Fatalf("zhang cells len = %d, want 1", len(zcells))
	}
	zcell := zcells[0].(map[string]interface{})
	if zcell["date"] != "2026-09-01" || asInt64(zcell["days"]) != 1 || asInt64(zcell["hours"]) != 2 ||
		asInt64(zcell["amount_cents"]) != 36000 || asInt64(zcell["record_count"]) != 2 {
		t.Fatalf("zhang cell = %v, want 2026-09-01 day1 hour2 amount36000", zcell)
	}
	ztot := zrow["totals"].(map[string]interface{})
	if asInt64(ztot["attend_days"]) != 1 || asInt64(ztot["days"]) != 1 || asInt64(ztot["hours"]) != 2 ||
		asInt64(ztot["amount_cents"]) != 36000 {
		t.Fatalf("zhang totals = %v", ztot)
	}

	lrow := workers[1].(map[string]interface{})
	if lrow["worker_name"] != "李四" {
		t.Fatalf("workers[1] = %v, want 李四", lrow["worker_name"])
	}
	lcells, _ := lrow["cells"].([]interface{})
	if len(lcells) != 2 {
		t.Fatalf("li cells len = %d, want 2", len(lcells))
	}
	ltot := lrow["totals"].(map[string]interface{})
	if asInt64(ltot["attend_days"]) != 1 || asInt64(ltot["rest_count"]) != 1 || asInt64(ltot["amount_cents"]) != 20000 {
		t.Fatalf("li totals = %v, want attend1 rest1 amount20000", ltot)
	}
	total := data["total"].(map[string]interface{})
	if asInt64(total["attend_days"]) != 2 || asInt64(total["rest_count"]) != 1 || asInt64(total["amount_cents"]) != 56000 {
		t.Fatalf("grid total = %v", total)
	}

	// 按工地筛选：只显示该工地有记录的人
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance/grid?year=2026&month=9&project_id="+itoa(int(projA)), 1, nil)
	data = mustOK(t, w, out, "grid project filter")
	if data["project_name"] != "A 工地" || len(data["workers"].([]interface{})) != 1 {
		t.Fatalf("project grid = %v workers=%v", data["project_name"], data["workers"])
	}
	if data["workers"].([]interface{})[0].(map[string]interface{})["worker_name"] != "张三" {
		t.Fatalf("project grid should only contain zhang")
	}

	// 未分工地：只有李四的休息记录
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance/grid?year=2026&month=9&project_id=none", 1, nil)
	data = mustOK(t, w, out, "grid no project")
	if data["project_name"] != "未分工地" || len(data["workers"].([]interface{})) != 1 {
		t.Fatalf("no-project grid = %v workers=%v", data["project_name"], data["workers"])
	}
	nrow := data["workers"].([]interface{})[0].(map[string]interface{})
	if nrow["worker_name"] != "李四" {
		t.Fatalf("no-project grid worker = %v", nrow["worker_name"])
	}
	if len(nrow["cells"].([]interface{})) != 1 || nrow["cells"].([]interface{})[0].(map[string]interface{})["rest"] != true {
		t.Fatalf("no-project grid cells = %v", nrow["cells"])
	}

	// 非法工地参数
	w, _ = doReq(r, http.MethodGet, "/api/worklog/attendance/grid?year=2026&month=9&project_id=abc", 1, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid project grid = %d, want 400", w.Code)
	}

	// 员工只能看到自己的网格
	bindEmployee(t, db, zhang, 7)
	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance/grid?year=2026&month=9", 7, nil)
	data = mustOK(t, w, out, "employee grid")
	employees := data["workers"].([]interface{})
	if len(employees) != 1 || employees[0].(map[string]interface{})["worker_name"] != "张三" {
		t.Fatalf("employee grid workers = %v, want only 张三", employees)
	}
}

func TestAttendanceGridRangeMode(t *testing.T) {
	r, _ := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 30000, 0, 0)
	seedRecord(t, r, 1, zhang, "2026-09-02", "day", "1", 0)

	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/grid?start=2026-09-01&end=2026-09-03", 1, nil)
	data := mustOK(t, w, out, "grid range")
	if data["mode"] != "range" || data["start"] != "2026-09-01" || data["end"] != "2026-09-03" {
		t.Fatalf("range grid mode/start/end = %v/%v/%v", data["mode"], data["start"], data["end"])
	}
	if dates := data["dates"].([]interface{}); len(dates) != 3 {
		t.Fatalf("range dates len = %d, want 3", len(dates))
	}
	if len(data["workers"].([]interface{})) != 1 {
		t.Fatalf("range workers = %v", data["workers"])
	}

	// 时间段超过 731 天
	w, _ = doReq(r, http.MethodGet, "/api/worklog/attendance/grid?start=2024-01-01&end=2026-01-02", 1, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("too-long range = %d, want 400", w.Code)
	}
}

func TestAttendanceXLSXDownload(t *testing.T) {
	r, db := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 30000, 0, 0)
	seedRecord(t, r, 1, zhang, "2026-09-01", "day", "1", 0)
	seedRecord(t, r, 1, zhang, "2026-09-02", "rest", "0", 0)

	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/attendance.xlsx?year=2026&month=9", 1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("xlsx download = %d, body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "spreadsheetml.sheet") {
		t.Fatalf("xlsx content-type = %q", ct)
	}
	body := w.Body.Bytes()
	if !bytes.HasPrefix(body, []byte("PK")) {
		t.Fatalf("xlsx should be a zip, head=%q", body[:min(len(body), 8)])
	}
	workbook := readZipEntry(t, body, "xl/workbook.xml")
	for _, want := range []string{"考勤表", "工资条", "逐日明细"} {
		if !strings.Contains(workbook, want) {
			t.Fatalf("workbook missing sheet %q: %s", want, workbook)
		}
	}
	sheet1 := readZipEntry(t, body, "xl/worksheets/sheet1.xml")
	if !strings.Contains(sheet1, "张三") || !strings.Contains(sheet1, "休") {
		t.Fatalf("attendance sheet missing worker/rest: %s", sheet1)
	}

	// 员工下载：只包含本人
	bindEmployee(t, db, zhang, 7)
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/attendance.xlsx?year=2026&month=9", 7, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("employee xlsx download = %d, body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(readZipEntry(t, w.Body.Bytes(), "xl/worksheets/sheet1.xml"), "张三") {
		t.Fatalf("employee xlsx should contain own name")
	}
}

func TestNasExportLifecycle(t *testing.T) {
	r, db := setupTest(t)
	dir := t.TempDir()
	if err := sysconfig.UpdateConfig(db, exportDirConfigKey, dir, 0); err != nil {
		t.Fatalf("set export dir: %v", err)
	}
	zhang := priceWorker(t, r, 1, "张三", 30000, 0, 0)
	seedRecord(t, r, 1, zhang, "2026-09-01", "day", "1", 0)

	// 存 Excel 到 NAS
	w, out := doReq(r, http.MethodPost, "/api/worklog/attendance/save-xlsx?year=2026&month=9", 1, nil)
	data := mustOK(t, w, out, "save xlsx")
	file, _ := data["file"].(string)
	if !strings.HasSuffix(file, ".xlsx") || data["dir"] != dir || data["kind"] != "xlsx" || asInt64(data["size"]) == 0 {
		t.Fatalf("save xlsx data = %v", data)
	}
	raw, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatalf("saved xlsx not found: %v", err)
	}
	if !bytes.HasPrefix(raw, []byte("PK")) {
		t.Fatalf("saved xlsx is not a zip")
	}

	// 列表 + 信息
	w, out = doReq(r, http.MethodGet, "/api/worklog/exports", 1, nil)
	list := mustOK(t, w, out, "exports list")
	if list["enabled"] != true || list["dir"] != dir {
		t.Fatalf("exports list enabled/dir = %v/%v", list["enabled"], list["dir"])
	}
	files, _ := list["files"].([]interface{})
	if len(files) != 1 {
		t.Fatalf("exports files len = %d, want 1", len(files))
	}
	first := files[0].(map[string]interface{})
	if first["name"] != file || first["kind"] != "xlsx" || !strings.Contains(first["url"].(string), "exports/download") {
		t.Fatalf("exports first = %v", first)
	}

	w, out = doReq(r, http.MethodGet, "/api/worklog/exports/info", 1, nil)
	info := mustOK(t, w, out, "exports info")
	if info["enabled"] != true || info["dir"] != dir || asInt64(info["file_count"]) != 1 {
		t.Fatalf("exports info = %v", info)
	}

	// 上传图片到 pictures 子目录
	png := testPNG()
	w, out = postMultipartFile(t, r, "/api/worklog/attendance/save-image", 1, map[string]string{
		"start": "2026-09-01", "end": "2026-09-30", "project_name": "A 工地",
	}, "考勤表.png", png)
	idata := mustOK(t, w, out, "save image")
	imgFile, _ := idata["file"].(string)
	if !strings.HasSuffix(imgFile, ".png") || idata["kind"] != "image" || idata["dir"] != filepath.Join(dir, picturesSubdir) {
		t.Fatalf("save image data = %v", idata)
	}
	if _, err := os.Stat(filepath.Join(dir, picturesSubdir, imgFile)); err != nil {
		t.Fatalf("saved image not found: %v", err)
	}

	// 列表里图片名带 pictures/ 前缀
	w, out = doReq(r, http.MethodGet, "/api/worklog/exports", 1, nil)
	list = mustOK(t, w, out, "exports list with image")
	files = list["files"].([]interface{})
	if len(files) != 2 {
		t.Fatalf("exports files len = %d, want 2", len(files))
	}
	foundImage := false
	for _, item := range files {
		if item.(map[string]interface{})["name"] == picturesSubdir+"/"+imgFile {
			foundImage = true
		}
	}
	if !foundImage {
		t.Fatalf("image not listed with pictures/ prefix: %v", files)
	}

	// 下载 Excel 与原文件一致
	w, _ = doReq(r, http.MethodGet, "/api/worklog/exports/download?name="+url.QueryEscape(file), 1, nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), raw) {
		t.Fatalf("download xlsx = %d, size=%d want %d", w.Code, w.Body.Len(), len(raw))
	}
	// 下载图片
	w, _ = doReq(r, http.MethodGet, "/api/worklog/exports/download?name="+url.QueryEscape(picturesSubdir+"/"+imgFile), 1, nil)
	if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), png) {
		t.Fatalf("download image = %d", w.Code)
	}

	// 删除 Excel
	w, out = doReq(r, http.MethodPost, "/api/worklog/exports/remove", 1, map[string]interface{}{"name": file})
	rem := mustOK(t, w, out, "remove export")
	if len(rem["removed"].([]interface{})) != 1 || len(rem["failed"].([]interface{})) != 0 {
		t.Fatalf("remove export = %v", rem)
	}
	if _, err := os.Stat(filepath.Join(dir, file)); !os.IsNotExist(err) {
		t.Fatalf("removed xlsx still exists: %v", err)
	}

	// 员工：不允许 NAS 保存/信息/下载/删除，列表返回空
	bindEmployee(t, db, zhang, 8)
	w, _ = doReq(r, http.MethodPost, "/api/worklog/attendance/save-xlsx?year=2026&month=9", 8, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee save xlsx = %d, want 403", w.Code)
	}
	w, out = doReq(r, http.MethodGet, "/api/worklog/exports", 8, nil)
	list = mustOK(t, w, out, "employee exports list")
	if list["enabled"] != false || len(list["files"].([]interface{})) != 0 {
		t.Fatalf("employee exports list = %v", list)
	}
	w, _ = doReq(r, http.MethodGet, "/api/worklog/exports/info", 8, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee exports info = %d, want 403", w.Code)
	}
	w, _ = doReq(r, http.MethodGet, "/api/worklog/exports/download?name="+url.QueryEscape(picturesSubdir+"/"+imgFile), 8, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee download = %d, want 403", w.Code)
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/exports/remove", 8, map[string]interface{}{"name": imgFile})
	if w.Code != http.StatusForbidden {
		t.Fatalf("employee remove = %d, want 403", w.Code)
	}
}

func TestNasExportValidation(t *testing.T) {
	r, db := setupTest(t)
	zhang := priceWorker(t, r, 1, "张三", 30000, 0, 0)
	seedRecord(t, r, 1, zhang, "2026-09-01", "day", "1", 0)

	// 时间段起止颠倒
	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/attendance.xlsx?start=2026-10-10&end=2026-10-01", 1, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad range = %d, want 400", w.Code)
	}
	// 非法工地
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/attendance.xlsx?year=2026&month=9&project_id=abc", 1, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad project = %d, want 400", w.Code)
	}

	// 相对路径的导出目录：信息接口报错、保存失败
	if err := sysconfig.UpdateConfig(db, exportDirConfigKey, "relative/dir", 0); err != nil {
		t.Fatalf("set relative export dir: %v", err)
	}
	w, out := doReq(r, http.MethodGet, "/api/worklog/exports/info", 1, nil)
	info := mustOK(t, w, out, "info with relative dir")
	errText, _ := info["error"].(string)
	if !strings.Contains(errText, "绝对") {
		t.Fatalf("info error = %q, want 绝对路径提示", errText)
	}
	w, _ = doReq(r, http.MethodPost, "/api/worklog/attendance/save-xlsx?year=2026&month=9", 1, nil)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("save with relative dir = %d, want 500", w.Code)
	}

	// 非图片内容
	if err := sysconfig.UpdateConfig(db, exportDirConfigKey, t.TempDir(), 0); err != nil {
		t.Fatalf("reset export dir: %v", err)
	}
	w, _ = postMultipartFile(t, r, "/api/worklog/attendance/save-image", 1, nil, "not-image.txt", []byte("hello"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("non-image upload = %d, want 400", w.Code)
	}

	// 删除文件名不允许路径穿越
	w, out = doReq(r, http.MethodPost, "/api/worklog/exports/remove", 1, map[string]interface{}{"name": "../evil"})
	rem := mustOK(t, w, out, "remove traversal")
	if len(rem["removed"].([]interface{})) != 0 || len(rem["failed"].([]interface{})) != 1 {
		t.Fatalf("remove traversal = %v", rem)
	}
}

func TestBuildXLSXEscapesText(t *testing.T) {
	data, err := buildXLSX([]xlsxSheet{{
		Name:       "测试/表",
		Cols:       []float64{12, 10},
		Merges:     []string{"A1:B1"},
		FreezeRows: 1,
		FreezeCols: 1,
		Rows: [][]xlsxCell{
			{xlsxText("a<b&c", xlsxStyleBody), xlsxNum(12.5, xlsxStyleMoney)},
			{xlsxText("ctrl\x01char", xlsxStyleDefault)},
		},
	}})
	if err != nil {
		t.Fatalf("buildXLSX: %v", err)
	}
	workbook := readZipEntry(t, data, "xl/workbook.xml")
	if !strings.Contains(workbook, "测试表") || strings.Contains(workbook, "测试/表") {
		t.Fatalf("sheet name not sanitized: %s", workbook)
	}
	sheet := readZipEntry(t, data, "xl/worksheets/sheet1.xml")
	if !strings.Contains(sheet, "a&lt;b&amp;c") {
		t.Fatalf("xlsx text not escaped: %s", sheet)
	}
	if !strings.Contains(sheet, "<v>12.5</v>") {
		t.Fatalf("xlsx number cell missing: %s", sheet)
	}
	if !strings.Contains(sheet, "ctrlchar") || strings.ContainsRune(sheet, '\x01') {
		t.Fatalf("xlsx control char not stripped: %s", sheet)
	}
}
