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

// computeAmountCents 金额（分）= 数量 × 单价（分），四舍五入。
func computeAmountCents(q *big.Rat, unitPriceCents int64) int64 {
	total := new(big.Rat).Mul(q, new(big.Rat).SetInt64(unitPriceCents))
	f, _ := total.Float64()
	return int64(math.Round(f))
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
	"cash":    "现金",
	"wechat":  "微信",
	"alipay":  "支付宝",
	"bank":    "银行转账",
	"other":   "其他",
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
