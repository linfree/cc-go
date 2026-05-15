package bridge

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/linfree/cc-go/internal/claude"
	"github.com/linfree/cc-go/internal/config"
	"github.com/linfree/cc-go/internal/store"
	"github.com/linfree/cc-go/internal/wechat"
)

const maxWeChatMsgLen = 3500

type BufferMediaType int

const (
	BufText BufferMediaType = iota
	BufImage
	BufFile
	BufVideo
)

type bufferedMessage struct {
	text      string
	isPerm    bool
	mediaType BufferMediaType
	filePath  string
}

type WechatInfo struct {
	SendBudget    int    `json:"send_budget"`
	BufferMode    bool   `json:"buffer_mode"`
	BufferedCount int    `json:"buffered_count"`
	LastMsgTime   string `json:"last_msg_time"`
}

type Bridge struct {
	config        *config.Config
	store         *store.Store
	wechatClient  *wechat.Client
	activeSess    *claude.Session
	activeSessID  string
	activeLogger  *claude.SessionLogger
	newSession    *store.Session
	pendingPerms  []pendingPermission
	mu            sync.Mutex
	eventBus      chan interface{}
	stopCh        chan struct{}
	typingStopCh  chan struct{}
	sendBudget    int
	bufferMode    bool
	msgBuffer     []bufferedMessage
	pendingText   bytes.Buffer
	lastMsgTime   string
}

type pendingPermission struct {
	RequestID string
	ToolName  string
	ToolInput map[string]interface{}
}

type PendingPermissionInfo struct {
	RequestID string                 `json:"request_id"`
	ToolName  string                 `json:"tool_name"`
	ToolInput map[string]interface{} `json:"tool_input"`
}

