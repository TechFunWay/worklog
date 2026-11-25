// subsidy_stats_test.go — v0.6.0「统计与导出跟随改造」的回归用例。
//
// 覆盖四件事（口径出处：docs/06-结算分期借支工地补贴需求评估.md）：
//  1. §2.4-A 现存真 bug：statsByProject 的 advance_cents 从来没被任何查询赋值、恒为 0，
//     「按工地统计」净额虚高。现在借支有了 project_id，按 GROUP BY project_id 补上；
//     未指定工地（project_id IS NULL）的散借支不进任何一行（含「未分工地」行），
//     与「散借支只在按人结算时抵扣」的结算口径一致，否则就是重复扣钱。
//  2. 看板待付口径：due_cents / due_count（已结算未付清）与 unsettled_cents（未结算记工）
//     是两套钱，互不污染；员工只看自己。
//  3. 导出列一律追加在末尾（记工 +工钱/补贴/扣款、借支 +工地、结算 +调整/已付/尚欠/付款状态），
//     付款状态出中文，负数调整项不能被公式注入防护转义成文本，工地名里的 = 前缀仍要挡。
//  4. JSON 备份必须带 payments（结算单付款流水），否则导出再恢复会丢流水。
//
// 造数一律直接落库（不依赖 lead 并行改动的记工/结算写入口），
// 断言只打在读侧，避免把别人的 WIP 变成这里的红灯。
package worklog

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// ---- 造数辅助（sub 前缀，避免与其它测试文件的 helper 撞名）----

func subProject(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()
	p := Project{UserID: 1, Name: name, Status: "active"}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("造工地 %s: %v", name, err)
	}
	return p.ID
}

func subWorker(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()
	w := Worker{UserID: 1, Name: name, Status: "active"}
	if err := db.Create(&w).Error; err != nil {
		t.Fatalf("造工人 %s: %v", name, err)
	}
	return w.ID
}

// subRecord 落一笔已审核记工。wage/subsidy/deduct 由调用方给，
// amount 按不变式 amount = wage + subsidy − deduct 现算（与 amount.go 同一条式子）。
func subRecord(t *testing.T, db *gorm.DB, workerID uint, projectID *uint, date string, wage, subsidy, deduct int64) {
	t.Helper()
	rec := WorkRecord{
		UserID: 1, WorkerID: workerID, ProjectID: projectID, Date: date,
		Type: TypeDay, Quantity: "1", UnitPriceCents: wage,
		WageCents: wage, SubsidyCents: subsidy, DeductCents: deduct,
		AmountCents: wage + subsidy - deduct, Status: RecordStatusApproved,
	}
	if err := db.Create(&rec).Error; err != nil {
		t.Fatalf("造记工 %s: %v", date, err)
	}
}

func subAdvance(t *testing.T, db *gorm.DB, workerID uint, projectID *uint, date string, cents int64) {
	t.Helper()
	a := Advance{UserID: 1, WorkerID: workerID, ProjectID: projectID, Date: date, AmountCents: cents, Method: "cash"}
	if err := db.Create(&a).Error; err != nil {
		t.Fatalf("造借支 %s: %v", date, err)
	}
}

// subSettlement 落一张结算单（不走 settlement 接口，那条链路由 lead 并行改造中）。
// 数据集归属跟着工人档案走，跨用户隔离的用例才是真的跨用户。
func subSettlement(t *testing.T, db *gorm.DB, workerID, projectID *uint, payable, paid, workCents int64) *Settlement {
	t.Helper()
	var w Worker
	if err := db.First(&w, *workerID).Error; err != nil {
		t.Fatalf("取结算单对应的工人档案失败: %v", err)
	}
	s := Settlement{
		UserID: w.UserID, WorkerID: *workerID, WorkerName: w.Name,
		PeriodStart: "2026-10-01", PeriodEnd: "2026-10-31",
		WorkAmountCents: workCents, PayableCents: payable, PaidCents: paid,
		PayStatus: payStatusOf(payable, paid), SettledAt: time.Now(),
	}
	if projectID != nil {
		s.ProjectID = projectID
	}
	if err := db.Create(&s).Error; err != nil {
		t.Fatalf("造结算单: %v", err)
	}
	return &s
}

