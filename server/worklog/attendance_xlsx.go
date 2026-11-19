// attendance_xlsx.go — 考勤表 Excel 工作簿（三个工作表）。
//
//  1. 考勤表：月度/时间段网格，行=工人、列=日期，格内是当天的点工/计时/计件/休息；
//  2. 工资条：每行一个工人的考勤天数与点工、计时、计件的数量与工钱，
//     表尾汇总区再列「调整项 / 已付 / 尚欠」三行（v0.6.0 结算调整与分期实付）；
//  3. 逐日明细：每条有记录的人·日期一行，带备注，方便核对。
//
// ⚠️ 列序纪律：v0.6.0 起所有新列（补贴 / 扣款 / 调整 / 已付 / 尚欠）一律追加在末尾，
// 既有列一个都不挪、不删、不改名，粉丝手里的 Excel 模板才不会错位。
//
// ⚠️ 金额口径：本文件所有「工钱 / 金额」列都是 SUM(amount_cents) 的结果，补贴已在里面、
// 扣款已减掉；「其中补贴 / 其中扣款」两列只是把合计拆开给人看，绝不能再相加一次。
package worklog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"smallgo/server/logger"
)

// payslipSettlementSummary 工资条表尾部三行的结算口径汇总。
//
// 数据来自 settlements（不是记工），所以它和上面的「工钱合计」不是同一套账：
//
//	AdjustCents  期间相关结算单的调整项合计（抹零/罚款为负，奖励为正）；
//	PaidCents    已付合计（付款流水聚合）；
//	DueCents     尚欠合计 = 各单 max(应付 − 已付, 0) 之和，多付不倒扣别的单；
//	Count        命中的结算单张数（0 表示本期间还没有出过工资单，三行都写 0）。
//
// 「期间相关」= 结算单的考勤期间与本表导出期间有重叠（period_start<=end 且
// period_end>=start）；跨期结算单会同时出现在两张考勤表里，这是刻意的对账提示，
// 不是重复计钱（这三行不参与上面任何合计）。
type payslipSettlementSummary struct {
	AdjustCents int64
	PaidCents   int64
	DueCents    int64
	Count       int64
}

// settlementSummaryForPayslip 按导出期间与工人范围汇总结算单的调整/已付/尚欠。
// SQL 层一次聚合，不逐单捞。
func settlementSummaryForPayslip(db *gorm.DB, spec *attendanceExportSpec) (payslipSettlementSummary, error) {
	var out payslipSettlementSummary
	q := db.Model(&Settlement{}).Where(
		"user_id = ? AND period_start <= ? AND period_end >= ?", spec.dsUID, spec.end, spec.start)
	if spec.workerID > 0 {
		q = q.Where("worker_id = ?", spec.workerID)
	}
	// 工地过滤只对「指定某个工地」生效：按人结算出来的工资单 project_id 为空，
	// 但它可能同时包含这个工地与别的工地的工，套上 pf.apply 的 `project_id IS NULL`
	// 反而会把该看工资单漏掉。「未分工地 / 全部工地」两种筛选下都不按工地裁剪工资单。
	if id := spec.pf.onlyID(); id != 0 {
		q = q.Where("project_id = ?", id)
	}
	var row struct {
		Adjust int64
		Paid   int64
		Due    int64
		Cnt    int64
	}
	sql := "COALESCE(SUM(adjust_cents),0) AS adjust, " +
		"COALESCE(SUM(paid_cents),0) AS paid, " +
		"COALESCE(SUM(CASE WHEN payable_cents > paid_cents THEN payable_cents - paid_cents ELSE 0 END),0) AS due, " +
		"COUNT(*) AS cnt"
	if err := q.Select(sql).Scan(&row).Error; err != nil {
		return out, err
	}
	out.AdjustCents, out.PaidCents, out.DueCents, out.Count = row.Adjust, row.Paid, row.Due, row.Cnt
	return out, nil
}

