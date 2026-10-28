package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
)

// CleanupExpiredPromptData removes prompt-bearing usage fields, sampled eval
// samples, and reports after the configured retention period.
func CleanupExpiredPromptData(ctx context.Context, repo *repository.Repository, retentionDays int) (int64, error) {
	if retentionDays < 1 {
		return 0, fmt.Errorf("prompt retention must be at least one day")
	}
	cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	var removed int64

	result := repo.DB.WithContext(ctx).Model(&model.UsageLog{}).
		Where("created_at < ? AND (prompt_preview IS NOT NULL OR prompt_for_eval IS NOT NULL)", cutoff).
		Updates(map[string]any{"prompt_preview": nil, "prompt_for_eval": nil})
	if result.Error != nil {
		return removed, result.Error
	}
	removed += result.RowsAffected

	result = repo.DB.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&model.EvalRun{})
	if result.Error != nil {
		return removed, result.Error
	}
	removed += result.RowsAffected

	result = repo.DB.WithContext(ctx).Where("created_at < ? AND source LIKE 'sampled%'", cutoff).Delete(&model.EvalSample{})
	if result.Error != nil {
		return removed, result.Error
	}
	removed += result.RowsAffected
	return removed, nil
}

// StartPromptRetention starts a daily cleanup loop and exits with ctx.
func StartPromptRetention(ctx context.Context, repo *repository.Repository, retentionDays int) {
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cleanupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
				if removed, err := CleanupExpiredPromptData(cleanupCtx, repo, retentionDays); err != nil {
					slog.Error("prompt retention cleanup failed")
				} else if removed > 0 {
					slog.Info("prompt retention cleanup completed", "removed_records", removed)
				}
				cancel()
			}
		}
	}()
}
