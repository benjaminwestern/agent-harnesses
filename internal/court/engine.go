// Package court provides Court runtime functionality.
package court

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
	"github.com/benjaminwestern/agentic-control/pkg/controlplane/embedded"
	"github.com/benjaminwestern/agentic-control/pkg/features"
)

// Engine defines Court runtime data.
type Engine struct {
	configDir     string
	dataDir       string
	dbPath        string
	workerCommand string
	controlPlane  RuntimeControlPlane
	store         *Store
}

var errWorkerCancelled = errors.New("worker cancelled")

// NewEngine provides Court runtime functionality.
func NewEngine(opts EngineOptions) (*Engine, error) {
	configDir := opts.ConfigDir
	dataDir := opts.DataDir
	if opts.RootDir != "" {
		if configDir == "" {
			configDir = opts.RootDir
		}
		if dataDir == "" {
			dataDir = opts.RootDir
		}
	}
	if configDir == "" {
		var err error
		configDir, err = DefaultConfigDir()
		if err != nil {
			return nil, err
		}
	}
	if dataDir == "" {
		var err error
		dataDir, err = DefaultDataDir()
		if err != nil {
			return nil, err
		}
	}
	absConfigDir, err := filepath.Abs(configDir)
	if err != nil {
		return nil, wrapErr("resolve config directory", err)
	}
	absDataDir, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, wrapErr("resolve data directory", err)
	}
	dbPath := opts.DBPath
	if dbPath == "" {
		dbPath = filepath.Join(absDataDir, "court.db")
	}
	workerCommand := opts.WorkerCommand
	if workerCommand == "" {
		workerCommand, _ = os.Executable()
	}
	controlPlane := opts.ControlPlane
	if controlPlane == nil {
		controlPlane = embedded.New()
	}

	store, err := OpenStore(dbPath)
	if err != nil {
		return nil, err
	}

	// Apply features from config if available
	if config, err := LoadCourtConfig(absConfigDir); err == nil {
		features.Apply(config.Features)
	}

	return &Engine{
		configDir:     absConfigDir,
		dataDir:       absDataDir,
		dbPath:        dbPath,
		workerCommand: workerCommand,
		controlPlane:  controlPlane,
		store:         store,
	}, nil
}

// Close provides Court runtime functionality.
func (e *Engine) Close() error {
	return e.store.Close()
}

// RootDir provides Court runtime functionality.
func (e *Engine) RootDir() string {
	return e.configDir
}

// ConfigDir provides Court runtime functionality.
func (e *Engine) ConfigDir() string {
	return e.configDir
}

// DataDir provides Court runtime functionality.
func (e *Engine) DataDir() string {
	return e.dataDir
}

// DBPath provides Court runtime functionality.
func (e *Engine) DBPath() string {
	return e.dbPath
}

