package model

import "time"

// BillingDaily 每日账单聚合（由 usage_logs 定时汇总生成，可重建）。
type BillingDaily struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Date             string    `gorm:"size:10;not null;uniqueIndex:uk_date_key" json:"date"`
	APIKeyID         uint64    `gorm:"not null;uniqueIndex:uk_date_key" json:"api_key_id"`
	RequestCount     int       `json:"request_count"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Cost             float64   `gorm:"type:decimal(12,6);not null;default:0" json:"cost"`
	CreatedAt        time.Time `json:"created_at"`
}

// TableName 指定表名。
func (BillingDaily) TableName() string { return "billing_daily" }