// subBindEmployee 把 memberUID 绑成 ownerUID 数据集下 workerID 的员工账号。
func subBindEmployee(t *testing.T, db *gorm.DB, ownerUID, workerID, memberUID uint) {
	t.Helper()
	team := Team{Name: "补贴回归班组", OwnerUserID: ownerUID, InviteCode: "sub" + itoa(int(memberUID))}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("造班组: %v", err)
	}
	wid := workerID
	if err := db.Create(&TeamMember{TeamID: team.ID, UserID: memberUID, Role: RoleWorker, WorkerID: &wid, Status: "active"}).Error; err != nil {
		t.Fatalf("造成员: %v", err)
	}
}

func subRowByName(t *testing.T, rows []interface{}, name string) map[string]interface{} {
	t.Helper()
	for _, raw := range rows {
		row := raw.(map[string]interface{})
		if row["project_name"] == name {
			return row
		}
	}
	t.Fatalf("找不到名为 %q 的行：%v", name, rows)
	return nil
}

// subCSVLines 去掉 UTF-8 BOM 后按行拆开（WriteCSV 带 BOM 是给 Excel 认编码用的）。
func subCSVLines(body string) []string {
	return strings.Split(strings.TrimPrefix(body, "\ufeff"), "\n")
}

func subZipEntry(t *testing.T, data []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("打开 xlsx zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("打开 %s: %v", name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("读 %s: %v", name, err)
		}
		return string(b)
	}
	t.Fatalf("xlsx 里没有 %s", name)
	return ""
}

// ---- 1. 按工地统计：借支终于进账了（§2.4-A）----

// TestStatsByProjectAdvanceCents 按工地统计的 advance_cents 必须等于该工地指定借支之和，
// 且未指定工地的借支一分钱都不许进任何一行（包括「未分工地」行）。
func TestStatsByProjectAdvanceCents(t *testing.T) {
	r, db := setupTest(t)
	wid := subWorker(t, db, "张三")
	pidA := subProject(t, db, "A 工地")
	pidB := subProject(t, db, "B 工地")
	pidC := subProject(t, db, "C 工地")

	// A：工钱 300 + 补贴 50 − 扣款 20 = 330 元
	subRecord(t, db, wid, &pidA, "2026-10-01", 30000, 5000, 2000)
	// B：200 元
	subRecord(t, db, wid, &pidB, "2026-10-02", 20000, 0, 0)
	// 未分工地：100 元
	subRecord(t, db, wid, nil, "2026-10-03", 10000, 0, 0)

	// 借支：A 80 元、B 30 元、未指定工地 999 元、只有借支没有记工的 C 地 10 元
	subAdvance(t, db, wid, &pidA, "2026-10-05", 8000)
	subAdvance(t, db, wid, &pidB, "2026-10-05", 3000)
	subAdvance(t, db, wid, nil, "2026-10-05", 99900)
	subAdvance(t, db, wid, &pidC, "2026-10-06", 1000)

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 1, nil)
	data := mustOK(t, w, out, "按工地统计")
	rows := data["rows"].([]interface{})

	rowA := subRowByName(t, rows, "A 工地")
	if got := subInt(rowA["work_amount_cents"]); got != 33000 {
		t.Fatalf("A 地 work_amount_cents = %d, want 33000（应得合计已含补贴已减扣款）", got)
	}
	if got := subInt(rowA["subsidy_cents"]); got != 5000 {
		t.Fatalf("A 地 subsidy_cents = %d, want 5000", got)
	}
	if got := subInt(rowA["deduct_cents"]); got != 2000 {
		t.Fatalf("A 地 deduct_cents = %d, want 2000", got)
	}
	if got := subInt(rowA["advance_cents"]); got != 8000 {
		t.Fatalf("A 地 advance_cents = %d, want 8000（§2.4-A 的 bug：这里以前恒为 0）", got)
	}
	if got := subInt(rowA["payable_cents"]); got != 25000 {
		t.Fatalf("A 地 payable_cents = %d, want 25000（330 − 80）", got)
	}

	rowB := subRowByName(t, rows, "B 工地")
	if got := subInt(rowB["advance_cents"]); got != 3000 {
		t.Fatalf("B 地 advance_cents = %d, want 3000", got)
	}
	if got := subInt(rowB["payable_cents"]); got != 17000 {
		t.Fatalf("B 地 payable_cents = %d, want 17000", got)
	}

	// 未分工地的记工行不许被塞进散借支（散借支归按人结算抵扣）。
	rowNone := subRowByName(t, rows, "未分工地")
	if got := subInt(rowNone["advance_cents"]); got != 0 {
		t.Fatalf("未分工地 advance_cents = %d, want 0（未指定工地的借支不进任何工地行）", got)
	}
	if got := subInt(rowNone["payable_cents"]); got != 10000 {
		t.Fatalf("未分工地 payable_cents = %d, want 10000", got)
	}

	// 只有借支、本月没记工的工地也要出行，钱不能凭空消失。
	rowC := subRowByName(t, rows, "C 工地")
	if got := subInt(rowC["advance_cents"]); got != 1000 {
		t.Fatalf("C 地 advance_cents = %d, want 1000", got)
	}
	if got := subInt(rowC["payable_cents"]); got != -1000 {
		t.Fatalf("C 地 payable_cents = %d, want -1000（只借出钱、本月没工，如实为负）", got)
	}

	total := data["total"].(map[string]interface{})
	if got := subInt(total["work_amount_cents"]); got != 63000 {
		t.Fatalf("合计 work_amount_cents = %d, want 63000", got)
	}
	if got := subInt(total["advance_cents"]); got != 12000 {
		t.Fatalf("合计 advance_cents = %d, want 12000（80+30+10，999 元散借支不算）", got)
	}
	if got := subInt(total["payable_cents"]); got != 51000 {
		t.Fatalf("合计 payable_cents = %d, want 51000", got)
	}
}

