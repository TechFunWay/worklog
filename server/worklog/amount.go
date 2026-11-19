package worklog

import (
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"smallgo/server/response"
)

// parseQuantity 解析数量 decimal 文本（最多两位小数，正数）。
func parseQuantity(s string) (*big.Rat, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("数量不能为空")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok || r.Sign() <= 0 {
		return nil, fmt.Errorf("数量格式不正确")
	}
	if r.Cmp(new(big.Rat).SetInt64(1_000_000)) > 0 {
		return nil, fmt.Errorf("数量过大")
	}
	if r.Denom().Cmp(new(big.Int).SetInt64(100)) > 0 {
		return nil, fmt.Errorf("数量最多支持两位小数")
	}
	return r, nil
}

// maxUnitPriceCents 单价上限：1000 万元（1e9 分）。防止极端输入让金额溢出，
// 也避免把工资录成天文数字。数量本身已限制在 100 万以内。
const maxUnitPriceCents int64 = 1_000_000_000

// 补贴/扣款上限：单笔 100 万元（1e8 分）。与 maxUnitPriceCents 同一思路，
// 防止极端输入把 int64 分撑爆，也挡掉「多打一个 0」这类手滑。
const (
	maxSubsidyCents int64 = 100_000_000
	maxDeductCents  int64 = 100_000_000
)

// recordAmountCents 记工金额的唯一计算入口（v0.6.0 起）。
//
//	纯工钱 wage = 数量 × 单价
//	应得合计 total = wage + 补贴 − 扣款
//
// AmountCents 是全应用唯一的「金额真相」：统计、考勤、对工、结算一律只 SUM 它。
// 所以补贴与扣款必须在这里并进 total，任何地方不得绕过本函数直接改 AmountCents
// —— 否则下游 16 处 SUM 会静默算错工资。
//
// 扣穿（补贴不够抵）返回错误而不是负数：记工金额恒 ≥ 0，
// 「倒欠老板钱」请用借支或结算调整项表达，避免污染所有 SUM 与考勤合计的语义。
func recordAmountCents(q *big.Rat, unitPriceCents, subsidyCents, deductCents int64) (wage, total int64, err error) {
	if subsidyCents < 0 {
		return 0, 0, fmt.Errorf("补贴不能为负数")
	}
	if deductCents < 0 {
		return 0, 0, fmt.Errorf("扣款不能为负数")
	}
	if subsidyCents > maxSubsidyCents {
		return 0, 0, fmt.Errorf("补贴过大，请核对")
	}
	if deductCents > maxDeductCents {
		return 0, 0, fmt.Errorf("扣款过大，请核对")
	}
	wage = computeAmountCents(q, unitPriceCents)
	total = wage + subsidyCents - deductCents
	if total < 0 {
		return 0, 0, fmt.Errorf("扣款大于工钱，请改用借支或结算调整项记录")
	}
	return wage, total, nil
}

// payStatusOf 结算单付款状态：只由服务端算，前端不自己判。
// payable ≤ 0 的结算单（借支倒挂）没有「待付」可言，一律视为已结清。
func payStatusOf(payableCents, paidCents int64) string {
	if payableCents <= 0 || paidCents >= payableCents {
		return PayStatusPaid
	}
	if paidCents <= 0 {
		return PayStatusUnpaid
	}
	return PayStatusPartial
}

// computeAmountCents 金额（分）= 数量 × 单价（分），四舍五入。
// 用 big.Rat 精确计算后取整，不再经过 float64，避免大金额丢分/溢出。
// 记工金额请走 recordAmountCents；本函数只做「数量 × 单价」这一段纯乘法。
func computeAmountCents(q *big.Rat, unitPriceCents int64) int64 {
	total := new(big.Rat).Mul(q, new(big.Rat).SetInt64(unitPriceCents))
	s := total.FloatString(0) // 四舍五入，0.5 向远离零方向
	n, ok := new(big.Int).SetString(s, 10)
	if !ok || !n.IsInt64() {
		return 0
	}
	return n.Int64()
}

// quantityMultipleOK 校验数量符合类型的步进：day 0.5 步进、hour 0.25 步进、piece 整数。
func quantityMultipleOK(typ string, q *big.Rat) bool {
	var step *big.Rat
	switch typ {
	case TypeDay:
		step = new(big.Rat).SetFloat64(0.5)
	case TypeHour:
		step = new(big.Rat).SetFloat64(0.25)
	case TypePiece:
		step = new(big.Rat).SetInt64(1)
	default:
		return false
	}
	quotient := new(big.Rat).Quo(q, step)
	return quotient.Denom().Cmp(new(big.Int).SetInt64(1)) == 0
}

// normalizeQuantity 输出规整的数量文本（去掉多余尾零）。
func normalizeQuantity(q *big.Rat) string {
	return strings.TrimRight(strings.TrimRight(q.FloatString(2), "0"), ".")
}

const (
	TypeDay   = "day"
	TypeHour  = "hour"
	TypePiece = "piece"
	TypeRest  = "rest"
)

var typeLabels = map[string]string{
	TypeDay:   "点工",
	TypeHour:  "点时",
	TypePiece: "计件",
	TypeRest:  "休息",
}

var methodLabels = map[string]string{
	"cash":   "现金",
	"wechat": "微信",
	"alipay": "支付宝",
	"bank":   "银行转账",
	"other":  "其他",
}

// dailyFromMonthly 月薪折算日薪：月薪 ÷ 当月实际天数，四舍五入到分。
// 缺勤一天扣的钱 = 这里的折算日薪 × 缺勤天数（记工时缺勤日不记工即自然扣款）。
func dailyFromMonthly(monthlyCents int64, date string) int64 {
	if monthlyCents <= 0 {
		return 0
	}
	days := daysInMonthOf(date)
	if days <= 0 {
		days = 30
	}
	return int64(math.Round(float64(monthlyCents) / float64(days)))
}

// daysInMonthOf 解析 YYYY-MM-DD 所在月份的实际天数；解析失败返回 0。
func daysInMonthOf(date string) int {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return 0
	}
	return daysInMonth(t.Year(), int(t.Month()))
}

// daysInMonth 某年某月的实际天数。
func daysInMonth(year, month int) int {
	return time.Date(year, time.Month(month)+1, 0, 0, 0, 0, 0, time.Local).Day()
}

func centsToYuanString(cents int64) string {
	neg := ""
	if cents < 0 {
		neg = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", neg, cents/100, cents%100)
}

// notFound 404 错误响应（框架 response 包未提供该快捷方法）。
func notFound(c *gin.Context, msg string) {
	response.Error(c, 404, response.CodeNotFound, msg)
}
