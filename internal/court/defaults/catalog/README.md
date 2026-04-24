# Court resources

This directory makes Court resources first-class project citizens. The same
catalog shape is used globally under `~/.config/court`, project-locally under
`.court`, and inside the embedded defaults installed by `court setup`.

You can define and override these resource types with Markdown files and YAML
frontmatter.

Each resource can also expose reusable aliases with `alias:` or `aliases:` in
frontmatter. Aliases are checked in one shared namespace across agents, roles,
juries, presets, and evals so they stay unambiguous for CLI and UI routing.

The supported resource types are:

- `agents/` for reusable execution surfaces such as model, tools, and
  permissions
- `roles/` for clerk, juror, and judge personas plus their system prompts
- `juries/` for juror groupings
- `presets/` for runnable court configurations
- `evals/` for repeatable case sets that compare presets across the same tasks

Configuration rolls up from broad to specific:

1. `~/.config/court`
2. parent workspace `.court` directories
3. the nearest/nested workspace `.court`

Runtime state is stored separately under `~/.local/share/court`.

## Frontmatter guide

### Agents

Use agents for reusable runtime defaults.

```md
---
id: default_reviewer
aliases: review-agent
title: Default reviewer agent
backend: opencode
backends:
  opencode:
    provider: google
    model: opencode/gemini-3-flash
  codex:
    provider: openai
    model: gpt-5.4
tools: read,bash,grep,find,ls,edit,write
permissions: workspace-write
---
```

Flat `backend`, `provider`, and `model` fields are still accepted for one-off
catalog entries. Prefer `backends:` for reusable agents and roles because each
runtime has its own provider and model naming conventions.

### Roles

Use roles for behaviour and system prompts.

```md
---
id: reviewer
aliases: general-reviewer
kind: juror
title: General reviewer
agent: default_reviewer
---

You are an experienced staff engineer acting as a juror.
```

### Juries

Use juries for juror groupings.

```md
---
id: standard_review
aliases: standard-jury
title: Standard review jury
jurors: reviewer,security,performance
---
```

### Presets

Use presets for runnable court modes.

```md
---
id: parallel
aliases: quick-review
name: Parallel jury
routing: broadcast
jury: standard_review
verdictMode: best_answer
correctionsEnabled: false
---
```

Advanced preset fields let you tune policy without changing the Court core:

- `assignmentStrategy`: `explicit` or `heuristic_auto`
- `maxAssignmentRetries`: bounded retries per juror-assignment lane
- `retryDelayMs`: base retry delay in milliseconds
- `retryBackoff`: retry multiplier for later attempts
- `requireFinalJudgeForPromotion`: whether a run needs a final judge before the
  default self-honing gate can pass
- `minAverageFindingConfidence`: minimum average juror confidence for the
  default scorecard gate
- `minVerdictConfidence`: minimum final-judge confidence for the default
  scorecard gate
- `maxCriticalSuggestions`: maximum critical inline-judge warnings allowed for
  the default scorecard gate

The built-in convention is to auto-assign when the clerk leaves juror targets
empty, then retry each juror-assignment lane once with a `3000ms` base delay
and `2x` backoff. For supervised self-honing, the default gate also expects a
final judge, a `0.7` average juror-confidence floor, a `0.75` verdict-confidence
floor, and zero critical inline-judge warnings. That means juror-only presets
remain useful for exploration, but they stay blocked for promotion unless you
relax that policy explicitly.

`clerk`, `inlineJudge`, and `finalJudge` are all optional in preset
frontmatter. That lets you express a plain parallel jury, a clerk-routed court,
a judged review, or any layered combination. Court treats `parallel` as
the default preset when you omit a preset name.

Clerk dockets can include assignment dependencies. A clerk can stage work with
`dependsOn` edges instead of spawning the whole jury plan at once.

### Evals

Use eval suites for repeatable case sets that compare one or more presets.

```md
---
id: self_honing_v1
title: Court self-honing v1 baseline
presets: review,review_inline,collaborative_review
repetitions: 1
timeoutMs: 600000
promotePreset: collaborative_review
---

## routing_recovery | Routing and recovery semantics
Review the runtime and confirm failed or empty-result juror lanes cannot
silently pass.
```

Each `##` section becomes one eval case. The section heading format is:

- `## case_id`
- or `## case_id | Human title`

Eval execution is part of the Court core roadmap. Eval definitions use the
same markdown/frontmatter catalog model so they can be exposed by the CLI, UI,
or service bindings without changing the catalog format.

## Targeting And Presets

Aliases make court resources feel more like reusable tools or skills. Today the
CLI runs presets directly:

