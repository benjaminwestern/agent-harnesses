// Package orchestration defines Agentic Control's native workflow orchestration
// layer.
//
// This layer sits above pkg/controlplane and below higher-level workflow
// modules such as Court. It is the right home for generic concurrent fan-out,
// worker scheduling, runtime target selection, cancellation, aggregation, and
// reusable workflow primitives that should benefit both Court and non-Court
// product flows.
package orchestration
