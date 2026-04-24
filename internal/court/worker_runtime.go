// Package court provides Court runtime functionality.
package court

import "context"

func (e *Engine) runBackendWorker(ctx context.Context, run Run, worker Worker) (string, string, RuntimeIdentity, error) {
	if err := e.validateBackend(worker.Backend); err != nil {
		return "", "", RuntimeIdentity{}, err
	}
	return e.runAgenticControlWorker(ctx, run, worker)
}
