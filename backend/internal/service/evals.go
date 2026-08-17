package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/proxy"
	"aegis-gateway/internal/repository"
)

// EvalService 评测飞轮（PRD Epic-8）：
// 真实调用采样 → 人工打标 → A/B 模型回归评测（质量分 + 成本 + 延迟）。
type EvalService struct {
	repo      *repository.Repository
	providers *ProviderService
	models    *ModelService
	upstream  *proxy.UpstreamClient
}

// NewEvalService 构造 EvalService。
func NewEvalService(repo *repository.Repository, providers *ProviderService, models *ModelService, upstream *proxy.UpstreamClient) *EvalService {
	return &EvalService{repo: repo, providers: providers, models: models, upstream: upstream}
}

// ---- 数据集 ----

// CreateDataset 创建评测数据集。
func (s *EvalService) CreateDataset(name, desc string) (*model.EvalDataset, error) {
	ds := &model.EvalDataset{Name: name, Description: desc, Status: 1}
	if err := s.repo.DB.Create(ds).Error; err != nil {
		return nil, err
	}
	return ds, nil
}

// ListDatasets 列出数据集。
func (s *EvalService) ListDatasets() ([]model.EvalDataset, error) {
	var ds []model.EvalDataset
	err := s.repo.DB.Order("id DESC").Find(&ds).Error
	return ds, err
}

// ListSamples 列出数据集样本。
func (s *EvalService) ListSamples(datasetID uint64) ([]model.EvalSample, error) {
	var samples []model.EvalSample
	err := s.repo.DB.Where("dataset_id = ?", datasetID).Order("id ASC").Find(&samples).Error
	return samples, err
}

// AddSample 手动添加样本。
func (s *EvalService) AddSample(datasetID uint64, prompt, reference, source string) error {
	return s.repo.DB.Create(&model.EvalSample{
		DatasetID: datasetID, Prompt: prompt, Reference: reference, Source: source,
	}).Error
}

// LabelSample 人工打标：1 好 / 0 差。
func (s *EvalService) LabelSample(sampleID uint64, label int8) error {
	return s.repo.DB.Model(&model.EvalSample{}).Where("id = ?", sampleID).
		Update("label", label).Error
}

