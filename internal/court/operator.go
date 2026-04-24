// Package court provides Court runtime functionality.
package court

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

// InitDefaultsRequest defines Court runtime data.
type InitDefaultsRequest struct {
	Scope     SetupScope
	TargetDir string
	Workspace string
	Backend   string
	Model     string
	Force     bool
	DryRun    bool
}

// StartRunOptions defines Court runtime data.
type StartRunOptions struct {
	Task            string
	Preset          string
	Workflow        string
	DelegationScope string
	Backend         string
	Workspace       string
	Model           string
	ModelOptions    RuntimeModelOptions
	Selection       *contract.ModelSelection
}

type RunStatusView = orchestration.RunStatusView[Run, Worker]

type MonitorSnapshot = orchestration.MonitorSnapshot[Run, Worker, RuntimeRequest, Event]

type WatchUpdate = orchestration.WatchUpdate[Run, Event]

type WatchOptions = orchestration.WatchOptions[Run, Event]

type PromotedThreadResult struct {
	RunID    string                 `json:"run_id"`
	WorkerID string                 `json:"worker_id"`
	Thread   contract.TrackedThread `json:"thread"`
}

func (e *Engine) BestPromotableWorkerID(ctx context.Context, runID string) (string, error) {
	trace, err := e.TraceRun(ctx, runID)
	if err != nil {
		return "", err
	}
	judges := make([]WorkerTrace, 0)
	others := make([]WorkerTrace, 0)
	for _, worker := range trace.Workers {
		if worker.RuntimeSession == nil || worker.Worker.Status != WorkerCompleted {
			continue
		}
		if worker.Worker.RoleKind == RoleJudge {
			judges = append(judges, worker)
			continue
		}
		others = append(others, worker)
	}
	if len(judges) == 1 {
		return judges[0].Worker.ID, nil
	}
	if len(judges) > 1 {
		return "", fmt.Errorf("run %q has multiple completed judge threads; inspect `agent_control court trace %s` and choose explicitly with --worker-id", runID, runID)
	}
	if len(others) == 1 {
		return others[0].Worker.ID, nil
	}
	if len(others) == 0 {
		return "", fmt.Errorf("run %q has no completed worker with a tracked runtime thread", runID)
	}
	return "", fmt.Errorf("run %q has multiple completed worker threads and no explicit semantic winner; inspect `agent_control court trace %s` and choose explicitly with --worker-id", runID, runID)
}

// EngineOptionsFromEnvironment provides Court runtime functionality.
func EngineOptionsFromEnvironment() EngineOptions {
	configDir := os.Getenv("COURT_CONFIG_DIR")
	dataDir := os.Getenv("COURT_DATA_DIR")
	legacyRoot := os.Getenv("COURT_ROOT")
	if configDir == "" && legacyRoot != "" {
		configDir = legacyRoot
	}
	if dataDir == "" && legacyRoot != "" {
		dataDir = legacyRoot
	}
	return EngineOptions{
		ConfigDir:     configDir,
		DataDir:       dataDir,
		DBPath:        os.Getenv("COURT_DB"),
		WorkerCommand: os.Getenv("COURT_WORKER_COMMAND"),
	}
}

// InitDefaults provides Court runtime functionality.
func InitDefaults(req InitDefaultsRequest) (InitDefaultsResult, error) {
	setup, err := SetupDefaults(SetupDefaultsRequest{
		Scope:     req.Scope,
		TargetDir: req.TargetDir,
		Workspace: req.Workspace,
		Force:     req.Force,
		DryRun:    req.DryRun,
	})
	if err != nil {
		return InitDefaultsResult{}, err
	}
	config, err := WriteDefaultConfig(req.Scope, req.TargetDir, req.Workspace, req.Backend, req.Model, req.Force, req.DryRun)
	if err != nil {
		return InitDefaultsResult{}, err
	}
	return InitDefaultsResult{Setup: setup, Config: config}, nil
}

// StartRunWithOptions provides Court runtime functionality.
func (e *Engine) StartRunWithOptions(ctx context.Context, opts StartRunOptions) (Run, error) {
	task := strings.TrimSpace(opts.Task)
	if task == "" {
		return Run{}, fmt.Errorf("task is required")
	}
	var workflow WorkflowMode
	if opts.Workflow != "" {
		var ok bool
		workflow, ok = ParseWorkflowMode(opts.Workflow)
		if !ok {
			return Run{}, fmt.Errorf("unsupported workflow %q", opts.Workflow)
		}
	}
	var delegationScope DelegationScope
	if opts.DelegationScope != "" {
		var ok bool
		delegationScope, ok = ParseDelegationScope(opts.DelegationScope)
		if !ok {
			return Run{}, fmt.Errorf("unsupported delegation scope %q", opts.DelegationScope)
		}
	}
	return e.StartRun(ctx, StartRunRequest{
		Task:            task,
		Preset:          opts.Preset,
		Workflow:        workflow,
		DelegationScope: delegationScope,
		Backend:         opts.Backend,
		Workspace:       opts.Workspace,
		Model:           opts.Model,
		ModelOptions:    opts.ModelOptions,
		Selection:       opts.Selection,
	})
}

