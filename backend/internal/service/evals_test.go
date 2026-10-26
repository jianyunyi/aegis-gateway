package service

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"aegis-gateway/internal/model"
	"aegis-gateway/internal/proxy"
	"aegis-gateway/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type regressionFixture struct {
	Name      string            `json:"name"`
	Prompt    string            `json:"prompt"`
	Reference string            `json:"reference"`
	Label     int8              `json:"label"`
	Messages  []evalChatMessage `json:"messages"`
}

type recordedChatRequest struct {
	Model    string            `json:"model"`
	Messages []evalChatMessage `json:"messages"`
}

type recordedJudgeContext struct {
	Prompt       string            `json:"prompt"`
	Messages     []evalChatMessage `json:"messages"`
	Reference    string            `json:"reference"`
	HumanLabel   *int8             `json:"human_label"`
	CandidateOne string            `json:"candidate_1"`
	CandidateTwo string            `json:"candidate_2"`
}

type evalTestHarness struct {
	service       *EvalService
	db            *gorm.DB
	datasetID     uint64
	fixtures      []regressionFixture
	server        *httptest.Server
	mu            sync.Mutex
	candidateReqs []recordedChatRequest
	judgeContexts []recordedJudgeContext
	withJudge     bool
	failSecondB   bool
}

func loadEvalRegressionFixtures(t *testing.T) []regressionFixture {
	t.Helper()
	path := filepath.Join("testdata", "eval_regression.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixed eval regression set: %v", err)
	}
	var fixtures []regressionFixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatalf("decode fixed eval regression set: %v", err)
	}
	if len(fixtures) < 2 {
		t.Fatalf("fixed regression set must include at least two cases")
	}
	return fixtures
}

func newEvalTestHarness(t *testing.T, withJudge, failSecondB bool) *evalTestHarness {
	t.Helper()
	h := &evalTestHarness{
		fixtures:    loadEvalRegressionFixtures(t),
		withJudge:   withJudge,
		failSecondB: failSecondB,
	}
	h.server = httptest.NewServer(http.HandlerFunc(h.serveModel))
	t.Cleanup(h.server.Close)

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Provider{}, &model.Model{}, &model.EvalDataset{}, &model.EvalSample{}, &model.EvalRun{}, &model.UsageLog{}); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}
	h.db = db
	repo := &repository.Repository{DB: db}
	providers := NewProviderService(repo, "eval-test-master-secret")
	provider, err := providers.Create("eval-upstream", h.server.URL, "stub-token", 1, 0)
	if err != nil {
		t.Fatalf("create test provider: %v", err)
	}
	models := NewModelService(repo)
	for _, candidate := range []struct {
		name     string
		priceIn  float64
		priceOut float64
	}{
		{name: "candidate-a", priceIn: 0.01, priceOut: 0.02},
		{name: "candidate-b", priceIn: 0.03, priceOut: 0.04},
	} {
		if err := models.Create(&model.Model{
			ProviderID: provider.ID, Name: candidate.name, PriceIn: candidate.priceIn,
			PriceOut: candidate.priceOut, Enabled: 1,
		}); err != nil {
			t.Fatalf("create candidate model %s: %v", candidate.name, err)
		}
	}
	if withJudge {
		if err := models.Create(&model.Model{
			ProviderID: provider.ID, Name: "judge", PriceIn: 0.05, PriceOut: 0.06, Enabled: 1,
		}); err != nil {
			t.Fatalf("create judge model: %v", err)
		}
	}
	dataset := &model.EvalDataset{Name: "fixed-regression", Description: "local test set", Status: 1}
	if err := db.Create(dataset).Error; err != nil {
		t.Fatalf("create eval dataset: %v", err)
	}
	for _, fixture := range h.fixtures {
		messages, err := json.Marshal(fixture.Messages)
		if err != nil {
			t.Fatalf("encode messages: %v", err)
		}
		label := fixture.Label
		if err := db.Create(&model.EvalSample{
			DatasetID: dataset.ID, Prompt: fixture.Prompt, ChatMessages: string(messages),
			Reference: fixture.Reference, Label: &label, Source: "manual",
		}).Error; err != nil {
			t.Fatalf("create eval sample: %v", err)
		}
	}
	h.datasetID = dataset.ID
	h.service = NewEvalService(repo, providers, models, proxy.NewUpstreamClient(3*time.Second))
	return h
}

