package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Session struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	WorkDir      string    `json:"work_dir"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	ClaudePID    int       `json:"claude_pid"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	HistoryPath  string    `json:"history_path"`
}

type Store struct {
	db *sql.DB
	mu sync.RWMutex
}

func Open() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbDir := filepath.Join(home, ".cc-go")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dbDir, "cc-go.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL DEFAULT '',
		work_dir TEXT NOT NULL DEFAULT '',
		model TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'idle',
		claude_pid INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		history_path TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) ResetActiveSessions() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE sessions SET status = 'stopped' WHERE status = 'active'`)
	return err
}

func (s *Store) InsertSession(sess *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO sessions (id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sess.ID, sess.Name, sess.WorkDir, sess.Model, sess.Status, sess.ClaudePID,
		sess.CreatedAt, sess.LastActiveAt, sess.HistoryPath,
	)
	return err
}

func (s *Store) UpdateSessionID(oldID, newID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE sessions SET id = ? WHERE id = ?`, newID, oldID)
	return err
}

func (s *Store) UpdateSessionStatus(id, status string, pid int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`UPDATE sessions SET status = ?, claude_pid = ?, last_active_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, pid, id,
	)
	return err
}

func (s *Store) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path FROM sessions WHERE id = ?`, id)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ListSessions() ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path FROM sessions ORDER BY last_active_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath); err != nil {
			return nil, err
		}
		result = append(result, sess)
	}
	return result, rows.Err()
}

func (s *Store) GetActiveSession() (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path FROM sessions WHERE status = 'active' LIMIT 1`)
	var sess Session
	err := row.Scan(&sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}