// buildAttendanceXLSX 生成考勤表 Excel。
func buildAttendanceXLSX(db *gorm.DB, spec *attendanceExportSpec) ([]byte, error) {
	grid, err := buildAttendanceGrid(db, spec)
	if err != nil {
		return nil, err
	}
	// 结算汇总查不到不该让整张考勤表导出失败：降级成全 0，工资条照样出得来。
	summary, err := settlementSummaryForPayslip(db, spec)
	if err != nil {
		logger.Warn("考勤表工资条结算汇总失败，按 0 输出: %v", err)
		summary = payslipSettlementSummary{}
	}
	return buildXLSX([]xlsxSheet{
		attendanceGridSheet(grid),
		attendancePayslipSheet(grid, summary),
		attendanceDetailSheet(grid),
	})
}

func attendanceSheetInfo(grid *attendanceGridData) string {
	worker := ""
	if len(grid.Workers) == 1 {
		worker = "   工人：" + grid.Workers[0].WorkerName
	}
	// 表头第二行的说明位：v0.6.0 起「工钱 / 金额」列已含补贴、已减扣款，
	// 列名沿用不改（下游模板按列位与列名取数），改在这里把口径讲明白，
	// 免得老板拿「数量 × 单价」去核工资条说我们算错。
	return fmt.Sprintf("时间：%s 至 %s   工地：%s%s   导出时间：%s   注：工钱/金额已含补贴、已减扣款",
		grid.Start, grid.End, grid.ProjectName, worker, time.Now().Format("2006-01-02 15:04"))
}

// attendanceGridText 把一天的多条记录压缩成一个简短的格内文字。
func attendanceGridText(c *attendanceGridCell) string {
	if c == nil {
		return ""
	}
	parts := make([]string, 0, 4)
	if c.Rest {
		parts = append(parts, "休")
	}
	if c.Days > 0 {
		parts = append(parts, trimFloat(c.Days)+"工")
	}
	if c.Hours > 0 {
		parts = append(parts, trimFloat(c.Hours)+"时")
	}
	if c.Pieces > 0 {
		parts = append(parts, trimFloat(c.Pieces)+"件")
	}
	return strings.Join(parts, " ")
}

func dayOfMonth(date string) string {
	if len(date) >= 10 {
		if n, err := strconv.Atoi(date[8:10]); err == nil {
			return strconv.Itoa(n)
		}
	}
	return date
}

func yuanCell(cents int64, style int) xlsxCell {
	return xlsxNum(float64(cents)/100, style)
}

