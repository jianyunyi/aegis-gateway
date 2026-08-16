package model

import "time"

// Provider 上游模型提供商（DeepSeek / OpenAI / Ollama 等）。
type Provider struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:32;uniqueIndex;not null" json:"name"`
	BaseURL   string    `gorm:"size:255;not null" json:"base_url"`
	APIKeyEnc string    `gorm:"size:512" json:"-"` // AES-256 加密存储，绝不明文
	Enabled   int8      `gorm:"not null;default:1" json:"enabled"`
	Priority  int       `gorm:"not null;default:0" json:"priority"` // 故障切换优先级，小者优先
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (Provider) TableName() string { return "providers" }