// RunStatus provides Court runtime functionality.
func (e *Engine) RunStatus(ctx context.Context, runID string) (RunStatusView, error) {
	run, err := e.GetRun(ctx, runID)
	if err != nil {
		return RunStatusView{}, err
	}
	workers, err := e.ListWorkers(ctx, run.ID)
	if err != nil {
		return RunStatusView{}, err
	}
	return RunStatusView{Run: run, Workers: workers}, nil
}

// ReconcileAndGetRun provides Court runtime functionality.
func (e *Engine) ReconcileAndGetRun(ctx context.Context, runID string) (Run, error) {
	if err := e.ReconcileRun(ctx, runID); err != nil {
		return Run{}, err
	}
	return e.GetRun(ctx, runID)
}

// CompletedVerdict provides Court runtime functionality.
func (e *Engine) CompletedVerdict(ctx context.Context, runID string) (string, error) {
	run, err := e.GetRun(ctx, runID)
	if err != nil {
		return "", err
	}
	if run.Verdict == "" {
		return "", fmt.Errorf("run %s has no verdict yet; current status is %s", run.ID, run.Status)
	}
	return run.Verdict, nil
}

// RuntimeRequestAnswersFromJSON provides Court runtime functionality.
func RuntimeRequestAnswersFromJSON(raw string) ([]RuntimeRequestAnswer, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var answers []RuntimeRequestAnswer
	if err := json.Unmarshal([]byte(raw), &answers); err != nil {
		return nil, fmt.Errorf("invalid --answers-json: %w", err)
	}
	return answers, nil
}

// MonitorSnapshot provides Court runtime functionality.
func (e *Engine) MonitorSnapshot(ctx context.Context, runID string, eventLimit int) (MonitorSnapshot, error) {
	run, err := e.GetRun(ctx, runID)
	if err != nil {
		return MonitorSnapshot{}, err
	}
	workers, err := e.ListWorkers(ctx, runID)
	if err != nil {
		return MonitorSnapshot{}, err
	}
	requests, err := e.ListRuntimeRequests(ctx, runID, RuntimeRequestOpen)
	if err != nil {
		return MonitorSnapshot{}, err
	}
	events, err := e.ListEvents(ctx, runID, 0)
	if err != nil {
		return MonitorSnapshot{}, err
	}
	if eventLimit > 0 && len(events) > eventLimit {
		events = events[len(events)-eventLimit:]
	}
	return MonitorSnapshot{
		Run:          run,
		Workers:      workers,
		OpenRequests: requests,
		RecentEvents: events,
		UpdatedAt:    time.Now(),
	}, nil
}

// WatchRun provides Court runtime functionality.
func (e *Engine) WatchRun(ctx context.Context, runID string, opts WatchOptions) error {
	poll := opts.PollInterval
	if poll <= 0 {
		poll = time.Second
	}
	var cursor int64
	for {
		events, err := e.ListEvents(ctx, runID, cursor)
		if err != nil {
			return err
		}
		for _, event := range events {
			cursor = event.ID
			if err := emitWatchUpdate(opts.OnUpdate, WatchUpdate{Event: &event}); err != nil {
				return err
			}
		}
		terminal, err := e.watchTerminalRun(ctx, runID, opts)
		if err != nil {
			return err
		}
		if terminal {
			return nil
		}
		select {
		case <-ctx.Done():
			return wrapErr("watch context", ctx.Err())
		case <-time.After(poll):
		}
	}
}

func (e *Engine) PromoteWorkerThread(ctx context.Context, runID string, workerID string) (PromotedThreadResult, error) {
	trace, err := e.TraceRun(ctx, runID)
	if err != nil {
		return PromotedThreadResult{}, err
	}
	for _, worker := range trace.Workers {
		if worker.Worker.ID != workerID {
			continue
		}
		if worker.RuntimeSession == nil {
			return PromotedThreadResult{}, fmt.Errorf("worker %q has no tracked runtime thread", workerID)
		}
		thread, err := e.controlPlane.GetThread(ctx, worker.RuntimeSession.Session.SessionID, worker.RuntimeSession.Session.ProviderSessionID)
		if err != nil {
			return PromotedThreadResult{}, err
		}
		metadata := thread.Metadata
		metadata.PromotedFromRunID = runID
		metadata.PromotedFromWorkerID = worker.Worker.ID
		metadata.PromotedFromRoleID = worker.Worker.RoleID
		metadata.PromotedAsBase = true
		if err := e.controlPlane.SetThreadMetadata(ctx, thread.ThreadID, metadata); err != nil {
			return PromotedThreadResult{}, err
		}
		thread.Metadata = metadata
		return PromotedThreadResult{RunID: runID, WorkerID: workerID, Thread: *thread}, nil
	}
	return PromotedThreadResult{}, fmt.Errorf("worker %q not found in run %q", workerID, runID)
}

func (e *Engine) watchTerminalRun(ctx context.Context, runID string, opts WatchOptions) (bool, error) {
	if !opts.StopOnTerminal {
		return false, nil
	}
	run, err := e.GetRun(ctx, runID)
	if err != nil {
		return false, err
	}
	if !orchestration.IsTerminalRunStatus(orchestration.RunStatus(run.Status)) {
		return false, nil
	}
	return true, emitWatchUpdate(opts.OnUpdate, WatchUpdate{TerminalRun: &run})
}

func emitWatchUpdate(callback func(WatchUpdate) error, update WatchUpdate) error {
	if callback == nil {
		return nil
	}
	return callback(update)
}
