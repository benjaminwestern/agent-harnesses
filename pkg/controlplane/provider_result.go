package controlplane

import (
	"errors"
	"fmt"
	"strings"
)

type ProviderUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	VectorCount      int `json:"vector_count,omitempty"`
}

type ProviderResultMetadata struct {
	Provider      string         `json:"provider,omitempty"`
	Model         string         `json:"model,omitempty"`
	BaseURL       string         `json:"base_url,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	RequestCount  int            `json:"request_count,omitempty"`
	StatusCode    int            `json:"status_code,omitempty"`
	LatencyMillis int64          `json:"latency_millis,omitempty"`
	LatencyNanos  int64          `json:"latency_nanos,omitempty"`
	FinishReason  string         `json:"finish_reason,omitempty"`
	OutputKind    string         `json:"output_kind,omitempty"`
	Usage         ProviderUsage  `json:"usage,omitempty"`
	Error         *ProviderError `json:"error,omitempty"`
}

type ProviderError struct {
	Kind       string `json:"kind,omitempty"`
	Message    string `json:"message,omitempty"`
	Type       string `json:"type,omitempty"`
	Param      string `json:"param,omitempty"`
	Code       any    `json:"code,omitempty"`
	StatusCode int    `json:"status_code,omitempty"`
	Body       string `json:"body,omitempty"`
	Retryable  bool   `json:"retryable,omitempty"`
}

type ProviderResultError struct {
	Message string                 `json:"message"`
	Result  ProviderResultMetadata `json:"provider_result"`
	Cause   error                  `json:"-"`
}

// ProviderResultCarrier is implemented by errors that preserve structured
// provider metadata.
type ProviderResultCarrier interface {
	ProviderResult() (ProviderResultMetadata, bool)
}

type ProviderResultSummary struct {
	Count         int                      `json:"count"`
	StatusCodes   map[string]int           `json:"status_codes,omitempty"`
	OutputKinds   map[string]int           `json:"output_kinds,omitempty"`
	FinishReasons map[string]int           `json:"finish_reasons,omitempty"`
	ErrorKinds    map[string]int           `json:"error_kinds,omitempty"`
	RequestIDs    []string                 `json:"request_ids,omitempty"`
	Samples       []ProviderResultMetadata `json:"samples,omitempty"`
}

type ResultAggregator struct {
	Count         int
	StatusCodes   map[string]int
	OutputKinds   map[string]int
	FinishReasons map[string]int
	ErrorKinds    map[string]int
	RequestIDs    []string
	Samples       []ProviderResultMetadata
}

func NewProviderResultError(message string, result ProviderResultMetadata, cause error) *ProviderResultError {
	return &ProviderResultError{
		Message: strings.TrimSpace(message),
		Result:  result,
		Cause:   cause,
	}
}

func (e *ProviderResultError) Error() string {
	if e == nil {
		return ""
	}
	parts := []string{firstNonEmptyString(e.Message, "provider result error")}
	if e.Result.Provider != "" {
		parts = append(parts, fmt.Sprintf("provider=%s", e.Result.Provider))
	}
	if e.Result.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", e.Result.Model))
	}
	if e.Result.BaseURL != "" {
		parts = append(parts, fmt.Sprintf("base_url=%s", e.Result.BaseURL))
	}
	if e.Result.FinishReason != "" {
		parts = append(parts, fmt.Sprintf("finish_reason=%s", e.Result.FinishReason))
	}
	if e.Result.OutputKind != "" {
		parts = append(parts, fmt.Sprintf("output_kind=%s", e.Result.OutputKind))
	}
	if e.Result.Error != nil && e.Result.Error.Message != "" {
		parts = append(parts, fmt.Sprintf("provider_error=%s", e.Result.Error.Message))
	}
	return strings.Join(parts, "; ")
}

func (e *ProviderResultError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func (e *ProviderResultError) ProviderResult() (ProviderResultMetadata, bool) {
	if e == nil {
		return ProviderResultMetadata{}, false
	}
	return e.Result, true
}

func (a *ResultAggregator) Add(result ProviderResultMetadata) {
	if a == nil {
		return
	}
	a.Count++
	if result.StatusCode != 0 {
		incrementStringCount(&a.StatusCodes, fmt.Sprintf("%d", result.StatusCode))
	}
	if result.OutputKind != "" {
		incrementStringCount(&a.OutputKinds, result.OutputKind)
	}
	if result.FinishReason != "" {
		incrementStringCount(&a.FinishReasons, result.FinishReason)
	}
	if result.Error != nil && result.Error.Kind != "" {
		incrementStringCount(&a.ErrorKinds, result.Error.Kind)
	}
	if result.RequestID != "" {
		a.RequestIDs = appendUniqueString(a.RequestIDs, result.RequestID)
	}
	a.Samples = append(a.Samples, result)
}

func (a *ResultAggregator) AddError(err error) bool {
	if a == nil || err == nil {
		return false
	}
	var carrier ProviderResultCarrier
	if !errors.As(err, &carrier) {
		return false
	}
	result, ok := carrier.ProviderResult()
	if !ok {
		return false
	}
	a.Add(result)
	return true
}

func (a *ResultAggregator) Merge(other ResultAggregator) {
	if a == nil {
		return
	}
	a.Count += other.Count
	mergeStringCounts(&a.StatusCodes, other.StatusCodes)
	mergeStringCounts(&a.OutputKinds, other.OutputKinds)
	mergeStringCounts(&a.FinishReasons, other.FinishReasons)
	mergeStringCounts(&a.ErrorKinds, other.ErrorKinds)
	for _, requestID := range other.RequestIDs {
		a.RequestIDs = appendUniqueString(a.RequestIDs, requestID)
	}
	a.Samples = append(a.Samples, other.Samples...)
}

func (a *ResultAggregator) Summary() ProviderResultSummary {
	if a == nil {
		return ProviderResultSummary{}
	}
	return ProviderResultSummary{
		Count:         a.Count,
		StatusCodes:   copyStringCounts(a.StatusCodes),
		OutputKinds:   copyStringCounts(a.OutputKinds),
		FinishReasons: copyStringCounts(a.FinishReasons),
		ErrorKinds:    copyStringCounts(a.ErrorKinds),
		RequestIDs:    append([]string(nil), a.RequestIDs...),
		Samples:       append([]ProviderResultMetadata(nil), a.Samples...),
	}
}

func mergeProviderMetadata(metadata map[string]any, result ProviderResultMetadata) map[string]any {
	if metadata == nil {
		metadata = map[string]any{}
	}
	if result.Provider != "" {
		metadata["provider"] = result.Provider
	}
	if result.Model != "" {
		metadata["model"] = result.Model
	}
	if result.BaseURL != "" {
		metadata["base_url"] = result.BaseURL
	}
	if result.RequestID != "" {
		metadata["request_id"] = result.RequestID
	}
	if result.RequestCount != 0 {
		metadata["request_count"] = result.RequestCount
	}
	if result.StatusCode != 0 {
		metadata["status_code"] = result.StatusCode
	}
	if result.LatencyMillis != 0 {
		metadata["latency_millis"] = result.LatencyMillis
	}
	if result.LatencyNanos != 0 {
		metadata["latency_nanos"] = result.LatencyNanos
	}
	if result.FinishReason != "" {
		metadata["finish_reason"] = result.FinishReason
	}
	if result.OutputKind != "" {
		metadata["output_kind"] = result.OutputKind
	}
	if result.Error != nil {
		metadata["error"] = result.Error
	}
	metadata["usage"] = result.Usage
	metadata["provider_result"] = result
	return metadata
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func incrementStringCount(target *map[string]int, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	if *target == nil {
		*target = map[string]int{}
	}
	(*target)[key]++
}

func mergeStringCounts(target *map[string]int, source map[string]int) {
	for key, count := range source {
		if count == 0 {
			continue
		}
		if *target == nil {
			*target = map[string]int{}
		}
		(*target)[key] += count
	}
}

func copyStringCounts(source map[string]int) map[string]int {
	if source == nil {
		return nil
	}
	out := make(map[string]int, len(source))
	for key, count := range source {
		out[key] = count
	}
	return out
}

func appendUniqueString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
