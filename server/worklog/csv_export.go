// csv_export.go — CSV 导出的公式注入防护。
//
// 为什么需要：电子表格（Excel / WPS / Numbers）会把以 = + - @ 开头的单元格
// 当成公式执行。工记导出的工人姓名、备注、工地名都是用户录入内容，原样写进
// CSV 就构成 CSV 公式注入——包工头把工资表转给会计、或导出后转存时可能触发
// =HYPERLINK(...) 一类外链请求。
//
// 为什么做成包装函数而不是在每处 WriteCSV 前手工转义：本仓库有 5 处导出
// （记工明细、工资月报、考勤表、借支记录、结算记录），逐个改必然漏；包装
// 函数把「安全」变成默认路径，新增导出只要调 writeCSV 就自带防护。
//
// 与 collect 的实现口径一致（collect/server/collect/export.go 的 csvCell），
// 家族统一做法：命中危险首字符时前置一个单引号，电子表格会按文本处理。
package worklog

import (
	"github.com/gin-gonic/gin"

	"smallgo/server/utils"
)

// csvCell 公式注入防护：电子表格会把 = + - @ 开头的单元格当公式执行。
func csvCell(s string) string {
	if s == "" {
		return s
	}
	switch s[0] {
	case '=', '+', '-', '@':
		return "'" + s
	}
	return s
}

// writeCSV 先做公式注入防护再交给框架的 WriteCSV（负责 BOM 与下载头）。
func writeCSV(c *gin.Context, filename string, rows [][]string) {
	safe := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(row))
		for j, cell := range row {
			cells[j] = csvCell(cell)
		}
		safe[i] = cells
	}
	utils.WriteCSV(c, filename, safe)
}
