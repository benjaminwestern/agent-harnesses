package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type BatchEvaluationOptions struct {
	Items       []DatasetItemRecord
	Prompt      string
	TargetModel string
	JudgeModel  string
	Mode        ReductionMode
}

func parseEvalTarget(raw string) FanoutTarget {
	parts := strings.SplitN(raw, "=", 2)
	if len(parts) == 1 {
		return FanoutTarget{Backend: "openaicompatible", Model: raw}
	}
	return FanoutTarget{Backend: parts[0], Model: parts[1]}
}

func RunBatchEvaluation(ctx context.Context, controller FanoutController, opts BatchEvaluationOptions) ([]EvaluationResultRecord, error) {
	targetFanout := parseEvalTarget(opts.TargetModel)
	judgeFanout := parseEvalTarget(opts.JudgeModel)

	var results []EvaluationResultRecord

	for _, item := range opts.Items {
		start := time.Now()

		targetOpts := FanoutOptions{
			Prompt:      fmt.Sprintf("%s\n\nInput:\n%s", opts.Prompt, item.InputPayload),
			Targets:     []FanoutTarget{targetFanout},
			EventBuffer: 1024,
		}

		resFanout, err := RunFanout(ctx, controller, targetOpts)
		if err != nil {
			return results, fmt.Errorf("run fanout for item %s: %w", item.ID, err)
		}
		if len(resFanout.Targets) == 0 || resFanout.Targets[0].Error != "" {
			return results, fmt.Errorf("target generation failed for item %s: %s", item.ID, resFanout.Targets[0].Error)
		}

		targetOutput := resFanout.Targets[0].Text

		evalPrompt := FormatInlineEvalPrompt(GetRubric(opts.Prompt), item.InputPayload, targetOutput, item.TargetOutput)

		fanoutRes := FanoutResult{
			Prompt: evalPrompt,
			Targets: []FanoutTargetResult{
				{
					Target: FanoutTarget{Label: "target_output"},
					Text:   targetOutput,
				},
			},
		}

		reduceRes, err := RunReduction(ctx, controller, ReductionModeEvaluate, fanoutRes, judgeFanout, false)
		if err != nil {
			return results, fmt.Errorf("run reduction for item %s: %w", item.ID, err)
		}

		var parsedScore struct {
			Score     float64 `json:"score"`
			Rationale string  `json:"rationale"`
			Passed    bool    `json:"passed"`
		}
		if err := json.Unmarshal([]byte(reduceRes.JSON), &parsedScore); err != nil {
			return results, fmt.Errorf("failed to parse judge output for item %s: %w", item.ID, err)
		}

		resultRec := EvaluationResultRecord{
			ID:            uuid.New().String(),
			DatasetItemID: item.ID,
			Score:         parsedScore.Score,
			Rationale:     parsedScore.Rationale,
			Passed:        parsedScore.Passed,
			LatencyMS:     int(time.Since(start).Milliseconds()),
			CostUSD:       resFanout.TotalCostUSD + reduceRes.RecordedCostUSD,
		}

		results = append(results, resultRec)
	}

	return results, nil
}

type JudgeAlignmentOptions struct {
	Items      []DatasetItemRecord
	Prompt     string
	JudgeModel string
	Mode       ReductionMode
}

type JudgeAlignmentMetrics struct {
	MeanSquaredError float64 `json:"mean_squared_error"`
	Accuracy         float64 `json:"accuracy"`
	TotalEvaluated   int     `json:"total_evaluated"`
	CostUSD          float64 `json:"cost_usd"`
}

