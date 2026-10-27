package budget

import (
	"context"
	"strings"
	"testing"
	"time"

	"aegis-gateway/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
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

func TestExceededUsesMySQLLedgerWhenRedisHasNoCounter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UsageLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	rows := []model.UsageLog{
		{RequestID: "this-month", APIKeyID: 42, ModelName: "m", Kind: "chat", Cost: 1.25, CreatedAt: now},
		{RequestID: "other-key", APIKeyID: 43, ModelName: "m", Kind: "chat", Cost: 9, CreatedAt: now},
		{RequestID: "old-month", APIKeyID: 42, ModelName: "m", Kind: "chat", Cost: 8, CreatedAt: monthStart.Add(-time.Second)},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	b := New(nil, db)
	exceeded, spent, err := b.Exceeded(context.Background(), 42, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !exceeded || spent != 1.25 {
		t.Fatalf("budget exceeded=%v spent=%v, want true / 1.25", exceeded, spent)
	}
}
