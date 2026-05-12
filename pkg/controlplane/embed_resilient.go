package controlplane

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
)

type ResilientEmbedConfig struct {
	MaxConcurrency int
	EnableCache    bool
	MaxRetries     int
	SplitOversized bool
	Progress       func(ResilientEmbedProgress) error
}

type ResilientEmbedProgress struct {
	Phase          string
	BatchIndex     int
	BatchCount     int
	ItemIndex      int
	ItemCount      int
	TokenCount     int
	SplitPart      int
	SplitTotal     int
	ProviderResult *ProviderResultMetadata
}

type ResilientEmbedResult struct {
	Vectors        [][]float64
	Reused         []bool
	ProviderResult ProviderResultSummary
}

type resilientEmbedJob struct {
	text    string
	indexes []int
}

type resilientEmbedder struct {
	ctx      context.Context
	provider EmbeddingProvider
	config   ResilientEmbedConfig
	input    EmbeddingInput
	cache    map[string][]float64
	sem      chan struct{}

	mu         sync.Mutex
	aggregator ResultAggregator
}

func ResilientEmbedBatch(ctx context.Context, provider EmbeddingProvider, config ResilientEmbedConfig, input EmbeddingInput) (*ResilientEmbedResult, error) {
	if provider == nil {
		return nil, fmt.Errorf("embedding provider is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	embedder := &resilientEmbedder{
		ctx:      ctx,
		provider: provider,
		config:   config,
		input:    input,
	}
	if config.EnableCache {
		embedder.cache = map[string][]float64{}
	}
	if config.MaxConcurrency > 0 {
		embedder.sem = make(chan struct{}, config.MaxConcurrency)
	}

	jobs, reused := embedder.jobs(input.Texts)
	vectors := make([][]float64, len(input.Texts))
	jobVectors, err := embedder.embedJobs(jobs, 0)
	if err != nil {
		return nil, err
	}
	for i, job := range jobs {
		for _, index := range job.indexes {
			vectors[index] = append([]float64(nil), jobVectors[i]...)
		}
	}
	return &ResilientEmbedResult{
		Vectors:        vectors,
		Reused:         reused,
		ProviderResult: embedder.summary(),
	}, nil
}

func (e *resilientEmbedder) jobs(texts []string) ([]resilientEmbedJob, []bool) {
	reused := make([]bool, len(texts))
	if !e.config.EnableCache {
		jobs := make([]resilientEmbedJob, 0, len(texts))
		for index, text := range texts {
			jobs = append(jobs, resilientEmbedJob{text: text, indexes: []int{index}})
		}
		return jobs, reused
	}
	byKey := map[string]int{}
	jobs := make([]resilientEmbedJob, 0, len(texts))
	for index, text := range texts {
		key := e.cacheKey(text)
		if existing, ok := byKey[key]; ok {
			jobs[existing].indexes = append(jobs[existing].indexes, index)
			reused[index] = true
			_ = e.progress(ResilientEmbedProgress{
				Phase:     "cache_hit",
				ItemIndex: index,
				ItemCount: len(texts),
			})
			continue
		}
		byKey[key] = len(jobs)
		jobs = append(jobs, resilientEmbedJob{text: text, indexes: []int{index}})
	}
	return jobs, reused
}

func (e *resilientEmbedder) embedJobs(jobs []resilientEmbedJob, attempt int) ([][]float64, error) {
	if len(jobs) == 0 {
		return nil, nil
	}
	if e.config.EnableCache {
		if cached, ok := e.cachedVectors(jobs); ok {
			return cached, nil
		}
	}

	out, err := e.callProvider(jobs)
	if err == nil {
		if len(out.Vectors) != len(jobs) {
			return nil, fmt.Errorf("embedding provider returned %d vectors for %d texts", len(out.Vectors), len(jobs))
		}
		e.addResult(out.ProviderResult)
		if e.config.EnableCache {
			e.storeCache(jobs, out.Vectors)
		}
		return cloneVectors(out.Vectors), nil
	}
	e.addError(err)
	if !IsRetryableProviderError(err) || attempt >= e.maxRetries() {
		return nil, err
	}
	if len(jobs) > 1 {
		mid := len(jobs) / 2
		return e.embedSplitBatches(jobs[:mid], jobs[mid:], attempt+1)
	}
	if !e.config.SplitOversized {
		return nil, err
	}
	return e.embedOversizedSingleton(jobs[0], attempt+1)
}

func (e *resilientEmbedder) callProvider(jobs []resilientEmbedJob) (*EmbeddingOutput, error) {
	if e.sem != nil {
		select {
		case e.sem <- struct{}{}:
			defer func() { <-e.sem }()
		case <-e.ctx.Done():
			return nil, e.ctx.Err()
		}
	}
	if err := e.progress(ResilientEmbedProgress{
		Phase:      "batch_start",
		BatchCount: 1,
		ItemCount:  len(jobs),
		TokenCount: tokenCountForJobs(jobs),
	}); err != nil {
		return nil, err
	}
	input := e.input
	input.Texts = make([]string, 0, len(jobs))
	for _, job := range jobs {
		input.Texts = append(input.Texts, job.text)
	}
	out, err := e.provider.GenerateEmbeddings(e.ctx, input)
	if err != nil {
		result, ok := ProviderResultFromError(err)
		progress := ResilientEmbedProgress{Phase: "batch_error", ItemCount: len(jobs), TokenCount: tokenCountForJobs(jobs)}
		if ok {
			progress.ProviderResult = &result
		}
		if progressErr := e.progress(progress); progressErr != nil {
			return nil, progressErr
		}
		return nil, err
	}
	if out == nil {
		return nil, fmt.Errorf("embedding provider returned nil output")
	}
	result := out.ProviderResult
	if err := e.progress(ResilientEmbedProgress{
		Phase:          "batch_success",
		ItemCount:      len(jobs),
		TokenCount:     tokenCountForJobs(jobs),
		ProviderResult: &result,
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *resilientEmbedder) embedSplitBatches(left []resilientEmbedJob, right []resilientEmbedJob, attempt int) ([][]float64, error) {
	if e.sem == nil {
		leftVectors, err := e.embedJobs(left, attempt)
		if err != nil {
			return nil, err
		}
		rightVectors, err := e.embedJobs(right, attempt)
		if err != nil {
			return nil, err
		}
		return append(leftVectors, rightVectors...), nil
	}

	var leftVectors, rightVectors [][]float64
	var leftErr, rightErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		leftVectors, leftErr = e.embedJobs(left, attempt)
	}()
	go func() {
		defer wg.Done()
		rightVectors, rightErr = e.embedJobs(right, attempt)
	}()
	wg.Wait()
	if leftErr != nil {
		return nil, leftErr
	}
	if rightErr != nil {
		return nil, rightErr
	}
	return append(leftVectors, rightVectors...), nil
}

func (e *resilientEmbedder) embedOversizedSingleton(job resilientEmbedJob, attempt int) ([][]float64, error) {
	parts := splitProviderRetryText(job.text)
	if len(parts) < 2 {
		return nil, fmt.Errorf("embedding text cannot be split further")
	}
	partJobs := make([]resilientEmbedJob, 0, len(parts))
	for i, part := range parts {
		if err := e.progress(ResilientEmbedProgress{
			Phase:      "split",
			ItemIndex:  job.indexes[0],
			SplitPart:  i + 1,
			SplitTotal: len(parts),
			TokenCount: tokenCount(part),
		}); err != nil {
			return nil, err
		}
		partJobs = append(partJobs, resilientEmbedJob{text: part, indexes: job.indexes})
	}
	partVectors, err := e.embedJobs(partJobs, attempt)
	if err != nil {
		return nil, err
	}
	weights := make([]int, 0, len(parts))
	for _, part := range parts {
		weights = append(weights, len([]rune(part)))
	}
	return [][]float64{weightedAverageVector(partVectors, weights)}, nil
}

func (e *resilientEmbedder) cachedVectors(jobs []resilientEmbedJob) ([][]float64, bool) {
	vectors := make([][]float64, 0, len(jobs))
	for _, job := range jobs {
		vector, ok := e.cache[e.cacheKey(job.text)]
		if !ok {
			return nil, false
		}
		vectors = append(vectors, append([]float64(nil), vector...))
	}
	return vectors, true
}

func (e *resilientEmbedder) storeCache(jobs []resilientEmbedJob, vectors [][]float64) {
	for i, job := range jobs {
		e.cache[e.cacheKey(job.text)] = append([]float64(nil), vectors[i]...)
	}
}

func (e *resilientEmbedder) cacheKey(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%s\x00%s\x00%x", e.input.ModelSelection.Provider, e.input.ModelSelection.Model, sum)
}

func (e *resilientEmbedder) maxRetries() int {
	if e.config.MaxRetries <= 0 {
		return 0
	}
	return e.config.MaxRetries
}

func (e *resilientEmbedder) progress(progress ResilientEmbedProgress) error {
	if e.config.Progress == nil {
		return nil
	}
	return e.config.Progress(progress)
}

func (e *resilientEmbedder) addResult(result ProviderResultMetadata) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.aggregator.Add(result)
}

func (e *resilientEmbedder) addError(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.aggregator.AddError(err)
}

func (e *resilientEmbedder) summary() ProviderResultSummary {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.aggregator.Summary()
}

func IsRetryableProviderError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	result, ok := ProviderResultFromError(err)
	if !ok {
		return false
	}
	if result.Error != nil && result.Error.Retryable {
		return true
	}
	switch result.StatusCode {
	case 408, 409, 425, 429, 502, 503, 504:
		return true
	case 400, 401, 403, 404:
		return false
	default:
		return result.StatusCode >= 500
	}
}

