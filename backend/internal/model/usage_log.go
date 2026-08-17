package model

import "time"

// UsageLog 每次调用的全量日志，是观测与计费的事实来源。
type UsageLog struct {
	ID               uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	RequestID        string    `gorm:"size:32;uniqueIndex;not null" json:"request_id"`
	APIKeyID         uint64    `gorm:"not null;index:idx_key_time,priority:1" json:"api_key_id"`
	UserID           uint64    `json:"user_id"`
	ProviderID       uint64    `json:"provider_id"`
	ModelName        string    `gorm:"size:64;not null;index:idx_model_time,priority:1" json:"model_name"`
	Kind             string    `gorm:"size:16;not null" json:"kind"` // chat / completion / embedding
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Cost             float64   `gorm:"type:decimal(12,6);not null;default:0" json:"cost"` // 元
	LatencyMs        int       `json:"latency_ms"`
	TTFTMs           *int      `json:"ttft_ms"` // 流式首字延迟
	Status           int16     `gorm:"not null;default:0" json:"status"` // 0 成功 / 4xx / 5xx
	ErrorCode        string    `gorm:"size:32" json:"error_code"`
	Cached           int8      `gorm:"not null;default:0" json:"cached"`
	RoutedBy         string    `gorm:"size:16" json:"routed_by"` // manual / rule / heuristic / llm
	UpstreamModel    string    `gorm:"size:64" json:"upstream_model"`
	PromptPreview    string    `gorm:"size:500" json:"prompt_preview"` // 截断的 prompt（评测采样用；企业版需脱敏）
	CreatedAt        time.Time `gorm:"index:idx_key_time,priority:2;index:idx_model_time,priority:2" json:"created_at"`
}

// TableName 指定表名。
func (UsageLog) TableName() string { return "usage_logs" }
