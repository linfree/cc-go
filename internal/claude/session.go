package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

type SessionStatus string

const (
	StatusStopped  SessionStatus = "stopped"
	StatusStarting SessionStatus = "starting"
	StatusActive   SessionStatus = "active"
	StatusError    SessionStatus = "error"
)

type Session struct {
	ID       string
	Name     string
	WorkDir  string
	Model    string
	Status   SessionStatus
	cliPath  string
	permMode string
	resumeID string
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stdout   io.ReadCloser
	stderr   io.ReadCloser
	mu          sync.Mutex
	stopCh      chan struct{}
	eventCh     chan Event
	closeEventOnce sync.Once
}

type StartOptions struct {
	CLIPath   string
	WorkDir   string
	Model     string
	PermMode  string
	SessionID string
	ResumeID  string
	Name      string
	EnvVars   []string
}

func Start(opts StartOptions) (*Session, error) {
	s := &Session{
		Name:     opts.Name,
		WorkDir:  opts.WorkDir,
		Model:    opts.Model,
		Status:   StatusStarting,
		cliPath:  opts.CLIPath,
		permMode: opts.PermMode,
		resumeID: opts.ResumeID,
		stopCh:   make(chan struct{}),
		eventCh:  make(chan Event, 100),
	}

	args := []string{
		"-p",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--verbose",
		"--permission-mode", opts.PermMode,
		"--permission-prompt-tool", "stdio",
		"--max-turns", "0",
	}
	if opts.ResumeID != "" {
		args = append(args, "--resume", opts.ResumeID)
	}
	if opts.SessionID != "" {
		args = append(args, "--session-id", opts.SessionID)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}

	env := os.Environ()
	env = filterOut(env, "CLAUDECODE")
	env = append(env, opts.EnvVars...)

	s.cmd = exec.Command(s.cliPath, args...)
	s.cmd.Dir = opts.WorkDir
	s.cmd.Env = env
	setHideWindow(s.cmd)

	var err error
	s.stdin, err = s.cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}
	s.stdout, err = s.cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	s.stderr, err = s.cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := s.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude: %w", err)
	}

	go s.readStdout()
	go s.readStderr()

	s.Status = StatusActive
	return s, nil
}

func (s *Session) SendMessage(text string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != StatusActive {
		return fmt.Errorf("session not active: %s", s.Status)
	}
	msg := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": text,
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.stdin, "%s\n", data)
	return err
}

func (s *Session) RespondPermission(requestID string, allow bool, reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusStopped || s.Status == "stopping" {
		return fmt.Errorf("session is stopped")
	}

	response := map[string]interface{}{
		"subtype":    "success",
		"request_id": requestID,
	}
	if allow {
		response["response"] = map[string]interface{}{
			"behavior":     "allow",
			"updatedInput": map[string]interface{}{},
		}
	} else {
		response["response"] = map[string]interface{}{
			"behavior": "deny",
			"message":  reason,
		}
	}

	envelope := map[string]interface{}{
		"type":     "control_response",
		"response": response,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.stdin, "%s\n", data)
	return err
}

func (s *Session) RespondWithAnswer(requestID string, toolInput map[string]interface{}, answer string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.Status == StatusStopped || s.Status == "stopping" {
		return fmt.Errorf("session is stopped")
	}

	// Clone and inject answer into the answers field per protocol spec
	updatedInput := cloneMap(toolInput)
	if questions, ok := updatedInput["questions"].([]interface{}); ok && len(questions) > 0 {
		answers := make(map[string]string)
		for _, q := range questions {
			if qm, ok := q.(map[string]interface{}); ok {
				key, _ := qm["question"].(string)
				if key == "" {
					key, _ = qm["header"].(string)
				}
				answers[key] = answer
			}
		}
		updatedInput["answers"] = answers
	} else {
		updatedInput["text"] = answer
	}

	response := map[string]interface{}{
		"subtype":    "success",
		"request_id": requestID,
		"response": map[string]interface{}{
			"behavior":     "allow",
			"updatedInput": updatedInput,
		},
	}

	envelope := map[string]interface{}{
		"type":     "control_response",
		"response": response,
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(s.stdin, "%s\n", data)
	return err
}

func cloneMap(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		if vm, ok := v.(map[string]interface{}); ok {
			result[k] = cloneMap(vm)
		} else if va, ok := v.([]interface{}); ok {
			cloned := make([]interface{}, len(va))
			for i, av := range va {
				if avm, ok := av.(map[string]interface{}); ok {
					cloned[i] = cloneMap(avm)
				} else {
					cloned[i] = av
				}
			}
			result[k] = cloned
		} else {
			result[k] = v
		}
	}
	return result
}

func (s *Session) Events() <-chan Event { return s.eventCh }

func (s *Session) Stop() error {
	s.mu.Lock()
	if s.Status == StatusStopped || s.Status == "stopping" {
		s.mu.Unlock()
		return nil
	}
	s.Status = "stopping"
	s.mu.Unlock()

	s.stdin.Close()
	close(s.stopCh)

	// Kill immediately on Windows since stdin close doesn't terminate the process
	if s.cmd.Process != nil {
		s.cmd.Process.Kill()
	}

	done := make(chan struct{})
	go func() {
		s.cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}

	s.mu.Lock()
	s.Status = StatusStopped
	s.mu.Unlock()
	s.closeEventOnce.Do(func() { close(s.eventCh) })
	return nil
}

func (s *Session) PID() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return 0
}

func (s *Session) readStdout() {
	defer func() {
		s.mu.Lock()
		if s.Status == StatusActive {
			s.Status = StatusError
		}
		s.mu.Unlock()
		s.closeEventOnce.Do(func() { close(s.eventCh) })
	}()
	scanner := bufio.NewScanner(s.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var raw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		evt := parseEvent(raw, s.ID)
		select {
		case s.eventCh <- evt:
		case <-s.stopCh:
			return
		}
	}
}

func (s *Session) readStderr() {
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		fmt.Fprintf(os.Stderr, "[claude stderr] %s\n", scanner.Text())
	}
}

func filterOut(env []string, key string) []string {
	var result []string
	prefix := key + "="
	for _, e := range env {
		if len(e) < len(prefix) || e[:len(prefix)] != prefix {
			result = append(result, e)
		}
	}
	return result
}