// attendanceGridSheet 工作表一：考勤网格。
func attendanceGridSheet(grid *attendanceGridData) xlsxSheet {
	colCount := 1 + len(grid.Dates) + 5
	lastCol := xlsxColName(colCount)
	rows := make([][]xlsxCell, 0, len(grid.Workers)+4)

	title := make([]xlsxCell, colCount)
	title[0] = xlsxText("考勤表", xlsxStyleTitle)
	rows = append(rows, title)

	info := make([]xlsxCell, colCount)
	info[0] = xlsxText(attendanceSheetInfo(grid), xlsxStyleInfo)
	rows = append(rows, info)

	header := make([]xlsxCell, 0, colCount)
	header = append(header, xlsxText("姓名", xlsxStyleHeader))
	for _, d := range grid.Dates {
		header = append(header, xlsxText(dayOfMonth(d), xlsxStyleHeader))
	}
	// 「工钱(元)」= 该工人本期间的应得合计，补贴已并入、扣款已减掉；
	// 拆项看「工资条」表末尾的补贴/扣款两列。列名沿用不改，避免下游模板错位。
	header = append(header,
		xlsxText("点工(天)", xlsxStyleHeader),
		xlsxText("计时(小时)", xlsxStyleHeader),
		xlsxText("计件", xlsxStyleHeader),
		xlsxText("休息", xlsxStyleHeader),
		xlsxText("工钱(元)", xlsxStyleHeader),
	)
	rows = append(rows, header)

	for _, w := range grid.Workers {
		byDate := make(map[string]*attendanceGridCell, len(w.Cells))
		for i := range w.Cells {
			byDate[w.Cells[i].Date] = &w.Cells[i]
		}
		row := make([]xlsxCell, 0, colCount)
		row = append(row, xlsxText(w.WorkerName, xlsxStyleBodyLeft))
		for _, d := range grid.Dates {
			row = append(row, xlsxText(attendanceGridText(byDate[d]), xlsxStyleBody))
		}
		row = append(row,
			xlsxNum(w.Totals.Days, xlsxStyleBody),
			xlsxNum(w.Totals.Hours, xlsxStyleBody),
			xlsxNum(w.Totals.Pieces, xlsxStyleBody),
			xlsxNum(float64(w.Totals.RestCount), xlsxStyleBody),
			yuanCell(w.Totals.AmountCents, xlsxStyleMoney),
		)
		rows = append(rows, row)
	}

	total := make([]xlsxCell, colCount)
	total[0] = xlsxText("合计", xlsxStyleTotal)
	for i := 1; i < 1+len(grid.Dates); i++ {
		total[i] = xlsxText("", xlsxStyleTotal)
	}
	total = append(total,
		xlsxNum(grid.Total.Days, xlsxStyleTotal),
		xlsxNum(grid.Total.Hours, xlsxStyleTotal),
		xlsxNum(grid.Total.Pieces, xlsxStyleTotal),
		xlsxNum(float64(grid.Total.RestCount), xlsxStyleTotal),
		yuanCell(grid.Total.AmountCents, xlsxStyleTotalMoney),
	)
	rows = append(rows, total)

	cols := make([]float64, 0, colCount)
	cols = append(cols, 12)
	for range grid.Dates {
		cols = append(cols, 5.2)
	}
	cols = append(cols, 8.5, 9.5, 8, 6.5, 11.5)

	return xlsxSheet{
		Name:       "考勤表",
		Cols:       cols,
		Rows:       rows,
		Merges:     []string{"A1:" + lastCol + "1", "A2:" + lastCol + "2"},
		FreezeRows: 3,
		FreezeCols: 1,
	}
}

