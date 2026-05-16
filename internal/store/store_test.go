package store

import (
	"os"
	"testing"
	"time"
)

func setupStore(t *testing.T) (*Store, string) {
	t.Helper()
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)
	os.Setenv("USERPROFILE", tmpDir)
	s, err := Open()
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s, tmpDir
}

func TestStoreCRUD(t *testing.T) {
	s, _ := setupStore(t)

	now := time.Now()
	sess := &Session{
		ID:           "test-session-1",
		Name:         "test",
		WorkDir:      "/tmp/test",
		Status:       "idle",
		ClaudePID:    0,
		CreatedAt:    now,
		LastActiveAt: now,
		HistoryPath:  "/some/path.jsonl",
	}
	if err := s.InsertSession(sess); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := s.GetSession("test-session-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "test" {
		t.Errorf("expected name test, got %s", got.Name)
	}
	if got.WorkDir != "/tmp/test" {
		t.Errorf("expected /tmp/test, got %s", got.WorkDir)
	}
}

func TestUpdateSessionStatus(t *testing.T) {
	s, _ := setupStore(t)

	s.InsertSession(&Session{
		ID:      "test-update-1",
		Name:    "test",
		WorkDir: "/tmp",
		Status:  "idle",
	})
	if err := s.UpdateSessionStatus("test-update-1", "active", 12345); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := s.GetSession("test-update-1")
	if got.Status != "active" {
		t.Errorf("expected active, got %s", got.Status)
	}
	if got.ClaudePID != 12345 {
		t.Errorf("expected pid 12345, got %d", got.ClaudePID)
	}
}

func TestListSessions(t *testing.T) {
	s, _ := setupStore(t)

	for i, id := range []string{"a", "b", "c"} {
		s.InsertSession(&Session{ID: id, Name: id, Status: "idle", LastActiveAt: time.Now().Add(time.Duration(i) * time.Hour)})
	}
	list, err := s.ListSessions()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3, got %d", len(list))
	}
	// Should be ordered by last_active_at DESC
	if list[0].ID != "c" {
		t.Errorf("expected c first, got %s", list[0].ID)
	}
}

func TestGetActiveSession(t *testing.T) {
	s, _ := setupStore(t)

	s.InsertSession(&Session{ID: "idle-1", Status: "idle"})
	s.InsertSession(&Session{ID: "active-1", Status: "active"})
	s.InsertSession(&Session{ID: "idle-2", Status: "idle"})

	got, err := s.GetActiveSession()
	if err != nil {
		t.Fatalf("get active: %v", err)
	}
	if got.ID != "active-1" {
		t.Errorf("expected active-1, got %s", got.ID)
	}
}

func TestGetActiveSession_None(t *testing.T) {
	s, _ := setupStore(t)
	s.InsertSession(&Session{ID: "idle-1", Status: "idle"})
	_, err := s.GetActiveSession()
	if err == nil {
		t.Error("expected error when no active session")
	}
}

func TestDeleteSession(t *testing.T) {
	s, _ := setupStore(t)

	s.InsertSession(&Session{ID: "to-delete", Status: "idle"})
	if err := s.DeleteSession("to-delete"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err := s.GetSession("to-delete")
	if err == nil {
		t.Error("expected error after delete")
	}
}