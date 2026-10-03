package service

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

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
	err := s.repo.DB.WithContext(ctx).Model(&model.UsageLog{}).
		Select("DATE_FORMAT(created_at, '%Y-%m-%d') AS date, api_key_id, COUNT(*) AS request_count, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens, COALESCE(SUM(cost),0) AS cost").
		Where("created_at >= ?", since).
		Group("DATE_FORMAT(created_at, '%Y-%m-%d'), api_key_id").
		Order("date DESC, api_key_id ASC").
		Scan(&rows).Error
	return rows, err
}

// Aggregate 将最近 days 天的聚合结果 upsert 进 billing_daily（预聚合归档表）。
func (s *BillingService) Aggregate(ctx context.Context, days int) (int, error) {
	var count int
	err := s.repo.DB.WithContext(ctx).Connection(func(db *gorm.DB) error {
		var acquired int
		if err := db.Raw("SELECT GET_LOCK(?, 0)", "aegis:billing:aggregate").Scan(&acquired).Error; err != nil {
			return err
		}
		if acquired != 1 {
			return nil
		}
		defer func() {
			// The caller may have cancelled; release on the same pinned session.
			releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			db.WithContext(releaseCtx).Exec("SELECT RELEASE_LOCK(?)", "aegis:billing:aggregate")
		}()
		local := NewBillingService(&repository.Repository{DB: db, Redis: s.repo.Redis})
		var err error
		count, err = local.aggregate(ctx, days)
		return err
	})
	return count, err
}

func (s *BillingService) aggregate(ctx context.Context, days int) (int, error) {
	rows, err := s.Daily(ctx, days)
	if err != nil {
		return 0, err
	}
	for _, row := range rows {
		row.CreatedAt = time.Now()
		if err := s.repo.DB.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "date"}, {Name: "api_key_id"}}, DoUpdates: clause.AssignmentColumns([]string{"request_count", "prompt_tokens", "completion_tokens", "total_tokens", "cost"})}).Create(&row).Error; err != nil {
			return 0, err
		}
	}

	return len(rows), nil
}

// ReconcileQuota 对账 Redis 配额计数与 MySQL used_tokens（ADR-005：MySQL 为事实来源）。
// 返回检查数/修正数。仅对设置了配额（quota_tokens>0）的 Key 生效。
func (s *BillingService) ReconcileQuota(ctx context.Context) (checked, corrected int, err error) {
	var keys []model.ApiKey
	if err := s.repo.DB.WithContext(ctx).Where("quota_tokens > 0").Find(&keys).Error; err != nil {
		return 0, 0, err
	}
	for _, key := range keys {
		checked++
		changed, err := s.reconcileQuotaKey(ctx, key.ID)
		if err != nil {
			return checked, corrected, err
		}
		if changed {
			corrected++
		}
	}

	return checked, corrected, nil
}

// WATCH fences concurrent projections and reconcilers without holding a MySQL
// row lock across Redis I/O. Read Redis before MySQL, then compare-and-set.
func (s *BillingService) reconcileQuotaKey(parent context.Context, id uint64) (bool, error) {
	ctx, cancel := context.WithTimeout(parent, time.Second)
	defer cancel()
	redisKey := "quota:" + strconv.FormatUint(id, 10)
	for {
		changed := false
		err := s.repo.Redis.Watch(ctx, func(tx *redis.Tx) error {
			val, err := tx.Get(ctx, redisKey).Int64()
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			missing := errors.Is(err, redis.Nil)
			var current model.ApiKey
			if err := s.repo.DB.WithContext(ctx).First(&current, id).Error; err != nil {
				return err
			}
			if !missing && val == current.UsedTokens {
				return nil
			}
			_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, redisKey, current.UsedTokens, 0)
				return nil
			})
			changed = err == nil
			return err
		}, redisKey)
		if !errors.Is(err, redis.TxFailedErr) {
			return changed, err
		}
		if err := ctx.Err(); err != nil {
			return false, err
		}
	}
}

// Reconcile 一次完整对账：聚合账单 + 配额修正，返回摘要。供定时任务与手动触发共用。
type ReconcileResult struct {
	AggregatedRows int `json:"aggregated_rows"`
	KeysChecked    int `json:"keys_checked"`
	KeysCorrected  int `json:"keys_corrected"`
}

// Reconcile 执行聚合 + 配额对账。
func (s *BillingService) Reconcile(ctx context.Context, days int) (*ReconcileResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
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
