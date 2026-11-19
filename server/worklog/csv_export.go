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

// numericCell 判断单元格是否是本程序自己格式化出来的纯数字（可带负号、至多一个小数点）。
//
// 为什么要单独放行：csvCell 把 `-` 开头一律加前缀，是为了挡住「用户录入内容伪装成公式」；
// 但结算调整项（抹零/罚款）本来就是**负数**，centsToYuanString(-5000) 得到 "-50.00"，
// 被转义成 "'-50.00" 后 Excel 按文本存，会计那一列求和直接漏掉这笔调整。
// 纯数字不可能携带公式载荷（没有 = + @ 前导、没有任何字母），所以这一类原样写出。
// 判定刻意严格：只允许 可选负号 + 数字 + 至多一个小数点，其它一律回落到 csvCell 转义。
func numericCell(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i = 1
	}
	digits, dots := 0, 0
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
		case c == '.':
			dots++
			if dots > 1 {
				return false
			}
		default:
			return false
		}
	}
	return digits > 0
}

// csvCellSafe 程序自己算出来的数字原样写，其余一律走公式注入防护。
func csvCellSafe(s string) string {
	if numericCell(s) {
		return s
	}
	return csvCell(s)
}

// writeCSV 先做公式注入防护再交给框架的 WriteCSV（负责 BOM 与下载头）。
//
// 所有导出都必须走这里：v0.6.0 新增的 工钱/补贴/扣款/工地/调整项/已付/尚欠/付款状态
// 这些列里，既有程序算出的金额，也有用户录入的工地名与调整原因，逐格过 csvCellSafe 才安全。
func writeCSV(c *gin.Context, filename string, rows [][]string) {
	safe := make([][]string, len(rows))
	for i, row := range rows {
		cells := make([]string, len(row))
		for j, cell := range row {
			cells[j] = csvCellSafe(cell)
		}
		safe[i] = cells
	}
	utils.WriteCSV(c, filename, safe)
}

// csvPayStatusLabels 结算付款状态的中文标签（导出用；与 models.go 的 PayStatus* 常量一一对应）。
// 前缀 csv 是刻意的：这是「导出侧的文案」，不是业务口径；接口与前端要标签请各自维护，
// 也顺便避开与 settlement.go 将来可能新增的同名映射表撞车（同包内重复声明会编译失败）。
var csvPayStatusLabels = map[string]string{
	PayStatusUnpaid:  "未付",
	PayStatusPartial: "部分支付",
	PayStatusPaid:    "已付清",
}

// csvPayStatusLabel 付款状态出中文（CSV 里写 unpaid 老板看不懂）。
//
// 空值/未知值不直接写英文：按金额现算一次状态兜底 —— v0.6.0 之前的老结算单没有
// pay_status，升级脚本会刷成 paid，万一漏刷也不能在导出里凭空造出一条「未付」。
func csvPayStatusLabel(payStatus string, payableCents, paidCents int64) string {
	if label, ok := csvPayStatusLabels[payStatus]; ok {
		return label
	}
	return csvPayStatusLabels[payStatusOf(payableCents, paidCents)]
}
