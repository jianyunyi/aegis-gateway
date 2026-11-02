package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAddManualSampleRequiresAcknowledgementAndExpiresAfterThirtyDays(t *testing.T) {
	db := newEvalRetentionTestDB(t)
	service := &EvalService{repo: &repository.Repository{DB: db}}
	if err := service.AddManualSample(1, "synthetic prompt", "reference", false); err == nil {
		t.Fatal("manual sample accepted without data-policy acknowledgement")
	}
	var count int64
	if err := db.Model(&model.EvalSample{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("unconfirmed insert count=%d err=%v", count, err)
	}
	if err := service.AddManualSample(1, "synthetic prompt", "reference", true); err != nil {
		t.Fatalf("add confirmed manual sample: %v", err)
	}
	var sample model.EvalSample
	if err := db.First(&sample).Error; err != nil {
		t.Fatal(err)
	}
	if sample.ExpiresAt == nil || sample.ManualPolicyConfirmedAt == nil {
		t.Fatalf("new manual sample lacks retention/audit timestamps: %+v", sample)
	}
	if delta := sample.ExpiresAt.Sub(sample.CreatedAt); delta < ManualEvalSampleRetention-time.Second || delta > ManualEvalSampleRetention+time.Second {
		t.Fatalf("expiry delta %s, want %s", delta, ManualEvalSampleRetention)
	}
}

func TestDeleteSampleRedactsOnlyMatchingStoredReportText(t *testing.T) {
	db := newEvalRetentionTestDB(t)
	service := &EvalService{repo: &repository.Repository{DB: db}}
	if err := service.AddManualSample(7, "synthetic alpha prompt", "alpha answer", true); err != nil {
		t.Fatal(err)
	}
	if err := service.AddManualSample(7, "synthetic beta prompt", "beta answer", true); err != nil {
		t.Fatal(err)
	}
	var samples []model.EvalSample
	if err := db.Order("id ASC").Find(&samples).Error; err != nil {
		t.Fatal(err)
	}
	results := []EvalSampleResult{
		{Index: 1, SampleID: samples[0].ID, Prompt: samples[0].Prompt, Reference: samples[0].Reference, OutA: "alpha output", OutB: "alpha output B", ErrorA: "alpha error", JudgeError: "alpha judge error", ScoreA: floatPtr(1.0)},
		{Index: 2, SampleID: samples[1].ID, Prompt: samples[1].Prompt, Reference: samples[1].Reference, OutA: "beta output", OutB: "beta output B", ScoreA: floatPtr(2.0)},
	}
	reportJSON, err := json.Marshal(EvalRunReport{Summary: EvalReportSummary{TotalSamples: 2}, Samples: results})
	if err != nil {
		t.Fatal(err)
	}
	report := string(reportJSON)
	if err := db.Create(&model.EvalRun{DatasetID: 7, ModelA: "a", ModelB: "b", Status: 1, CreatedAt: time.Now(), Report: &report}).Error; err != nil {
		t.Fatal(err)
	}

	if err := service.DeleteSample(samples[0].ID); err != nil {
		t.Fatalf("delete one sample: %v", err)
	}
	if err := service.DeleteSample(samples[0].ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("second delete err=%v, want record not found", err)
	}
	var remaining []model.EvalSample
	if err := db.Find(&remaining).Error; err != nil || len(remaining) != 1 || remaining[0].ID != samples[1].ID {
		t.Fatalf("unexpected remaining samples: %+v err=%v", remaining, err)
	}
	var run model.EvalRun
	if err := db.First(&run).Error; err != nil {
		t.Fatal(err)
	}
	var saved EvalRunReport
	if err := json.Unmarshal([]byte(*run.Report), &saved); err != nil {
		t.Fatalf("decode scrubbed report: %v", err)
	}
	if len(saved.Samples) != 2 || saved.Samples[0].Prompt != "" || saved.Samples[0].Reference != "" || saved.Samples[0].OutA != "" || saved.Samples[0].OutB != "" || saved.Samples[0].ErrorA != "" || saved.Samples[0].JudgeError != "" || saved.Samples[0].ScoreA == nil {
		t.Fatalf("deleted sample text was not scrubbed or numeric result was lost: %+v", saved.Samples[0])
	}
	if saved.Samples[1].Prompt != samples[1].Prompt || saved.Samples[1].Reference != samples[1].Reference {
		t.Fatalf("unrelated sample report was changed: %+v", saved.Samples[1])
	}
}

func TestAdminDeletionRequiresMaintenanceBackupForLegacySamples(t *testing.T) {
	db := newEvalRetentionTestDB(t)
	service := &EvalService{repo: &repository.Repository{DB: db}}
	legacy := []model.EvalSample{
		{DatasetID: 1, Prompt: "legacy manual sample", Source: "manual", CreatedAt: time.Now().Add(-90 * 24 * time.Hour)},
		{DatasetID: 1, Prompt: "legacy sampled record", Source: "sampled", CreatedAt: time.Now().Add(-90 * 24 * time.Hour)},
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	for _, sample := range legacy {
		if err := service.DeleteSampleFromAdmin(sample.ID); !errors.Is(err, ErrLegacySampleRequiresRehearsal) {
			t.Fatalf("legacy %s sample delete err=%v, want maintenance-only error", sample.Source, err)
		}
	}
	var remaining int64
	if err := db.Model(&model.EvalSample{}).Count(&remaining).Error; err != nil || remaining != 2 {
		t.Fatalf("legacy samples should remain, count=%d err=%v", remaining, err)
	}

	if err := service.AddManualSample(1, "new synthetic sample", "", true); err != nil {
		t.Fatal(err)
	}
	var created model.EvalSample
	if err := db.Where("prompt = ?", "new synthetic sample").First(&created).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteSampleFromAdmin(created.ID); err != nil {
		t.Fatalf("delete new expiring sample through admin: %v", err)
	}
}

func newEvalRetentionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.EvalSample{}, &model.EvalRun{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func floatPtr(value float64) *float64 { return &value }