// StartRun provides Court runtime functionality.
func (e *Engine) StartRun(ctx context.Context, req StartRunRequest) (Run, error) {
	if strings.TrimSpace(req.Task) == "" {
		return Run{}, fmt.Errorf("task is required")
	}
	workspace := req.Workspace
	if workspace == "" {
		workspace = "."
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return Run{}, wrapErr("resolve workspace", err)
	}
	roots, err := e.catalogRoots(workspace)
	if err != nil {
		return Run{}, err
	}
	config, err := LoadCourtConfigFromRoots(roots)
	if err != nil {
		return Run{}, err
	}
	// Apply features from config
	features.Apply(config.Features)
	if req.Selection != nil {
		registry := api.BuildModelRegistry(e.controlPlane.Describe().Runtimes)
		normalized := api.NormalizeModelSelection(registry, *req.Selection)
		target := api.RuntimeTargetFromSelection(normalized)
		req.Backend = target.Backend
		req.Model = target.Model
		req.ModelOptions = target.Options
	}

	if req.Backend == "" {
		req.Backend = firstNonEmpty(config.Defaults.Backend, "opencode")
	}
	if err := e.validateBackend(req.Backend); err != nil {
		return Run{}, err
	}
	if strings.TrimSpace(req.Preset) == "" {
		req.Preset = config.Defaults.Preset
	}
	defaultRuntime := RuntimeBackendConfig{}
	if strings.TrimSpace(req.Model) == "" {
		defaultRuntime = config.Defaults.BackendDefaults(req.Backend)
	}
	validation := api.ValidateSessionTarget(e.controlPlane.Describe().Runtimes, api.RuntimeTarget{
		Backend: req.Backend,
		Model:   firstNonEmpty(req.Model, defaultRuntime.Model),
		Options: api.MergeModelOptions(req.ModelOptions, defaultRuntime.ModelOptions),
	})
	if validation.HasErrors() {
		return Run{}, fmt.Errorf("run target is invalid: %s", validation.Issues[0].Message)
	}
	if (strings.TrimSpace(req.Model) != "" || (req.Selection != nil && strings.TrimSpace(req.Selection.Model) != "")) && validation.HasIssueCode("model_unlisted") {
		return Run{}, fmt.Errorf("run target is invalid: %s", validation.Issues[0].Message)
	}
	preset, err := e.resolvePresetForWorkspace(req.Preset, workspace)
	if err != nil {
		return Run{}, err
	}
	if err := e.validatePresetBackends(preset, req.Backend); err != nil {
		return Run{}, err
	}

	now := time.Now()
	workflowOverride := firstNonEmpty(string(req.Workflow), config.Defaults.Workflow)
	if workflowOverride != "" {
		if _, ok := ParseWorkflowMode(workflowOverride); !ok {
			return Run{}, fmt.Errorf("unsupported workflow %q", workflowOverride)
		}
	}
	workflow := ResolveWorkflowMode(workflowOverride, preset.Workflow)
	delegationScopeOverride := firstNonEmpty(string(req.DelegationScope), config.Defaults.DelegationScope)
	if delegationScopeOverride != "" {
		if _, ok := ParseDelegationScope(delegationScopeOverride); !ok {
			return Run{}, fmt.Errorf("unsupported delegation scope %q", delegationScopeOverride)
		}
	}
	delegationScope := ResolveDelegationScope(delegationScopeOverride, DelegationScopePreset)
	run := Run{
		ID:                  "run-" + randomID(),
		CourtID:             req.CourtID,
		Task:                req.Task,
		Preset:              preset.ID,
		Workflow:            workflow,
		DelegationScope:     delegationScope,
		Backend:             req.Backend,
		Model:               req.Model,
		ModelOptions:        req.ModelOptions,
		DefaultProvider:     defaultRuntime.Provider,
		DefaultModel:        defaultRuntime.Model,
		DefaultModelOptions: defaultRuntime.ModelOptions,
		Workspace:           workspace,
		Status:              RunRunning,
		Phase:               initialPhase(preset, workflow),
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := e.store.CreateRun(ctx, run); err != nil {
		return Run{}, err
	}
	plan := BuildPlan(req.Task)
	docket := BuildDocket(plan, preset, workflow)
	docket.DelegationScope = delegationScope
	e.addArtifact(ctx, run.ID, "", "plan", "json", plan)
	e.addArtifact(ctx, run.ID, "", "docket", "json", docket)
	e.emit(ctx, run.ID, "", "run.started", fmt.Sprintf("started %s run with %s", preset.ID, req.Backend), "")

	initialRoles := initialRolesFor(preset, workflow)
	if err := e.spawnRoles(ctx, run, initialRoles); err != nil {
		if failedWorkerID := failedWorkerIDFromRunStartError(err); failedWorkerID != "" {
			return Run{}, err
		}
		return Run{}, e.failRunStart(ctx, run.ID, "", err)
	}
	return run, nil
}

// GetRun provides Court runtime functionality.
func (e *Engine) GetRun(ctx context.Context, runID string) (Run, error) {
	return e.store.GetRun(ctx, runID)
}

// ListRuns provides Court runtime functionality.
func (e *Engine) ListRuns(ctx context.Context) ([]Run, error) {
	return e.store.ListRuns(ctx)
}

// ListWorkers provides Court runtime functionality.
func (e *Engine) ListWorkers(ctx context.Context, runID string) ([]Worker, error) {
	return e.store.ListWorkers(ctx, runID)
}

// ListEvents provides Court runtime functionality.
func (e *Engine) ListEvents(ctx context.Context, runID string, after int64) ([]Event, error) {
	return e.store.ListEvents(ctx, runID, after)
}

// ListArtifacts provides Court runtime functionality.
func (e *Engine) ListArtifacts(ctx context.Context, runID string) ([]Artifact, error) {
	return e.store.ListArtifacts(ctx, runID)
}

// TraceRun provides Court runtime functionality.
func (e *Engine) TraceRun(ctx context.Context, runID string) (RunTrace, error) {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return RunTrace{}, err
	}
	workers, err := e.store.ListWorkers(ctx, runID)
	if err != nil {
		return RunTrace{}, err
	}
	events, err := e.store.ListEvents(ctx, runID, 0)
	if err != nil {
		return RunTrace{}, err
	}
	artifacts, err := e.store.ListArtifacts(ctx, runID)
	if err != nil {
		return RunTrace{}, err
	}
	controls, err := e.store.ListWorkerControls(ctx, runID)
	if err != nil {
		return RunTrace{}, err
	}
	attempts, err := e.store.ListWorkerAttempts(ctx, runID)
	if err != nil {
		return RunTrace{}, err
	}
	requests, err := e.store.ListRuntimeRequests(ctx, runID, "")
	if err != nil {
		return RunTrace{}, err
	}

	eventsByWorker := make(map[string][]Event)
	artifactsByWorker := make(map[string][]Artifact)
	controlsByWorker := make(map[string][]WorkerControlRequest)
	attemptsByWorker := make(map[string][]WorkerAttempt)
	requestsByWorker := make(map[string][]RuntimeRequest)
	var runEvents []Event
	var runArtifacts []Artifact
	for _, event := range events {
		if event.WorkerID == "" {
			runEvents = append(runEvents, event)
			continue
		}
		eventsByWorker[event.WorkerID] = append(eventsByWorker[event.WorkerID], event)
	}
	for _, artifact := range artifacts {
		if artifact.WorkerID == "" {
			runArtifacts = append(runArtifacts, artifact)
			continue
		}
		artifactsByWorker[artifact.WorkerID] = append(artifactsByWorker[artifact.WorkerID], artifact)
	}
	for _, control := range controls {
		controlsByWorker[control.WorkerID] = append(controlsByWorker[control.WorkerID], control)
	}
	for _, attempt := range attempts {
		attemptsByWorker[attempt.WorkerID] = append(attemptsByWorker[attempt.WorkerID], attempt)
	}
	for _, request := range requests {
		requestsByWorker[request.WorkerID] = append(requestsByWorker[request.WorkerID], request)
	}

	workerTraces := make([]WorkerTrace, 0, len(workers))
	for _, worker := range workers {
		workerTrace := WorkerTrace{
			Worker:    worker,
			Attempts:  attemptsByWorker[worker.ID],
			Events:    eventsByWorker[worker.ID],
			Artifacts: artifactsByWorker[worker.ID],
			Controls:  controlsByWorker[worker.ID],
			Requests:  requestsByWorker[worker.ID],
		}
		if strings.TrimSpace(worker.ResultJSON) != "" {
			var result WorkerResult
			if err := json.Unmarshal([]byte(worker.ResultJSON), &result); err == nil {
				workerTrace.StructuredResult = &result
			}
		}
		if worker.RuntimeSessionID != "" || worker.RuntimeProviderSessionID != "" {
			if tracked, err := e.controlPlane.GetTrackedSession(ctx, worker.RuntimeSessionID, worker.RuntimeProviderSessionID); err == nil {
				workerTrace.RuntimeSession = tracked
			}
		}
		workerTraces = append(workerTraces, workerTrace)
	}

	return RunTrace{
		Run:       run,
		Workers:   workerTraces,
		Events:    runEvents,
		Artifacts: runArtifacts,
	}, nil
}