func (h *evalTestHarness) serveModel(w http.ResponseWriter, r *http.Request) {
	var request recordedChatRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if request.Model == "judge" {
		if len(request.Messages) == 0 {
			http.Error(w, "missing judge prompt", http.StatusBadRequest)
			return
		}
		instruction := request.Messages[0].Content
		start := strings.Index(instruction, "{\"prompt\"")
		if start < 0 {
			http.Error(w, "judge context missing", http.StatusBadRequest)
			return
		}
		var judgeContext recordedJudgeContext
		if err := json.Unmarshal([]byte(instruction[start:]), &judgeContext); err != nil {
			http.Error(w, "invalid judge context", http.StatusBadRequest)
			return
		}
		h.mu.Lock()
		h.judgeContexts = append(h.judgeContexts, judgeContext)
		h.mu.Unlock()
		if strings.Contains(instruction, "candidate-a") || strings.Contains(instruction, "candidate-b") {
			http.Error(w, "candidate identity leaked to judge", http.StatusBadRequest)
			return
		}
		scoreOne, scoreTwo := 2, 2
		if strings.HasPrefix(judgeContext.CandidateOne, "correct:") {
			scoreOne = 9
		}
		if strings.HasPrefix(judgeContext.CandidateTwo, "correct:") {
			scoreTwo = 9
		}
		content := fmt.Sprintf(`{"score_candidate_1":%d,"score_candidate_2":%d}`, scoreOne, scoreTwo)
		writeChatResponse(w, content)
		return
	}

	prompt := lastUserMessageFromMessages(request.Messages)
	h.mu.Lock()
	h.candidateReqs = append(h.candidateReqs, request)
	h.mu.Unlock()
	fixture, ok := h.fixtureFor(prompt)
	if !ok {
		http.Error(w, "prompt does not match fixed regression set", http.StatusBadRequest)
		return
	}
	if request.Model == "candidate-b" && h.failSecondB && prompt == h.fixtures[0].Prompt {
		http.Error(w, "injected candidate failure", http.StatusServiceUnavailable)
		return
	}
	if request.Model == "candidate-a" {
		writeChatResponse(w, "correct: "+fixture.Reference)
		return
	}
	if request.Model == "candidate-b" {
		writeChatResponse(w, "incorrect: "+strings.Repeat("This is a deliberately verbose but wrong answer. ", 8))
		return
	}
	http.Error(w, "unknown model", http.StatusBadRequest)
}

func (h *evalTestHarness) fixtureFor(prompt string) (regressionFixture, bool) {
	for _, fixture := range h.fixtures {
		if fixture.Prompt == prompt {
			return fixture, true
		}
	}
	return regressionFixture{}, false
}

