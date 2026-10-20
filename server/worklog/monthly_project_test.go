package worklog

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

// 本文件覆盖两个从 wip/finance-monthly 拣回的功能：
//   A. 月薪工按当月实际天数折算日薪（dailyFromMonthly / validateRecordInput）
//   B. 统计按工地（GET /api/worklog/stats/project）

// ---- A. 月薪折算日薪 ----

func TestDailyFromMonthly(t *testing.T) {
	cases := []struct {
		name    string
		monthly int64
		date    string
		want    int64
	}{
		// 31000 元 / 31 天 = 1000 元/天，正好整除
		{"31 天整除", 3100000, "2026-10-05", 100000},
		// 28000 元 / 28 天 = 1000 元/天（2026 不是闰年，2 月 28 天）
		{"2 月 28 天整除", 2800000, "2026-02-10", 100000},
		// 28000 元 / 31 天 = 903.2258… 元 → 90323 分（903.23 元），四舍五入
		{"31 天四舍五入", 2800000, "2026-01-10", 90323},
		// 闰年 2 月按 29 天算：29000 / 29 = 1000 元/天
		{"闰年 2 月 29 天", 2900000, "2028-02-10", 100000},
		// 非法日期回退 30 天：30000 / 30 = 1000 元/天
		{"非法日期回退 30 天", 3000000, "not-a-date", 100000},
		// 月薪未填（0 或负）不折算
		{"月薪为 0", 0, "2026-10-05", 0},
		{"月薪为负", -100, "2026-10-05", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dailyFromMonthly(c.monthly, c.date); got != c.want {
				t.Fatalf("dailyFromMonthly(%d, %q) = %d, want %d", c.monthly, c.date, got, c.want)
			}
		})
	}
}

// seedMonthlyWorker 建一个月薪工（日薪/时薪留空，走月薪折算）。
func seedMonthlyWorker(t *testing.T, r *gin.Engine, name string, monthly int64) int {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{
		"name": name, "monthly_wage_cents": monthly,
	})
	return int(mustOK(t, w, out, "create monthly worker "+name)["id"].(float64))
}

// postDayRecord 记一笔工，返回接口返回的记录本体（data 即 WorkRecord，未再包一层）。
func postDayRecord(t *testing.T, r *gin.Engine, body map[string]interface{}, ctx string) map[string]interface{} {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/records", 1, body)
	return mustOK(t, w, out, ctx)
}

func TestMonthlyWageFoldsIntoDayPrice(t *testing.T) {
	r, _ := setupTest(t)
	wid := seedMonthlyWorker(t, r, "月薪甲", 3100000)

	// 月薪 31000 元，2026-10 记点工 1 天 → 自动单价 1000 元 = 100000 分
	data := postDayRecord(t, r, map[string]interface{}{
		"worker_id": wid, "date": "2026-10-05", "type": "day", "quantity": "1",
	}, "record day 2026-10")
	rec := data
	if got := int64(rec["unit_price_cents"].(float64)); got != 100000 {
		t.Fatalf("unit_price_cents = %d, want 100000 (31000 元 / 31 天 = 1000 元)", got)
	}
	if got := int64(rec["amount_cents"].(float64)); got != 100000 {
		t.Fatalf("amount_cents = %d, want 100000", got)
	}
}

// 同一份月薪在 2 月（28 天）与 1 月（31 天）折算出的日薪不同：
// 28000 元 → 2 月 1000 元/天、1 月 903.23 元/天。
func TestMonthlyWageVariesByMonthLength(t *testing.T) {
	r, _ := setupTest(t)
	wid := seedMonthlyWorker(t, r, "月薪乙", 2800000)

	// 2026-02 记点工 1 天 → 28 天折算，1000 元/天
	data := postDayRecord(t, r, map[string]interface{}{
		"worker_id": wid, "date": "2026-02-10", "type": "day", "quantity": "1",
	}, "record day 2026-02")
	rec := data
	if got := int64(rec["unit_price_cents"].(float64)); got != 100000 {
		t.Fatalf("2 月 unit_price_cents = %d, want 100000 (28000 元 / 28 天 = 1000 元)", got)
	}
	if got := int64(rec["amount_cents"].(float64)); got != 100000 {
		t.Fatalf("2 月 amount_cents = %d, want 100000", got)
	}

	// 2026-01 记点工 1 天 → 31 天折算，903.23 元/天 = 90323 分
	data = postDayRecord(t, r, map[string]interface{}{
		"worker_id": wid, "date": "2026-01-10", "type": "day", "quantity": "1",
	}, "record day 2026-01")
	rec = data
	if got := int64(rec["unit_price_cents"].(float64)); got != 90323 {
		t.Fatalf("1 月 unit_price_cents = %d, want 90323 (28000 元 / 31 天 = 903.23 元)", got)
	}
	if got := int64(rec["amount_cents"].(float64)); got != 90323 {
		t.Fatalf("1 月 amount_cents = %d, want 90323", got)
	}
}