func (e *Engine) validatePresetBackends(preset Preset, defaultBackend string) error {
	for _, role := range preset.Roles {
		backend := firstNonEmpty(defaultBackend, role.Backend)
		if err := e.validateBackend(backend); err != nil {
			return fmt.Errorf("role %q: %w", role.ID, err)
		}
	}
	return nil
}

func (e *Engine) validateBackend(backend string) error {
	_, err := api.ValidateSessionBackend(e.controlPlane.Describe().Runtimes, backend)
	return err
}

func (e *Engine) supportedBackends() []string {
	return api.SupportedSessionBackends(e.controlPlane.Describe().Runtimes)
}

// RuntimeDescriptors returns the current agentic-control runtime descriptors.
func (e *Engine) RuntimeDescriptors() []contract.RuntimeDescriptor {
	return e.controlPlane.Describe().Runtimes
}

func (e *Engine) resolvePresetForWorkspace(id string, workspace string) (Preset, error) {
	roots, err := e.catalogRoots(workspace)
	if err != nil {
		return Preset{}, err
	}
	if preset, ok, err := LoadPresetFromRoots(roots, id); err != nil {
		return Preset{}, err
	} else if ok {
		return preset, nil
	}
	return ResolvePreset(id)
}

// ListAvailablePresets provides Court runtime functionality.
func (e *Engine) ListAvailablePresets(workspace string) ([]Preset, error) {
	roots, err := e.catalogRoots(workspace)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	markdownPresets, err := ListMarkdownPresetsFromRoots(roots)
	if err != nil {
		return nil, err
	}
	builtins := ListPresets()
	presets := make([]Preset, 0, len(markdownPresets)+len(builtins))
	for _, preset := range markdownPresets {
		seen[preset.ID] = struct{}{}
		presets = append(presets, preset)
	}
	for _, preset := range builtins {
		if _, ok := seen[preset.ID]; ok {
			continue
		}
		presets = append(presets, preset)
	}
	sort.Slice(presets, func(i, j int) bool {
		return presets[i].ID < presets[j].ID
	})
	return presets, nil
}

func (e *Engine) catalogRoots(workspace string) ([]string, error) {
	roots := []string{e.configDir}
	if workspace != "" {
		projectRoots, err := DiscoverProjectConfigDirs(workspace)
		if err != nil {
			return nil, err
		}
		roots = append(roots, projectRoots...)
	}
	return cleanPathList(roots), nil
}

