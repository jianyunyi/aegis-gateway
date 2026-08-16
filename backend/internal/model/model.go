package model

import "time"

// Model 模型目录与计价信息。Tier 用于语义路由分级（cheap/normal/strong）。
type Model struct {
	ID            uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	ProviderID    uint64    `gorm:"not null;index" json:"provider_id"`
	Name          string    `gorm:"size:64;not null" json:"name"` // 上游模型名，如 deepseek-chat
	DisplayName   string    `gorm:"size:64" json:"display_name"`
	Tier          string    `gorm:"size:16;not null;default:normal" json:"tier"`
	ContextWindow int       `gorm:"not null;default:8192" json:"context_window"`
	PriceIn       float64   `gorm:"type:decimal(10,6);not null;default:0" json:"price_in"`  // 元 / 1K 输入 token
	PriceOut      float64   `gorm:"type:decimal(10,6);not null;default:0" json:"price_out"` // 元 / 1K 输出 token
	Enabled       int8      `gorm:"not null;default:1" json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (Model) TableName() string { return "models" }