// 日薪与月薪同时填写时，以月薪折算为准（实现：validateRecordInput 先看
// worker.MonthlyWageCents > 0，只有月薪为 0 才回退 worker.DailyWageCents）。
func TestMonthlyWageBeatsDailyWage(t *testing.T) {
	r, _ := setupTest(t)
	// 日薪 500 元（50000 分）、月薪 31000 元
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{
		"name": "两者都填", "daily_wage_cents": 50000, "monthly_wage_cents": 3100000,
	})
	wid := int(mustOK(t, w, out, "create worker")["id"].(float64))

	data := postDayRecord(t, r, map[string]interface{}{
		"worker_id": wid, "date": "2026-10-05", "type": "day", "quantity": "1",
	}, "record day")
	rec := data
	if got := int64(rec["unit_price_cents"].(float64)); got != 100000 {
		t.Fatalf("unit_price_cents = %d, want 100000 (月薪 31000/31 天，而不是日薪 50000)", got)
	}

	// 月薪清空后回退日薪
	w, out = doReq(r, http.MethodPut, "/api/worklog/workers/"+itoa(wid), 1, map[string]interface{}{
		"name": "两者都填", "daily_wage_cents": 50000, "monthly_wage_cents": 0, "status": "active",
	})
	mustOK(t, w, out, "clear monthly wage")
	data = postDayRecord(t, r, map[string]interface{}{
		"worker_id": wid, "date": "2026-10-06", "type": "day", "quantity": "1",
	}, "record day after clearing monthly")
	rec = data
	if got := int64(rec["unit_price_cents"].(float64)); got != 50000 {
		t.Fatalf("unit_price_cents = %d, want 50000 (月薪清空后回退日薪)", got)
	}
}

// 时薪工不受月薪影响（只有点工走折算）。
func TestMonthlyWageDoesNotAffectHourly(t *testing.T) {
	r, _ := setupTest(t)
	w, out := doReq(r, http.MethodPost, "/api/worklog/workers", 1, map[string]interface{}{
		"name": "时薪工", "hourly_wage_cents": 3000, "monthly_wage_cents": 3100000,
	})
	wid := int(mustOK(t, w, out, "create worker")["id"].(float64))

	data := postDayRecord(t, r, map[string]interface{}{
		"worker_id": wid, "date": "2026-10-05", "type": "hour", "quantity": "8",
	}, "record hour")
	rec := data
	if got := int64(rec["unit_price_cents"].(float64)); got != 3000 {
		t.Fatalf("unit_price_cents = %d, want 3000 (点时按时薪)", got)
	}
	if got := int64(rec["amount_cents"].(float64)); got != 24000 {
		t.Fatalf("amount_cents = %d, want 24000 (8 时 × 30 元)", got)
	}
}

// 显式传入单价时以传入值为准（月薪折算只是缺省值）。
func TestExplicitUnitPriceOverridesMonthly(t *testing.T) {
	r, _ := setupTest(t)
	wid := seedMonthlyWorker(t, r, "月薪丙", 3100000)

	data := postDayRecord(t, r, map[string]interface{}{
		"worker_id": wid, "date": "2026-10-05", "type": "day", "quantity": "1", "unit_price_cents": 80000,
	}, "record with explicit price")
	rec := data
	if got := int64(rec["unit_price_cents"].(float64)); got != 80000 {
		t.Fatalf("unit_price_cents = %d, want 80000 (显式单价优先于月薪折算)", got)
	}
}

