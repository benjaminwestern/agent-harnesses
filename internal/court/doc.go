// Package court provides Agentic Control's multi-agent workflow engine.
//
// Court owns workflow semantics, catalog resolution, persisted SQLite state,
// worker orchestration, artifacts, requests, and verdict generation. It runs
// in process and reuses the shared runtime control-plane layer from
// pkg/controlplane.
package court
