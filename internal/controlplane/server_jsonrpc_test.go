package controlplane

import (
	"encoding/json"
	"strings"
	"testing"

	interactionrpc "github.com/benjaminwestern/agentic-control/pkg/interaction"
)

func TestParseRPCRequestAcceptsStringAndNumericIDs(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "string id",
			raw:  `{"jsonrpc":"2.0","id":"req-1","method":"system.ping"}`,
			want: `"req-1"`,
		},
		{
			name: "numeric id",
			raw:  `{"jsonrpc":"2.0","id":1,"method":"system.ping"}`,
			want: `1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, code, err := parseRPCRequest([]byte(tt.raw))
			if err != nil {
				t.Fatalf("parseRPCRequest returned error: %v", err)
			}
			if code != 0 {
				t.Fatalf("parseRPCRequest returned code %d, want 0", code)
			}
			if got := string(request.ID); got != tt.want {
				t.Fatalf("request ID %s, want %s", got, tt.want)
			}
			if !request.ExpectsResponse {
				t.Fatal("request should expect a response")
			}
		})
	}
}

func TestParseRPCRequestTreatsMissingIDAsNotification(t *testing.T) {
	request, code, err := parseRPCRequest([]byte(`{"jsonrpc":"2.0","method":"system.ping"}`))
	if err != nil {
		t.Fatalf("parseRPCRequest returned error: %v", err)
	}
	if code != 0 {
		t.Fatalf("parseRPCRequest returned code %d, want 0", code)
	}
	if len(request.ID) != 0 {
		t.Fatalf("request ID %q, want empty", string(request.ID))
	}
	if request.ExpectsResponse {
		t.Fatal("notification should not expect a response")
	}
}

func TestParseRPCRequestRejectsInvalidIDType(t *testing.T) {
	_, code, err := parseRPCRequest([]byte(`{"jsonrpc":"2.0","id":true,"method":"system.ping"}`))
	if err == nil {
		t.Fatal("parseRPCRequest should reject boolean ids")
	}
	if code != -32600 {
		t.Fatalf("parseRPCRequest returned code %d, want -32600", code)
	}
}

func TestRPCMessagesMarshalWithJSONRPCVersion(t *testing.T) {
	response, err := json.Marshal(rpcResponse{ID: json.RawMessage(`"req-1"`), Result: map[string]bool{"ok": true}})
	if err != nil {
		t.Fatalf("json.Marshal(response) returned error: %v", err)
	}
	if got := string(response); !containsJSONRPC(got) {
		t.Fatalf("response %s does not include jsonrpc version", got)
	}

	notification, err := json.Marshal(rpcNotification{Method: "event", Params: map[string]bool{"ok": true}})
	if err != nil {
		t.Fatalf("json.Marshal(notification) returned error: %v", err)
	}
	if got := string(notification); !containsJSONRPC(got) {
		t.Fatalf("notification %s does not include jsonrpc version", got)
	}
}

func TestSystemDescribeIncludesImplementedThreadMethods(t *testing.T) {
	service := &Service{
		interaction: interactionrpc.NewClient("/tmp/agentic-interaction-test.sock"),
	}

	methods := make(map[string]struct{}, len(service.Describe().Methods))
	for _, method := range service.Describe().Methods {
		methods[method] = struct{}{}
	}

	for _, method := range []string{
		"thread.set_name",
		"thread.set_metadata",
		"thread.fork",
		"thread.rollback",
		"thread.read",
	} {
		if _, ok := methods[method]; !ok {
			t.Fatalf("system.describe is missing implemented method %q", method)
		}
	}
}

func containsJSONRPC(raw string) bool {
	return len(raw) > 0 && json.Valid([]byte(raw)) && strings.Contains(raw, `"jsonrpc":"2.0"`)
}