```bash
court run --preset <preset-or-alias> "<task>"
```

The CLI is intentionally thin. Richer targeting surfaces should still call the
same Go core operations and consume the same SQLite-backed trace data.

The CLI is only an exposure layer. UI bindings should consume the same Go core
operations and SQLite-backed trace data rather than reimplementing orchestration
or runtime integration.

## Project-local repo review court

This project ships a dedicated repo-understanding court.

### Presets

- `repo_review`: clerk-routed jury plus a final judge that synthesises the
  output into one onboarding-friendly verdict
- `repo_review_jury_only`: the same specialist jury and clerk, but without a
  final judge when you want raw findings instead of a synthesized verdict

### What it is designed to answer

The repo review court is tuned for questions such as:

- "Please review this repo. I want to start working in it."
- "Please review this repo. I want to understand how Y feature is
  implemented."
- "Please review this repo. Break down its data flow / state flow so I can
  understand it."
- "Please review this repo versus X and give me a feature matrix."

The clerk normalises the ask before routing work. If the prompt is
underspecified, it defaults to a high-level review across the major areas:
what the repo does, why it exists, how it works, its logical map, its major
flows, and where a newcomer should start.

### Included roles

- `repo_purpose_analyst`: what the repo does, why it exists, and the workflows
  it serves
- `repo_architecture_analyst`: how it is built, the logical map, and feature
  implementation tracing
- `repo_flow_analyst`: runtime, data, and state flow tracing
- `repo_comparison_analyst`: benefits over `X`, tradeoffs, and a
  feature-matrix-ready comparison when `X` is supplied
- `repo_review_clerk`: interprets the ask and routes assignments
- `repo_review_judge`: synthesises the findings into a skimmable final review

### Example commands

```bash
court run --preset repo_review "please review this repo, I want to start working in it"
court run --preset repo_review "please review this repo, I want to understand how the court core is implemented"
court run --preset repo_review "please review this repo, break down its data flow and state flow"
court run --preset repo_review "please review this repo versus tmux and include a feature matrix"
court run --preset repo_review_jury_only "please review this repo, I want the raw jury findings"
```

## Project-local security courts

This project ships two dedicated security courts grounded in the bundled
security-review role prompts and compatible external skill sources:

- `security-best-practices`
- `security-threat-model`

These courts are deliberately broad. They do **not** reduce security review to
only HTTP or TCP surfaces. They are designed to sweep across whatever the repo
actually contains, including browser, CLI, file, parser, queue, worker,
subprocess, config, environment, build, CI, release, admin, and local
operational surfaces when those are in scope.

### Security best-practices court

Preset:
- `security_best_practices_review`

The preset name already selects the security-review lens. The task can stay
short, such as `review this repo` or `review this subsystem`.

What it is designed to answer:
- "Review this repo against security best practices and give me a prioritised
  report."
- "Help me make this codebase more secure by default."
- "Review this project for security issues across the whole stack, not just the
  web server."

What it covers:
- backend and server-side security posture
- frontend and browser security posture when a frontend exists
- auth, sessions, secrets, and sensitive-data handling
- local files, config, env vars, queues, workers, scripts, build and CI
  surfaces
- runtime, deployment, dependency, and operational configuration

Output behaviour:
- the final judge writes `security_best_practices_report.md` by default
- findings are grouped by severity and given `SBP-###` IDs

Example:

```bash
court run --preset security_best_practices_review "review this repo"
court run --preset security_best_practices_review "review this subsystem"
```

### Security threat-model court

Preset:
- `security_threat_model_review`

The preset name already selects the threat-model lens. The task can stay short,
such as `threat model this repo`.

What it is designed to answer:
- "Threat model this repo."
- "Threat model this subpath or subsystem."
- "Give me a repo-grounded threat model that includes operational and local
  attack surfaces, not just internet-facing ones."

What it covers:
- system model and entrypoints
- trust boundaries, assets, and attacker capabilities
- non-network and operational surfaces such as CLI, files, queues, workers,
  build, CI, release, and admin tooling
- concrete abuse paths, mitigations, and focus paths for manual review

Output behaviour:
- the final judge writes `<repo-or-dir-name>-threat-model.md`
- when service context is missing, the judge begins with an assumption-
  validation check-in and then produces a provisional threat model
- threat rows use `TM-###` IDs

Example:

```bash
court run --preset security_threat_model_review "threat model this repo"
court run --preset security_threat_model_review "threat model this subsystem"
```

## Project-local performance, flow, and grug review court

This project ships a dedicated court for performance, data flow, harmful
duplication, interface fragmentation, and grug-style simplification.

