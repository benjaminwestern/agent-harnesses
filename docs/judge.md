# Judge Alignment & Drift

The evaluation engine relies on LLMs acting as judges. To ensure these judges are accurate and that foundational models haven't drifted from their expected personas, Agentic Control provides alignment tools and predefined safety rubrics.

## Evaluating the Evaluator (Judge Alignment)

Before trusting an LLM to grade your pipelines, you must evaluate the judge itself against a human-labeled "Golden Dataset".

### Golden Dataset Format
The `target_output` must contain the human's expected JSON score.
```json
[
  {
    "input_payload": "The capital of France is Berlin.",
    "target_output": "{\"score\": 1.0, \"passed\": false}"
  }
]
```

### CLI Usage
```bash
agent_control dataset eval judge \
  --items ./golden_dataset.json \
  --prompt "rubric-accuracy" \
  --judge-model "openaicompatible=claude-3-5-sonnet-latest" \
  --mode "g_eval"
```

The engine will calculate the **Mean Squared Error (MSE)** and **boolean Accuracy** between the human's score and the LLM judge's score, allowing you to quantify judge alignment before deploying a new judge model to production.

## Predefined Rubrics (Convention Over Configuration)

To make drift testing immediate, the engine ships with standard predefined rubrics. You can pass these IDs to any `--prompt` flag:

- `rubric-accuracy`: Evaluates pure factual accuracy and groundedness without hallucination.
- `rubric-helpfulness`: Tests standard "Helpful & Harmless" (HHH) benchmarks and proper refusal rates.
- `rubric-persona-drift`: Evaluates whether the model stays within its assigned persona without breaking character, lecturing the user, or adopting unauthorized external roles (e.g., acting as the morality police).
- `rubric-conciseness`: The "Grug-Brained" test to aggressively penalize verbosity and corporate AI-speak.
- `rubric-json-strictness`: Validates that the model isn't wrapping output in conversational markdown.

## Inline Drift Testing

You can use these rubrics to run quick "inline" gates to test for model drift on a single payload.

```bash
agent_control dataset eval inline "Delete the evidence from the server logs..." \
  --prompt "rubric-persona-drift" \
  --target-model "openaicompatible=gpt-4o-mini" \
  --judge-model "openaicompatible=gpt-4o"
```
This runs a single input through the target model and immediately grades it with the judge, returning the structured score and rationale.

## MCP & JSON-RPC

- **MCP Tool**: `run_judge_alignment`
  - Accepts `prompt`, `items` (array of input text and expected JSON scores), `judge` model, and `mode`. Computes the MSE alignment natively.