# Evaluation Engine

Agentic Control provides a native, headless evaluation engine to validate model outputs against ground truth datasets using an LLM-as-a-judge.

## Evaluation Modes

The engine supports two primary evaluation modes, exposed via the `--mode` flag:

1.  **Strict JSON Evaluation (`evaluate`)**: The default mode. The judge model is forced to return a strict JSON schema: `{"score": <float>, "rationale": "<string>", "passed": <bool>}`. If the model fails to adhere to the schema, the engine automatically catches the `jsonschema` validation error and initiates a repair loop to fix the output.
2.  **Logprob Weighting (G-Eval) (`g_eval`)**: An advanced, highly-calibrated mode. Instead of parsing JSON, the engine asks the model to output a single integer (1 to 5). It then extracts the logarithmic probabilities (`logprobs`) of those tokens from the provider API, applies a mathematical exponential weighting (`math.Exp()`), and calculates a continuous float score (e.g., `4.23`). This bypasses text extraction vulnerabilities; if the provider does not return usable score-token logprobs, the reduction fails with an explicit G-Eval error instead of parsing the text.

## Running Batch Evaluations

You can run evaluations offline against a JSON file containing an array of dataset items.

### Dataset Item Format
```json
[
  {
    "input_payload": "Translate 'hello' to French.",
    "target_output": "Bonjour"
  }
]
```

### CLI Usage
```bash
agent_control dataset eval run \
  --items ./dataset.json \
  --prompt "rubric-accuracy" \
  --target-model "openaicompatible=gpt-4o-mini" \
  --judge-model "openaicompatible=gpt-4o" \
  --mode "g_eval"
```
The orchestrator will:
1. Pass the `input_payload` to the `target-model`.
2. Pass the resulting output and the `target_output` (Ground Truth) to the `judge-model` using the specified prompt rubric.
3. Output the results as an NDJSON array containing the score, rationale, latency, and cost.

## MCP & JSON-RPC

The evaluation engine is fully exposed over MCP and JSON-RPC.

- **MCP Tool**: `run_batch_evaluation`
  - Accepts `prompt`, `items` (array of input/truth pairs), `target` model, `judge` model, and `mode`.
- **JSON-RPC**: `session/new` with `response_schema` natively handles strict extraction. For full evaluations, downstream clients can utilize the orchestration JSON-RPC endpoints.
