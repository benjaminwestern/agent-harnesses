# Feature Flags

Feature flags allow enabling or disabling functionality without changing code. They can be defined in `config.toml` or overridden via environment variables.

## Available Flags

| Flag | Description | Config Key | Environment Variable |
|------|-------------|------------|----------------------|
| `EnableVerboseLogging` | Enables detailed debug information in logs and events. | `verbose_logging` | `AC_FEATURE_VERBOSE_LOGGING` |
| `EnableParallelExecution` | Allows the engine to spawn multiple workers in parallel where supported. | `parallel_execution` | `AC_FEATURE_PARALLEL_EXECUTION` |
| `UseExperimentalProvider` | Enables the use of the new experimental LLM provider backend. | `experimental_provider` | `AC_FEATURE_EXPERIMENTAL_PROVIDER` |

## Usage

### Config File (`config.toml`)

```toml
[features]
verbose_logging = true
parallel_execution = false
experimental_provider = "off"
```

### Environment Variables

Environment variables take precedence over the configuration file.

```bash
export AC_FEATURE_VERBOSE_LOGGING=true
agent_control court run ...
```

### In Code

Access feature flags through the `Engine` or the `Config` struct.

```go
if engine.Features().EnableVerboseLogging {
    // perform verbose logging
}
```