func lastUserMessageFromMessages(messages []evalChatMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

func writeChatResponse(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%q}}],"usage":{"prompt_tokens":10,"completion_tokens":20}}`, content)
}

func decodeEvalReport(t *testing.T, run *model.EvalRun) EvalRunReport {
	t.Helper()
	if run.Report == nil {
		t.Fatal("run report is nil")
	}
	var report EvalRunReport
	if err := json.Unmarshal([]byte(*run.Report), &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return report
}

func TestRunEvaluationFixedRegressionSetUsesPairedBlindScores(t *testing.T) {
	h := newEvalTestHarness(t, true, false)
	run, err := h.service.RunEvaluation(context.Background(), h.datasetID, "candidate-a", "candidate-b")
	if err != nil {
		t.Fatalf("run evaluation: %v", err)
	}
	if run.Status != 1 {
		t.Fatalf("status = %d, want complete (1)", run.Status)
	}
	if run.ScoreA == nil || *run.ScoreA != 9 || run.ScoreB == nil || *run.ScoreB != 2 {
		t.Fatalf("scores = %v / %v, want 9 / 2", run.ScoreA, run.ScoreB)
	}
	if math.Abs(run.CostA-0.001) > 1e-9 || math.Abs(run.CostB-0.0022) > 1e-9 {
		t.Fatalf("costs = %.6f / %.6f, want 0.001000 / 0.002200", run.CostA, run.CostB)
	}
	report := decodeEvalReport(t, run)
	if strings.Contains(*run.Report, "chat_messages") {
		t.Fatal("run report exposed internal full chat messages")
	}
	if report.Summary.TotalSamples != 2 || report.Summary.ScoredPairs != 2 || report.Summary.FailureCount != 0 {
		t.Fatalf("unexpected report summary: %+v", report.Summary)
	}
	if math.Abs(report.Summary.JudgeCost-0.0034) > 1e-9 {
		t.Fatalf("judge cost = %.6f, want 0.003400", report.Summary.JudgeCost)
	}
	if len(report.Samples) != len(h.fixtures) || len(h.judgeContexts) != len(h.fixtures) {
		t.Fatalf("samples/judge calls = %d/%d, want %d", len(report.Samples), len(h.judgeContexts), len(h.fixtures))
	}
	for i, fixture := range h.fixtures {
		row := report.Samples[i]
		if row.Prompt != fixture.Prompt || row.Reference != fixture.Reference || row.Label == nil || *row.Label != fixture.Label {
			t.Fatalf("sample %d lost question/reference/label: %+v", i, row)
		}
		judgeCtx := h.judgeContexts[i]
		if judgeCtx.Prompt != fixture.Prompt || !reflect.DeepEqual(judgeCtx.Messages, fixture.Messages) || judgeCtx.Reference != fixture.Reference || judgeCtx.HumanLabel == nil || *judgeCtx.HumanLabel != fixture.Label {
			t.Fatalf("judge context %d missing sample context: %+v", i, judgeCtx)
		}
		if strings.Contains(judgeCtx.CandidateOne, "candidate-a") || strings.Contains(judgeCtx.CandidateTwo, "candidate-b") {
			t.Fatalf("model identities leaked into judge candidates: %+v", judgeCtx)
		}
	}
	if len(h.candidateReqs) != len(h.fixtures)*2 {
		t.Fatalf("candidate calls = %d, want %d", len(h.candidateReqs), len(h.fixtures)*2)
	}
	for _, fixture := range h.fixtures {
		var aMessages, bMessages []evalChatMessage
		for _, req := range h.candidateReqs {
			if lastUserMessageFromMessages(req.Messages) != fixture.Prompt {
				continue
			}
			switch req.Model {
			case "candidate-a":
				aMessages = req.Messages
			case "candidate-b":
				bMessages = req.Messages
			}
		}
		if !reflect.DeepEqual(aMessages, fixture.Messages) || !reflect.DeepEqual(bMessages, fixture.Messages) || !reflect.DeepEqual(aMessages, bMessages) {
			t.Fatalf("models did not receive identical full chat context for %q: A=%+v B=%+v", fixture.Prompt, aMessages, bMessages)
		}
	}
}

func TestRunEvaluationWithoutJudgeDoesNotInventQualityScores(t *testing.T) {
	h := newEvalTestHarness(t, false, false)
	run, err := h.service.RunEvaluation(context.Background(), h.datasetID, "candidate-a", "candidate-b")
	if err != nil {
		t.Fatalf("run evaluation: %v", err)
	}
	if run.Status != 3 {
		t.Fatalf("status = %d, want partial (3)", run.Status)
	}
	if run.ScoreA != nil || run.ScoreB != nil {
		t.Fatalf("scores without judge must be null, got %v / %v", run.ScoreA, run.ScoreB)
	}
	report := decodeEvalReport(t, run)
	if report.Summary.ScoredPairs != 0 || report.Summary.JudgeFailures != len(h.fixtures) || report.Summary.ScoringAvailable {
		t.Fatalf("unexpected no-judge summary: %+v", report.Summary)
	}
	for i, sample := range report.Samples {
		if sample.JudgeError == "" || sample.ScoreA != nil || sample.ScoreB != nil {
			t.Fatalf("sample %d should have a visible judge error and no score: %+v", i, sample)
		}
	}
}

func TestRunEvaluationPreservesSuccessfulSideWhenOtherSideFails(t *testing.T) {
	h := newEvalTestHarness(t, true, true)
	run, err := h.service.RunEvaluation(context.Background(), h.datasetID, "candidate-a", "candidate-b")
	if err != nil {
		t.Fatalf("run evaluation: %v", err)
	}
	if run.Status != 3 {
		t.Fatalf("status = %d, want partial (3)", run.Status)
	}
	report := decodeEvalReport(t, run)
	if len(report.Samples) != 2 || report.Summary.FailedB != 1 || report.Summary.SuccessfulA != 2 || report.Summary.SuccessfulB != 1 {
		t.Fatalf("failure details lost: %+v, samples=%d", report.Summary, len(report.Samples))
	}
	failed := report.Samples[0]
	if failed.OutA == "" || failed.ErrorA != "" || failed.ErrorB == "" || !strings.Contains(failed.ErrorB, "503") {
		t.Fatalf("successful A output or B failure reason missing: %+v", failed)
	}
	if failed.ScoreA != nil || failed.ScoreB != nil {
		t.Fatalf("unpaired failed sample must not have blind scores: %+v", failed)
	}
	if report.Samples[1].ScoreA == nil || report.Samples[1].ScoreB == nil {
		t.Fatalf("successful paired sample should retain scores: %+v", report.Samples[1])
	}
}

func TestSampleFromLogsUsesFullMessagesAndAvoidsTruncatedLegacyPreview(t *testing.T) {
	h := newEvalTestHarness(t, true, false)
	longPrompt := strings.Repeat("很长的上下文", 100) + "尾部未截断标记"
	fullMessages, err := json.Marshal([]evalChatMessage{{Role: "system", Content: "完整系统指令"}, {Role: "user", Content: longPrompt}})
	if err != nil {
		t.Fatal(err)
	}
	logs := []model.UsageLog{
		{RequestID: "full", ModelName: "candidate-a", Kind: "chat", Status: 0, PromptPreview: strings.Repeat("x", 500), PromptForEval: string(fullMessages), CreatedAt: time.Now().Add(-48 * time.Hour)},
		{RequestID: "legacy-short", ModelName: "candidate-a", Kind: "chat", Status: 0, PromptPreview: "较早的完整短提示词", CreatedAt: time.Now().Add(-24 * time.Hour)},
		{RequestID: "legacy-truncated", ModelName: "candidate-a", Kind: "chat", Status: 0, PromptPreview: strings.Repeat("截", 500), CreatedAt: time.Now()},
		{RequestID: "failed", ModelName: "candidate-a", Kind: "chat", Status: 500, PromptPreview: "失败请求不应采样"},
		{RequestID: "non-chat", ModelName: "candidate-a", Kind: "embedding", Status: 0, PromptPreview: "非聊天请求不应采样"},
	}
	if err := h.db.Create(&logs).Error; err != nil {
		t.Fatalf("insert usage logs: %v", err)
	}
	added, err := h.service.SampleFromLogs(h.datasetID, 10, "candidate-a")
	if err != nil {
		t.Fatalf("sample logs: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want full + short legacy samples", added)
	}
	samples, err := h.service.ListSamples(h.datasetID)
	if err != nil {
		t.Fatalf("list samples: %v", err)
	}
	serializedSamples, err := json.Marshal(samples)
	if err != nil {
		t.Fatalf("marshal eval samples: %v", err)
	}
	if strings.Contains(string(serializedSamples), "chat_messages") {
		t.Fatal("eval sample API model exposed internal full chat messages")
	}
	foundFull, foundLegacy := false, false
	for _, sample := range samples {
		if sample.Prompt == longPrompt {
			foundFull = sample.Source == "sampled" && sample.ChatMessages == string(fullMessages)
		}
		if sample.Prompt == "较早的完整短提示词" {
			foundLegacy = sample.Source == "sampled_legacy" && sample.ChatMessages == ""
		}
		if strings.Contains(sample.Prompt, "失败请求") || strings.Contains(sample.Prompt, "非聊天请求") || sample.Prompt == strings.Repeat("截", 500) {
			t.Fatalf("ineligible or truncated log was sampled: %+v", sample)
		}
	}
	if !foundFull || !foundLegacy {
		t.Fatalf("full/legacy sample classification incorrect: full=%t legacy=%t", foundFull, foundLegacy)
	}
	added, err = h.service.SampleFromLogs(h.datasetID, 10, "candidate-a")
	if err != nil {
		t.Fatalf("resample logs: %v", err)
	}
	if added != 0 {
		t.Fatalf("duplicate samples were added on second pass: %d", added)
	}
}

func TestParsePairScoresRejectsOutOfRangeAndMalformedOutput(t *testing.T) {
	for _, output := range []string{
		`{"score_candidate_1":0,"score_candidate_2":8}`,
		`{"score_candidate_1":11,"score_candidate_2":8}`,
		`{"score_candidate_1":"bad","score_candidate_2":8}`,
		`not json`,
	} {
		if _, _, ok := parsePairScoresFromJudge(output); ok {
			t.Errorf("accepted invalid judge result %q", output)
		}
	}
	one, two, ok := parsePairScoresFromJudge(`preface {"score_candidate_1":"8","score_candidate_2":7.5} suffix`)
	if !ok || one != 8 || two != 7.5 {
		t.Fatalf("valid scores parsed as %v, %v, %t", one, two, ok)
	}
}

func TestMigrationDefinesEvalSamplingFields(t *testing.T) {
	migration, err := os.ReadFile(filepath.Join("..", "..", "migrations", "001_init.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	for _, field := range []string{"prompt_preview", "prompt_for_eval", "chat_messages"} {
		if !strings.Contains(string(migration), field) {
			t.Errorf("production migration missing %q column", field)
		}
	}
	upgrade, err := os.ReadFile(filepath.Join("..", "..", "migrations", "002_eval_prompt_storage.sql"))
	if err != nil {
		t.Fatalf("read incremental migration: %v", err)
	}
	for _, field := range []string{"prompt_preview", "prompt_for_eval", "chat_messages", "information_schema.columns"} {
		if !strings.Contains(string(upgrade), field) {
			t.Errorf("incremental migration missing %q", field)
		}
	}
}
