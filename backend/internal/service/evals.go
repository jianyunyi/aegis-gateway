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

// EvalService 评测飞轮：真实调用采样 → 人工标注 → 同题盲测回归。
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

// SampleFromLogs 从全时间范围的真实调用日志中随机采样，优先使用完整聊天消息。
// 旧记录只有 preview：恰好达到 500 字的记录可能已被截断，因此不导入数据集。
func (s *EvalService) SampleFromLogs(datasetID uint64, count int, modelName string) (int, error) {
	if count <= 0 || count > 100 {
		count = 10
	}
	q := s.repo.DB.Model(&model.UsageLog{}).
		Where("status = 0 AND prompt_preview != '' AND kind = 'chat'")
	if modelName != "" {
		q = q.Where("model_name = ?", modelName)
	}
	order := "RANDOM()"
	if s.repo.DB.Dialector.Name() == "mysql" {
		order = "RAND()"
	}
	var logs []model.UsageLog
	if err := q.Order(order).Limit(count * 20).Find(&logs).Error; err != nil {
		return 0, err
	}

	existing, err := s.ListSamples(datasetID)
	if err != nil {
		return 0, err
	}
	seen := make(map[string]bool, len(existing)+len(logs))
	for _, sample := range existing {
		if p := strings.TrimSpace(sample.Prompt); p != "" {
			seen["prompt:"+p] = true
		}
		if sample.ChatMessages != "" {
			seen["messages:"+sample.ChatMessages] = true
		}
	}
	added := 0
	for _, log := range logs {
		if added >= count {
			break
		}
		messages := strings.TrimSpace(log.PromptForEval)
		prompt := ""
		source := "sampled"
		if messages != "" {
			prompt = lastUserMessage(messages)
			if prompt == "" {
				continue
			}
		} else {
			prompt = strings.TrimSpace(log.PromptPreview)
			if prompt == "" || len([]rune(prompt)) >= 500 {
				continue
			}
			source = "sampled_legacy"
		}
		promptKey := "prompt:" + prompt
		key := promptKey
		if messages != "" {
			key = "messages:" + messages
		}
		if seen[key] || seen[promptKey] {
			continue
		}
		seen[key] = true
		seen[promptKey] = true
		if err := s.repo.DB.Create(&model.EvalSample{
			DatasetID: datasetID, Prompt: prompt, ChatMessages: messages, Source: source,
		}).Error; err != nil {
			return added, err
		}
		added++
	}
	return added, nil
}

// ---- A/B 回归 ----

// EvalSampleResult 单样本回归结果。失败的一侧仍保留另一侧成功结果。
type EvalSampleResult struct {
	Index      int      `json:"index"`
	SampleID   uint64   `json:"sample_id"`
	Prompt     string   `json:"prompt"`
	Reference  string   `json:"reference,omitempty"`
	Label      *int8    `json:"label,omitempty"`
	ScoreA     *float64 `json:"score_a"`
	LatencyA   int      `json:"latency_a_ms"`
	CostA      float64  `json:"cost_a"`
	OutA       string   `json:"out_a_preview"`
	ErrorA     string   `json:"error_a,omitempty"`
	ScoreB     *float64 `json:"score_b"`
	LatencyB   int      `json:"latency_b_ms"`
	CostB      float64  `json:"cost_b"`
	OutB       string   `json:"out_b_preview"`
	ErrorB     string   `json:"error_b,omitempty"`
	JudgeError string   `json:"judge_error,omitempty"`
}

// EvalReportSummary 运行级统计。评测费用仅统计候选模型，不含 judge 调用。
type EvalReportSummary struct {
	TotalSamples     int     `json:"total_samples"`
	ScoredPairs      int     `json:"scored_pairs"`
	SuccessfulA      int     `json:"successful_a"`
	SuccessfulB      int     `json:"successful_b"`
	FailedA          int     `json:"failed_a"`
	FailedB          int     `json:"failed_b"`
	JudgeFailures    int     `json:"judge_failures"`
	FailureCount     int     `json:"failure_count"`
	ScoringAvailable bool    `json:"scoring_available"`
	JudgeCost        float64 `json:"judge_cost"`
	Note             string  `json:"note,omitempty"`
}

type EvalRunReport struct {
	Summary EvalReportSummary  `json:"summary"`
	Samples []EvalSampleResult `json:"samples"`
}