The court is grounded in the commonly known grug-brained developer principles:
- simple and obvious beats clever and abstract
- direct data flow beats hidden magic
- a little honest duplication can be cheaper than the wrong abstraction
- one boring interface is often better than many bespoke ones
- fewer moving parts, fewer shapes, and fewer special cases usually win

### Presets

- `performance_flow_review`: clerk-routed specialist jury plus a final judge
  that synthesises the findings into one grug-aware review
- `performance_flow_jury_only`: the same specialist jury and clerk, but without
  a final judge when you want raw findings

The preset name already selects the grug / performance / flow lens. The task
can stay short, such as `review this repo` or `review this subsystem`.

### What it is designed to answer

- "Review this repo for performance anti-patterns and poor implementations."
- "Break down the data flow and point out where it gets too complex."
- "Find duplicate functions or duplicated systems that should be consolidated."
- "Show me where multiple implementations could be standardised behind a
  simpler unified interface."
- "Review feature X through a grug-brained lens."

### Included roles

- `hot_path_performance_analyst`: wasted work, hot paths, repeated work, and
  likely scale bottlenecks
- `data_flow_complexity_analyst`: data flow, state flow, control-flow
  complexity, and source-to-sink tracing
- `duplication_consolidation_analyst`: harmful duplication, near-duplicate
  logic, and drift-prone reimplementations
- `interface_unification_analyst`: fragmented subsystem contracts and
  unification opportunities
- `grug_simplicity_analyst`: overengineering, anti-patterns, magic, needless
  abstraction, and non-local reasoning traps
- `performance_flow_clerk`: interprets the ask and routes assignments
- `performance_flow_judge`: synthesises the findings using grug-style
  simplicity rules

### Output behaviour

- the final judge writes `performance_flow_review.md` by default
- the verdict emphasises:
  - hot paths and wasted work
  - data and state-flow complexity
  - harmful duplication
  - interface unification opportunities
  - grug-brained simplification opportunities

### Example commands

```bash
court run --preset performance_flow_review "review this repo"
court run --preset performance_flow_review "review this subsystem"
court run --preset performance_flow_review "review the court runtime"
court run --preset performance_flow_jury_only "review this repo"
```

## Project-local convention-over-configuration court

This project ships a dedicated court for convention over configuration,
omakase-style integrated defaults, and Ruby-on-Rails-doctrine-inspired review.

I interpret "omacon" here as the commonly known Rails doctrine phrase
"omakase".

The court is grounded in these principles:
- convention over configuration
- the menu is omakase: coherent defaults over a buffet of bespoke choices
- one obvious, predictable way beats many local variants
- programmer happiness comes from consistency and discoverability
- escape hatches are fine, but they should be rare and bounded

### Presets

- `convention_over_configuration_review`: clerk-routed specialist jury plus a
  final judge that synthesises the findings into one doctrine-aware review
- `convention_over_configuration_jury_only`: the same specialist jury and
  clerk, but without a final judge when you want raw findings

The preset name already selects the convention-over-configuration / omakase /
doctrine lens. The task can stay short, such as `review this repo` or `review
this subsystem`.

### What it is designed to answer

- "Review this repo for convention-over-configuration anti-patterns."
- "Show me where this codebase is too configurable or too bespoke."
- "Find the places where the repo fights framework or local conventions."
- "Identify escape hatches, custom layers, or fragmented subsystem choices that
  should be standardised."
- "Review this subsystem through an omakase / Rails-doctrine lens."

### Included roles

- `convention_alignment_analyst`: naming, layout, structural predictability,
  and convention drift
- `configuration_sprawl_analyst`: flags, env vars, wiring, option matrices,
  and excessive configuration
- `omakase_integration_analyst`: integrated-stack coherence versus bespoke
  subsystem variety
- `escape_hatch_antipattern_analyst`: costly convention bypasses, mini-
  frameworks, magic, and custom layers
- `developer_happiness_consistency_analyst`: predictability, consistency,
  discoverability, and cross-cutting anti-patterns
- `convention_over_configuration_clerk`: interprets the ask and routes
  assignments
- `convention_over_configuration_judge`: synthesises the findings into a
  doctrine-aware review

### Output behaviour

- the final judge writes `convention_over_configuration_review.md` by default
- the verdict emphasises:
  - convention alignment
  - configuration sprawl
  - omakase and integrated-stack issues
  - escape hatches and bespoke layers
  - developer happiness and consistency

### Example commands

```bash
court run --preset convention_over_configuration_review "review this repo"
court run --preset convention_over_configuration_review "review this subsystem"
court run --preset convention_over_configuration_review "review the court runtime"
court run --preset convention_over_configuration_jury_only "review this repo"
```
