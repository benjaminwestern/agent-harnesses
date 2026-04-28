package opencode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupStaleOwnedServersRemovesDeadOwnerRecord(t *testing.T) {
	t.Setenv(serverOwnerDirEnv, t.TempDir())
	path := writeOwnerRecordForTest(t, serverOwnerRecord{
		Kind:      serverOwnerKind,
		OwnerPID:  999999999,
		ServerPID: 0,
		BaseURL:   "http://127.0.0.1:1",
	})

	cleanupStaleOwnedServers()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("owner record still exists after cleanup: %v", err)
	}
}

func TestCleanupStaleOwnedServersKeepsLiveOwnerRecord(t *testing.T) {
	t.Setenv(serverOwnerDirEnv, t.TempDir())
	path := writeOwnerRecordForTest(t, serverOwnerRecord{
		Kind:      serverOwnerKind,
		OwnerPID:  os.Getpid(),
		ServerPID: 0,
		BaseURL:   "http://127.0.0.1:1",
	})

	cleanupStaleOwnedServers()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("owner record was removed for live owner: %v", err)
	}
}

func writeOwnerRecordForTest(t *testing.T, record serverOwnerRecord) string {
	t.Helper()
	dir := serverOwnerDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "server-test.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
