# Synthetic Data Generation

Agentic Control provides a native pipeline to generate schema-enforced synthetic datasets for fine-tuning or evaluation baselining.

## Dynamic Schema Generation

If you need a strict JSON schema but don't want to write it manually, you can ask the AI to act as a data architect and generate a Draft 7 JSON Schema from a plain English description.

```bash
agent_control dataset generate-schema "A collection of 3-star restaurant reviews" > schema.json
```
*Note: The engine automatically instructs the AI to wrap the root properties inside an array to ensure compatibility with batch generation.*

## Zero-Shot / Few-Shot Generation

Once you have a schema, you can forcefully generate synthetic rows that strictly adhere to it. The engine will dynamically inject the schema into the provider request (`response_format: { type: "json_schema" }`) and trigger automatic repair loops if the model hallucinates invalid structures.

```bash
agent_control dataset synth "Generate restaurant reviews set in Tokyo." \
  --schema ./schema.json \
  --count 5 \
  --format ndjson > synthetic_dataset.ndjson
```

### NDJSON Output
By default, the CLI formats generated arrays into **Newline Delimited JSON (NDJSON)**. This is the industry standard format required for manual human review pipelines and downstream ingestion into databases or fine-tuning APIs.

## Few-Shot Formatting

Under the hood, if you pass existing approved `DatasetItemRecords` to the orchestration engine, `GenerateSyntheticData` will format up to 5 of them into a strict few-shot context block (Input / Output mapping) before sending the request to the fanout controller, allowing you to seamlessly tell the model to "make more like this".

## MCP Integration

The synthetic generation pipeline is available to upstream orchestrators via MCP:

- **MCP Tool**: `generate_synthetic_data`
  - Accepts `prompt`, `schema` (the JSON schema object), `count`, and `target` model.
  - Returns the generated items as a stringified JSON array.