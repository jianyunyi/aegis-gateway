package budget

import (
	"strings"
	"testing"
)

func TestMonthKey(t *testing.T) {
	k := MonthKey(42)
	parts := strings.Split(k, ":")
	if len(parts) != 3 || parts[0] != "budget" || parts[1] != "42" || len(parts[2]) != 6 {
		t.Fatalf("MonthKey 格式异常: %s", k)
	}
	if _, err := parseMonthSuffix(parts[2]); err != nil {
		t.Fatalf("月份后缀非法: %v", err)
	}
}

func parseMonthSuffix(s string) (int, error) {
	if len(s) != 6 {
		return 0, ErrParse
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, ErrParse
		}
	}
	return 0, nil
}