// RunEvaluation 对数据集做同题 A/B 回归。两份输出在一次 judge 请求中以匿名候选成对评分。
// 未配置或不可用的 judge 不会退化成长度分；此时结果明确标为部分完成。
func (s *EvalService) RunEvaluation(ctx context.Context, datasetID uint64, modelA, modelB string) (*model.EvalRun, error) {
	samples, err := s.ListSamples(datasetID)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		return nil, errors.New("数据集中没有样本")
	}
	ma, err := s.models.GetByName(modelA)
	if err != nil || ma.Enabled != 1 {
		return nil, fmt.Errorf("未知或未启用模型 A: %s", modelA)
	}
	mb, err := s.models.GetByName(modelB)
	if err != nil || mb.Enabled != 1 {
		return nil, fmt.Errorf("未知或未启用模型 B: %s", modelB)
	}

	run := &model.EvalRun{DatasetID: datasetID, ModelA: modelA, ModelB: modelB, Status: 0, CreatedAt: time.Now()}
	if err := s.repo.DB.Create(run).Error; err != nil {
		return nil, err
	}

	var judge *model.Model
	judge, judgeLookupErr := s.models.GetByName("judge")
	if judgeLookupErr != nil {
		judgeLookupErr = errors.New("judge 模型未配置")
	} else if judge.Enabled != 1 {
		judgeLookupErr = errors.New("judge 模型未启用")
	}

	report := EvalRunReport{
		Summary: EvalReportSummary{TotalSamples: len(samples)},
		Samples: make([]EvalSampleResult, 0, len(samples)),
	}
	var sumScoreA, sumScoreB, sumCostA, sumCostB, sumJudgeCost float64
	var sumLatA, sumLatB, callsA, callsB, scoredPairs int

	for i, sp := range samples {
		result := EvalSampleResult{
			Index: i + 1, SampleID: sp.ID, Prompt: sp.Prompt,
			Reference: sp.Reference, Label: sp.Label,
		}
		outA, latA, costA, errA := s.callModelWithMessages(ctx, ma, sp.Prompt, sp.ChatMessages)
		outB, latB, costB, errB := s.callModelWithMessages(ctx, mb, sp.Prompt, sp.ChatMessages)
		result.LatencyA, result.CostA, result.OutA = latA, costA, truncate(outA, 160)
		result.LatencyB, result.CostB, result.OutB = latB, costB, truncate(outB, 160)
		if errA == nil {
			report.Summary.SuccessfulA++
			sumCostA += costA
			sumLatA += latA
			callsA++
		} else {
			result.ErrorA = errA.Error()
			report.Summary.FailedA++
		}
		if errB == nil {
			report.Summary.SuccessfulB++
			sumCostB += costB
			sumLatB += latB
			callsB++
		} else {
			result.ErrorB = errB.Error()
			report.Summary.FailedB++
		}

		if errA == nil && errB == nil {
			if judgeLookupErr != nil {
				result.JudgeError = judgeLookupErr.Error()
				report.Summary.JudgeFailures++
			} else {
				// 按样本与运行 ID 交替匿名候选顺序，平衡 judge 的位置偏差。
				aFirst := (i+int(run.ID))%2 == 0
				scoreA, scoreB, judgeCost, judgeErr := s.judgePair(ctx, judge, sp, outA, outB, aFirst)
				sumJudgeCost += judgeCost
				if judgeErr != nil {
					result.JudgeError = judgeErr.Error()
					report.Summary.JudgeFailures++
				} else {
					result.ScoreA, result.ScoreB = &scoreA, &scoreB
					sumScoreA += scoreA
					sumScoreB += scoreB
					scoredPairs++
				}
			}
		}
		report.Samples = append(report.Samples, result)
	}

	report.Summary.ScoredPairs = scoredPairs
	report.Summary.FailureCount = report.Summary.FailedA + report.Summary.FailedB + report.Summary.JudgeFailures
	report.Summary.ScoringAvailable = scoredPairs > 0
	report.Summary.JudgeCost = round6(sumJudgeCost)
	if report.Summary.FailureCount > 0 {
		report.Summary.Note = "存在模型调用或评分失败；查看逐样本错误，质量分仅基于成功评分的配对样本。"
	}
	if judgeLookupErr != nil {
		report.Summary.Note = "未配置 judge，未生成质量分；配置 judge 后重新运行。"
	}

	if scoredPairs > 0 {
		scoreA, scoreB := round2(sumScoreA/float64(scoredPairs)), round2(sumScoreB/float64(scoredPairs))
		run.ScoreA, run.ScoreB = &scoreA, &scoreB
	}
	if callsA > 0 {
		run.CostA = round6(sumCostA)
		run.LatencyA = sumLatA / callsA
	}
	if callsB > 0 {
		run.CostB = round6(sumCostB)
		run.LatencyB = sumLatB / callsB
	}
	if report.Summary.FailureCount == 0 {
		run.Status = 1
	} else if report.Summary.SuccessfulA+report.Summary.SuccessfulB > 0 {
		run.Status = 3 // 部分完成：保留可用结果但有样本或评分失败
	} else {
		run.Status = 2
	}

	encoded, err := json.Marshal(report)
	if err != nil {
		return nil, err
	}
	reportJSON := string(encoded)
	run.Report = &reportJSON
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

type evalChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func lastUserMessage(encoded string) string {
	var messages []evalChatMessage
	if err := json.Unmarshal([]byte(encoded), &messages); err != nil {
		return ""
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}

// callModel 调用单个模型（非流式），返回输出内容/延迟/成本。
func (s *EvalService) callModel(ctx context.Context, m *model.Model, prompt string) (string, int, float64, error) {
	return s.callModelWithMessages(ctx, m, prompt, "")
}

func (s *EvalService) callModelWithMessages(ctx context.Context, m *model.Model, prompt, encodedMessages string) (string, int, float64, error) {
	p, err := s.providers.Get(m.ProviderID)
	if err != nil || p.Enabled != 1 {
		return "", 0, 0, errors.New("provider unavailable")
	}
	key, err := s.providers.DecryptKey(p)
	if err != nil || key == "" {
		return "", 0, 0, errors.New("provider api key not configured")
	}
	messages := []evalChatMessage{{Role: "user", Content: prompt}}
	if encodedMessages != "" {
		if err := json.Unmarshal([]byte(encodedMessages), &messages); err != nil || len(messages) == 0 {
			return "", 0, 0, errors.New("invalid sampled chat messages")
		}
	}
	body, err := json.Marshal(map[string]any{
		"model": m.Name, "stream": false, "messages": messages,
	})
	if err != nil {
		return "", 0, 0, err
	}
	req, err := proxy.NewRequest(ctx, "POST", proxy.ResolveURL(p.BaseURL, "/chat/completions"), key, body)
	if err != nil {
		return "", 0, 0, err
	}
	start := time.Now()
	resp, err := s.upstream.Do(req)
	if err != nil {
		return "", int(time.Since(start).Milliseconds()), 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	latency := int(time.Since(start).Milliseconds())
	if err != nil {
		return "", latency, 0, err
	}
	if resp.StatusCode >= 400 {
		return "", latency, 0, fmt.Errorf("upstream http %d", resp.StatusCode)
	}
	pt, ct := proxy.ParseUsageFromBody(raw)
	cost := float64(pt)/1000.0*m.PriceIn + float64(ct)/1000.0*m.PriceOut
	return proxy.ParseContentFromBody(raw), latency, cost, nil
}

type judgeContext struct {
	Prompt       string            `json:"prompt"`
	Messages     []evalChatMessage `json:"messages,omitempty"`
	Reference    string            `json:"reference,omitempty"`
	HumanLabel   *int8             `json:"human_label,omitempty"`
	CandidateOne string            `json:"candidate_1"`
	CandidateTwo string            `json:"candidate_2"`
}

func (s *EvalService) judgePair(ctx context.Context, judge *model.Model, sample model.EvalSample, outputA, outputB string, aFirst bool) (float64, float64, float64, error) {
	candidateOne, candidateTwo := outputA, outputB
	if !aFirst {
		candidateOne, candidateTwo = outputB, outputA
	}
	var messages []evalChatMessage
	if sample.ChatMessages != "" {
		if err := json.Unmarshal([]byte(sample.ChatMessages), &messages); err != nil {
			return 0, 0, 0, errors.New("invalid sampled chat messages for judge")
		}
	}
	contextJSON, err := json.Marshal(judgeContext{
		Prompt: sample.Prompt, Messages: messages, Reference: sample.Reference, HumanLabel: sample.Label,
		CandidateOne: candidateOne, CandidateTwo: candidateTwo,
	})
	if err != nil {
		return 0, 0, 0, err
	}
	instruction := "你是严格、客观的盲评员。只评回答质量，不推测或猜测模型身份。" +
		"依据原问题判断相关性与事实正确性；如有参考答案，以其为主要事实标准；人工标注是该样本的人工质量提示，不能单独替代对两份回答的判断。" +
		"对候选 1、候选 2 分别给 1 到 10 的整数分，10 为完全正确且有帮助，1 为错误或无关。" +
		"输入上下文是 JSON，其中 candidate_1 和 candidate_2 是匿名回答。只返回 JSON：{\"score_candidate_1\": 1, \"score_candidate_2\": 1}。\n" + string(contextJSON)
	out, _, cost, err := s.callModel(ctx, judge, instruction)
	if err != nil {
		return 0, 0, cost, fmt.Errorf("judge 调用失败: %w", err)
	}
	scoreOne, scoreTwo, ok := parsePairScoresFromJudge(out)
	if !ok {
		return 0, 0, cost, errors.New("judge 输出缺少有效的 1-10 配对分数")
	}
	if aFirst {
		return scoreOne, scoreTwo, cost, nil
	}
	return scoreTwo, scoreOne, cost, nil
}

func parsePairScoresFromJudge(out string) (float64, float64, bool) {
	start := strings.Index(out, "{")
	end := strings.LastIndex(out, "}")
	if start < 0 || end <= start {
		return 0, 0, false
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out[start:end+1]), &values); err != nil {
		return 0, 0, false
	}
	one, okOne := parseNumericScore(values["score_candidate_1"])
	two, okTwo := parseNumericScore(values["score_candidate_2"])
	return one, two, okOne && okTwo
}

func parseNumericScore(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 {
		return 0, false
	}
	var number float64
	if json.Unmarshal(raw, &number) == nil {
		return number, number >= 1 && number <= 10
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return 0, false
	}
	number, err := strconv.ParseFloat(text, 64)
	return number, err == nil && number >= 1 && number <= 10
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