// RunWorker provides Court runtime functionality.
func (e *Engine) RunWorker(ctx context.Context, workerID string) error {
	return orchestration.ExecuteWorker(ctx, workerID, orchestration.WorkerExecutionHooks[Run, Worker, RuntimeIdentity]{
		Load: func(ctx context.Context, workerID string) (Worker, Run, error) {
			worker, err := e.store.GetWorker(ctx, workerID)
			if err != nil {
				return Worker{}, Run{}, err
			}
			run, err := e.store.GetRun(ctx, worker.RunID)
			if err != nil {
				return Worker{}, Run{}, err
			}
			return worker, run, nil
		},
		IsTerminal: func(worker Worker) bool {
			return worker.Status == WorkerCompleted || worker.Status == WorkerFailed || worker.Status == WorkerCancelled
		},
		MarkRunning: func(ctx context.Context, workerID string) error {
			return e.store.UpdateWorkerRunning(ctx, workerID)
		},
		OnStarted: func(ctx context.Context, run Run, worker Worker) {
			e.emit(ctx, run.ID, worker.ID, "worker.started", fmt.Sprintf("%s started", worker.RoleTitle), "")
		},
		Execute: func(ctx context.Context, run Run, worker Worker) (string, string, RuntimeIdentity, error) {
			return e.runBackendWorker(ctx, run, worker)
		},
		HasIdentity: func(identity RuntimeIdentity) bool {
			return identity.SessionID != "" || identity.ProviderSessionID != "" || identity.TranscriptPath != "" || identity.PID != 0
		},
		PersistIdentity: func(ctx context.Context, workerID string, identity RuntimeIdentity) error {
			return e.store.UpdateWorkerRuntimeIdentity(ctx, workerID, identity)
		},
		IsCancelledError: func(err error) bool {
			return errors.Is(err, errWorkerCancelled)
		},
		MarkCancelled: func(ctx context.Context, workerID string, result string, resultJSON string) error {
			return e.store.CompleteWorker(ctx, workerID, WorkerCancelled, result, resultJSON, "worker cancelled")
		},
		OnCancelled: func(ctx context.Context, run Run, worker Worker, result string, resultJSON string) {
			e.emit(ctx, run.ID, worker.ID, "worker.cancelled", fmt.Sprintf("%s cancelled", worker.RoleTitle), "")
		},
		MarkFailed: func(ctx context.Context, workerID string, result string, resultJSON string, errText string) error {
			return e.store.CompleteWorker(ctx, workerID, WorkerFailed, result, resultJSON, errText)
		},
		OnFailed: func(ctx context.Context, run Run, worker Worker, result string, resultJSON string, errText string) {
			e.emit(ctx, run.ID, worker.ID, "worker.failed", fmt.Sprintf("%s failed: %s", worker.RoleTitle, errText), "")
		},
		Reload: func(ctx context.Context, workerID string) (Worker, error) {
			return e.store.GetWorker(ctx, workerID)
		},
		IsCancelled: func(worker Worker) bool {
			return worker.Status == WorkerCancelled
		},
		MarkCompleted: func(ctx context.Context, workerID string, result string, resultJSON string) error {
			return e.store.CompleteWorker(ctx, workerID, WorkerCompleted, result, resultJSON, "")
		},
		OnCompleted: func(ctx context.Context, run Run, worker Worker, result string, resultJSON string) {
			if resultJSON != "" {
				e.addArtifact(ctx, run.ID, worker.ID, "worker_result", "json", json.RawMessage(resultJSON))
			}
			e.emit(ctx, run.ID, worker.ID, "worker.completed", fmt.Sprintf("%s completed", worker.RoleTitle), "")
		},
		Reconcile: func(ctx context.Context, run Run) error {
			return e.ReconcileRun(ctx, run.ID)
		},
	})
}

