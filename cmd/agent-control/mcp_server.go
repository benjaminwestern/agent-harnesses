package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/benjaminwestern/agentic-control/internal/orchestration"
	"github.com/benjaminwestern/agentic-control/pkg/contract"
	api "github.com/benjaminwestern/agentic-control/pkg/controlplane"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpServer struct {
	client      *socketRPCClient
	workspaceID string
}

func newMCPServer(socketPath, workspaceID string) *mcpServer {
	return &mcpServer{
		client:      newSocketRPCClient(socketPath),
		workspaceID: workspaceID,
	}
}

func (s *mcpServer) ServeStdio(ctx context.Context) error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	encoder := json.NewEncoder(os.Stdout)
	var encoderMu sync.Mutex

	for scanner.Scan() {
		line := scanner.Bytes()
		var req mcpRequest
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}

		go func(req mcpRequest) {
			res := s.handleRequest(ctx, req)
			if res != nil {
				encoderMu.Lock()
				_ = encoder.Encode(res)
				encoderMu.Unlock()
			}
		}(req)
	}

	return scanner.Err()
}

func (s *mcpServer) handleRequest(ctx context.Context, req mcpRequest) *mcpResponse {
	if len(req.ID) == 0 {
		return nil
	}

	res := &mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		res.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"resources": map[string]any{},
				"tools":     map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "agentic-control",
				"version": "1.0.0",
			},
		}

	case "ping":
		res.Result = map[string]any{}

	case "resources/list":
		docs, err := s.client.ListDocuments(ctx, s.workspaceID)
		if err != nil {
			res.Error = &mcpError{Code: -32000, Message: err.Error()}
			return res
		}

		resources := make([]map[string]any, 0, len(docs))
		for _, doc := range docs {
			resources = append(resources, map[string]any{
				"uri":      fmt.Sprintf("document://%s", doc.ID),
				"name":     doc.Name,
				"mimeType": "text/markdown",
			})
		}

		res.Result = map[string]any{
			"resources": resources,
		}

	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			res.Error = &mcpError{Code: -32602, Message: "invalid params"}
			return res
		}

		if !strings.HasPrefix(params.URI, "document://") {
			res.Error = &mcpError{Code: -32001, Message: "unknown resource URI scheme"}
			return res
		}

		docID := strings.TrimPrefix(params.URI, "document://")
		doc, err := s.client.GetDocument(ctx, s.workspaceID, docID)
		if err != nil {
			res.Error = &mcpError{Code: -32000, Message: err.Error()}
			return res
		}

		res.Result = map[string]any{
			"contents": []map[string]any{
				{
					"uri":      params.URI,
					"mimeType": "text/markdown",
					"text":     doc.Content,
				},
			},
		}

	case "tools/list":
		res.Result = map[string]any{
			"tools": []map[string]any{
				{
					"name":        "generate_synthetic_data",
					"description": "Generate synthetic dataset items using a specific model and JSON schema. Returns the generated items as a JSON string.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt": map[string]any{
								"type":        "string",
								"description": "The prompt instructing the model on what to generate.",
							},
							"schema": map[string]any{
								"type":        "object",
								"description": "The JSON schema to strictly enforce for the generated items.",
							},
							"count": map[string]any{
								"type":        "integer",
								"description": "The number of items to generate.",
								"default":     5,
							},
							"target": map[string]any{
								"type":        "string",
								"description": "The target runtime and model (e.g. openaicompatible=gpt-4o).",
								"default":     "openaicompatible=gpt-4o",
							},
						},
						"required": []string{"prompt", "schema"},
					},
				},
				{
					"name":        "run_evaluation",
					"description": "Run an LLM-as-a-judge evaluation against a provided JSON string using a specific model.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt": map[string]any{
								"type":        "string",
								"description": "The evaluation prompt or grading rubric.",
							},
							"output_to_evaluate": map[string]any{
								"type":        "string",
								"description": "The string (e.g. JSON output) to be evaluated by the judge.",
							},
							"target": map[string]any{
								"type":        "string",
								"description": "The target judge runtime and model (e.g. openaicompatible=claude-3-5-sonnet-latest).",
								"default":     "openaicompatible=gpt-4o",
							},
						},
						"required": []string{"prompt", "output_to_evaluate"},
					},
				},
				{
					"name":        "run_batch_evaluation",
					"description": "Run an LLM-as-a-judge evaluation across an array of dataset items.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt": map[string]any{
								"type":        "string",
								"description": "The evaluation prompt or grading rubric.",
							},
							"items": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"input_payload": map[string]any{"type": "string"},
										"target_output": map[string]any{"type": "string"},
									},
									"required": []string{"input_payload", "target_output"},
								},
								"description": "The items to test.",
							},
							"target": map[string]any{
								"type":        "string",
								"description": "The target model generating responses.",
								"default":     "openaicompatible=gpt-4o-mini",
							},
							"judge": map[string]any{
								"type":        "string",
								"description": "The judge model scoring responses.",
								"default":     "openaicompatible=gpt-4o",
							},
							"mode": map[string]any{
								"type":        "string",
								"description": "The evaluation mode ('evaluate' or 'g_eval').",
								"default":     "evaluate",
							},
						},
						"required": []string{"prompt", "items"},
					},
				},
				{
					"name":        "run_judge_alignment",
					"description": "Evaluate a judge model against human-labelled dataset items and return alignment metrics.",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"prompt": map[string]any{
								"type":        "string",
								"description": "The evaluation prompt or grading rubric.",
							},
							"items": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"input_payload":  map[string]any{"type": "string"},
										"target_output":  map[string]any{"type": "string"},
										"targetOutput":   map[string]any{"type": "string"},
										"expected_score": map[string]any{"type": "number"},
									},
									"required": []string{"input_payload", "target_output"},
								},
								"description": "Human-labelled items. target_output should contain the expected judge JSON with score and passed fields.",
							},
							"judge": map[string]any{
								"type":        "string",
								"description": "The judge model to evaluate.",
								"default":     "openaicompatible=gpt-4o",
							},
							"mode": map[string]any{
								"type":        "string",
								"description": "The evaluation mode ('evaluate' or 'g_eval').",
								"default":     "evaluate",
							},
						},
						"required": []string{"prompt", "items"},
					},
				},
				{
					"name":        "memory_set",
					"description": "Set a key-value pair in workspace memory",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"key":   map[string]any{"type": "string"},
							"value": map[string]any{"type": "string"},
						},
						"required": []string{"key", "value"},
					},
				},
				{
					"name":        "memory_get",
					"description": "Get a value from workspace memory by key",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"key": map[string]any{"type": "string"}},
						"required":   []string{"key"},
					},
				},
				{
					"name":        "memory_delete",
					"description": "Delete a key from memory",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"key": map[string]any{"type": "string"}},
						"required":   []string{"key"},
					},
				},
				{
					"name":        "memory_list",
					"description": "List all memory keys",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "documents_write",
					"description": "Write to a document",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":    map[string]any{"type": "string"},
							"content": map[string]any{"type": "string"},
						},
						"required": []string{"name", "content"},
					},
				},
				{
					"name":        "documents_append",
					"description": "Append to a document",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":      map[string]any{"type": "string"},
							"content": map[string]any{"type": "string"},
						},
						"required": []string{"id", "content"},
					},
				},
				{
					"name":        "documents_add_metadata",
					"description": "Add metadata key/value to a document",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":    map[string]any{"type": "string"},
							"key":   map[string]any{"type": "string"},
							"value": map[string]any{"type": "string"},
						},
						"required": []string{"id", "key", "value"},
					},
				},
				{
					"name":        "documents_rename",
					"description": "Rename a document",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"name": map[string]any{"type": "string"},
						},
						"required": []string{"id", "name"},
					},
				},
				{
					"name":        "documents_archive",
					"description": "Archive a document",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":       map[string]any{"type": "string"},
							"archived": map[string]any{"type": "boolean"},
						},
						"required": []string{"id", "archived"},
					},
				},
				{
					"name":        "documents_clear",
					"description": "Clear a document",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "documents_get",
					"description": "Get a document by ID",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "documents_delete",
					"description": "Delete a document by ID",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "documents_list",
					"description": "List all documents",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "tasks_create",
					"description": "Create a new task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"title": map[string]any{"type": "string"},
							"body":  map[string]any{"type": "string"},
						},
						"required": []string{"title"},
					},
				},
				{
					"name":        "tasks_add_metadata",
					"description": "Add metadata key/value to a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":    map[string]any{"type": "string"},
							"key":   map[string]any{"type": "string"},
							"value": map[string]any{"type": "string"},
						},
						"required": []string{"id", "key", "value"},
					},
				},
				{
					"name":        "tasks_add_tag",
					"description": "Add tag to a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":  map[string]any{"type": "string"},
							"tag": map[string]any{"type": "string"},
						},
						"required": []string{"id", "tag"},
					},
				},
				{
					"name":        "tasks_remove_tag",
					"description": "Remove tag from a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":  map[string]any{"type": "string"},
							"tag": map[string]any{"type": "string"},
						},
						"required": []string{"id", "tag"},
					},
				},
				{
					"name":        "tasks_set_blockers",
					"description": "Set blockers for a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":          map[string]any{"type": "string"},
							"blocker_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
						},
						"required": []string{"id", "blocker_ids"},
					},
				},
				{
					"name":        "tasks_add_blocker",
					"description": "Add a blocker to a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":         map[string]any{"type": "string"},
							"blocker_id": map[string]any{"type": "string"},
						},
						"required": []string{"id", "blocker_id"},
					},
				},
				{
					"name":        "tasks_remove_blocker",
					"description": "Remove a blocker from a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":         map[string]any{"type": "string"},
							"blocker_id": map[string]any{"type": "string"},
						},
						"required": []string{"id", "blocker_id"},
					},
				},
				{
					"name":        "tasks_lock",
					"description": "Lock a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":       map[string]any{"type": "string"},
							"actor_id": map[string]any{"type": "string"},
						},
						"required": []string{"id", "actor_id"},
					},
				},
				{
					"name":        "tasks_unlock",
					"description": "Unlock a task",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":       map[string]any{"type": "string"},
							"actor_id": map[string]any{"type": "string"},
						},
						"required": []string{"id", "actor_id"},
					},
				},
				{
					"name":        "tasks_get",
					"description": "Get a task by ID",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "tasks_delete",
					"description": "Delete a task by ID",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "tasks_list",
					"description": "List open tasks in the workspace",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{},
					},
				},
				{
					"name":        "tasks_comments_create",
					"description": "Create a task comment",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"task_id": map[string]any{"type": "string"},
							"author":  map[string]any{"type": "string"},
							"body":    map[string]any{"type": "string"},
						},
						"required": []string{"task_id", "author", "body"},
					},
				},
				{
					"name":        "tasks_comments_update",
					"description": "Update a task comment",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"body": map[string]any{"type": "string"},
						},
						"required": []string{"id", "body"},
					},
				},
				{
					"name":        "tasks_comments_delete",
					"description": "Delete a task comment",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "tasks_comments_list",
					"description": "List task comments",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"task_id": map[string]any{"type": "string"}},
						"required":   []string{"task_id"},
					},
				},
				{
					"name":        "wakeups_set",
					"description": "Set a wakeup",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"owner_id":  map[string]any{"type": "string"},
							"due_at_ms": map[string]any{"type": "number"},
							"body":      map[string]any{"type": "string"},
						},
						"required": []string{"owner_id", "due_at_ms", "body"},
					},
				},
				{
					"name":        "wakeups_get",
					"description": "Get a wakeup",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "wakeups_cancel",
					"description": "Cancel a wakeup",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "wakeups_pause",
					"description": "Pause a wakeup",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "wakeups_resume",
					"description": "Resume a wakeup",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"id": map[string]any{"type": "string"}},
						"required":   []string{"id"},
					},
				},
				{
					"name":        "wakeups_reset",
					"description": "Reset a wakeup",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":        map[string]any{"type": "string"},
							"due_at_ms": map[string]any{"type": "number"},
						},
						"required": []string{"id", "due_at_ms"},
					},
				},
				{
					"name":        "wakeups_list_pending",
					"description": "List pending wakeups",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "leases_acquire",
					"description": "Acquire a lease",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"lock_key":      map[string]any{"type": "string"},
							"owner_id":      map[string]any{"type": "string"},
							"expires_at_ms": map[string]any{"type": "number"},
						},
						"required": []string{"lock_key", "owner_id", "expires_at_ms"},
					},
				},
				{
					"name":        "leases_release",
					"description": "Release a lease",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"lock_key": map[string]any{"type": "string"},
							"owner_id": map[string]any{"type": "string"},
						},
						"required": []string{"lock_key", "owner_id"},
					},
				},
				{
					"name":        "leases_get",
					"description": "Get a lease",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"lock_key": map[string]any{"type": "string"}},
						"required":   []string{"lock_key"},
					},
				},
				{
					"name":        "leases_reset",
					"description": "Reset (force delete) a lease",
					"inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"lock_key": map[string]any{"type": "string"}},
						"required":   []string{"lock_key"},
					},
				},
			},
		}

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			res.Error = &mcpError{Code: -32602, Message: "invalid params"}
			return res
		}

		resultText := ""

		switch params.Name {
		case "generate_synthetic_data":
			prompt, _ := params.Arguments["prompt"].(string)
			schemaAny, _ := params.Arguments["schema"].(map[string]any)
			countFloat, _ := params.Arguments["count"].(float64)
			target, _ := params.Arguments["target"].(string)

			count := int(countFloat)
			if count == 0 {
				count = 5
			}
			if target == "" {
				target = "openaicompatible=gpt-4o"
			}

			controller := fanoutController("")
			fanoutOpts := orchestration.FanoutOptions{
				Prompt:  orchestration.BuildSyntheticDataPrompt(prompt, count, nil),
				Targets: parseFanoutTargets([]string{target}, nil),
				ModelOptions: api.ModelOptions{
					ResponseSchema: schemaAny,
				},
				EventBuffer: 1024,
			}

			resFanout, err := orchestration.RunFanout(ctx, controller, fanoutOpts)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}

			if len(resFanout.Targets) == 0 || resFanout.Targets[0].Error != "" {
				return s.toolError(req.ID, fmt.Sprintf("generation failed: %s", resFanout.Targets[0].Error))
			}

			resultText = resFanout.Targets[0].Text

		case "run_evaluation":
			prompt, _ := params.Arguments["prompt"].(string)
			outputToEvaluate, _ := params.Arguments["output_to_evaluate"].(string)
			targetRaw, _ := params.Arguments["target"].(string)

			if targetRaw == "" {
				targetRaw = "openaicompatible=gpt-4o"
			}

			controller := fanoutController("")

			// Re-use FanoutResult as input for reduction
			fanoutRes := orchestration.FanoutResult{
				Prompt: prompt,
				Targets: []orchestration.FanoutTargetResult{
					{
						Target: orchestration.FanoutTarget{Label: "target_output"},
						Text:   outputToEvaluate,
					},
				},
			}

			targets := parseFanoutTargets([]string{targetRaw}, nil)
			if len(targets) == 0 {
				return s.toolError(req.ID, "invalid target")
			}

			reduceRes, err := orchestration.RunReduction(ctx, controller, orchestration.ReductionModeEvaluate, fanoutRes, targets[0], false)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}

			resultText = reduceRes.JSON

		case "run_batch_evaluation":
			prompt, _ := params.Arguments["prompt"].(string)
			itemsRaw, _ := params.Arguments["items"].([]any)
			target, _ := params.Arguments["target"].(string)
			judge, _ := params.Arguments["judge"].(string)
			mode, _ := params.Arguments["mode"].(string)

			if target == "" {
				target = "openaicompatible=gpt-4o-mini"
			}
			if judge == "" {
				judge = "openaicompatible=gpt-4o"
			}
			if mode == "" {
				mode = "evaluate"
			}

			var items []orchestration.DatasetItemRecord
			itemsBytes, _ := json.Marshal(itemsRaw)
			_ = json.Unmarshal(itemsBytes, &items)

			controller := fanoutController("")

			res, err := orchestration.RunBatchEvaluation(ctx, controller, orchestration.BatchEvaluationOptions{
				Items:       items,
				Prompt:      prompt,
				TargetModel: target,
				JudgeModel:  judge,
				Mode:        orchestration.ReductionMode(mode),
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}

			out, _ := json.MarshalIndent(res, "", "  ")
			resultText = string(out)

		case "run_judge_alignment":
			prompt, _ := params.Arguments["prompt"].(string)
			itemsRaw, _ := params.Arguments["items"].([]any)
			judge, _ := params.Arguments["judge"].(string)
			mode, _ := params.Arguments["mode"].(string)

			if judge == "" {
				judge = "openaicompatible=gpt-4o"
			}
			if mode == "" {
				mode = "evaluate"
			}

			var items []orchestration.DatasetItemRecord
			itemsBytes, _ := json.Marshal(itemsRaw)
			_ = json.Unmarshal(itemsBytes, &items)

			controller := fanoutController("")

			res, err := orchestration.RunJudgeAlignmentEvaluation(ctx, controller, orchestration.JudgeAlignmentOptions{
				Items:      items,
				Prompt:     prompt,
				JudgeModel: judge,
				Mode:       orchestration.ReductionMode(mode),
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}

			out, _ := json.MarshalIndent(res, "", "  ")
			resultText = string(out)

		case "memory_set":
			key, _ := params.Arguments["key"].(string)
			value, _ := params.Arguments["value"].(string)
			err := s.client.SetMemory(ctx, contract.MemoryEntry{
				WorkspaceID: s.workspaceID,
				Key:         key,
				Value:       value,
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = fmt.Sprintf("Successfully set memory key %q", key)

		case "memory_get":
			key, _ := params.Arguments["key"].(string)
			entry, err := s.client.GetMemory(ctx, s.workspaceID, key)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = entry.Value

		case "memory_delete":
			key, _ := params.Arguments["key"].(string)
			err := s.client.DeleteMemory(ctx, s.workspaceID, key)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = fmt.Sprintf("Successfully deleted memory key %q", key)

		case "memory_list":
			items, err := s.client.ListMemory(ctx, s.workspaceID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(items, "", "  ")
			resultText = string(b)

		case "documents_write":
			name, _ := params.Arguments["name"].(string)
			content, _ := params.Arguments["content"].(string)
			doc, err := s.client.WriteDocument(ctx, contract.Document{
				WorkspaceID: s.workspaceID,
				Name:        name,
				Content:     content,
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = fmt.Sprintf("Successfully wrote document %q (ID: %s, Revision: %d)", name, doc.ID, doc.Revision)

		case "documents_append":
			id, _ := params.Arguments["id"].(string)
			content, _ := params.Arguments["content"].(string)
			err := s.client.AppendDocument(ctx, s.workspaceID, id, content)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully appended to document."

		case "documents_add_metadata":
			id, _ := params.Arguments["id"].(string)
			key, _ := params.Arguments["key"].(string)
			value, _ := params.Arguments["value"].(string)
			err := s.client.AddDocumentMetadata(ctx, s.workspaceID, id, map[string]any{key: value})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully added metadata to document."

		case "documents_rename":
			id, _ := params.Arguments["id"].(string)
			name, _ := params.Arguments["name"].(string)
			err := s.client.RenameDocument(ctx, s.workspaceID, id, name)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully renamed document."

		case "documents_archive":
			id, _ := params.Arguments["id"].(string)
			archived, _ := params.Arguments["archived"].(bool)
			err := s.client.ArchiveDocument(ctx, s.workspaceID, id, archived)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully updated document archive status."

		case "documents_clear":
			id, _ := params.Arguments["id"].(string)
			err := s.client.ClearDocument(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully cleared document."

		case "documents_get":
			id, _ := params.Arguments["id"].(string)
			doc, err := s.client.GetDocument(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(doc, "", "  ")
			resultText = string(b)

		case "documents_delete":
			id, _ := params.Arguments["id"].(string)
			err := s.client.DeleteDocument(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully deleted document."

		case "documents_list":
			docs, err := s.client.ListDocuments(ctx, s.workspaceID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(docs, "", "  ")
			resultText = string(b)

		case "tasks_create":
			title, _ := params.Arguments["title"].(string)
			body, _ := params.Arguments["body"].(string)
			task, err := s.client.CreateTask(ctx, contract.Task{
				WorkspaceID: s.workspaceID,
				Title:       title,
				Body:        body,
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = fmt.Sprintf("Successfully created task (ID: %s)", task.ID)

		case "tasks_add_metadata":
			id, _ := params.Arguments["id"].(string)
			key, _ := params.Arguments["key"].(string)
			value, _ := params.Arguments["value"].(string)
			err := s.client.AddTaskMetadata(ctx, s.workspaceID, id, map[string]any{key: value})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully added task metadata."

		case "tasks_add_tag":
			id, _ := params.Arguments["id"].(string)
			tag, _ := params.Arguments["tag"].(string)
			err := s.client.AddTaskTag(ctx, s.workspaceID, id, tag)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully added tag."

		case "tasks_remove_tag":
			id, _ := params.Arguments["id"].(string)
			tag, _ := params.Arguments["tag"].(string)
			err := s.client.RemoveTaskTag(ctx, s.workspaceID, id, tag)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully removed tag."

		case "tasks_set_blockers":
			id, _ := params.Arguments["id"].(string)
			blockerIDsRaw, _ := params.Arguments["blocker_ids"].([]any)
			var blockerIDs []string
			for _, b := range blockerIDsRaw {
				blockerIDs = append(blockerIDs, fmt.Sprint(b))
			}
			err := s.client.SetTaskBlockers(ctx, s.workspaceID, id, blockerIDs)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully set blockers."

		case "tasks_add_blocker":
			id, _ := params.Arguments["id"].(string)
			blockerID, _ := params.Arguments["blocker_id"].(string)
			err := s.client.AddTaskBlocker(ctx, s.workspaceID, id, blockerID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully added blocker."

		case "tasks_remove_blocker":
			id, _ := params.Arguments["id"].(string)
			blockerID, _ := params.Arguments["blocker_id"].(string)
			err := s.client.RemoveTaskBlocker(ctx, s.workspaceID, id, blockerID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully removed blocker."

		case "tasks_lock":
			id, _ := params.Arguments["id"].(string)
			actorID, _ := params.Arguments["actor_id"].(string)
			err := s.client.LockTask(ctx, s.workspaceID, id, actorID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully locked task."

		case "tasks_unlock":
			id, _ := params.Arguments["id"].(string)
			actorID, _ := params.Arguments["actor_id"].(string)
			err := s.client.UnlockTask(ctx, s.workspaceID, id, actorID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully unlocked task."

		case "tasks_get":
			id, _ := params.Arguments["id"].(string)
			task, err := s.client.GetTask(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(task, "", "  ")
			resultText = string(b)

		case "tasks_delete":
			id, _ := params.Arguments["id"].(string)
			err := s.client.DeleteTask(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully deleted task."

		case "tasks_list":
			tasks, err := s.client.ListTasks(ctx, s.workspaceID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(tasks, "", "  ")
			resultText = string(b)

		case "tasks_comments_create":
			taskID, _ := params.Arguments["task_id"].(string)
			author, _ := params.Arguments["author"].(string)
			body, _ := params.Arguments["body"].(string)
			comment, err := s.client.CreateTaskComment(ctx, contract.TaskComment{
				TaskID: taskID,
				Author: author,
				Body:   body,
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = fmt.Sprintf("Successfully created task comment (ID: %s)", comment.ID)

		case "tasks_comments_update":
			id, _ := params.Arguments["id"].(string)
			body, _ := params.Arguments["body"].(string)
			err := s.client.UpdateTaskComment(ctx, id, body)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully updated task comment."

		case "tasks_comments_delete":
			id, _ := params.Arguments["id"].(string)
			err := s.client.DeleteTaskComment(ctx, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully deleted task comment."

		case "tasks_comments_list":
			taskID, _ := params.Arguments["task_id"].(string)
			comments, err := s.client.ListTaskComments(ctx, taskID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(comments, "", "  ")
			resultText = string(b)

		case "wakeups_set":
			ownerID, _ := params.Arguments["owner_id"].(string)
			body, _ := params.Arguments["body"].(string)
			dueAtMS, _ := params.Arguments["due_at_ms"].(float64)
			err := s.client.SetWakeup(ctx, contract.Wakeup{
				WorkspaceID: s.workspaceID,
				OwnerID:     ownerID,
				DueAtMS:     int64(dueAtMS),
				Body:        body,
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully set wakeup."

		case "wakeups_get":
			id, _ := params.Arguments["id"].(string)
			w, err := s.client.GetWakeup(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(w, "", "  ")
			resultText = string(b)

		case "wakeups_cancel":
			id, _ := params.Arguments["id"].(string)
			err := s.client.CancelWakeup(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully cancelled wakeup."

		case "wakeups_pause":
			id, _ := params.Arguments["id"].(string)
			err := s.client.PauseWakeup(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully paused wakeup."

		case "wakeups_resume":
			id, _ := params.Arguments["id"].(string)
			err := s.client.ResumeWakeup(ctx, s.workspaceID, id)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully resumed wakeup."

		case "wakeups_reset":
			id, _ := params.Arguments["id"].(string)
			dueAtMS, _ := params.Arguments["due_at_ms"].(float64)
			err := s.client.ResetWakeup(ctx, s.workspaceID, id, int64(dueAtMS))
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully reset wakeup."

		case "wakeups_list_pending":
			wakeups, err := s.client.ListPendingWakeups(ctx, s.workspaceID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(wakeups, "", "  ")
			resultText = string(b)

		case "leases_acquire":
			lockKey, _ := params.Arguments["lock_key"].(string)
			ownerID, _ := params.Arguments["owner_id"].(string)
			expiresAtMS, _ := params.Arguments["expires_at_ms"].(float64)
			acquired, err := s.client.AcquireLease(ctx, contract.Lease{
				WorkspaceID: s.workspaceID,
				LockKey:     lockKey,
				OwnerID:     ownerID,
				ExpiresAtMS: int64(expiresAtMS),
			})
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = fmt.Sprintf("Acquired: %v", acquired)

		case "leases_release":
			lockKey, _ := params.Arguments["lock_key"].(string)
			ownerID, _ := params.Arguments["owner_id"].(string)
			err := s.client.ReleaseLease(ctx, s.workspaceID, lockKey, ownerID)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully released lease."

		case "leases_get":
			lockKey, _ := params.Arguments["lock_key"].(string)
			lease, err := s.client.GetLease(ctx, s.workspaceID, lockKey)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			b, _ := json.MarshalIndent(lease, "", "  ")
			resultText = string(b)

		case "leases_reset":
			lockKey, _ := params.Arguments["lock_key"].(string)
			err := s.client.ResetLease(ctx, s.workspaceID, lockKey)
			if err != nil {
				return s.toolError(req.ID, err.Error())
			}
			resultText = "Successfully reset lease."

		default:
			res.Error = &mcpError{Code: -32601, Message: fmt.Sprintf("unknown tool: %s", params.Name)}
			return res
		}

		res.Result = map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": resultText,
				},
			},
			"isError": false,
		}

	default:
		res.Error = &mcpError{Code: -32601, Message: fmt.Sprintf("unknown method: %s", req.Method)}
	}

	return res
}

func (s *mcpServer) toolError(id json.RawMessage, message string) *mcpResponse {
	return &mcpResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: map[string]any{
			"content": []map[string]any{
				{
					"type": "text",
					"text": message,
				},
			},
			"isError": true,
		},
	}
}