type WSEvent struct {
	Event     string      `json:"event"`
	SessionID string      `json:"session_id,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Status    string      `json:"status,omitempty"`
	Tool      string      `json:"tool,omitempty"`
	Type      string      `json:"type,omitempty"`
	Content   string      `json:"content,omitempty"`
	Data      interface{} `json:"data,omitempty"`
}

func New(cfg *config.Config, s *store.Store) *Bridge {
	br := &Bridge{
		config:   cfg,
		store:    s,
		eventBus: make(chan interface{}, 200),
		stopCh:   make(chan struct{}),
	}
	br.SyncSessions()
	go br.periodicSync()
	return br
}

func (b *Bridge) Close() {
	close(b.stopCh)
}

func (b *Bridge) periodicSync() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			b.SyncSessions()
		case <-b.stopCh:
			return
		}
	}
}

// SyncSessions merges Claude's discovered JSONL sessions into the store database.
func (b *Bridge) SyncSessions() {
	sessions, err := claude.DiscoverSessions()
	if err != nil {
		log.Printf("[bridge] sync sessions error: %v", err)
		return
	}
	var discovered []struct {
		ID           string
		Name         string
		WorkDir      string
		Model        string
		Modified     string
		MessageCount int
		GitBranch    string
		FilePath     string
	}
	for _, s := range sessions {
		name := s.FirstPrompt
		if storeSess, _ := b.store.GetSession(s.ID); storeSess != nil && storeSess.Name != "" {
			name = storeSess.Name
		}
		discovered = append(discovered, struct {
			ID           string
			Name         string
			WorkDir      string
			Model        string
			Modified     string
			MessageCount int
			GitBranch    string
			FilePath     string
		}{s.ID, name, s.ProjectPath, s.Model, s.Modified, s.MessageCount, s.GitBranch, s.FilePath})
	}
	b.store.SyncFromDiscovery(discovered)
}

func (b *Bridge) SetWechatClient(wc *wechat.Client) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.wechatClient = wc
}

func (b *Bridge) EventBus() <-chan interface{} {
	return b.eventBus
}

func (b *Bridge) RespondPermission(requestID string, allow bool) error {
	b.mu.Lock()
	sess := b.activeSess
	idx := -1
	for i, p := range b.pendingPerms {
		if p.RequestID == requestID {
			idx = i
			break
		}
	}
	if idx >= 0 {
		b.pendingPerms = append(b.pendingPerms[:idx], b.pendingPerms[idx+1:]...)
	}
	b.mu.Unlock()

	if sess == nil {
		return fmt.Errorf("no active session")
	}
	if allow {
		return sess.RespondPermission(requestID, true, "")
	}
	return sess.RespondPermission(requestID, false, "用户拒绝")
}

func (b *Bridge) RespondWithAnswer(requestID string, answer string) error {
	b.mu.Lock()
	sess := b.activeSess
	idx := -1
	var toolInput map[string]interface{}
	for i, p := range b.pendingPerms {
		if p.RequestID == requestID {
			idx = i
			toolInput = p.ToolInput
			break
		}
	}
	if idx >= 0 {
		b.pendingPerms = append(b.pendingPerms[:idx], b.pendingPerms[idx+1:]...)
	}
	b.mu.Unlock()

	if sess == nil {
		return fmt.Errorf("no active session")
	}
	return sess.RespondWithAnswer(requestID, toolInput, answer)
}

func (b *Bridge) ActiveSession() *claude.Session {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activeSess
}

func (b *Bridge) emit(evt WSEvent) {
	select {
	case b.eventBus <- evt:
	default:
	}
}

func (b *Bridge) HandleWechatMessage(msg wechat.Message) {
	b.resetSendBudget()
	b.flushMessageBuffer(msg)

	b.mu.Lock()
	b.lastMsgTime = time.Now().Format(time.RFC3339)
	b.mu.Unlock()

	text := strings.TrimSpace(msg.Text)

	helpCmd := b.config.BotCommandByKeyword("/help")
	sessionsCmd := b.config.BotCommandByKeyword("/sessions")
	switchCmd := b.config.BotCommandByKeyword("/switch")
	statusCmd := b.config.BotCommandByKeyword("/status")
	stopCmd := b.config.BotCommandByKeyword("/stop")
	yCmd := b.config.BotCommandByKeyword("/y")
	nCmd := b.config.BotCommandByKeyword("/n")
	rCmd := b.config.BotCommandByKeyword("/r")
	reloginCmd := b.config.BotCommandByKey("relogin")

	if helpCmd != nil && helpCmd.Enabled && text == helpCmd.Keyword {
		var sb strings.Builder
		sb.WriteString("**可用指令**\n\n")
		for _, c := range b.config.BotCommands {
			if c.Key == "help" || !c.Enabled {
				continue
			}
			sb.WriteString(fmt.Sprintf("- `%s` %s\n", c.Keyword, c.Description))
		}
		b.sendWechatBudgetedSingle(sb.String())
		return
	}
	if sessionsCmd != nil && sessionsCmd.Enabled && text == sessionsCmd.Keyword {
		b.handleSessions(msg)
		return
	}
	if switchCmd != nil && switchCmd.Enabled {
		if after, ok := strings.CutPrefix(text, switchCmd.Keyword+" "); ok {
			b.resumeSession(after, msg)
			return
		}
	}
	if statusCmd != nil && statusCmd.Enabled && text == statusCmd.Keyword {
		b.handleStatus(msg)
		return
	}
	if stopCmd != nil && stopCmd.Enabled && text == stopCmd.Keyword {
		b.handleStop(msg)
		return
	}
	if yCmd != nil && yCmd.Enabled {
		if text == yCmd.Keyword {
			b.handlePermissionResponse(true, 1, msg)
			return
		}
		if after, ok := strings.CutPrefix(text, yCmd.Keyword+" "); ok {
			b.handleBatchApprove(after, msg)
			return
		}
	}
	if nCmd != nil && nCmd.Enabled {
		if text == nCmd.Keyword {
			b.handlePermissionResponse(false, 1, msg)
			return
		}
		if after, ok := strings.CutPrefix(text, nCmd.Keyword+" "); ok {
			b.handleBatchReject(after, msg)
			return
		}
	}
	if reloginCmd != nil && reloginCmd.Enabled && text == reloginCmd.Keyword {
		b.handleRelogin(msg)
		return
	}

	// /r <text> — respond to AskUserQuestion with custom answer
	if rCmd != nil && rCmd.Enabled && strings.HasPrefix(text, rCmd.Keyword+" ") {
		after := strings.TrimPrefix(text, rCmd.Keyword+" ")
		b.mu.Lock()
		idx := -1
		for i, p := range b.pendingPerms {
			if _, ok := p.ToolInput["questions"]; ok {
				idx = i
				break
			}
		}
		if idx < 0 {
			b.mu.Unlock()
			b.sendWechatBudgetedSingle("当前没有待回答的提问")
			return
		}
		found := b.pendingPerms[idx]
		b.pendingPerms = append(b.pendingPerms[:idx], b.pendingPerms[idx+1:]...)
		sess := b.activeSess
		remaining := len(b.pendingPerms)
		b.mu.Unlock()
		if sess == nil {
			b.sendWechatBudgetedSingle("会话已结束")
			return
		}
		if err := sess.RespondWithAnswer(found.RequestID, found.ToolInput, after); err != nil {
			b.sendWechatBudgetedSingle(fmt.Sprintf("回复失败: %v", err))
		} else {
			b.sendWechatBudgetedSingle(fmt.Sprintf("✅ 已回复: %s", after))
			if remaining == 0 {
				b.restartTyping(msg.FromUserID, msg.ContextToken)
			}
		}
		return
	}

	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil || sess.Status != claude.StatusActive {
		b.sendWechatBudgetedSingle("当前没有活跃的 Claude 会话。请先在 Web 界面选择或启动会话。")
		return
	}

	b.stopTyping()
	stopCh := b.startTypingHeartbeat(msg.FromUserID, msg.ContextToken)
	b.mu.Lock()
	b.typingStopCh = stopCh
	b.mu.Unlock()

	if err := sess.SendMessage(text); err != nil {
		b.stopTyping()
		b.sendWechatBudgetedSingle(fmt.Sprintf("发送消息失败: %v", err))
	} else {
		b.mu.Lock()
		if b.activeLogger != nil {
			b.activeLogger.User(text)
		}
		b.mu.Unlock()
	}
}

func (b *Bridge) handleSessions(msg wechat.Message) {
	storeSessions, err := b.store.ListSessions()
	if err != nil || len(storeSessions) == 0 {
		b.sendWechatBudgetedSingle("暂无会话记录")
		return
	}
	activeID := b.ActiveSessionID()
	var sb strings.Builder
	sb.WriteString("**会话列表（最近10条）**\n\n---\n\n")
	limit := 10
	if len(storeSessions) < limit {
		limit = len(storeSessions)
	}
	for _, s := range storeSessions[:limit] {
		statusLabel := "○"
		if s.ID == activeID {
			statusLabel = "●"
		}
		name := s.Name
		if name == "" {
			name = s.WorkDir
		}
		sb.WriteString(fmt.Sprintf("%s #%d %s\n\n", statusLabel, s.Seq, name))
	}
	sb.WriteString("---\n回复 `/switch <编号>` 切换会话")
	b.sendWechatBudgetedSingle(sb.String())
}

func (b *Bridge) handleStatus(msg wechat.Message) {
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil {
		b.sendWechatBudgetedSingle("当前无活跃会话")
		return
	}
	b.sendWechatBudgetedSingle(fmt.Sprintf("**会话状态**\n> 名称: %s\n> 状态: %s\n> 目录: %s", sess.Name, sess.Status, sess.WorkDir))
}

func (b *Bridge) handleStop(msg wechat.Message) {
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil {
		b.sendWechatBudgetedSingle("当前无活跃会话")
		return
	}
	b.StopSession()
	b.sendWechatBudgetedSingle("会话已停止")
}

func (b *Bridge) handleRelogin(msg wechat.Message) {
	if b.wechatClient == nil {
		b.sendWechatBudgetedSingle("微信客户端未初始化")
		return
	}
	if err := b.wechatClient.TriggerRelogin(); err != nil {
		b.sendWechatBudgetedSingle(fmt.Sprintf("重新登录失败: %v", err))
	} else {
		b.sendWechatBudgetedSingle("已发送重新登录链接，请点击链接扫码登录。")
	}
}

func (b *Bridge) handleBatchApprove(arg string, msg wechat.Message) {
	if arg == "all" {
		b.mu.Lock()
		count := len(b.pendingPerms)
		b.mu.Unlock()
		b.handlePermissionResponse(true, count, msg)
		return
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		b.sendWechatBudgetedSingle("用法: /y <数量> 或 /y all")
		return
	}
	b.handlePermissionResponse(true, n, msg)
}

func (b *Bridge) handleBatchReject(arg string, msg wechat.Message) {
	if arg == "all" {
		b.mu.Lock()
		count := len(b.pendingPerms)
		b.mu.Unlock()
		b.handlePermissionResponse(false, count, msg)
		return
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		b.sendWechatBudgetedSingle("用法: /n <数量> 或 /n all")
		return
	}
	b.handlePermissionResponse(false, n, msg)
}

func (b *Bridge) handlePermissionResponse(allow bool, count int, msg wechat.Message) {
	b.mu.Lock()
	if len(b.pendingPerms) == 0 {
		b.mu.Unlock()
		b.sendWechatBudgetedSingle("当前没有待处理的权限请求")
		return
	}
	if count > len(b.pendingPerms) {
		count = len(b.pendingPerms)
	}
	toProcess := b.pendingPerms[:count]
	b.pendingPerms = b.pendingPerms[count:]
	sess := b.activeSess
	b.mu.Unlock()
	if sess == nil {
		b.sendWechatBudgetedSingle("会话已结束，无法响应权限请求")
		return
	}

	action := "已拒绝"
	if allow {
		action = "已批准"
	}
	for _, p := range toProcess {
		if allow {
			sess.RespondPermission(p.RequestID, true, "")
		} else {
			sess.RespondPermission(p.RequestID, false, "用户拒绝")
		}
	}
	if count == 1 {
		b.sendWechatBudgetedSingle(fmt.Sprintf("%s `%s`", action, toProcess[0].ToolName))
	} else {
		b.sendWechatBudgetedSingle(fmt.Sprintf("批量%s %d 个权限请求", action, count))
	}

	b.mu.Lock()
	remaining := len(b.pendingPerms)
	b.mu.Unlock()
	if remaining > 0 {
		hasAskUser := false
		b.mu.Lock()
		for _, p := range b.pendingPerms {
			if _, ok := p.ToolInput["questions"]; ok {
				hasAskUser = true
				break
			}
		}
		b.mu.Unlock()
		hint := fmt.Sprintf("还有 %d 个权限请求待处理，回复 `/y` 批准  `/n` 拒绝", remaining)
		if hasAskUser {
			hint += "  `/r <回答>` 回答提问"
		}
		b.sendWechatBudgetedSingle(hint)
	} else {
		b.restartTyping(msg.FromUserID, msg.ContextToken)
	}
}

func (b *Bridge) restartTyping(toID, contextToken string) {
	b.stopTyping()
	stopCh := b.startTypingHeartbeat(toID, contextToken)
	b.mu.Lock()
	b.typingStopCh = stopCh
	b.mu.Unlock()
}

func (b *Bridge) restartTypingFromLast() {
	if b.wechatClient == nil {
		return
	}
	ct := b.wechatClient.LastContact()
	if ct.FromID != "" {
		b.restartTyping(ct.FromID, ct.ContextToken)
	}
}

func (b *Bridge) StartSession(opts claude.StartOptions) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.activeSess != nil {
		b.StopSessionLocked()
	}

	if opts.CLIPath == "" {
		opts.CLIPath = b.config.ClaudeCLIPath
	}
	if opts.PermMode == "" {
		opts.PermMode = b.config.PermissionMode
	}
	if len(opts.EnvVars) == 0 {
		opts.EnvVars = b.config.ParseEnvVars()
	}

	sess, err := claude.Start(opts)
	if err != nil {
		return err
	}
	b.activeSess = sess

	sessionID := opts.ResumeID
	b.activeSessID = sessionID

	var logger *claude.SessionLogger
	if sessionID != "" {
		logger, _ = claude.NewSessionLogger(sessionID)
	}
	b.activeLogger = logger

	if sessionID != "" {
if err := b.store.InsertSession(&store.Session{
			ID:      sessionID,
			Name:    opts.Name,
			WorkDir: opts.WorkDir,
			Model:   opts.Model,
			Status:  "active",
		}); err != nil {
			log.Printf("[bridge] insert session error: %v", err)
		}
		b.newSession = nil
	} else {
		b.newSession = &store.Session{
			Name:    opts.Name,
			WorkDir: opts.WorkDir,
			Model:   opts.Model,
			Status:  "active",
		}
	}

	go b.readClaudeEvents(sess, logger)

	if sessionID != "" {
		b.emit(WSEvent{Event: "session_status_changed", SessionID: sessionID, Status: "active"})
		if b.wechatClient != nil && b.config.IsPushEnabled("session_status") {
			ct := b.wechatClient.LastContact()
			if ct.FromID != "" {
				b.wechatClient.SendMessage(ct.FromID, ct.ContextToken,
					fmt.Sprintf("✅ **已接管会话**\n\n**名称：** %s\n\n**会话ID：** `%s`\n\n**目录：** %s",
						opts.Name, sessionID, opts.WorkDir))
			}
		}
	}
	return nil
}

func (b *Bridge) readClaudeEvents(sess *claude.Session, logger *claude.SessionLogger) {
	defer func() {
		if r := recover(); r != nil {
			b.stopTyping()
			logger.Error(fmt.Sprintf("panic: %v", r))
			b.sendWechatToLast(fmt.Sprintf("**内部错误** %v", r))
			b.emit(WSEvent{Event: "session_status_changed", SessionID: b.activeSessID, Status: "error"})
		}
	}()
	defer b.stopTyping()
	defer logger.Close()
	for evt := range sess.Events() {
		switch evt.Type {
		case claude.EventSystem:
			if evt.SessionID != "" && b.activeSessID == "" {
				b.mu.Lock()
				b.activeSessID = evt.SessionID
				if b.newSession != nil {
					b.newSession.ID = evt.SessionID
					if err := b.store.InsertSession(b.newSession); err != nil {
						log.Printf("[bridge] insert session error: %v", err)
					}
					b.newSession = nil
				} else {
					b.store.UpdateSessionID("", evt.SessionID)
				}
				if b.activeLogger != nil {
					b.activeLogger.Close()
				}
				logger2, _ := claude.NewSessionLogger(evt.SessionID)
				b.activeLogger = logger2
				logger = logger2
				b.emit(WSEvent{Event: "session_status_changed", SessionID: evt.SessionID, Status: "active"})
				b.mu.Unlock()
			}
		case claude.EventAssistant:
			if evt.Text != "" {
				logger.Assistant(evt.Text)
				if b.config.IsPushEnabled("claude_response") {
					b.mu.Lock()
					b.pendingText.WriteString(evt.Text)
					b.mu.Unlock()
				}
			}
			for _, block := range evt.Content {
				if block.Type == "tool_use" {
					logger.ToolUse(block.Name, fmt.Sprintf("%v", truncateInput(block.Input)))
					b.flushWechatBudgeted()
					if b.config.IsPushEnabled("tool_use") {
						b.sendWechatBudgetedSingle(fmt.Sprintf("**工具调用** `%s`\n> %v", block.Name, truncateInput(block.Input)))
					}
				}
				if block.Type == "thinking" {
					logger.Thinking(block.Thinking)
				}
			}
			b.emit(WSEvent{
				Event:     "claude_output",
				SessionID: b.activeSessID,
				Type:      "text",
				Content:   evt.Text,
			})

		case claude.EventControlRequest:
			b.stopTyping()
			b.mu.Lock()
			b.pendingPerms = append(b.pendingPerms, pendingPermission{
				RequestID: evt.RequestID,
				ToolName:  evt.ToolName,
				ToolInput: evt.ToolInput,
			})
			count := len(b.pendingPerms)
			b.mu.Unlock()

			logger.PermissionRequest(evt.ToolName, fmt.Sprintf("%v", truncateInput(evt.ToolInput)))

			permMsg := b.formatPermissionWeChat(count, evt.ToolName, evt.ToolInput)
			b.sendWechatBudgetedPerm(permMsg)
			b.emit(WSEvent{
				Event:     "permission_request",
				SessionID: b.activeSessID,
				RequestID: evt.RequestID,
				Tool:      evt.ToolName,
				Data:      evt.ToolInput,
			})

		case claude.EventResult:
			logger.Result(evt.StopReason, evt.DurationMs, evt.NumTurns)
			b.flushWechatBudgeted()
			if evt.StopReason != "tool_use" {
				b.stopTyping()
			}
			if b.config.IsPushEnabled("session_status") {
				b.sendWechatBudgetedSingle(fmt.Sprintf("✅ **完成** %s (耗时 %dms, %d 轮)",
					evt.StopReason, evt.DurationMs, evt.NumTurns))
			}

		case claude.EventError:
			logger.Error(evt.Error)
			b.stopTyping()
			b.sendWechatToLast(fmt.Sprintf("❌ **错误** %s", evt.Error))
			b.emit(WSEvent{Event: "session_status_changed", SessionID: b.activeSessID, Status: "error"})

		case claude.EventControlCancel:
			b.mu.Lock()
			for i, p := range b.pendingPerms {
				if p.RequestID == evt.RequestID {
					b.pendingPerms = append(b.pendingPerms[:i], b.pendingPerms[i+1:]...)
					break
				}
			}
			remaining := len(b.pendingPerms)
			b.mu.Unlock()
			if remaining == 0 {
				b.restartTypingFromLast()
			}
			b.sendWechatBudgetedSingle(fmt.Sprintf("权限请求已取消，剩余 %d 个", remaining))
			b.emit(WSEvent{
				Event:     "permission_cancel",
				SessionID: b.activeSessID,
				Data:      evt.RequestID,
			})
		}
	}
	// Session event stream ended — Claude process exited
	sid := b.activeSessID
	status := sess.Status
	b.StopSession()
	if status == claude.StatusError {
		b.sendWechatBudgetedSingle("Claude 会话异常退出")
	} else {
		b.sendWechatBudgetedSingle("Claude 会话已结束")
	}
	b.emit(WSEvent{Event: "session_status_changed", SessionID: sid, Status: "stopped"})
}

func (b *Bridge) formatPermissionWeChat(count int, toolName string, input map[string]interface{}) string {
	if questions, ok := input["questions"].([]interface{}); ok && len(questions) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**提问** [%d]\n---\n", count))
		for _, q := range questions {
			qm, ok := q.(map[string]interface{})
			if !ok {
				continue
			}
			header, _ := qm["header"].(string)
			question, _ := qm["question"].(string)
			title := header
			if question != "" {
				title = question
			}
			if title != "" {
				sb.WriteString(fmt.Sprintf("> %s\n\n", title))
			}
			if opts, ok := qm["options"].([]interface{}); ok {
				for i, o := range opts {
					om, ok := o.(map[string]interface{})
					if !ok {
						continue
					}
					label, _ := om["label"].(string)
					desc, _ := om["description"].(string)
					sb.WriteString(fmt.Sprintf("%d. **%s**\n", i+1, label))
					if desc != "" {
						sb.WriteString(fmt.Sprintf("   %s\n", desc))
					}
				}
			}
		}
		sb.WriteString("---\n回复 `/r <回答>` 回答提问")
		return sb.String()
	}

	return fmt.Sprintf("**权限请求** [%d]\n> 工具: `%s`\n> %s\n---\n回复 `/y` 批准  `/n` 拒绝",
		count, toolName, truncateInput(input))
}

func (b *Bridge) StopSession() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.StopSessionLocked()
}

func (b *Bridge) StopSessionLocked() {
	if b.activeSess == nil {
		return
	}
	b.pendingPerms = nil
	b.newSession = nil
	b.msgBuffer = nil
	b.bufferMode = false
	b.pendingText.Reset()
	if b.activeLogger != nil {
		b.activeLogger.Close()
		b.activeLogger = nil
	}
	b.store.UpdateSessionStatus(b.activeSessID, "stopped", 0)
	b.activeSess.Stop()
	b.emit(WSEvent{Event: "session_status_changed", SessionID: b.activeSessID, Status: "stopped"})
	b.activeSess = nil
	b.activeSessID = ""
}

func (b *Bridge) resumeSession(arg string, msg wechat.Message) {
	// Try seq number first, then fallback to session ID
	var sessionID, workDir, name string
	if seq, err := strconv.Atoi(arg); err == nil {
		sessMeta, err := b.store.GetSessionBySeq(seq)
		if err != nil || sessMeta == nil {
			b.sendWechatBudgetedSingle(fmt.Sprintf("找不到会话编号 #%d", seq))
			return
		}
		sessionID = sessMeta.ID
		workDir = sessMeta.WorkDir
		name = sessMeta.Name
	} else {
		sessionID = arg
		sessMeta, err := b.store.GetSession(arg)
		if err == nil {
			workDir = sessMeta.WorkDir
			name = sessMeta.Name
		} else if hs := claude.FindSession(arg); hs != nil {
			workDir = hs.ProjectPath
			name = hs.FirstPrompt
		}
	}
	if workDir == "" {
		b.sendWechatBudgetedSingle(fmt.Sprintf("找不到会话: %s", arg))
		return
	}
	err := b.StartSession(claude.StartOptions{
		WorkDir:  workDir,
		Name:     name,
		ResumeID: sessionID,
	})
	if err != nil {
		b.sendWechatBudgetedSingle(fmt.Sprintf("恢复会话失败: %v", err))
	}
}

func (b *Bridge) ActiveSessionID() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activeSessID
}

func (b *Bridge) GetPendingPermissions() []PendingPermissionInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]PendingPermissionInfo, len(b.pendingPerms))
	for i, p := range b.pendingPerms {
		result[i] = PendingPermissionInfo{
			RequestID: p.RequestID,
			ToolName:  p.ToolName,
			ToolInput: p.ToolInput,
		}
	}
	return result
}

func (b *Bridge) GetWechatInfo() WechatInfo {
	b.mu.Lock()
	defer b.mu.Unlock()
	return WechatInfo{
		SendBudget:    b.sendBudget,
		BufferMode:    b.bufferMode,
		BufferedCount: len(b.msgBuffer),
		LastMsgTime:   b.lastMsgTime,
	}
}

// BotAPIBudgetResult is returned by BudgetedBotSend.
type BotAPIBudgetResult struct {
	CanSend         bool
	Buffered        bool
	BudgetExhausted bool
}

// BudgetedBotSend checks send budget for a Bot API message.
// If budget is exhausted, the message is buffered in the queue.
func (b *Bridge) BudgetedBotSend(text string, mediaType BufferMediaType, filePath string) BotAPIBudgetResult {
	b.mu.Lock()
	if b.bufferMode {
		b.addBufLocked(bufferedMessage{text: text, mediaType: mediaType, filePath: filePath})
		b.mu.Unlock()
		return BotAPIBudgetResult{Buffered: true, BudgetExhausted: true}
	}
	if b.sendBudget <= 0 {
		b.bufferMode = true
		b.mu.Unlock()
		ct := b.wechatClient.LastContact()
		if ct.FromID != "" {
			b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, b.formatBudgetExhaustedReminder())
		}
		b.mu.Lock()
		b.addBufLocked(bufferedMessage{text: text, mediaType: mediaType, filePath: filePath})
		b.mu.Unlock()
		return BotAPIBudgetResult{Buffered: true, BudgetExhausted: true}
	}
	b.sendBudget--
	b.mu.Unlock()
	return BotAPIBudgetResult{CanSend: true}
}

func (b *Bridge) SendUserMessage(text string) error {
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()
	if sess == nil {
		return fmt.Errorf("no active session")
	}
	return sess.SendMessage(text)
}

func (b *Bridge) sendWechat(msg wechat.Message, text string) {
	if b.wechatClient == nil || b.wechatClient.Status() != wechat.StatusConnected {
		return
	}
	for _, chunk := range splitLongMessage(text, maxWeChatMsgLen) {
		b.wechatClient.SendMessage(msg.FromUserID, msg.ContextToken, chunk)
	}
}

func (b *Bridge) startTypingHeartbeat(toID, contextToken string) chan struct{} {
	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		defer func() { b.sendTypingOnce(toID, contextToken, false) }()

		b.sendTypingOnce(toID, contextToken, true)

		for {
			select {
			case <-ticker.C:
				b.sendTypingOnce(toID, contextToken, true)
			case <-stopCh:
				return
			}
		}
	}()
	return stopCh
}

func (b *Bridge) sendTypingOnce(toID, contextToken string, typing bool) {
	if b.wechatClient == nil || b.wechatClient.Status() != wechat.StatusConnected {
		return
	}
	b.wechatClient.SendTyping(toID, contextToken, typing)
}

func (b *Bridge) sendWechatToLast(text string) {
	if b.wechatClient == nil {
		return
	}
	ct := b.wechatClient.LastContact()
	if ct.FromID == "" {
		return
	}
	for _, chunk := range splitLongMessage(text, maxWeChatMsgLen) {
		b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, chunk)
	}
}

func (b *Bridge) resetSendBudget() {
	b.mu.Lock()
	b.sendBudget = b.config.Wechat.GetSendBudgetLimit()
	b.pendingText.Reset()
	b.mu.Unlock()
}

func (b *Bridge) addToBufferLocked(text string, isPerm bool) {
	b.addBufLocked(bufferedMessage{text: text, isPerm: isPerm, mediaType: BufText})
}

func (b *Bridge) addBufLocked(m bufferedMessage) {
	maxBuf := b.config.Wechat.GetMaxBufferedMessages()
	for len(b.msgBuffer) >= maxBuf {
		evicted := false
		for i, bm := range b.msgBuffer {
			if !bm.isPerm {
				b.msgBuffer = append(b.msgBuffer[:i], b.msgBuffer[i+1:]...)
				evicted = true
				break
			}
		}
		if !evicted {
			return
		}
	}
	b.msgBuffer = append(b.msgBuffer, m)
}

func (b *Bridge) flushMessageBuffer(msg wechat.Message) {
	b.mu.Lock()
	if !b.bufferMode || len(b.msgBuffer) == 0 {
		b.mu.Unlock()
		return
	}
	if b.wechatClient == nil || b.wechatClient.Status() != wechat.StatusConnected {
		b.mu.Unlock()
		return
	}
	msgs := b.msgBuffer
	b.msgBuffer = nil
	// keep bufferMode=true during flush to block new API sends
	b.mu.Unlock()

	sent := 0
	maxBudget := b.config.Wechat.GetSendBudgetLimit()

	toID := msg.FromUserID
	ctxToken := msg.ContextToken

	// flushOne sends a merged text group or a media message, returns true if sent.
	var textGroup []string
	flushTextGroup := func() bool {
		if len(textGroup) == 0 {
			return false
		}
		var sb strings.Builder
		for i, t := range textGroup {
			if i > 0 {
				sb.WriteString("\n\n---\n\n")
			}
			sb.WriteString(fmt.Sprintf("**消息%d:**\n\n%s", i+1, t))
		}
		b.sendWechat(msg, sb.String())
		textGroup = textGroup[:0]
		return true
	}

	sendItem := func() {
		sent++
		b.mu.Lock()
		if b.sendBudget > 0 {
			b.sendBudget--
		}
		b.mu.Unlock()
	}

	for i := 0; i < len(msgs); i++ {
		bm := msgs[i]
		switch bm.mediaType {
		case BufImage, BufFile, BufVideo:
			// Flush pending text group first
			if flushTextGroup() {
				sendItem()
			}
			// Check budget for media
			if sent >= maxBudget {
				// Put remaining messages back in buffer
				b.putBackBuffer(msgs[i:], textGroup)
				return
			}
			switch bm.mediaType {
			case BufImage:
				b.wechatClient.SendImage(toID, ctxToken, bm.filePath)
			case BufFile:
				b.wechatClient.SendFile(toID, ctxToken, bm.filePath)
			case BufVideo:
				b.wechatClient.SendVideo(toID, ctxToken, bm.filePath)
			}
			sendItem()
		default:
			if bm.text != "" {
				textGroup = append(textGroup, bm.text)
			}
			// Flush text group if next item is media or end of list
			nextIsMedia := false
			if i+1 < len(msgs) {
				nextIsMedia = msgs[i+1].mediaType == BufImage || msgs[i+1].mediaType == BufFile || msgs[i+1].mediaType == BufVideo
			}
			if nextIsMedia || i == len(msgs)-1 {
				if flushTextGroup() {
					sendItem()
				}
			}
		}
		// Stop if we've used the budget
		if sent >= maxBudget && i+1 < len(msgs) {
			b.putBackBuffer(msgs[i+1:], textGroup)
			return
		}
	}

	// All messages sent, clear buffer mode
	b.mu.Lock()
	b.bufferMode = false
	b.mu.Unlock()
}

// putBackBuffer returns unsent messages to the buffer and re-enters buffer mode.
func (b *Bridge) putBackBuffer(remaining []bufferedMessage, pendingText []string) {
	b.mu.Lock()
	b.bufferMode = true
	for _, t := range pendingText {
		b.msgBuffer = append(b.msgBuffer, bufferedMessage{text: t, mediaType: BufText})
	}
	b.msgBuffer = append(b.msgBuffer, remaining...)
	b.mu.Unlock()

	ct := b.wechatClient.LastContact()
	if ct.FromID != "" {
		b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, b.formatBudgetExhaustedReminder())
	}
}

func (b *Bridge) flushWechatBudgeted() {
	b.mu.Lock()
	if b.bufferMode {
		if b.pendingText.Len() > 0 {
			b.addToBufferLocked(b.pendingText.String(), false)
			b.pendingText.Reset()
		}
		b.mu.Unlock()
		return
	}
	if b.pendingText.Len() == 0 {
		b.mu.Unlock()
		return
	}
	text := b.pendingText.String()
	b.pendingText.Reset()
	b.mu.Unlock()

	if b.wechatClient == nil {
		return
	}
	ct := b.wechatClient.LastContact()
	if ct.FromID == "" {
		return
	}

	chunks := splitLongMessage(text, maxWeChatMsgLen)

	for i, chunk := range chunks {
		b.mu.Lock()
		if b.sendBudget <= 0 {
			if !b.bufferMode {
				b.bufferMode = true
				b.mu.Unlock()
				b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, b.formatBudgetExhaustedReminder())
			} else {
				b.mu.Unlock()
			}
			b.mu.Lock()
			for _, c := range chunks[i:] {
				b.addToBufferLocked(c, false)
			}
			b.mu.Unlock()
			return
		}
		b.sendBudget--
		b.mu.Unlock()
		b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, chunk)
	}
}

func (b *Bridge) sendWechatBudgetedSingle(text string) bool {
	b.mu.Lock()
	if b.bufferMode {
		b.addToBufferLocked(text, false)
		b.mu.Unlock()
		return false
	}
	if b.sendBudget <= 0 {
		b.bufferMode = true
		b.mu.Unlock()
		ct := b.wechatClient.LastContact()
		if ct.FromID != "" {
			b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, b.formatBudgetExhaustedReminder())
		}
		b.mu.Lock()
		b.addToBufferLocked(text, false)
		b.mu.Unlock()
		return false
	}
	b.sendBudget--
	b.mu.Unlock()

	if b.wechatClient == nil {
		return false
	}
	ct := b.wechatClient.LastContact()
	if ct.FromID == "" {
		return false
	}
	b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, text)
	return true
}

func (b *Bridge) sendWechatBudgetedPerm(text string) bool {
	b.mu.Lock()
	if b.bufferMode {
		b.addToBufferLocked(text, true)
		b.mu.Unlock()
		return false
	}
	if b.sendBudget <= 0 {
		b.bufferMode = true
		b.mu.Unlock()
		ct := b.wechatClient.LastContact()
		if ct.FromID != "" {
			b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, b.formatBudgetExhaustedReminder())
		}
		b.mu.Lock()
		b.addToBufferLocked(text, true)
		b.mu.Unlock()
		return false
	}
	b.sendBudget--
	b.mu.Unlock()

	if b.wechatClient == nil {
		return false
	}
	ct := b.wechatClient.LastContact()
	if ct.FromID == "" {
		return false
	}
	b.wechatClient.SendMessage(ct.FromID, ct.ContextToken, text)
	return true
}

func (b *Bridge) formatBudgetExhaustedReminder() string {
	cmd := "/"
	if c := b.config.BotCommandByKey("activate"); c != nil && c.Enabled {
		cmd = c.Keyword
	}
	return fmt.Sprintf("⚠️ 本轮消息额度已用完，回复 `%s` 激活消息窗口继续接收", cmd)
}

func (b *Bridge) getActivationCommand() string {
	if cmd := b.config.BotCommandByKey("activate"); cmd != nil && cmd.Enabled {
		return cmd.Keyword
	}
	return "/"
}

func (b *Bridge) stopTyping() {
	b.mu.Lock()
	ch := b.typingStopCh
	b.typingStopCh = nil
	b.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func splitLongMessage(text string, maxLen int) []string {
	if utf8.RuneCountInString(text) <= maxLen {
		return []string{text}
	}
	suffixLen := len(" [999/999]")
	chunkLen := maxLen - suffixLen
	if chunkLen < 1 {
		chunkLen = 1
	}
	var chunks []string
	runes := []rune(text)
	total := (len(runes) + chunkLen - 1) / chunkLen
	for i := 0; i < len(runes); i += chunkLen {
		end := i + chunkLen
		if end > len(runes) {
			end = len(runes)
		}
		chunk := string(runes[i:end])
		if total > 1 {
			chunk += fmt.Sprintf(" [%d/%d]", len(chunks)+1, total)
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

func truncateInput(input map[string]interface{}) string {
	if cmd, ok := input["command"]; ok {
		s := fmt.Sprintf("%v", cmd)
		if len(s) > 200 {
			return s[:200] + "..."
		}
		return s
	}
	s := fmt.Sprintf("%v", input)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}