func ProviderResultFromError(err error) (ProviderResultMetadata, bool) {
	var carrier ProviderResultCarrier
	if errors.As(err, &carrier) {
		return carrier.ProviderResult()
	}
	return ProviderResultMetadata{}, false
}

func tokenCountForJobs(jobs []resilientEmbedJob) int {
	total := 0
	for _, job := range jobs {
		total += tokenCount(job.text)
	}
	return total
}

func tokenCount(text string) int {
	return len(strings.Fields(text))
}

func splitProviderRetryText(text string) []string {
	runes := []rune(text)
	if len(runes) < 2 {
		return nil
	}
	mid := len(runes) / 2
	return []string{string(runes[:mid]), string(runes[mid:])}
}

func weightedAverageVector(vectors [][]float64, weights []int) []float64 {
	if len(vectors) == 0 {
		return nil
	}
	out := make([]float64, len(vectors[0]))
	totalWeight := 0
	for i, vector := range vectors {
		weight := 1
		if i < len(weights) && weights[i] > 0 {
			weight = weights[i]
		}
		totalWeight += weight
		for j := range out {
			if j < len(vector) {
				out[j] += vector[j] * float64(weight)
			}
		}
	}
	if totalWeight == 0 {
		return out
	}
	for i := range out {
		out[i] /= float64(totalWeight)
	}
	return out
}

func cloneVectors(vectors [][]float64) [][]float64 {
	out := make([][]float64, len(vectors))
	for i, vector := range vectors {
		out[i] = append([]float64(nil), vector...)
	}
	return out
}
