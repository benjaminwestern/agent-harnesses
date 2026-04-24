// Package court provides Court runtime functionality.
package court

import "fmt"

const defaultPresetID = "review"

var presets = map[string]Preset{
	defaultPresetID: {
		ID:          defaultPresetID,
		Title:       "Review",
		Description: "Three parallel reviewers followed by a final judge.",
		Workflow:    WorkflowParallelConsensus,
		Roles: []Role{
			{
				ID:    "architecture_reviewer",
				Kind:  RoleJuror,
				Title: "Architecture Reviewer",
				Brief: "Review architecture, boundaries, state ownership, orchestration flow, maintainability, and likely failure modes.",
			},
			{
				ID:    "api_designer",
				Kind:  RoleJuror,
				Title: "API and DX Reviewer",
				Brief: "Review the public API, CLI workflow, configuration shape, naming, defaults, and whether routine changes are easy to make safely.",
			},
			{
				ID:    "testing_reviewer",
				Kind:  RoleJuror,
				Title: "Testing and Reliability Reviewer",
				Brief: "Review test coverage, concurrency risks, recovery behaviour, data durability, and operational confidence.",
			},
			{
				ID:    "final_judge",
				Kind:  RoleJudge,
				Title: "Final Judge",
				Brief: "Synthesize the worker findings into a direct verdict with priority decisions, implementation guidance, and residual risks.",
			},
		},
	},
	"parallel": {
		ID:          "parallel",
		Title:       "Parallel",
		Description: "Three general workers followed by a final judge.",
		Workflow:    WorkflowParallelConsensus,
		Roles: []Role{
			{ID: "worker_a", Kind: RoleJuror, Title: "Worker A", Brief: "Attack the task from an implementation perspective and produce concrete findings."},
			{ID: "worker_b", Kind: RoleJuror, Title: "Worker B", Brief: "Attack the task from a product and workflow perspective and produce concrete findings."},
			{ID: "worker_c", Kind: RoleJuror, Title: "Worker C", Brief: "Attack the task from a risk and verification perspective and produce concrete findings."},
			{ID: "final_judge", Kind: RoleJudge, Title: "Final Judge", Brief: "Synthesize the worker outputs into a practical verdict."},
		},
	},
	"single": {
		ID:          "single",
		Title:       "Single",
		Description: "One agentic-control worker. Useful for smoke testing the Court backend.",
		Workflow:    WorkflowParallelConsensus,
		Roles: []Role{
			{ID: "worker", Kind: RoleJudge, Title: "Worker", Brief: "Complete the task directly and produce a concise result."},
		},
	},
}

// ResolvePreset provides Court runtime functionality.
func ResolvePreset(id string) (Preset, error) {
	if id == "" {
		id = defaultPresetID
	}
	switch id {
	case "default":
		id = defaultPresetID
	case "sdk_self_review":
		id = defaultPresetID
	}
	preset, ok := presets[id]
	if !ok {
		return Preset{}, fmt.Errorf("unknown preset %q", id)
	}
	return preset, nil
}

// ListPresets provides Court runtime functionality.
func ListPresets() []Preset {
	out := make([]Preset, 0, len(presets))
	for _, preset := range presets {
		out = append(out, preset)
	}
	return out
}
