package service

import (
	"context"
	"testing"
	"time"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCleanupExpiredPromptData(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UsageLog{}, &model.EvalSample{}, &model.EvalRun{}); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -40)
	fresh := time.Now()
	expiredAt := time.Now().UTC().Add(-time.Hour)
	logs := []model.UsageLog{
		{RequestID: "expired", ModelName: "m", Kind: "chat", PromptPreview: "old email@example.com", PromptForEval: `[{"role":"user","content":"old"}]`, CreatedAt: old},
		{RequestID: "fresh", ModelName: "m", Kind: "chat", PromptPreview: "keep", PromptForEval: "keep", CreatedAt: fresh},
	}
	if err := db.Create(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.EvalSample{
		{DatasetID: 1, Prompt: "legacy sampled", Source: "sampled", CreatedAt: old},
		{DatasetID: 1, Prompt: "legacy manual", Source: "manual", CreatedAt: old},
		{DatasetID: 1, Prompt: "expired manual", Source: "manual", CreatedAt: old, ExpiresAt: &expiredAt},
		{DatasetID: 1, Prompt: "expired sampled", Source: "sampled", CreatedAt: old, ExpiresAt: &expiredAt},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.EvalRun{DatasetID: 1, ModelA: "a", ModelB: "b", Status: 1, CreatedAt: old}).Error; err != nil {
		t.Fatal(err)
	}
	removed, err := CleanupExpiredPromptData(context.Background(), &repository.Repository{DB: db}, 30)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 4 {
		t.Fatalf("removed=%d, want one log, two explicitly expired samples and one old run", removed)
	}
	var expired model.UsageLog
	if err := db.Where("request_id = ?", "expired").First(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if expired.PromptPreview != "" || expired.PromptForEval != "" {
		t.Fatalf("expired prompt data remained: %+v", expired)
	}
	var freshRow model.UsageLog
	if err := db.Where("request_id = ?", "fresh").First(&freshRow).Error; err != nil {
		t.Fatal(err)
	}
	if freshRow.PromptPreview != "keep" || freshRow.PromptForEval != "keep" {
		t.Fatalf("fresh data was altered: %+v", freshRow)
	}
	var samples []model.EvalSample
	if err := db.Find(&samples).Error; err != nil {
		t.Fatal(err)
	}
	if len(samples) != 2 {
		t.Fatalf("both legacy samples without explicit expiry must remain, got %+v", samples)
	}
	for _, sample := range samples {
		if sample.ExpiresAt != nil || (sample.Source != "manual" && sample.Source != "sampled") {
			t.Fatalf("unexpected remaining sample; historical NULL expiry must be preserved: %+v", sample)
		}
	}
	var runs int64
	if err := db.Model(&model.EvalRun{}).Count(&runs).Error; err != nil || runs != 0 {
		t.Fatalf("old eval report remains: count=%d err=%v", runs, err)
	}
}
