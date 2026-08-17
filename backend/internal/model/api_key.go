package model

import "time"

// ApiKey 调用方凭证。明文只在创建时返回一次，库中仅存 SHA-256 哈希与前缀。
type ApiKey struct {
	ID          uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string     `gorm:"size:64;not null" json:"name"`
	KeyHash     string     `gorm:"size:64;uniqueIndex;not null" json:"-"`
	KeyPrefix   string     `gorm:"size:16" json:"key_prefix"`
	UserID      uint64     `gorm:"not null;index" json:"user_id"`
	Status      int8       `gorm:"not null;default:1" json:"status"` // 1 启用 0 禁用
	QuotaTokens  int64      `gorm:"not null;default:0" json:"quota_tokens"` // 0 表示不限
	UsedTokens   int64      `gorm:"not null;default:0" json:"used_tokens"`  // 已用 token（M3 对账用）
	DefaultModel string     `gorm:"size:64" json:"default_model"`           // 规则路由：Key 级默认模型（可空）
	BudgetMonthly float64   `gorm:"type:decimal(12,4);not null;default:0" json:"budget_monthly"` // 月度预算（元），0=不限；4 位小数支持分级预算
	RPSLimit    int        `gorm:"not null;default:10" json:"rps_limit"`
	Burst       int        `gorm:"not null;default:20" json:"burst"`
	ExpiresAt   *time.Time `json:"expires_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName 指定表名。
func (ApiKey) TableName() string { return "api_keys" }
