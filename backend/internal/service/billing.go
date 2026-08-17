package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"gorm.io/gorm"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// BillingService 计费与对账：
// - Daily/Aggregate：usage_logs 实时聚合 + 预聚合表落库（归档）
// - ReconcileQuota：Redis 配额计数与 MySQL used_tokens 对账（ADR-005：以 MySQL 为准）
type BillingService struct {
	repo *repository.Repository
}

// NewBillingService 构造 BillingService。
func NewBillingService(repo *repository.Repository) *BillingService {
	return &BillingService{repo: repo}
}

// Daily 从 usage_logs 实时聚合最近 days 天的每日账单（按 date+api_key 分组）。
func (s *BillingService) Daily(ctx context.Context, days int) ([]model.BillingDaily, error) {
	var rows []model.BillingDaily
	since := time.Now().AddDate(0, 0, -days)
	err := s.repo.DB.Model(&model.UsageLog{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') AS date, api_key_id, COUNT(*) AS request_count, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens, COALESCE(SUM(cost),0) AS cost").
		Where("created_at >= ?", since).
		Group("DATE_FORMAT(created_at, '%Y-%m-%d'), api_key_id").
		Order("date DESC, api_key_id ASC").
		Scan(&rows).Error
	return rows, err
}

// Aggregate 将最近 days 天的聚合结果 upsert 进 billing_daily（预聚合归档表）。
func (s *BillingService) Aggregate(ctx context.Context, days int) (int, error) {
	rows, err := s.Daily(ctx, days)
	if err != nil {
		return 0, err
	}
	now := time.Now()
	for _, r := range rows {
		r.CreatedAt = now
		var exist model.BillingDaily
		err := s.repo.DB.Where("date = ? AND api_key_id = ?", r.Date, r.APIKeyID).First(&exist).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, err
			}
			if err := s.repo.DB.Create(&r).Error; err != nil {
				return 0, err
			}
			continue
		}
		r.ID = exist.ID
		if err := s.repo.DB.Save(&r).Error; err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// ReconcileQuota 对账 Redis 配额计数与 MySQL used_tokens（ADR-005：MySQL 为事实来源）。
// 返回检查数/修正数。仅对设置了配额（quota_tokens>0）的 Key 生效。
func (s *BillingService) ReconcileQuota(ctx context.Context) (checked, corrected int, err error) {
	var keys []model.ApiKey
	if err := s.repo.DB.Where("quota_tokens > 0").Find(&keys).Error; err != nil {
		return 0, 0, err
	}
	for _, k := range keys {
		checked++
		redisKey := "quota:" + strconv.FormatUint(k.ID, 10)
		redisVal, _ := s.repo.Redis.Get(ctx, redisKey).Int64()
		if redisVal != k.UsedTokens {
			if err := s.repo.Redis.Set(ctx, redisKey, k.UsedTokens, 0).Err(); err != nil {
				return checked, corrected, err
			}
			corrected++
			slog.Info("quota reconciled (mysql wins)",
				"api_key_id", k.ID, "redis", redisVal, "mysql", k.UsedTokens)
		}
	}
	return checked, corrected, nil
}

// Reconcile 一次完整对账：聚合账单 + 配额修正，返回摘要。供定时任务与手动触发共用。
type ReconcileResult struct {
	AggregatedRows int `json:"aggregated_rows"`
	KeysChecked    int `json:"keys_checked"`
	KeysCorrected  int `json:"keys_corrected"`
}

// Reconcile 执行聚合 + 配额对账。
func (s *BillingService) Reconcile(ctx context.Context, days int) (*ReconcileResult, error) {
	agg, err := s.Aggregate(ctx, days)
	if err != nil {
		return nil, err
	}
	checked, corrected, err := s.ReconcileQuota(ctx)
	if err != nil {
		return nil, err
	}
	return &ReconcileResult{AggregatedRows: agg, KeysChecked: checked, KeysCorrected: corrected}, nil
}

// StartTicker 启动定时对账协程（interval<=0 时不启动）。
func (s *BillingService) StartTicker(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		slog.Info("billing routine started", "interval", interval.String())
		for {
			select {
			case <-ctx.Done():
				slog.Info("billing routine stopped")
				return
			case <-t.C:
				res, err := s.Reconcile(ctx, 7)
				if err != nil {
					slog.Error("billing reconcile failed", "error", err)
					continue
				}
				slog.Info("billing reconcile done",
					"aggregated_rows", res.AggregatedRows,
					"keys_checked", res.KeysChecked,
					"keys_corrected", res.KeysCorrected)
			}
		}
	}()
}
