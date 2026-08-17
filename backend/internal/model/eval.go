package model

import "time"

// EvalDataset 评测数据集（样本集合）。
type EvalDataset struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"size:64;not null" json:"name"`
	Description string    `gorm:"size:255" json:"description"`
	Status      int8      `gorm:"not null;default:0" json:"status"` // 0 草稿 1 可用
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名。
func (EvalDataset) TableName() string { return "eval_datasets" }

// EvalSample 评测样本：来自真实调用采样或人工录入。
type EvalSample struct {
	ID        uint64    `gorm:"primaryKey;autoIncrement" json:"id"`
	DatasetID uint64    `gorm:"not null;index" json:"dataset_id"`
	Prompt    string    `gorm:"type:text;not null" json:"prompt"`
	Reference string    `gorm:"type:text" json:"reference"` // 参考答案（可选）
	Source    string    `gorm:"size:16;not null;default:manual" json:"source"` // sampled / manual
	Label     *int8     `json:"label"`                     // 人工标注：1 好 0 差 NULL 未标
	CreatedAt time.Time `json:"created_at"`
}

// TableName 指定表名。
func (EvalSample) TableName() string { return "eval_samples" }

// EvalRun 一次 A/B 回归评测的运行记录。
type EvalRun struct {
	ID         uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	DatasetID  uint64     `gorm:"not null;index" json:"dataset_id"`
	ModelA     string     `gorm:"size:64;not null" json:"model_a"`
	ModelB     string     `gorm:"size:64;not null" json:"model_b"`
	Status     int8       `gorm:"not null;default:0" json:"status"` // 0 运行中 1 完成 2 失败
	ScoreA     float64    `gorm:"type:decimal(5,2)" json:"score_a"`
	ScoreB     float64    `gorm:"type:decimal(5,2)" json:"score_b"`
	CostA      float64    `gorm:"type:decimal(12,6)" json:"cost_a"`
	CostB      float64    `gorm:"type:decimal(12,6)" json:"cost_b"`
	LatencyA   int        `json:"latency_a"` // 平均延迟 ms
	LatencyB   int        `json:"latency_b"`
	Report     *string    `gorm:"type:json" json:"report"` // 详细报告（逐样本结果）；nil 时不写入
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at"`
}

// TableName 指定表名。
func (EvalRun) TableName() string { return "eval_runs" }