// SampleFromLogs 从真实调用日志采样（分层：按模型），去重后写入数据集。
func (s *EvalService) SampleFromLogs(datasetID uint64, count int, modelName string) (int, error) {
	if count <= 0 || count > 100 {
		count = 10
	}
	q := s.repo.DB.Model(&model.UsageLog{}).
		Where("status = 0 AND prompt_preview != '' AND kind = 'chat'")
	if modelName != "" {
		q = q.Where("model_name = ?", modelName)
	}
	var logs []model.UsageLog
	if err := q.Order("id DESC").Limit(count * 3).Find(&logs).Error; err != nil {
		return 0, err
	}

	seen := map[string]bool{}
	added := 0
	for _, l := range logs {
		if added >= count {
			break
		}
		p := strings.TrimSpace(l.PromptPreview)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if err := s.repo.DB.Create(&model.EvalSample{
			DatasetID: datasetID, Prompt: p, Source: "sampled",
		}).Error; err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// ---- A/B 回归 ----

// EvalSampleResult 单样本回归结果。
type EvalSampleResult struct {
	Index    int     `json:"index"`
	Prompt   string  `json:"prompt"`
	ScoreA   float64 `json:"score_a"`
	LatencyA int     `json:"latency_a_ms"`
	CostA    float64 `json:"cost_a"`
	OutA     string  `json:"out_a_preview"`
	ScoreB   float64 `json:"score_b"`
	LatencyB int     `json:"latency_b_ms"`
	CostB    float64 `json:"cost_b"`
	OutB     string  `json:"out_b_preview"`
}

// RunEvaluation 对数据集跑一次 A/B 回归：两个模型分别回答全部样本，
// 打分采用 LLM-as-judge（模型目录中存在名为 judge 的模型时）或启发式兜底，
// 汇总质量分/成本/延迟后写入 eval_runs。
func (s *EvalService) RunEvaluation(ctx context.Context, datasetID uint64, modelA, modelB string) (*model.EvalRun, error) {
	samples, err := s.ListSamples(datasetID)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, errors.New("数据集中没有样本")
	}
	ma, err := s.models.GetByName(modelA)
	if err != nil {
		return nil, fmt.Errorf("未知模型 A: %s", modelA)
	}
	mb, err := s.models.GetByName(modelB)
	if err != nil {
		return nil, fmt.Errorf("未知模型 B: %s", modelB)
	}

	run := &model.EvalRun{DatasetID: datasetID, ModelA: modelA, ModelB: modelB, Status: 0, CreatedAt: time.Now()}
	if err := s.repo.DB.Create(run).Error; err != nil {
		return nil, err
	}

	results := make([]EvalSampleResult, 0, len(samples))
	var (
		sumScoreA, sumScoreB, sumCostA, sumCostB float64
		sumLatA, sumLatB                         int
	)
	for i, sp := range samples {
		outA, latA, costA, errA := s.callModel(ctx, ma, sp.Prompt)
		outB, latB, costB, errB := s.callModel(ctx, mb, sp.Prompt)
		if errA != nil || errB != nil {
			continue // 单样本失败跳过，不影响整体报告
		}
		scoreA := s.judgeScore(ctx, outA)
		scoreB := s.judgeScore(ctx, outB)

		sumScoreA += scoreA
		sumScoreB += scoreB
		sumCostA += costA
		sumCostB += costB
		sumLatA += latA
		sumLatB += latB

		results = append(results, EvalSampleResult{
			Index: i, Prompt: truncate(sp.Prompt, 120),
			ScoreA: scoreA, LatencyA: latA, CostA: costA, OutA: truncate(outA, 160),
			ScoreB: scoreB, LatencyB: latB, CostB: costB, OutB: truncate(outB, 160),
		})
	}

	n := len(results)
	if n == 0 {
		run.Status = 2 // 失败
	} else {
		run.Status = 1
		run.ScoreA = round2(sumScoreA / float64(n))
		run.ScoreB = round2(sumScoreB / float64(n))
		run.CostA = round6(sumCostA)
		run.CostB = round6(sumCostB)
		run.LatencyA = sumLatA / n
		run.LatencyB = sumLatB / n
	}
	report, _ := json.Marshal(results)
	rs := string(report)
	run.Report = &rs
	now := time.Now()
	run.FinishedAt = &now

	if err := s.repo.DB.Save(run).Error; err != nil {
		return nil, err
	}
	return run, nil
}

// GetRun 查询评测运行。
func (s *EvalService) GetRun(id uint64) (*model.EvalRun, error) {
	var run model.EvalRun
	if err := s.repo.DB.First(&run, id).Error; err != nil {
		return nil, err
	}
	return &run, nil
}

// ListRuns 列出评测运行。
func (s *EvalService) ListRuns() ([]model.EvalRun, error) {
	var runs []model.EvalRun
	err := s.repo.DB.Order("id DESC").Limit(50).Find(&runs).Error
	return runs, err
}

// ---- 内部 ----

// callModel 调用单个模型（非流式），返回输出内容/延迟/成本。
func (s *EvalService) callModel(ctx context.Context, m *model.Model, prompt string) (string, int, float64, error) {
	p, err := s.providers.Get(m.ProviderID)
	if err != nil || p.Enabled != 1 {
		return "", 0, 0, errors.New("provider unavailable")
	}
	key, err := s.providers.DecryptKey(p)
	if err != nil || key == "" {
		return "", 0, 0, errors.New("provider api key not configured")
	}
	body, _ := json.Marshal(map[string]any{
		"model":    m.Name,
		"stream":   false,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	})
	req, err := proxy.NewRequest(ctx, "POST", proxy.ResolveURL(p.BaseURL, "/chat/completions"), key, body)
	if err != nil {
		return "", 0, 0, err
	}
	start := time.Now()
	resp, err := s.upstream.Do(req)
	if err != nil {
		return "", 0, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, 0, err
	}
	if resp.StatusCode >= 400 {
		return "", 0, 0, fmt.Errorf("upstream http %d", resp.StatusCode)
	}
	pt, ct := proxy.ParseUsageFromBody(raw)
	latency := int(time.Since(start).Milliseconds())
	cost := float64(pt)/1000.0*m.PriceIn + float64(ct)/1000.0*m.PriceOut
	return proxy.ParseContentFromBody(raw), latency, cost, nil
}

// judgeScore 质量评分：LLM-as-judge（judge 模型存在时）或启发式兜底。
func (s *EvalService) judgeScore(ctx context.Context, output string) float64 {
	// 扩展点：模型目录中配置名为 judge 的模型则启用 LLM-as-judge
	if judge, err := s.models.GetByName("judge"); err == nil {
		instruction := "你是评分员。请对下面的回答按质量打分（1-10），只输出 JSON：{\"score\": n}\n\n回答：\n" + truncate(output, 2000)
		out, _, _, err := s.callModel(ctx, judge, instruction)
		if err == nil {
			if score, ok := parseScoreFromJudge(out); ok {
				return score
			}
		}
	}
	// 启发式兜底：非空 + 长度适中 → 中上分
	n := len([]rune(strings.TrimSpace(output)))
	if n == 0 {
		return 0
	}
	score := 5.0 + float64(n)/400.0
	if score > 10 {
		score = 10
	}
	return math.Round(score*10) / 10
}

// parseScoreFromJudge 从 judge 输出解析 {"score": n}。
func parseScoreFromJudge(out string) (float64, bool) {
	start := strings.Index(out, "{")
	if start < 0 {
		return 0, false
	}
	end := strings.LastIndex(out, "}")
	if end <= start {
		return 0, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(out[start:end+1]), &m); err != nil {
		return 0, false
	}
	switch v := m["score"].(type) {
	case float64:
		return v, true
	case string:
		f, err := strconv.ParseFloat(v, 64)
		return f, err == nil
	}
	return 0, false
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
func round6(v float64) float64 { return math.Round(v*1e6) / 1e6 }