// ReconcileRun provides Court runtime functionality.
func (e *Engine) ReconcileRun(ctx context.Context, runID string) error {
	run, err := e.store.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status == RunCompleted || run.Status == RunFailed || run.Status == RunCancelled {
		return nil
	}
	workers, err := e.store.ListWorkers(ctx, runID)
	if err != nil {
		return err
	}
	if len(workers) == 0 {
		return nil
	}
	summary := orchestration.SummarizeWorkerStatuses(workers, func(worker Worker) orchestration.WorkerStatus {
		return orchestration.WorkerStatus(worker.Status)
	})
	if summary.HasFailed {
		_ = e.store.UpdateRunStatus(ctx, runID, RunFailed, "")
		_ = e.store.UpdateRunPhase(ctx, runID, PhaseBlocked)
		e.emit(ctx, runID, "", "run.failed", "one or more workers failed", "")
		return nil
	}
	if summary.HasCancelled {
		_ = e.store.UpdateRunStatus(ctx, runID, RunCancelled, "")
		_ = e.store.UpdateRunPhase(ctx, runID, PhaseBlocked)
		e.emit(ctx, runID, "", "run.cancelled", "one or more workers were cancelled", "")
		return nil
	}

	preset, err := e.resolvePresetForWorkspace(run.Preset, run.Workspace)
	if err != nil {
		return err
	}
	if judgeCompleted(workers) {
		verdict := finalVerdict(workers)
		e.addArtifact(ctx, runID, "", "verdict", "markdown", verdict)
		if err := e.store.UpdateRunStatus(ctx, runID, RunCompleted, verdict); err != nil {
			return err
		}
		_ = e.store.UpdateRunPhase(ctx, runID, PhaseComplete)
		e.emit(ctx, runID, "", "run.completed", "run completed", "")
		return nil
	}
	if summary.HasRunningOrQueued {
		phase := EvaluatePhase(phaseInputForRun(run, preset, workers), ParticipantStatesFromWorkers(workers)).Phase
		_ = e.store.UpdateRunPhase(ctx, runID, phase)
		return nil
	}
	if clerkReadyForJurors(workers, preset) && !jurorsStarted(workers) {
		jurors := e.promoteClerkDocket(ctx, run, preset, workers)
		_ = e.store.UpdateRunPhase(ctx, runID, PhaseJurors)
		return e.spawnRoles(ctx, run, jurors)
	}
	if run.Workflow == WorkflowReviewOnly {
		return e.completeRun(ctx, runID, workers)
	}
	judges, err := e.judgeRolesForRun(ctx, run, preset)
	if err != nil {
		return err
	}
	if len(judges) == 0 {
		return e.completeRun(ctx, runID, workers)
	}
	var missingJudges []Role
	for _, judge := range judges {
		if !workerExists(workers, judge.ID) {
			missingJudges = append(missingJudges, judge)
		}
	}
	if len(missingJudges) == 0 {
		return nil
	}
	_ = e.store.UpdateRunPhase(ctx, runID, PhaseVerdict)
	return e.spawnRoles(ctx, run, missingJudges)
}

func (e *Engine) completeRun(ctx context.Context, runID string, workers []Worker) error {
	verdict := finalVerdict(workers)
	e.addArtifact(ctx, runID, "", "verdict", "markdown", verdict)
	if err := e.store.UpdateRunStatus(ctx, runID, RunCompleted, verdict); err != nil {
		return err
	}
	_ = e.store.UpdateRunPhase(ctx, runID, PhaseComplete)
	e.emit(ctx, runID, "", "run.completed", "run completed", "")
	return nil
}

func (e *Engine) spawnRoles(ctx context.Context, run Run, roles []Role) error {
	now := time.Now()
	workers := make([]Worker, 0, len(roles))
	for _, role := range roles {
		workers = append(workers, newWorker(run, role, now))
	}
	return orchestration.QueueAndLaunch(ctx, workers, orchestration.QueueAndLaunchHooks[Worker]{
		Persist: func(ctx context.Context, worker Worker) error {
			return e.store.CreateWorker(ctx, worker)
		},
		OnPersistError: func(_ context.Context, _ Worker, err error) (bool, error) {
			if strings.Contains(err.Error(), "UNIQUE") {
				return true, nil
			}
			return false, err
		},
		AfterPersist: func(ctx context.Context, worker Worker) error {
			e.emit(ctx, run.ID, worker.ID, "worker.queued", fmt.Sprintf("queued %s", worker.RoleTitle), "")
			return nil
		},
		Launch: func(ctx context.Context, worker Worker) error {
			if err := e.spawnWorker(ctx, worker.ID); err != nil {
				return fmt.Errorf("worker %s launch failed: %w", worker.ID, err)
			}
			return nil
		},
		OnLaunchError: func(ctx context.Context, worker Worker, err error) error {
			_ = e.store.CompleteWorker(ctx, worker.ID, WorkerFailed, "", "", err.Error())
			e.emit(ctx, run.ID, worker.ID, "worker.failed", fmt.Sprintf("worker spawn failed: %s", err.Error()), "")
			_ = e.store.UpdateRunStatus(ctx, run.ID, RunFailed, "")
			_ = e.store.UpdateRunPhase(ctx, run.ID, PhaseBlocked)
			e.emit(ctx, run.ID, "", "run.failed", fmt.Sprintf("worker spawn failed: %s", err.Error()), "")
			return err
		},
	})
}