func subInt(v interface{}) int64 {
	if f, ok := v.(float64); ok {
		return int64(f)
	}
	if n, ok := v.(int64); ok {
		return n
	}
	return -1 << 62
}

// ---- 2. 看板待付口径 ----

// TestStatsDashboardDueCents due_* 是「已结算还没付够」的钱，
// 与 unsettled_*（还没出工资条的记工）分文不相干，两边互不污染。
func TestStatsDashboardDueCents(t *testing.T) {
	r, db := setupTest(t)
	wid := subWorker(t, db, "张三")
	other := subWorker(t, db, "李四")

	// 张三：1000 元只付 400（欠 600）
	subSettlement(t, db, &wid, nil, 100000, 40000, 100000)
	// 张三：500 元付满（不欠）
	subSettlement(t, db, &wid, nil, 50000, 50000, 50000)
	// 李四：200 元分文未付（欠 200）
	subSettlement(t, db, &other, nil, 20000, 0, 20000)
	// 李四：多付 50（300 应付给了 350），不倒过来冲抵别的单
	subSettlement(t, db, &other, nil, 30000, 35000, 30000)

	// 还有一条没进任何工资条的记工（含补贴），只该出现在 unsettled 里。
	subRecord(t, db, wid, nil, "2026-10-20", 70000, 77700, 0) // 1477 元未结算

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/dashboard", 1, nil)
	data := mustOK(t, w, out, "看板")
	if got := subInt(data["due_cents"]); got != 80000 {
		t.Fatalf("due_cents = %d, want 80000（600+200，多付的那单不算负欠薪）", got)
	}
	// due_count 只数「还欠着钱」的单：付满的那张和多付的那张都不计数。
	if got := subInt(data["due_count"]); got != 2 {
		t.Fatalf("due_count = %d, want 2（1000/400 与 200/0 两张，付满与多付的不计）", got)
	}
	// 未结算池只有那条 1477 元的记工：待付改造一分钱都没挪进这个口径。
	if got := subInt(data["unsettled_cents"]); got != 147700 {
		t.Fatalf("unsettled_cents = %d, want 147700（未结算记工含补贴，与待付无关）", got)
	}
	if got := subInt(data["unsettled_count"]); got != 1 {
		t.Fatalf("unsettled_count = %d, want 1", got)
	}
}

