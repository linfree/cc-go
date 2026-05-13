package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Session struct {
	Seq          int       `json:"seq"`
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	WorkDir      string    `json:"work_dir"`
	Model        string    `json:"model"`
	Status       string    `json:"status"`
	ClaudePID    int       `json:"claude_pid"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	HistoryPath  string    `json:"history_path"`
	MessageCount int       `json:"message_count"`
	GitBranch    string    `json:"git_branch"`
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

const tableDef = `CREATE TABLE IF NOT EXISTS sessions (
	seq INTEGER PRIMARY KEY AUTOINCREMENT,
	id TEXT UNIQUE NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	work_dir TEXT NOT NULL DEFAULT '',
	model TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'idle',
	claude_pid INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	last_active_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	history_path TEXT NOT NULL DEFAULT '',
	message_count INTEGER NOT NULL DEFAULT 0,
	git_branch TEXT NOT NULL DEFAULT ''
)`

const allCols = `seq, id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path, message_count, git_branch`

func (s *Store) migrate() error {
	row := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name='sessions'`)
	var schema string
	if err := row.Scan(&schema); err != nil {
		// Table doesn't exist yet — create it.
		_, err := s.db.Exec(tableDef)
		return err
	}

	if strings.Contains(schema, "id TEXT PRIMARY KEY") {
		statements := []string{
			tableDef,
			`INSERT INTO sessions (seq, id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path)
			 SELECT rowid, id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path FROM sessions_old WHERE id IS NOT NULL AND id != ''`,
		}
		s.db.Exec(`ALTER TABLE sessions RENAME TO sessions_old`)
		for _, stmt := range statements {
			if _, err := s.db.Exec(stmt); err != nil {
				return err
			}
		}
		s.db.Exec(`DROP TABLE sessions_old`)
	} else if !strings.Contains(schema, "message_count") {
		s.db.Exec(`ALTER TABLE sessions ADD COLUMN message_count INTEGER NOT NULL DEFAULT 0`)
		s.db.Exec(`ALTER TABLE sessions ADD COLUMN git_branch TEXT NOT NULL DEFAULT ''`)
	}
	return nil
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
	createdAt := sess.CreatedAt
	lastActiveAt := sess.LastActiveAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	if lastActiveAt.IsZero() {
		lastActiveAt = time.Now()
	}
	_, err := s.db.Exec(
		`INSERT INTO sessions (id, name, work_dir, model, status, claude_pid, created_at, last_active_at, history_path, message_count, git_branch)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = CASE WHEN ? != '' THEN ? ELSE name END,
			work_dir = CASE WHEN ? != '' THEN ? ELSE work_dir END,
			model = CASE WHEN ? != '' THEN ? ELSE model END,
			status = ?,
			claude_pid = ?,
			last_active_at = ?`,
		sess.ID, sess.Name, sess.WorkDir, sess.Model, sess.Status, sess.ClaudePID,
		createdAt, lastActiveAt, sess.HistoryPath, sess.MessageCount, sess.GitBranch,
		sess.Name, sess.Name,
		sess.WorkDir, sess.WorkDir,
		sess.Model, sess.Model,
		sess.Status,
		sess.ClaudePID,
		lastActiveAt,
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

func scanSession(row *sql.Row) (*Session, error) {
	var sess Session
	err := row.Scan(&sess.Seq, &sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath, &sess.MessageCount, &sess.GitBranch)
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *Store) GetSession(id string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT `+allCols+` FROM sessions WHERE id = ?`, id)
	return scanSession(row)
}

func (s *Store) GetSessionBySeq(seq int) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT `+allCols+` FROM sessions WHERE seq = ?`, seq)
	return scanSession(row)
}

func (s *Store) ListSessions() ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(`SELECT ` + allCols + ` FROM sessions ORDER BY CASE WHEN status = 'active' THEN 0 ELSE 1 END, last_active_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Session
	for rows.Next() {
		var sess Session
		if err := rows.Scan(&sess.Seq, &sess.ID, &sess.Name, &sess.WorkDir, &sess.Model, &sess.Status, &sess.ClaudePID, &sess.CreatedAt, &sess.LastActiveAt, &sess.HistoryPath, &sess.MessageCount, &sess.GitBranch); err != nil {
			return nil, err
		}
		result = append(result, sess)
	}
	return result, rows.Err()
}

func (s *Store) GetActiveSession() (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT `+allCols+` FROM sessions WHERE status = 'active' LIMIT 1`)
	return scanSession(row)
}

func (s *Store) DeleteSession(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	return err
}

func (s *Store) UpdateSessionName(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`INSERT INTO sessions (id, name) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET name = ?`, id, name, name)
	return err
}

func (s *Store) UpdateSessionField(id, field string, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE sessions SET `+field+` = ? WHERE id = ?`, value, id)
	return err
}

func (s *Store) SyncFromDiscovery(discovered []struct {
	ID           string
	Name         string
	WorkDir      string
	Model        string
	Modified     string
	MessageCount int
	GitBranch    string
	FilePath     string
}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, d := range discovered {
		var count int
		row := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = ?`, d.ID)
		if err := row.Scan(&count); err != nil {
			continue
		}
			if count > 0 {
				if d.Modified != "" {
					modTime, err := time.Parse(time.RFC3339, d.Modified)
					if err == nil {
						s.db.Exec(`UPDATE sessions SET last_active_at = ? WHERE id = ? AND status != 'active'`, modTime, d.ID)
					}
				}
				if d.Model != "" {
					s.db.Exec(`UPDATE sessions SET model = ? WHERE id = ?`, d.Model, d.ID)
				}
				if d.MessageCount > 0 {
					s.db.Exec(`UPDATE sessions SET message_count = ? WHERE id = ?`, d.MessageCount, d.ID)
				}
				if d.GitBranch != "" {
					s.db.Exec(`UPDATE sessions SET git_branch = ? WHERE id = ?`, d.GitBranch, d.ID)
				}
				if d.FilePath != "" {
					s.db.Exec(`UPDATE sessions SET history_path = ? WHERE id = ?`, d.FilePath, d.ID)
				}
				continue
			}
			model := d.Model
			if model == "" {
				model = "unknown"
			}
			now := time.Now()
			createdAt := now
			lastActiveAt := now
			if d.Modified != "" {
				if t, err := time.Parse(time.RFC3339, d.Modified); err == nil {
					lastActiveAt = t
					createdAt = t
				}
			}
			s.db.Exec(
				`INSERT OR IGNORE INTO sessions (id, name, work_dir, model, status, created_at, last_active_at, message_count, git_branch, history_path) VALUES (?, ?, ?, ?, 'stopped', ?, ?, ?, ?, ?)`,
				d.ID, d.Name, d.WorkDir, model, createdAt, lastActiveAt, d.MessageCount, d.GitBranch, d.FilePath,
			)
	}
	return nil
}