// attendancePayslipSheet 工作表二：工资条（每人一行，按点工/计时/计件分项）。
//
// v0.6.0 起在末尾追加「补贴(元) / 扣款(元)」两列（既有 10 列一字未动），
// 并在合计行下面补「调整项 / 已付 / 尚欠」三行结算口径汇总；三行恒写（没有结算单
// 就写 0），表格结构稳定，下游按行号取数的模板不会错位。
func attendancePayslipSheet(grid *attendanceGridData, summary payslipSettlementSummary) xlsxSheet {
	const colCount = 12
	lastCol := xlsxColName(colCount)
	rows := make([][]xlsxCell, 0, len(grid.Workers)+4)

	title := make([]xlsxCell, colCount)
	title[0] = xlsxText("工资条", xlsxStyleTitle)
	rows = append(rows, title)

	info := make([]xlsxCell, colCount)
	info[0] = xlsxText(attendanceSheetInfo(grid), xlsxStyleInfo)
	rows = append(rows, info)

	rows = append(rows, []xlsxCell{
		xlsxText("姓名", xlsxStyleHeader),
		xlsxText("考勤天数", xlsxStyleHeader),
		xlsxText("点工(天)", xlsxStyleHeader),
		xlsxText("点工工钱(元)", xlsxStyleHeader),
		xlsxText("计时(小时)", xlsxStyleHeader),
		xlsxText("计时工钱(元)", xlsxStyleHeader),
		xlsxText("计件(数量)", xlsxStyleHeader),
		xlsxText("计件工钱(元)", xlsxStyleHeader),
		xlsxText("休息(天)", xlsxStyleHeader),
		// 工钱合计 = SUM(amount_cents)：补贴已加在里面、扣款已减掉，不是纯「数量 × 单价」。
		xlsxText("工钱合计(元)", xlsxStyleHeader),
		xlsxText("其中补贴(元)", xlsxStyleHeader),
		xlsxText("其中扣款(元)", xlsxStyleHeader),
	})

	for _, w := range grid.Workers {
		rows = append(rows, []xlsxCell{
			xlsxText(w.WorkerName, xlsxStyleBodyLeft),
			xlsxNum(float64(w.Totals.AttendDays), xlsxStyleBody),
			xlsxNum(w.Totals.Days, xlsxStyleBody),
			yuanCell(w.Totals.DayAmountCents, xlsxStyleMoney),
			xlsxNum(w.Totals.Hours, xlsxStyleBody),
			yuanCell(w.Totals.HourAmountCents, xlsxStyleMoney),
			xlsxNum(w.Totals.Pieces, xlsxStyleBody),
			yuanCell(w.Totals.PieceAmountCents, xlsxStyleMoney),
			xlsxNum(float64(w.Totals.RestCount), xlsxStyleBody),
			yuanCell(w.Totals.AmountCents, xlsxStyleMoney),
			// 末两列是「其中」，已从工钱合计里算过一次，只能看不能再加。
			yuanCell(w.Totals.SubsidyCents, xlsxStyleMoney),
			yuanCell(w.Totals.DeductCents, xlsxStyleMoney),
		})
	}

	rows = append(rows, []xlsxCell{
		xlsxText("合计", xlsxStyleTotal),
		xlsxNum(float64(grid.Total.AttendDays), xlsxStyleTotal),
		xlsxNum(grid.Total.Days, xlsxStyleTotal),
		yuanCell(grid.Total.DayAmountCents, xlsxStyleTotalMoney),
		xlsxNum(grid.Total.Hours, xlsxStyleTotal),
		yuanCell(grid.Total.HourAmountCents, xlsxStyleTotalMoney),
		xlsxNum(grid.Total.Pieces, xlsxStyleTotal),
		yuanCell(grid.Total.PieceAmountCents, xlsxStyleTotalMoney),
		xlsxNum(float64(grid.Total.RestCount), xlsxStyleTotal),
		yuanCell(grid.Total.AmountCents, xlsxStyleTotalMoney),
		yuanCell(grid.Total.SubsidyCents, xlsxStyleTotalMoney),
		yuanCell(grid.Total.DeductCents, xlsxStyleTotalMoney),
	})

	// 结算口径三行：来自 settlements（含跨期单），与上面的记工合计不同源，
	// 恒写 0 保持表格结构稳定；标签放在姓名列、金额落在「工钱合计」那一列（第 10 列）。
	note := fmt.Sprintf("%d 张工资单", summary.Count)
	rows = append(rows, payslipSummaryRow("调整项(元)", summary.AdjustCents, colCount, note))
	rows = append(rows, payslipSummaryRow("已付(元)", summary.PaidCents, colCount, ""))
	rows = append(rows, payslipSummaryRow("尚欠(元)", summary.DueCents, colCount, ""))

	return xlsxSheet{
		Name:       "工资条",
		Cols:       []float64{12, 9, 9, 13, 9, 13, 9, 13, 8, 13, 12, 12},
		Rows:       rows,
		Merges:     []string{"A1:" + lastCol + "1", "A2:" + lastCol + "2"},
		FreezeRows: 3,
		FreezeCols: 1,
	}
}

// payslipSummaryRow 工资条表尾「调整项 / 已付 / 尚欠」的一行：
// 标签占第 1 列、金额落在第 10 列（工钱合计那一列）下面，其余列留空但带上合计样式，
// 让行宽与其它行一致。第 2 列顺手写上命中张数（只在第一行写），
// 免得老板把这三行当成上面考勤合计的组成部分。
func payslipSummaryRow(label string, cents int64, colCount int, countNote string) []xlsxCell {
	row := make([]xlsxCell, colCount)
	for i := range row {
		row[i] = xlsxText("", xlsxStyleTotal)
	}
	row[0] = xlsxText(label, xlsxStyleTotal)
	if countNote != "" && colCount > 1 {
		row[1] = xlsxText(countNote, xlsxStyleInfo)
	}
	const moneyCol = 9 // 第 10 列：工钱合计(元)
	if moneyCol < colCount {
		row[moneyCol] = yuanCell(cents, xlsxStyleTotalMoney)
	}
	return row
}

