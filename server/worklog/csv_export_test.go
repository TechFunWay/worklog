package worklog

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestCSVCellFormulaInjection 锁住 CSV 公式注入防护。
func TestCSVCellFormulaInjection(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"张三", "张三"},
		{"=1+1", "'=1+1"},
		{"+100", "'+100"},
		{"-100", "'-100"},
		{"@SUM(A1)", "'@SUM(A1)"},
		{"1+1", "1+1"},
	}
	for _, c := range cases {
		if got := csvCell(c.in); got != c.want {
			t.Errorf("csvCell(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestWriteCSVEndToEnd 走真实响应链路验证转义生效且中文不受影响。
func TestWriteCSVEndToEnd(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	writeCSV(c, "attendance.csv", [][]string{
		{"工人", "备注"},
		{"=cmd|'/c calc'!A1", "@evil"},
		{"李四", "正常"},
	})

	body := w.Body.String()
	if !strings.Contains(body, "'=cmd|'/c calc'!A1") {
		t.Fatalf("= 开头的公式未被转义，响应: %s", body)
	}
	if !strings.Contains(body, "'@evil") {
		t.Fatalf("@ 开头的公式未被转义，响应: %s", body)
	}
	if !strings.Contains(body, "李四") || !strings.Contains(body, "正常") {
		t.Fatalf("普通中文被破坏，响应: %s", body)
	}
}