// TestStatsByProjectEmployeeScope 员工调「按工地统计」时，记工/借支/结算三条聚合
// 都只能算到自己头上 —— 这里同时钉住借支维度不许越权（班组里别人的借支不能出现）。
func TestStatsByProjectEmployeeScope(t *testing.T) {
	r, db := setupTest(t)
	zhang := subWorker(t, db, "张三")
	li := subWorker(t, db, "李四")
	pid := subProject(t, db, "共享工地")
	subBindEmployee(t, db, 1, zhang, 7)

	subRecord(t, db, zhang, &pid, "2026-10-01", 30000, 0, 0)
	subRecord(t, db, li, &pid, "2026-10-02", 90000, 0, 0)
	subAdvance(t, db, zhang, &pid, "2026-10-05", 1000)
	subAdvance(t, db, li, &pid, "2026-10-05", 80000)

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 7, nil)
	rows := mustOK(t, w, out, "员工按工地统计")["rows"].([]interface{})
	row := subRowByName(t, rows, "共享工地")
	if got := subInt(row["work_amount_cents"]); got != 30000 {
		t.Fatalf("员工看到 work_amount_cents = %d, want 30000（只能是自己那份工）", got)
	}
	if got := subInt(row["advance_cents"]); got != 1000 {
		t.Fatalf("员工看到 advance_cents = %d, want 1000（别人的借支不能串进来）", got)
	}

	// 老板视角仍然是全量。
	w, out = doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 1, nil)
	rows = mustOK(t, w, out, "老板按工地统计")["rows"].([]interface{})
	row = subRowByName(t, rows, "共享工地")
	if got := subInt(row["work_amount_cents"]); got != 120000 {
		t.Fatalf("老板 work_amount_cents = %d, want 120000", got)
	}
	if got := subInt(row["advance_cents"]); got != 81000 {
		t.Fatalf("老板 advance_cents = %d, want 81000", got)
	}
}

// TestStatsDashboardDueCentsEmployeeScope 员工看板只能看到自己那张单的欠薪。
func TestStatsDashboardDueCentsEmployeeScope(t *testing.T) {
	r, db := setupTest(t)
	zhang := subWorker(t, db, "张三")
	li := subWorker(t, db, "李四")
	subBindEmployee(t, db, 1, zhang, 7)

	subSettlement(t, db, &zhang, nil, 100000, 40000, 100000) // 张三欠 600
	subSettlement(t, db, &li, nil, 90000, 0, 90000)          // 李四欠 900

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/dashboard", 7, nil)
	data := mustOK(t, w, out, "员工看板")
	if got := subInt(data["due_cents"]); got != 60000 {
		t.Fatalf("员工 due_cents = %d, want 60000（只该看到自己的欠薪）", got)
	}
	if got := subInt(data["due_count"]); got != 1 {
		t.Fatalf("员工 due_count = %d, want 1", got)
	}
}

// ---- 3. 导出列 ----