// attendanceDetailSheet 工作表三：逐日明细（只列有记录的日期）。
//
// v0.6.0 起在末尾追加「补贴(元) / 扣款(元)」两列（既有 8 列顺序不变），
// 让「金额(元) ≠ 数量 × 单价」这种事在表里就能自查。
func attendanceDetailSheet(grid *attendanceGridData) xlsxSheet {
	const colCount = 10
	lastCol := xlsxColName(colCount)
	rows := make([][]xlsxCell, 0, 32)

	title := make([]xlsxCell, colCount)
	title[0] = xlsxText("考勤逐日明细", xlsxStyleTitle)
	rows = append(rows, title)

	info := make([]xlsxCell, colCount)
	info[0] = xlsxText(attendanceSheetInfo(grid), xlsxStyleInfo)
	rows = append(rows, info)

	rows = append(rows, []xlsxCell{
		xlsxText("姓名", xlsxStyleHeader),
		xlsxText("日期", xlsxStyleHeader),
		xlsxText("点工(天)", xlsxStyleHeader),
		xlsxText("计时(小时)", xlsxStyleHeader),
		xlsxText("计件(数量)", xlsxStyleHeader),
		xlsxText("休息", xlsxStyleHeader),
		// 金额 = 当天应得合计（补贴已并入、扣款已减掉）。
		xlsxText("金额(元)", xlsxStyleHeader),
		xlsxText("备注", xlsxStyleHeader),
		xlsxText("其中补贴(元)", xlsxStyleHeader),
		xlsxText("其中扣款(元)", xlsxStyleHeader),
	})

	for _, w := range grid.Workers {
		for _, c := range w.Cells {
			rest := ""
			if c.Rest {
				rest = "休"
			}
			rows = append(rows, []xlsxCell{
				xlsxText(w.WorkerName, xlsxStyleBodyLeft),
				xlsxText(c.Date, xlsxStyleBody),
				xlsxNum(c.Days, xlsxStyleBody),
				xlsxNum(c.Hours, xlsxStyleBody),
				xlsxNum(c.Pieces, xlsxStyleBody),
				xlsxText(rest, xlsxStyleBody),
				yuanCell(c.AmountCents, xlsxStyleMoney),
				xlsxText(strings.Join(c.Notes, "；"), xlsxStyleBodyLeft),
				// 拆项列追加在最末尾：金额里已经含过它们，只给对账看，不再参与合计。
				yuanCell(c.SubsidyCents, xlsxStyleMoney),
				yuanCell(c.DeductCents, xlsxStyleMoney),
			})
		}
	}

	rows = append(rows, []xlsxCell{
		xlsxText("合计", xlsxStyleTotal),
		xlsxText("", xlsxStyleTotal),
		xlsxNum(grid.Total.Days, xlsxStyleTotal),
		xlsxNum(grid.Total.Hours, xlsxStyleTotal),
		xlsxNum(grid.Total.Pieces, xlsxStyleTotal),
		xlsxNum(float64(grid.Total.RestCount), xlsxStyleTotal),
		yuanCell(grid.Total.AmountCents, xlsxStyleTotalMoney),
		xlsxText("", xlsxStyleTotal),
		yuanCell(grid.Total.SubsidyCents, xlsxStyleTotalMoney),
		yuanCell(grid.Total.DeductCents, xlsxStyleTotalMoney),
	})

	return xlsxSheet{
		Name: "逐日明细",
		// 末两列（其中补贴 / 其中扣款）是 v0.6.0 追加的拆项列，宽度与金额列一致。
		Cols:       []float64{12, 12, 9, 10, 10, 7, 12, 32, 12, 12},
		Rows:       rows,
		Merges:     []string{"A1:" + lastCol + "1", "A2:" + lastCol + "2"},
		FreezeRows: 3,
		FreezeCols: 1,
	}
}