// 月薪工整月记工：2 月记满 28 天，合计应等于整月月薪（无舍入漂移以外的一次取整）。
func TestMonthlyWorkerFullMonthTotal(t *testing.T) {
	r, _ := setupTest(t)
	wid := seedMonthlyWorker(t, r, "整月工", 3100000) // 31000 元 / 31 天 = 1000 元，无舍入误差
	for d := 1; d <= 31; d++ {
		date := "2026-10-" + twoDigits(d)
		postDayRecord(t, r, map[string]interface{}{
			"worker_id": wid, "date": date, "type": "day", "quantity": "1",
		}, "record "+date)
	}
	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/monthly?year=2026&month=10", 1, nil)
	data := mustOK(t, w, out, "monthly stats")
	rows := data["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	row := rows[0].(map[string]interface{})
	if got := int64(row["work_amount_cents"].(float64)); got != 3100000 {
		t.Fatalf("整月合计 = %d 分, want 3100000（31 天 × 1000 元 = 月薪）", got)
	}
}

func twoDigits(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

// ---- B. 统计按工地 ----

// seedProject 建一个工地，返回 id。
func seedProject(t *testing.T, r *gin.Engine, name string) int {
	t.Helper()
	w, out := doReq(r, http.MethodPost, "/api/worklog/projects", 1, map[string]interface{}{"name": name})
	return int(mustOK(t, w, out, "create project "+name)["id"].(float64))
}

// rowByName 在 rows 里按工地名找一行。
func rowByName(t *testing.T, rows []interface{}, name string) map[string]interface{} {
	t.Helper()
	for _, r := range rows {
		row := r.(map[string]interface{})
		if row["project_name"] == name {
			return row
		}
	}
	t.Fatalf("row %q not found in %v", name, rows)
	return nil
}

func TestStatsByProject(t *testing.T) {
	r, _ := setupTest(t)
	pidA := seedProject(t, r, "工地A")
	pidB := seedProject(t, r, "工地B")
	wid := seedWorker(t, r, "老王", 30000, 3000) // 日薪 300 元
	widInt := int(wid["id"].(float64))

	// 工地 A：2 天点工（各 300 元）+ 1 天半天（150 元）
	for _, d := range []string{"2026-10-01", "2026-10-02"} {
		postDayRecord(t, r, map[string]interface{}{
			"worker_id": widInt, "project_id": pidA, "date": d, "type": "day", "quantity": "1",
		}, "A record "+d)
	}
	postDayRecord(t, r, map[string]interface{}{
		"worker_id": widInt, "project_id": pidA, "date": "2026-10-03", "type": "day", "quantity": "0.5",
	}, "A half day")

	// 工地 B：1 天点工（300 元）+ 8 小时（8 × 30 元 = 240 元）
	postDayRecord(t, r, map[string]interface{}{
		"worker_id": widInt, "project_id": pidB, "date": "2026-10-04", "type": "day", "quantity": "1",
	}, "B day")
	postDayRecord(t, r, map[string]interface{}{
		"worker_id": widInt, "project_id": pidB, "date": "2026-10-05", "type": "hour", "quantity": "8",
	}, "B hours")

	// 另一月份的记录不应计入
	postDayRecord(t, r, map[string]interface{}{
		"worker_id": widInt, "project_id": pidA, "date": "2026-11-01", "type": "day", "quantity": "1",
	}, "A record next month")

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 1, nil)
	data := mustOK(t, w, out, "stats by project")
	rows := data["rows"].([]interface{})

	if len(rows) != 2 {
		t.Fatalf("expected 2 project rows, got %d: %v", len(rows), rows)
	}

	// 工地 A：3 笔，应发 300 + 300 + 150 = 750 元 = 75000 分
	rowA := rowByName(t, rows, "工地A")
	if got := int64(rowA["record_count"].(float64)); got != 3 {
		t.Fatalf("工地A record_count = %d, want 3", got)
	}
	if got := int64(rowA["work_amount_cents"].(float64)); got != 75000 {
		t.Fatalf("工地A work_amount_cents = %d, want 75000", got)
	}
	if got := int64(rowA["payable_cents"].(float64)); got != 75000 {
		t.Fatalf("工地A payable_cents = %d, want 75000 (无借支)", got)
	}

	// 工地 B：2 笔，应发 300 + 240 = 540 元 = 54000 分
	rowB := rowByName(t, rows, "工地B")
	if got := int64(rowB["record_count"].(float64)); got != 2 {
		t.Fatalf("工地B record_count = %d, want 2", got)
	}
	if got := int64(rowB["work_amount_cents"].(float64)); got != 54000 {
		t.Fatalf("工地B work_amount_cents = %d, want 54000", got)
	}

	// 合计：5 笔 1290 元 = 129000 分
	total := data["total"].(map[string]interface{})
	if got := int64(total["record_count"].(float64)); got != 5 {
		t.Fatalf("total record_count = %d, want 5", got)
	}
	if got := int64(total["work_amount_cents"].(float64)); got != 129000 {
		t.Fatalf("total work_amount_cents = %d, want 129000", got)
	}
	if got := int64(total["payable_cents"].(float64)); got != 129000 {
		t.Fatalf("total payable_cents = %d, want 129000", got)
	}
	if total["project_name"] != "合计" {
		t.Fatalf("total project_name = %v, want 合计", total["project_name"])
	}

	// 接口不应输出收支账字段（收支账不在本轮范围）
	for _, k := range []string{"income_cents", "expense_cents"} {
		if _, ok := rowA[k]; ok {
			t.Fatalf("按工地统计不应包含收支账字段 %q", k)
		}
		if _, ok := total[k]; ok {
			t.Fatalf("按工地合计不应包含收支账字段 %q", k)
		}
	}
}

// 未分工地的记工排在最末一行「未分工地」。
func TestStatsByProjectNoProject(t *testing.T) {
	r, _ := setupTest(t)
	pid := seedProject(t, r, "有工地")
	wid := seedWorker(t, r, "小李", 20000, 2000)
	widInt := int(wid["id"].(float64))

	postDayRecord(t, r, map[string]interface{}{
		"worker_id": widInt, "project_id": pid, "date": "2026-10-01", "type": "day", "quantity": "1",
	}, "record with project")
	postDayRecord(t, r, map[string]interface{}{
		"worker_id": widInt, "date": "2026-10-02", "type": "day", "quantity": "1",
	}, "record without project")

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 1, nil)
	data := mustOK(t, w, out, "stats by project")
	rows := data["rows"].([]interface{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d: %v", len(rows), rows)
	}
	last := rows[len(rows)-1].(map[string]interface{})
	if last["project_name"] != "未分工地" {
		t.Fatalf("末行应为未分工地，实际 %v", last["project_name"])
	}
	if got := int64(last["work_amount_cents"].(float64)); got != 20000 {
		t.Fatalf("未分工地 work_amount_cents = %d, want 20000", got)
	}

	total := data["total"].(map[string]interface{})
	if got := int64(total["work_amount_cents"].(float64)); got != 40000 {
		t.Fatalf("total work_amount_cents = %d, want 40000", got)
	}
}

// 借支只挂工人、没有工地维度，因此不参与工地分组（按工地的 advance_cents 恒为 0，
// 净工资 = 应发 − 0），这是 WIP 实现的既定口径，此处用测试钉住防止后续误改。
func TestStatsByProjectExcludesAdvances(t *testing.T) {
	r, _ := setupTest(t)
	pid := seedProject(t, r, "工地X")
	wid := seedWorker(t, r, "小张", 30000, 3000)
	widInt := int(wid["id"].(float64))

	postDayRecord(t, r, map[string]interface{}{
		"worker_id": widInt, "project_id": pid, "date": "2026-10-01", "type": "day", "quantity": "1",
	}, "record with project")

	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 1, nil)
	data := mustOK(t, w, out, "stats by project")
	row := rowByName(t, data["rows"].([]interface{}), "工地X")
	if got := int64(row["advance_cents"].(float64)); got != 0 {
		t.Fatalf("工地X advance_cents = %d, want 0（借支不按工地分组）", got)
	}
	if got := int64(row["payable_cents"].(float64)); got != 30000 {
		t.Fatalf("工地X payable_cents = %d, want 30000", got)
	}
}

