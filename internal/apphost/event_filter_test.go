package apphost

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

func TestRecentEventsFilteredBySessionAfterAndLimit(t *testing.T) {
	core := &Core{
		events: []ObservedEvent{
			testObservedEvent(1, "session-a"),
			testObservedEvent(2, "session-a"),
			testObservedEvent(3, "session-b"),
			testObservedEvent(4, "session-a"),
			testObservedEvent(5, "session-a"),
		},
	}

	got := core.RecentEventsFiltered(EventFilter{
		SessionID: " session-a ",
		After:     1,
		Limit:     2,
	})

	if sequences := eventSequences(got); !reflect.DeepEqual(sequences, []int64{4, 5}) {
		t.Fatalf("sequences = %#v, want %#v", sequences, []int64{4, 5})
	}
}

func TestHTTPRecentEventsEndpointUsesQueryFilters(t *testing.T) {
	core := &Core{
		events: []ObservedEvent{
			testObservedEvent(1, "session-a"),
			testObservedEvent(2, "session-a"),
			testObservedEvent(3, "session-b"),
			testObservedEvent(4, "session-a"),
			testObservedEvent(5, "session-a"),
		},
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/events/recent?session_id=session-a&after=1&limit=2", nil)
	NewHTTPServer(core).APIHandler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var events []ObservedEvent
	if err := json.NewDecoder(recorder.Body).Decode(&events); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if sequences := eventSequences(events); !reflect.DeepEqual(sequences, []int64{4, 5}) {
		t.Fatalf("sequences = %#v, want %#v", sequences, []int64{4, 5})
	}
}

func TestEventFilterFromRequestParsesQuery(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/events/recent?session_id=%20session-a%20&after=42&limit=7", nil)

	got := eventFilterFromRequest(request, 100)

	want := EventFilter{SessionID: "session-a", After: 42, Limit: 7}
	if got != want {
		t.Fatalf("filter = %#v, want %#v", got, want)
	}
}

func TestEventFilterFromRequestUsesFallbacks(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/events/recent?after=bogus&limit=bogus", nil)

	got := eventFilterFromRequest(request, 100)

	want := EventFilter{After: 0, Limit: 100}
	if got != want {
		t.Fatalf("filter = %#v, want %#v", got, want)
	}
}

func TestEventMatchesFilterUsesSessionAndAfter(t *testing.T) {
	filter := EventFilter{SessionID: "session-a", After: 10}

	tests := []struct {
		name  string
		event ObservedEvent
		want  bool
	}{
		{name: "skips sequence at after", event: testObservedEvent(10, "session-a"), want: false},
		{name: "skips session mismatch", event: testObservedEvent(11, "session-b"), want: false},
		{name: "matches later session event", event: testObservedEvent(11, "session-a"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eventMatchesFilter(tt.event, filter); got != tt.want {
				t.Fatalf("eventMatchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func testObservedEvent(sequence int64, sessionID string) ObservedEvent {
	return ObservedEvent{
		Sequence: sequence,
		Event: contract.RuntimeEvent{
			SessionID: sessionID,
		},
	}
}

func eventSequences(events []ObservedEvent) []int64 {
	sequences := make([]int64, 0, len(events))
	for _, event := range events {
		sequences = append(sequences, event.Sequence)
	}
	return sequences
}
