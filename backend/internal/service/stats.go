package service

import (
	"context"
	"time"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// StatsService 观测统计：大盘概览、趋势、调用日志查询。
type StatsService struct {
	repo *repository.Repository
}

// NewStatsService 构造 StatsService。
func NewStatsService(repo *repository.Repository) *StatsService {
	return &StatsService{repo: repo}
}

// StatsOverview 今日概览。
type StatsOverview struct {
	TodayRequests int64   `json:"today_requests"`
	TodayCost     float64 `json:"today_cost"`
	TodayTokens   int64   `json:"today_tokens"`
	SuccessRate   float64 `json:"success_rate"` // 0~1
	AvgLatencyMs  float64 `json:"avg_latency_ms"`
}

// Overview 查询今日请求量/成本/token/成功率/平均延迟。
func (s *StatsService) Overview(ctx context.Context) (*StatsOverview, error) {
	startOfDay := time.Now().Truncate(24 * time.Hour)
	row := struct {
		Requests   int64
		Cost       float64
		Tokens     int64
		Success    int64
		AvgLatency float64
	}{}
	err := s.repo.DB.Model(&model.UsageLog{}).
		Select("COUNT(*) AS requests, COALESCE(SUM(cost),0) AS cost, COALESCE(SUM(total_tokens),0) AS tokens, COALESCE(SUM(CASE WHEN status = 0 THEN 1 ELSE 0 END),0) AS success, COALESCE(AVG(latency_ms),0) AS avg_latency").
		Where("created_at >= ?", startOfDay).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}
	rate := 0.0
	if row.Requests > 0 {
		rate = float64(row.Success) / float64(row.Requests)
	}
	return &StatsOverview{
		TodayRequests: row.Requests,
		TodayCost:     row.Cost,
		TodayTokens:   row.Tokens,
		SuccessRate:   rate,
		AvgLatencyMs:  row.AvgLatency,
	}, nil
}

// TrendPoint 趋势数据点。
type TrendPoint struct {
	Date        string  `json:"date"`
	Requests    int64   `json:"requests"`
	Cost        float64 `json:"cost"`
	Tokens      int64   `json:"tokens"`
	SuccessRate float64 `json:"success_rate"`
}

// Trends 最近 days 天的按日聚合趋势。
func (s *StatsService) Trends(ctx context.Context, days int) ([]TrendPoint, error) {
	var rows []TrendPoint
	since := time.Now().AddDate(0, 0, -days)
	err := s.repo.DB.Model(&model.UsageLog{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') AS date, COUNT(*) AS requests, COALESCE(SUM(cost),0) AS cost, COALESCE(SUM(total_tokens),0) AS tokens, COALESCE(SUM(CASE WHEN status = 0 THEN 1 ELSE 0 END),0) / COUNT(*) AS success_rate").
		Where("created_at >= ?", since).
		Group("DATE_FORMAT(created_at, '%Y-%m-%d')").
		Order("date ASC").
		Scan(&rows).Error
	return rows, err
}

// LogFilter 调用日志查询条件。
type LogFilter struct {
	Page      int
	PageSize  int
	ModelName string
	APIKeyID  uint64
	Status    string // 0 成功 / 4xx / 5xx
}

// Logs 分页查询调用日志。
func (s *StatsService) Logs(ctx context.Context, f LogFilter) ([]model.UsageLog, int64, error) {
	q := s.repo.DB.Model(&model.UsageLog{})
	if f.ModelName != "" {
		q = q.Where("model_name = ?", f.ModelName)
	}
	if f.APIKeyID > 0 {
		q = q.Where("api_key_id = ?", f.APIKeyID)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	var logs []model.UsageLog
	err := q.Order("id DESC").
		Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize).
		Find(&logs).Error
	return logs, total, err
}