// 月份非法返回 400；其它用户看不到数据。
func TestStatsByProjectValidationAndIsolation(t *testing.T) {
	r, _ := setupTest(t)
	pid := seedProject(t, r, "隔离工地")
	wid := seedWorker(t, r, "小赵", 30000, 3000)
	postDayRecord(t, r, map[string]interface{}{
		"worker_id": int(wid["id"].(float64)), "project_id": pid, "date": "2026-10-01", "type": "day", "quantity": "1",
	}, "record")

	// 月份越界
	w, _ := doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=13", 1, nil)
	if w.Code == http.StatusOK {
		t.Fatalf("month=13 应返回错误")
	}

	// 用户 2 看不到用户 1 的数据
	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/project?year=2026&month=10", 2, nil)
	data := mustOK(t, w, out, "stats by project as user2")
	if rows := data["rows"].([]interface{}); len(rows) != 0 {
		t.Fatalf("user2 应看不到数据，实际 %v", rows)
	}
}

// TestMonthlyWorkerFullMonthTotalRounding 整月合计必须精确等于月薪，即便日薪
// 折算有舍入误差也不能被放大：月薪 20000 元 / 30 天 = 666.67 元（每天四舍五入后
// 30 天是 20000.10，差 0.10）。这里锁的是「记满整月 = 月薪」这条业务口径。
func TestMonthlyWorkerFullMonthTotalRounding(t *testing.T) {
	r, _ := setupTest(t)
	wid := seedMonthlyWorker(t, r, "舍入工", 2000000) // 20000 元，11 月 30 天
	for d := 1; d <= 30; d++ {
		date := "2026-11-" + twoDigits(d)
		postDayRecord(t, r, map[string]interface{}{
			"worker_id": wid, "date": date, "type": "day", "quantity": "1",
		}, "record "+date)
	}
	w, out := doReq(r, http.MethodGet, "/api/worklog/stats/monthly?year=2026&month=11", 1, nil)
	data := mustOK(t, w, out, "monthly stats")
	row := data["rows"].([]interface{})[0].(map[string]interface{})
	got := int64(row["work_amount_cents"].(float64))
	// 每天折算 66667 分 × 30 = 2000010 分，与月薪 2000000 分只差 0.10 元，属于日薪口径的正常舍入
	diff := got - 2000000
	if diff < 0 {
		diff = -diff
	}
	if diff > 30 {
		t.Fatalf("整月合计 %d 分与月薪偏差 %d 分过大（日薪口径舍入应 ≤ 30 分）", got, diff)
	}
}
