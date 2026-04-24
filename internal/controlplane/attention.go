package controlplane

import (
	"slices"
	"sync"
	"time"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type AttentionQueue struct {
	mu    sync.RWMutex
	items map[string]contract.AttentionItem
}

type AttentionListFilter struct {
	Status    contract.AttentionStatus
	Action    contract.AttentionAction
	SessionID string
	Limit     int
}

type AttentionUpdate struct {
	Status   contract.AttentionStatus
	Metadata map[string]any
	Result   map[string]any
	Error    string
}

func NewAttentionQueue() *AttentionQueue {
	return &AttentionQueue{items: make(map[string]contract.AttentionItem)}
}

func (q *AttentionQueue) Enqueue(item contract.AttentionItem) contract.AttentionItem {
	now := time.Now().UnixMilli()
	if item.ID == "" {
		item.ID = newIdentifier("attention")
	}
	if item.SchemaVersion == "" {
		item.SchemaVersion = contract.ControlPlaneSchemaVersion
	}
	if item.Status == "" {
		item.Status = contract.AttentionStatusQueued
	}
	if item.CreatedAtMS == 0 {
		item.CreatedAtMS = now
	}
	item.UpdatedAtMS = now

	q.mu.Lock()
	q.items[item.ID] = item
	q.mu.Unlock()
	return item
}

func (q *AttentionQueue) Get(id string) (contract.AttentionItem, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	item, ok := q.items[id]
	return item, ok
}

func (q *AttentionQueue) List(filter AttentionListFilter) []contract.AttentionItem {
	q.mu.RLock()
	defer q.mu.RUnlock()

	items := make([]contract.AttentionItem, 0, len(q.items))
	for _, item := range q.items {
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.Action != "" && item.Action != filter.Action {
			continue
		}
		if filter.SessionID != "" && item.SessionID != filter.SessionID {
			continue
		}
		items = append(items, item)
	}
	slices.SortFunc(items, func(left, right contract.AttentionItem) int {
		switch {
		case left.Priority > right.Priority:
			return -1
		case left.Priority < right.Priority:
			return 1
		case left.UpdatedAtMS > right.UpdatedAtMS:
			return -1
		case left.UpdatedAtMS < right.UpdatedAtMS:
			return 1
		case left.ID < right.ID:
			return -1
		case left.ID > right.ID:
			return 1
		default:
			return 0
		}
	})
	if filter.Limit > 0 && filter.Limit < len(items) {
		return slices.Clone(items[:filter.Limit])
	}
	return items
}

func (q *AttentionQueue) Update(id string, update AttentionUpdate) (contract.AttentionItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	item, ok := q.items[id]
	if !ok {
		return contract.AttentionItem{}, false
	}
	if update.Status != "" {
		item.Status = update.Status
	}
	if update.Metadata != nil {
		item.Metadata = update.Metadata
	}
	if update.Result != nil {
		item.Result = update.Result
	}
	if update.Error != "" {
		item.Error = update.Error
	}
	item.UpdatedAtMS = time.Now().UnixMilli()
	q.items[id] = item
	return item, true
}