// TestCSVExportColumns 三类 CSV 的新列一律在末尾，既有列序与列名一字不改，
// 付款状态出中文，负数调整项不被转义成文本，工地名里的公式前缀照旧挡掉。
func TestCSVExportColumns(t *testing.T) {
	r, db := setupTest(t)
	wid := subWorker(t, db, "张三")
	pid := subProject(t, db, "=HYPERLINK(恶意工地)")
	subRecord(t, db, wid, &pid, "2026-10-01", 30000, 5000, 2000)
	subAdvance(t, db, wid, &pid, "2026-10-05", 8000)
	s := subSettlement(t, db, &wid, nil, 89500, 40000, 100000)
	s.WorkerName = "张三"
	s.AdvanceAmountCents = 10000
	s.AdjustCents = -500
	s.AdjustNote = "抹零+迟到扣款"
	s.PayStatus = payStatusOf(s.PayableCents, s.PaidCents)
	if err := db.Save(s).Error; err != nil {
		t.Fatalf("改结算单: %v", err)
	}

	// 记工明细
	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/records.csv", 1, nil)
	recLines := subCSVLines(w.Body.String())
	if got, want := recLines[0], "日期,工人,类型,数量,单价(元),金额(元),工地,计件项目,状态,备注,工钱(元),补贴(元),扣款(元)"; got != want {
		t.Fatalf("记工 CSV 表头错位：\n got %q\nwant %q", got, want)
	}
	if len(recLines) < 2 || !strings.HasSuffix(recLines[1], ",300.00,50.00,20.00") {
		t.Fatalf("记工 CSV 数据行缺少 工钱/补贴/扣款 三列：%q", strings.Join(recLines, " | "))
	}
	// 恶意工地名仍然要过注入防护（新列同权）。
	if !strings.Contains(w.Body.String(), "'=HYPERLINK(恶意工地)") {
		t.Fatalf("新列里的公式前缀没被转义：%s", w.Body.String())
	}

	// 工资月报
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/monthly.csv?year=2026&month=10", 1, nil)
	if got, want := subCSVLines(w.Body.String())[0], "工人,日期,类型,数量,单价(元),金额(元),工地,计件项目,备注,工钱(元),补贴(元),扣款(元)"; got != want {
		t.Fatalf("月度 CSV 表头错位：\n got %q\nwant %q", got, want)
	}

	// 考勤表
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/attendance.csv?year=2026&month=10", 1, nil)
	if got, want := subCSVLines(w.Body.String())[0], "工人,日期,点工(天),点时(小时),计件(数量),休息,金额(元),备注,补贴(元),扣款(元)"; got != want {
		t.Fatalf("考勤 CSV 表头错位：\n got %q\nwant %q", got, want)
	}
	if !strings.Contains(w.Body.String(), "张三,2026-10-01,1,0,0,,330.00,,50.00,20.00") {
		t.Fatalf("考勤 CSV 数据行的补贴/扣款列不对：%s", w.Body.String())
	}

	// 借支
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/advances.csv", 1, nil)
	if got, want := subCSVLines(w.Body.String())[0], "日期,工人,金额(元),方式,状态,备注,工地"; got != want {
		t.Fatalf("借支 CSV 表头错位：\n got %q\nwant %q", got, want)
	}

	// 结算：付款状态中文；负数调整项不能被转义成 "'-5.00"
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/settlements.csv", 1, nil)
	lines := subCSVLines(w.Body.String())
	// 工号与开户行是 v0.6.1 新增的两列（在「工人」后与末尾）；敏感列（银行卡/身份证）默认不导出。
	if got, want := lines[0], "结算单号,工人,工号,期间起,期间止,应发(元),借支(元),实发(元),记工条数,结算时间,备注,调整项(元),调整原因,已付(元),尚欠(元),付款状态,开户行"; got != want {
		t.Fatalf("结算 CSV 表头错位：\n got %q\nwant %q", got, want)
	}
	if len(lines) < 2 {
		t.Fatalf("结算 CSV 没有数据行：%v", lines)
	}
	if !strings.Contains(lines[1], ",-5.00,抹零+迟到扣款,400.00,495.00,部分支付,") {
		t.Fatalf("结算 CSV 数据行的调整/已付/尚欠/付款状态不对：%q", lines[1])
	}
	if strings.Contains(lines[1], "'-5.00") {
		t.Fatalf("负数调整项被公式注入防护转义成文本，Excel 求和会漏掉这一列：%q", lines[1])
	}
}

// ---- 4. JSON 备份带付款流水 ----

