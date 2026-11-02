package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aegis-gateway/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// EvalSampleRetention is the initial expiry for newly created evaluation
// samples. Historical rows are deliberately not assigned an expiry by migration.
const EvalSampleRetention = 30 * 24 * time.Hour

// ManualEvalSampleRetention is retained as the manual-sample policy name.
const ManualEvalSampleRetention = EvalSampleRetention

var ErrLegacySampleRequiresRehearsal = errors.New("legacy evaluation samples must be inventoried and backed up in the isolated maintenance tool before deletion")

// AddManualSample adds a synthetic or fully de-identified manual sample only
// after the operator explicitly acknowledges the data-handling requirement.
func (s *EvalService) AddManualSample(datasetID uint64, prompt, reference string, policyConfirmed bool) error {
	if !policyConfirmed {
		return errors.New("manual evaluation samples require synthetic or fully de-identified content")
	}
	if datasetID == 0 || strings.TrimSpace(prompt) == "" {
		return errors.New("dataset_id and prompt are required")
	}
	now := time.Now().UTC()
	expiresAt := now.Add(ManualEvalSampleRetention)
	return s.repo.DB.Create(&model.EvalSample{
		DatasetID:               datasetID,
		Prompt:                  strings.TrimSpace(prompt),
		Reference:               reference,
		Source:                  "manual",
		CreatedAt:               now,
		ExpiresAt:               &expiresAt,
		ManualPolicyConfirmedAt: &now,
	}).Error
}

// DeleteSample removes exactly one sample and scrubs its text from stored run
// reports in the same dataset. Aggregate scores and non-text metrics remain.
func (s *EvalService) DeleteSample(sampleID uint64) error {
	return s.DeleteSampleContext(context.Background(), sampleID)
}

// DeleteSampleContext is the context-aware form used by retention cleanup.
func (s *EvalService) DeleteSampleContext(ctx context.Context, sampleID uint64) error {
	if sampleID == 0 {
		return errors.New("sample id must be positive")
	}
	return s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sample model.EvalSample
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&sample, "id = ?", sampleID).Error
		if err != nil {
			return err
		}
		if err := redactSampleFromReports(tx, sample); err != nil {
			return fmt.Errorf("redact evaluation reports for sample %d: %w", sampleID, err)
		}
		result := tx.Delete(&sample)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// DeleteSampleFromAdmin protects pre-migration samples from bypassing
// the inventory/backup/explicit-ID maintenance workflow.
func (s *EvalService) DeleteSampleFromAdmin(sampleID uint64) error {
	var sample model.EvalSample
	if err := s.repo.DB.Select("id", "source", "expires_at").First(&sample, "id = ?", sampleID).Error; err != nil {
		return err
	}
	if sample.ExpiresAt == nil {
		return ErrLegacySampleRequiresRehearsal
	}
	return s.DeleteSample(sampleID)
}

func redactSampleFromReports(tx *gorm.DB, sample model.EvalSample) error {
	var runs []model.EvalRun
	if err := tx.Where("dataset_id = ? AND report IS NOT NULL", sample.DatasetID).Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		redacted, changed := redactEvalReport(run.Report, sample)
		if !changed {
			continue
		}
		if err := tx.Model(&model.EvalRun{}).Where("id = ?", run.ID).Update("report", redacted).Error; err != nil {
			return err
		}
	}
	return nil
}

func redactEvalReport(raw *string, sample model.EvalSample) (*string, bool) {
	if raw == nil {
		return nil, false
	}
	redact := func(result *EvalSampleResult) {
		result.Prompt = ""
		result.Reference = ""
		result.Label = nil
		result.OutA = ""
		result.OutB = ""
		result.ErrorA = ""
		result.ErrorB = ""
		result.JudgeError = ""
	}
	matches := func(result EvalSampleResult) bool {
		return result.SampleID == sample.ID || (sample.Prompt != "" && result.Prompt == sample.Prompt)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal([]byte(*raw), &object); err == nil {
		if encodedSamples, ok := object["samples"]; ok {
			var samples []EvalSampleResult
			if json.Unmarshal(encodedSamples, &samples) == nil {
				changed := false
				for i := range samples {
					if matches(samples[i]) {
						redact(&samples[i])
						changed = true
					}
				}
				if changed {
					object["samples"], _ = json.Marshal(samples)
					encoded, err := json.Marshal(object)
					if err == nil {
						value := string(encoded)
						return &value, true
					}
				}
				return raw, false
			}
		}
	}

	// Older reports stored the result rows as a top-level array.
	var legacy []EvalSampleResult
	if err := json.Unmarshal([]byte(*raw), &legacy); err == nil && legacy != nil {
		changed := false
		for i := range legacy {
			if matches(legacy[i]) {
				redact(&legacy[i])
				changed = true
			}
		}
		if changed {
			encoded, err := json.Marshal(legacy)
			if err == nil {
				value := string(encoded)
				return &value, true
			}
		}
		return raw, false
	}

	// Unknown legacy report shapes are discarded if they contain this sample's
	// identifying text; this fails closed without altering unrelated reports.
	encoded := *raw
	containsText := (len([]rune(sample.Prompt)) >= 8 && strings.Contains(encoded, sample.Prompt)) ||
		(sample.Reference != "" && len([]rune(sample.Reference)) >= 8 && strings.Contains(encoded, sample.Reference))
	if containsText {
		return nil, true
	}
	return raw, false
}
