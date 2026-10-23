// attendance_xlsx.go — 考勤表 Excel 工作簿（三个工作表）。
//
// 1) 考勤表：月度/时间段网格，行=工人、列=日期，格内是当天的点工/计时/计件/休息；
// 2) 工资条：每行一个工人的考勤天数与点工、计时、计件的数量与工钱；
// 3) 逐日明细：每条有记录的人·日期一行，带备注，方便核对。
package worklog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// buildAttendanceXLSX 生成考勤表 Excel。
func buildAttendanceXLSX(db *gorm.DB, spec *attendanceExportSpec) ([]byte, error) {
	grid, err := buildAttendanceGrid(db, spec)
	if err != nil {
		return nil, err
	}
	return buildXLSX([]xlsxSheet{
		attendanceGridSheet(grid),
		attendancePayslipSheet(grid),
		attendanceDetailSheet(grid),
	})
}

func attendanceSheetInfo(grid *attendanceGridData) string {
	worker := ""
	if len(grid.Workers) == 1 {
		worker = "   工人：" + grid.Workers[0].WorkerName
	}
	return fmt.Sprintf("时间：%s 至 %s   工地：%s%s   导出时间：%s",
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
func attendancePayslipSheet(grid *attendanceGridData) xlsxSheet {
	const colCount = 10
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
		xlsxText("工钱合计(元)", xlsxStyleHeader),
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
	})

	return xlsxSheet{
		Name:       "工资条",
		Cols:       []float64{12, 9, 9, 13, 9, 13, 9, 13, 8, 13},
		Rows:       rows,
		Merges:     []string{"A1:" + lastCol + "1", "A2:" + lastCol + "2"},
		FreezeRows: 3,
		FreezeCols: 1,
	}
}

// attendanceDetailSheet 工作表三：逐日明细（只列有记录的日期）。
func attendanceDetailSheet(grid *attendanceGridData) xlsxSheet {
	const colCount = 8
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
		xlsxText("金额(元)", xlsxStyleHeader),
		xlsxText("备注", xlsxStyleHeader),
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
	})

	return xlsxSheet{
		Name:       "逐日明细",
		Cols:       []float64{12, 12, 9, 10, 10, 7, 12, 32},
		Rows:       rows,
		Merges:     []string{"A1:" + lastCol + "1", "A2:" + lastCol + "2"},
		FreezeRows: 3,
		FreezeCols: 1,
	}
}