// TestJSONExportIncludesPayments 备份少了 payments，换机恢复后结算单的已付金额
// 就成了无源之水，工资条从「已付清」倒退回「欠薪」。
func TestJSONExportIncludesPayments(t *testing.T) {
	r, db := setupTest(t)
	zhang := subWorker(t, db, "张三")
	li := subWorker(t, db, "李四")
	sZhang := subSettlement(t, db, &zhang, nil, 100000, 40000, 100000)
	sLi := subSettlement(t, db, &li, nil, 50000, 50000, 50000)

	pays := []SettlementPayment{
		{UserID: 1, SettlementID: sZhang.ID, WorkerID: zhang, AmountCents: 30000, Method: "wechat", PaidOn: "2026-11-01", Note: "第一笔"},
		{UserID: 1, SettlementID: sZhang.ID, WorkerID: zhang, AmountCents: 10000, Method: "cash", PaidOn: "2026-11-05", Note: "第二笔"},
		{UserID: 1, SettlementID: sLi.ID, WorkerID: li, AmountCents: 50000, Method: "bank", PaidOn: "2026-11-06", Note: "老李的"},
	}
	for i := range pays {
		if err := db.Create(&pays[i]).Error; err != nil {
			t.Fatalf("造付款流水: %v", err)
		}
	}

	// 老板视角：全量三笔
	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/json", 1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("export json http=%d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析备份 JSON: %v", err)
	}
	list, ok := payload["payments"].([]interface{})
	if !ok {
		t.Fatalf("备份里没有 payments 键：%v", payload)
	}
	if len(list) != 3 {
		t.Fatalf("payments 条数 = %d, want 3", len(list))
	}
	// 字段名与既有键风格一致（snake_case + *_cents）
	row := list[0].(map[string]interface{})
	for _, key := range []string{"id", "settlement_id", "worker_id", "amount_cents", "method", "paid_on", "note", "created_by", "created_at"} {
		if _, ok := row[key]; !ok {
			t.Fatalf("payments 缺字段 %q：%v", key, row)
		}
	}
	// 按 paid_on ASC, id ASC 排序，第一笔应该是张三 11-01 的 300 元
	if got := subInt(row["amount_cents"]); got != 30000 {
		t.Fatalf("payments[0].amount_cents = %d, want 30000", got)
	}

	// 员工视角：只有自己那两笔
	subBindEmployee(t, db, 1, zhang, 7)
	w, _ = doReq(r, http.MethodGet, "/api/worklog/export/json", 7, nil)
	payload = map[string]interface{}{}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("解析员工备份 JSON: %v", err)
	}
	list, _ = payload["payments"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("员工 payments 条数 = %d, want 2（只看自己的流水）", len(list))
	}
	for _, raw := range list {
		if got := subInt(raw.(map[string]interface{})["worker_id"]); got != int64(zhang) {
			t.Fatalf("员工备份里混进了别人的流水：worker_id = %d", got)
		}
	}
}

// ---- 5. Excel 拆项与工资条汇总行 ----