func RunJudgeAlignmentEvaluation(ctx context.Context, controller FanoutController, opts JudgeAlignmentOptions) (JudgeAlignmentMetrics, error) {
	judgeFanout := parseEvalTarget(opts.JudgeModel)
	var metrics JudgeAlignmentMetrics

	evalMode := opts.Mode
	if evalMode == "" {
		evalMode = ReductionModeEvaluate
	}

	var squaredErrorSum float64
	var correctCount int

	for _, item := range opts.Items {
		var humanScore struct {
			Score  float64 `json:"score"`
			Passed bool    `json:"passed"`
		}
		if err := json.Unmarshal([]byte(item.TargetOutput), &humanScore); err != nil {
			// Skip if target output isn't a human score
			continue
		}

		// The input payload contains the text to grade
		evalPrompt := FormatInlineEvalPrompt(GetRubric(opts.Prompt), "", item.InputPayload, "")

		fanoutRes := FanoutResult{
			Prompt: evalPrompt,
			Targets: []FanoutTargetResult{
				{
					Target: FanoutTarget{Label: "target_output"},
					Text:   item.InputPayload,
				},
			},
		}

		reduceRes, err := RunReduction(ctx, controller, evalMode, fanoutRes, judgeFanout, false)
		if err != nil {
			return metrics, fmt.Errorf("run judge reduction for item %s: %w", item.ID, err)
		}

		metrics.CostUSD += reduceRes.RecordedCostUSD

		var judgeScore struct {
			Score  float64 `json:"score"`
			Passed bool    `json:"passed"`
		}
		if err := json.Unmarshal([]byte(reduceRes.JSON), &judgeScore); err != nil {
			continue
		}

		metrics.TotalEvaluated++

		diff := judgeScore.Score - humanScore.Score
		squaredErrorSum += diff * diff

		if judgeScore.Passed == humanScore.Passed {
			correctCount++
		}
	}

	if metrics.TotalEvaluated > 0 {
		metrics.MeanSquaredError = squaredErrorSum / float64(metrics.TotalEvaluated)
		metrics.Accuracy = float64(correctCount) / float64(metrics.TotalEvaluated)
	}

	return metrics, nil
}

type DatasetEvaluationOptions struct {
	DatasetID   string
	PromptID    string
	TargetModel string
	JudgeModel  string
	Name        string
	Mode        ReductionMode
}

func RunDatasetEvaluation(ctx context.Context, ledger *SQLiteLedgerStore, controller FanoutController, opts DatasetEvaluationOptions) (EvaluationRecord, error) {
	_, err := ledger.GetDataset(ctx, opts.DatasetID)
	if err != nil {
		return EvaluationRecord{}, fmt.Errorf("get dataset: %w", err)
	}
	prompt, err := ledger.GetPrompt(ctx, opts.PromptID)
	if err != nil {
		return EvaluationRecord{}, fmt.Errorf("get prompt: %w", err)
	}

	eval := EvaluationRecord{
		ID:          uuid.New().String(),
		Name:        opts.Name,
		DatasetID:   opts.DatasetID,
		PromptID:    opts.PromptID,
		TargetModel: opts.TargetModel,
		JudgeModel:  opts.JudgeModel,
	}
	if err := ledger.UpsertEvaluation(ctx, eval); err != nil {
		return EvaluationRecord{}, fmt.Errorf("create evaluation record: %w", err)
	}

	items, err := ledger.ListDatasetItems(ctx, opts.DatasetID)
	if err != nil {
		return EvaluationRecord{}, fmt.Errorf("list dataset items: %w", err)
	}

	results, err := RunBatchEvaluation(ctx, controller, BatchEvaluationOptions{
		Items:       items,
		Prompt:      prompt.Content,
		TargetModel: opts.TargetModel,
		JudgeModel:  opts.JudgeModel,
		Mode:        opts.Mode,
	})
	if err != nil {
		return EvaluationRecord{}, err
	}

	for _, res := range results {
		res.EvaluationID = eval.ID
		if err := ledger.AddEvaluationResult(ctx, res); err != nil {
			return eval, fmt.Errorf("add evaluation result for item %s: %w", res.DatasetItemID, err)
		}
	}

	return eval, nil
}