func newWorker(run Run, role Role, now time.Time) Worker {
	backend := firstNonEmpty(run.Backend, role.Backend)
	scoped := backendConfigFor(role.Backends, backend)
	roleModel := ""
	roleProvider := ""
	roleModelOptions := RuntimeModelOptions{}
	if role.Backend == "" || role.Backend == backend {
		roleModel = role.Model
		roleProvider = role.Provider
		roleModelOptions = role.ModelOptions
	}
	model := firstNonEmpty(scoped.Model, roleModel, run.DefaultModel)
	modelOptions := api.MergeModelOptions(scoped.ModelOptions, roleModelOptions, run.DefaultModelOptions)
	provider := firstNonEmpty(scoped.Provider, roleProvider, run.DefaultProvider, api.InferModelProvider(model))
	if strings.TrimSpace(run.Model) != "" {
		model = strings.TrimSpace(run.Model)
		provider = firstNonEmpty(api.InferModelProvider(model), scoped.Provider, roleProvider)
	}
	if api.HasModelOptions(run.ModelOptions) {
		modelOptions = api.MergeModelOptions(run.ModelOptions, modelOptions)
	}
	return Worker{
		ID:           "wrk-" + randomID(),
		RunID:        run.ID,
		LaunchID:     "launch-" + randomID(),
		RoleID:       role.ID,
		RoleKind:     role.Kind,
		RoleTitle:    role.Title,
		Backend:      backend,
		Provider:     provider,
		Model:        model,
		ModelOptions: modelOptions,
		Agent:        role.Agent,
		Status:       WorkerQueued,
		Attempt:      1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

func (e *Engine) promoteClerkDocket(ctx context.Context, run Run, preset Preset, workers []Worker) []Role {
	plan := BuildPlan(run.Task)
	docket := BuildDocket(plan, preset, run.Workflow)
	docket.DelegationScope = run.DelegationScope
	jurors := jurorRoles(preset)
	docket.Source = DocketClerkHeuristic
	for _, worker := range workers {
		if worker.RoleKind != RoleClerk || worker.Status != WorkerCompleted {
			continue
		}
		docket.ClerkNotes = renderedWorkerResult(worker)
		if delegated, selectedJurors, ok := e.tryDelegatedDocket(ctx, run, preset, plan, worker, docket.ClerkNotes); ok {
			docket = delegated
			jurors = selectedJurors
			e.emit(ctx, run.ID, worker.ID, "docket.delegated", fmt.Sprintf("clerk selected %d juror(s) and %d judge(s)", len(docket.Assignments), len(docket.JudgeIDs)), "")
		}
		break
	}
	e.addArtifact(ctx, run.ID, "", "docket", "json", docket)
	e.emit(ctx, run.ID, "", "docket.ready", "clerk docket ready", "")
	return jurors
}

func (e *Engine) tryDelegatedDocket(ctx context.Context, run Run, preset Preset, plan PlanArtifact, worker Worker, clerkNotes string) (DocketArtifact, []Role, bool) {
	if !delegationScopeEnabled(run.DelegationScope) {
		return DocketArtifact{}, nil, false
	}
	catalog, err := e.delegationCatalogForRun(run)
	if err != nil {
		e.emit(ctx, run.ID, worker.ID, "docket.delegation_fallback", fmt.Sprintf("delegation catalog failed: %s", err.Error()), "")
		return DocketArtifact{}, nil, false
	}
	decision, ok, err := clerkDelegationDecision(worker.ResultJSON)
	if err != nil {
		e.emit(ctx, run.ID, worker.ID, "docket.delegation_fallback", fmt.Sprintf("delegation decision invalid: %s", err.Error()), "")
		return DocketArtifact{}, nil, false
	}
	if !ok {
		e.emit(ctx, run.ID, worker.ID, "docket.delegation_fallback", "clerk did not return a delegation decision; using preset jury", "")
		return DocketArtifact{}, nil, false
	}
	delegated, selectedJurors, err := buildDelegatedDocket(plan, run, preset, catalog, decision)
	if err != nil {
		e.emit(ctx, run.ID, worker.ID, "docket.delegation_fallback", fmt.Sprintf("delegation decision rejected: %s", err.Error()), "")
		return DocketArtifact{}, nil, false
	}
	for _, role := range selectedJurors {
		if err := e.validateBackend(firstNonEmpty(run.Backend, role.Backend)); err != nil {
			e.emit(ctx, run.ID, worker.ID, "docket.delegation_fallback", fmt.Sprintf("delegated juror %q backend rejected: %s", role.ID, err.Error()), "")
			return DocketArtifact{}, nil, false
		}
	}
	if len(selectedJurors) == 0 {
		return DocketArtifact{}, nil, false
	}
	delegated.ClerkNotes = clerkNotes
	return delegated, selectedJurors, true
}

func (e *Engine) spawnWorker(ctx context.Context, workerID string) error {
	return orchestration.LaunchDetachedCommand(ctx, orchestration.CommandLaunchRequest{
		Command: e.workerCommand,
		Args:    []string{"worker", workerID},
		Env: append([]string{
			"COURT_CONFIG_DIR=" + e.configDir,
			"COURT_DATA_DIR=" + e.dataDir,
			"COURT_DB=" + e.dbPath,
		}, workerEnvironment()...),
		LogPath: filepath.Join(e.dataDir, "logs", "worker-process.log"),
	})
}

func (e *Engine) failRunStart(ctx context.Context, runID string, workerID string, cause error) error {
	message := cause.Error()
	if workerID != "" {
		_ = e.store.CompleteWorker(ctx, workerID, WorkerFailed, "", "", message)
		e.emit(ctx, runID, workerID, "worker.failed", fmt.Sprintf("worker spawn failed: %s", message), "")
	}
	_ = e.store.UpdateRunStatus(ctx, runID, RunFailed, "")
	_ = e.store.UpdateRunPhase(ctx, runID, PhaseBlocked)
	e.emit(ctx, runID, "", "run.failed", fmt.Sprintf("run startup failed: %s", message), "")
	return cause
}

func (e *Engine) emit(ctx context.Context, runID, workerID, typ, message, payload string) {
	_ = e.store.AddEvent(ctx, Event{
		RunID:     runID,
		WorkerID:  workerID,
		Type:      typ,
		Message:   message,
		Payload:   payload,
		CreatedAt: time.Now(),
	})
}

func (e *Engine) addArtifact(ctx context.Context, runID, workerID, kind, format string, value any) {
	var content string
	switch typed := value.(type) {
	case string:
		content = typed
	case json.RawMessage:
		content = string(typed)
	default:
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return
		}
		content = string(data)
	}
	_ = e.store.AddArtifact(ctx, Artifact{
		RunID:     runID,
		WorkerID:  workerID,
		Kind:      kind,
		Format:    format,
		Content:   content,
		CreatedAt: time.Now(),
	})
}

func initialRolesFor(preset Preset, workflow WorkflowMode) []Role {
	clerks := clerkRoles(preset)
	if len(clerks) > 0 {
		return clerks
	}
	jurors := jurorRoles(preset)
	if len(jurors) > 0 {
		return jurors
	}
	if workflow == WorkflowReviewOnly {
		return nil
	}
	if judge, ok := judgeRole(preset); ok {
		return []Role{judge}
	}
	return nil
}

func clerkRoles(preset Preset) []Role {
	var roles []Role
	for _, role := range preset.Roles {
		if role.Kind == RoleClerk {
			roles = append(roles, role)
		}
	}
	return roles
}

func jurorRoles(preset Preset) []Role {
	var roles []Role
	for _, role := range preset.Roles {
		if role.Kind == RoleJuror {
			roles = append(roles, role)
		}
	}
	return roles
}

func initialPhase(preset Preset, workflow WorkflowMode) Phase {
	if len(clerkRoles(preset)) > 0 {
		return PhaseClerk
	}
	if len(jurorRoles(preset)) > 0 {
		return PhaseJurors
	}
	if workflow == WorkflowReviewOnly {
		return PhaseComplete
	}
	return PhaseVerdict
}

func phaseInputForRun(run Run, preset Preset, workers []Worker) PhaseInput {
	return PhaseInput{
		RequireClerk:        len(clerkRoles(preset)) > 0,
		DocketReady:         len(clerkRoles(preset)) == 0 || clerkCompleted(workers),
		InlineReviewEnabled: run.Workflow == WorkflowBoundedCorrection,
		VerdictEnabled:      run.Workflow != WorkflowReviewOnly,
		VerdictDisabled:     run.Workflow == WorkflowReviewOnly,
	}
}

func clerkCompleted(workers []Worker) bool {
	for _, worker := range workers {
		if worker.RoleKind == RoleClerk && worker.Status == WorkerCompleted {
			return true
		}
	}
	return false
}

func clerkReadyForJurors(workers []Worker, preset Preset) bool {
	if len(clerkRoles(preset)) == 0 || len(jurorRoles(preset)) == 0 {
		return false
	}
	for _, worker := range workers {
		if worker.RoleKind == RoleClerk && worker.Status != WorkerCompleted {
			return false
		}
	}
	return clerkCompleted(workers)
}

func jurorsStarted(workers []Worker) bool {
	for _, worker := range workers {
		if worker.RoleKind == RoleJuror {
			return true
		}
	}
	return false
}

func judgeRole(preset Preset) (Role, bool) {
	for _, role := range preset.Roles {
		if role.Kind == RoleJudge {
			return role, true
		}
	}
	return Role{}, false
}

func (e *Engine) judgeRolesForRun(ctx context.Context, run Run, preset Preset) ([]Role, error) {
	if !delegationScopeEnabled(run.DelegationScope) {
		if judge, ok := judgeRole(preset); ok {
			return []Role{judge}, nil
		}
		return nil, nil
	}
	docket, ok := e.latestDocket(ctx, run.ID)
	if !ok || len(docket.JudgeIDs) == 0 {
		return presetJudgeRoles(preset), nil
	}
	roots, err := e.delegationRoots(run)
	if err != nil {
		return nil, err
	}
	judges := make([]Role, 0, len(docket.JudgeIDs))
	for _, judgeID := range cleanStringList(docket.JudgeIDs) {
		role, err := delegatedJudgeRole(roots, preset, judgeID)
		if err != nil {
			return nil, err
		}
		judges = append(judges, role)
	}
	return judges, nil
}

func presetJudgeRoles(preset Preset) []Role {
	if judge, ok := judgeRole(preset); ok {
		return []Role{judge}
	}
	return nil
}

func delegatedJudgeRole(roots []string, preset Preset, judgeID string) (Role, error) {
	role, found, err := LoadRoleFromRoots(roots, judgeID)
	if err != nil {
		return Role{}, err
	}
	if found {
		role.Kind = RoleJudge
		return role, nil
	}
	if presetJudge, ok := judgeRole(preset); ok && presetJudge.ID == judgeID {
		presetJudge.Kind = RoleJudge
		return presetJudge, nil
	}
	return Role{}, fmt.Errorf("delegated judge %q not found", judgeID)
}

func (e *Engine) latestDocket(ctx context.Context, runID string) (DocketArtifact, bool) {
	artifacts, err := e.store.ListArtifacts(ctx, runID)
	if err != nil {
		return DocketArtifact{}, false
	}
	for i := len(artifacts) - 1; i >= 0; i-- {
		artifact := artifacts[i]
		if artifact.Kind != "docket" || artifact.Format != "json" {
			continue
		}
		var docket DocketArtifact
		if err := json.Unmarshal([]byte(artifact.Content), &docket); err != nil {
			continue
		}
		return docket, true
	}
	return DocketArtifact{}, false
}

func judgeCompleted(workers []Worker) bool {
	seenJudge := false
	for _, worker := range workers {
		if worker.RoleKind != RoleJudge {
			continue
		}
		seenJudge = true
		if worker.Status != WorkerCompleted {
			return false
		}
	}
	return seenJudge
}

func workerExists(workers []Worker, roleID string) bool {
	for _, worker := range workers {
		if worker.RoleID == roleID {
			return true
		}
	}
	return false
}

func finalVerdict(workers []Worker) string {
	var b strings.Builder
	var judgeResults []Worker
	for _, worker := range workers {
		if worker.RoleKind == RoleJudge {
			if renderedWorkerResult(worker) != "" {
				judgeResults = append(judgeResults, worker)
			}
		}
	}
	if len(judgeResults) == 1 {
		return renderedWorkerResult(judgeResults[0])
	}
	if len(judgeResults) > 1 {
		b.WriteString("# Court Verdict\n\n")
		for _, worker := range judgeResults {
			b.WriteString("## ")
			b.WriteString(worker.RoleTitle)
			b.WriteString("\n\n")
			b.WriteString(renderedWorkerResult(worker))
			b.WriteString("\n\n")
		}
		return strings.TrimSpace(b.String())
	}
	b.WriteString("# Court Verdict\n\n")
	for _, worker := range workers {
		result := renderedWorkerResult(worker)
		if result == "" {
			continue
		}
		b.WriteString("## ")
		b.WriteString(worker.RoleTitle)
		b.WriteString("\n\n")
		b.WriteString(result)
		b.WriteString("\n\n")
	}
	return b.String()
}

func renderedWorkerResult(worker Worker) string {
	if strings.TrimSpace(worker.ResultJSON) != "" {
		rendered, _, ok := parseWorkerResult(worker.ResultJSON)
		if ok {
			return rendered
		}
	}
	return strings.TrimSpace(worker.Result)
}

func randomID() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func workerEnvironment() []string {
	var env []string
	if socketPath := strings.TrimSpace(os.Getenv("AGENTIC_CONTROL_SOCKET_PATH")); socketPath != "" {
		env = append(env, "AGENTIC_CONTROL_SOCKET_PATH="+socketPath)
	}
	if stateDB := strings.TrimSpace(os.Getenv("AGENTIC_CONTROL_STATE_DB")); stateDB != "" {
		env = append(env, "AGENTIC_CONTROL_STATE_DB="+stateDB)
	}
	return env
}

func failedWorkerIDFromRunStartError(err error) string {
	message := err.Error()
	const prefix = "worker "
	const suffix = " launch failed:"
	if !strings.HasPrefix(message, prefix) {
		return ""
	}
	message = strings.TrimPrefix(message, prefix)
	if index := strings.Index(message, suffix); index > 0 {
		return strings.TrimSpace(message[:index])
	}
	return ""
}