// TestAttendanceXLSXSubsidyColumns 逐日明细加补贴/扣款列，工资条加 调整/已付/尚欠 行。
func TestAttendanceXLSXSubsidyColumns(t *testing.T) {
	r, db := setupTest(t)
	wid := subWorker(t, db, "张三")
	pid := subProject(t, db, "A 工地")
	subRecord(t, db, wid, &pid, "2026-10-01", 30000, 5000, 2000)
	subSettlement(t, db, &wid, &pid, 33000, 13000, 33000)

	w, _ := doReq(r, http.MethodGet, "/api/worklog/export/attendance.xlsx?year=2026&month=10", 1, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("attendance.xlsx http=%d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.Bytes()

	detail := subZipEntry(t, body, "xl/worksheets/sheet3.xml")
	for _, want := range []string{"其中补贴(元)", "其中扣款(元)", "金额(元)", "备注"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("逐日明细缺列 %q：\n%s", want, detail)
		}
	}
	// 列序：备注在前、拆项列追加在末尾
	if strings.Index(detail, "备注") > strings.Index(detail, "其中补贴(元)") {
		t.Fatalf("逐日明细的补贴列插到了备注前面，打乱了既有列序")
	}

	payslip := subZipEntry(t, body, "xl/worksheets/sheet2.xml")
	for _, want := range []string{"其中补贴(元)", "其中扣款(元)", "调整项(元)", "已付(元)", "尚欠(元)"} {
		if !strings.Contains(payslip, want) {
			t.Fatalf("工资条缺 %q：\n%s", want, payslip)
		}
	}
	// 尚欠 = 应付 330 − 已付 130 = 200 元
	if !strings.Contains(payslip, "200") {
		t.Fatalf("工资条的尚欠应该是 200：\n%s", payslip)
	}

	// 考勤表主表列序不许变（老模板按列位取数）
	grid := subZipEntry(t, body, "xl/worksheets/sheet1.xml")
	if !strings.Contains(grid, "工钱(元)") {
		t.Fatalf("考勤表主表的工钱列标题被动过：\n%s", grid)
	}
}

// TestSettlementSummaryNoCrossUser 结算汇总只算本数据集的结算单（跨用户不串数据）。
func TestSettlementSummaryNoCrossUser(t *testing.T) {
	_, db := setupTest(t)
	wid := subWorker(t, db, "张三")
	other := Worker{UserID: 2, Name: "别人家的工人", Status: "active"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatalf("造别人的工人: %v", err)
	}
	subSettlement(t, db, &wid, nil, 100000, 40000, 100000) // 本数据集：欠 600
	subSettlement(t, db, &other.ID, nil, 99900, 0, 99900)  // 别人的数据集
	subSettlement(t, db, &wid, nil, 0, 0, 0)               // 全 0 的单（无工无钱）不计欠薪

	spec := &attendanceExportSpec{dsUID: 1, start: "2026-10-01", end: "2026-10-31"}
	got, err := settlementSummaryForPayslip(db, spec)
	if err != nil {
		t.Fatalf("结算汇总查询失败: %v", err)
	}
	if got.DueCents != 60000 {
		t.Fatalf("工资条尚欠合计 = %d, want 60000（不能把别的 user_id 的单算进来）", got.DueCents)
	}
	if got.Count != 2 {
		t.Fatalf("工资条命中结算单 = %d, want 2（本数据集两张都在期间内，别人家那张不算）", got.Count)
	}
}

// TestCSVNumericCell 数字放行判定：程序算出的金额（含负数）原样写，
// 用户录入的内容一律还得走公式注入防护。这是 numericCell 唯一的护栏测试 ——
// 放宽一分就等于把 CSV 注入面打开一分，改那个函数必须先过这里。
func TestCSVNumericCell(t *testing.T) {
	numeric := []string{"0.00", "-50.00", "300.00", "1", "12.5", "-0.01"}
	for _, s := range numeric {
		if !numericCell(s) {
			t.Errorf("numericCell(%q) 应为 true", s)
		}
		if got := csvCellSafe(s); got != s {
			t.Errorf("csvCellSafe(%q) = %q, want 原样写出", s, got)
		}
	}
	notNumeric := []string{"", "=1+1", "-HYPERLINK(x)", "@a", "1.2.3", "-", "+1", "1e5", "张三", "- 5", "--5"}
	for _, s := range notNumeric {
		if numericCell(s) {
			t.Errorf("numericCell(%q) 应为 false", s)
		}
	}
	// 以 - 或 = 开头的用户录入内容仍然必须被挡（数字放行不许削弱既有防护）。
	for _, evil := range []string{"-1+cmd|/c calc!A0", "=HYPERLINK(http://evil)", "@SUM(A1)", "+1"} {
		if got := csvCellSafe(evil); !strings.HasPrefix(got, "'") {
			t.Errorf("注入型内容没被转义：%q -> %q", evil, got)
		}
	}
}

// TestCSVPayStatusLabel 付款状态中文标签与老数据兜底。
func TestCSVPayStatusLabel(t *testing.T) {
	cases := []struct {
		status        string
		payable, paid int64
		want          string
	}{
		{PayStatusPaid, 100, 100, "已付清"},
		{PayStatusPartial, 100, 40, "部分支付"},
		{PayStatusUnpaid, 100, 0, "未付"},
		// 老结算单没有 pay_status：按金额兜底，既不能漏字面量英文，也不能凭空报「未付」。
		{"", 100, 100, "已付清"},
		{"", 100, 0, "未付"},
		{"", 100, 40, "部分支付"},
		{"", -100, 0, "已付清"}, // 借支倒挂的负应付不算欠薪，与 payStatusOf 一致
		{"bogus", 50, 50, "已付清"},
	}
	for _, c := range cases {
		if got := csvPayStatusLabel(c.status, c.payable, c.paid); got != c.want {
			t.Errorf("csvPayStatusLabel(%q,%d,%d) = %q, want %q", c.status, c.payable, c.paid, got, c.want)
		}
	}
}

// TestAttendanceGridSubsidyNotDoubleCounted 考勤网格按类型拆的金额（Day/Hour/Piece）
// 已经把补贴算在各自桶里，横向的 SubsidyCents 只是拆项，绝不能再加进合计。
func TestAttendanceGridSubsidyNotDoubleCounted(t *testing.T) {
	r, db := setupTest(t)
	wid := subWorker(t, db, "孙七")
	subRecord(t, db, wid, nil, "2026-10-01", 30000, 5000, 0)
	// 点时 8 小时、单价 20 元：工钱 160 + 补贴 20 − 扣款 10 = 170 元
	hourRec := WorkRecord{UserID: 1, WorkerID: wid, Date: "2026-10-02", Type: TypeHour, Quantity: "8",
		UnitPriceCents: 2000, WageCents: 16000, SubsidyCents: 2000, DeductCents: 1000,
		AmountCents: 17000, Status: RecordStatusApproved}
	if err := db.Create(&hourRec).Error; err != nil {
		t.Fatalf("造点时记工: %v", err)
	}

	w, out := doReq(r, http.MethodGet, "/api/worklog/attendance/grid?start=2026-10-01&end=2026-10-02", 1, nil)
	tt := mustOK(t, w, out, "考勤网格")["total"].(map[string]interface{})
	amount := subInt(tt["amount_cents"])
	if amount != 52000 {
		t.Fatalf("total.amount_cents = %d, want 52000（350 + 170，合计只认 amount_cents）", amount)
	}
	// 三个桶都是 omitempty，为 0 的桶不会出现在 JSON 里，缺失按 0 处理。
	var split int64
	for _, k := range []string{"day_amount_cents", "hour_amount_cents", "piece_amount_cents"} {
		if v, ok := tt[k]; ok {
			split += subInt(v)
		}
	}
	if split != amount {
		t.Fatalf("Day+Hour+Piece = %d 必须等于 amount_cents = %d（补贴只落在自己类型的桶里，不进第二次）", split, amount)
	}
	if got := subInt(tt["subsidy_cents"]); got != 7000 {
		t.Fatalf("total.subsidy_cents = %d, want 7000（5000+2000，拆项不参与重算）", got)
	}
	if got := subInt(tt["deduct_cents"]); got != 1000 {
		t.Fatalf("total.deduct_cents = %d, want 1000", got)
	}
}

// TestStatsRangeAndMonthlySubsidy 带明细的区间统计、月度统计与月度考勤日历
// 都要能把补贴/扣款拆出来（合计仍等于 amount_cents）。
func TestStatsRangeAndMonthlySubsidy(t *testing.T) {
	r, db := setupTest(t)
	wid := subWorker(t, db, "周八")
	subRecord(t, db, wid, nil, "2026-10-01", 30000, 6000, 1500)

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/range?start=2026-10-01&end=2026-10-31", 1, nil)
	data := mustOK(t, w, out, "区间统计")
	if got := subInt(data["work_amount_cents"]); got != 34500 {
		t.Fatalf("work_amount_cents = %d, want 34500", got)
	}
	if got := subInt(data["subsidy_cents"]); got != 6000 {
		t.Fatalf("区间 subsidy_cents = %d, want 6000", got)
	}
	if got := subInt(data["deduct_cents"]); got != 1500 {
		t.Fatalf("区间 deduct_cents = %d, want 1500", got)
	}

	w, out = doReq(r, http.MethodGet, "/api/worklog/stats/monthly?year=2026&month=10", 1, nil)
	rows := mustOK(t, w, out, "月度统计")["rows"].([]interface{})
	row := rows[0].(map[string]interface{})
	if got := subInt(row["work_amount_cents"]); got != 34500 {
		t.Fatalf("月度 work_amount_cents = %d, want 34500", got)
	}
	if got := subInt(row["subsidy_cents"]); got != 6000 {
		t.Fatalf("月度 subsidy_cents = %d, want 6000", got)
	}
	if got := subInt(row["deduct_cents"]); got != 1500 {
		t.Fatalf("月度 deduct_cents = %d, want 1500", got)
	}

	w, out = doReq(r, http.MethodGet, "/api/worklog/attendance?year=2026&month=10&worker_id="+itoa(int(wid)), 1, nil)
	data = mustOK(t, w, out, "月度考勤")
	if got := subInt(data["subsidy_cents"]); got != 6000 {
		t.Fatalf("考勤 subsidy_cents = %d, want 6000", got)
	}
	cells := data["cells"].([]interface{})
	if len(cells) != 1 || subInt(cells[0].(map[string]interface{})["subsidy_cents"]) != 6000 {
		t.Fatalf("考勤格子没带出补贴：%v", cells)
	}
}
