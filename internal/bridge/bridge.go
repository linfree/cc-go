package bridge

import (
	"fmt"
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
	typingStopCh  chan struct{}
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
	return &Bridge{
		config:   cfg,
		store:    s,
		eventBus: make(chan interface{}, 200),
	}
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
	text := strings.TrimSpace(msg.Text)

	helpCmd := b.config.BotCommandByKeyword("/help")
	sessionsCmd := b.config.BotCommandByKeyword("/sessions")
	switchCmd := b.config.BotCommandByKeyword("/switch")
	statusCmd := b.config.BotCommandByKeyword("/status")
	stopCmd := b.config.BotCommandByKeyword("/stop")
	yCmd := b.config.BotCommandByKeyword("/y")
	nCmd := b.config.BotCommandByKeyword("/n")

	if helpCmd != nil && helpCmd.Enabled && text == helpCmd.Keyword {
		var sb strings.Builder
		sb.WriteString("**可用指令**\n")
		for _, c := range b.config.BotCommands {
			if c.Key == "help" || !c.Enabled {
				continue
			}
			sb.WriteString(fmt.Sprintf("`%s` %s\n", c.Keyword, c.Description))
		}
		b.sendWechat(msg, sb.String())
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

	// /r <text> — respond to AskUserQuestion with custom answer
	if strings.HasPrefix(text, "/r ") {
		after := strings.TrimPrefix(text, "/r ")
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
			b.sendWechat(msg, "当前没有待回答的提问")
			return
		}
		found := b.pendingPerms[idx]
		b.pendingPerms = append(b.pendingPerms[:idx], b.pendingPerms[idx+1:]...)
		sess := b.activeSess
		remaining := len(b.pendingPerms)
		b.mu.Unlock()
		if sess == nil {
			b.sendWechat(msg, "会话已结束")
			return
		}
		if err := sess.RespondWithAnswer(found.RequestID, found.ToolInput, after); err != nil {
			b.sendWechat(msg, fmt.Sprintf("回复失败: %v", err))
		} else {
			b.sendWechat(msg, fmt.Sprintf("✅ 已回复: %s", after))
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
		b.sendWechat(msg, "当前没有活跃的 Claude 会话。请先在 Web 界面选择或启动会话。")
		return
	}

	b.stopTyping()
	stopCh := b.startTypingHeartbeat(msg.FromUserID, msg.ContextToken)
	b.mu.Lock()
	b.typingStopCh = stopCh
	b.mu.Unlock()

	if err := sess.SendMessage(text); err != nil {
		b.stopTyping()
		b.sendWechat(msg, fmt.Sprintf("发送消息失败: %v", err))
	} else {
		b.mu.Lock()
		if b.activeLogger != nil {
			b.activeLogger.User(text)
		}
		b.mu.Unlock()
	}
}

func (b *Bridge) handleSessions(msg wechat.Message) {
	historySessions, err := claude.DiscoverSessions()
	if err != nil {
		b.sendWechat(msg, "获取会话列表失败")
		return
	}
	if len(historySessions) == 0 {
		b.sendWechat(msg, "暂无会话记录")
		return
	}
	activeID := b.ActiveSessionID()
	var sb strings.Builder
	sb.WriteString("**会话列表（最近10条）**\n\n---\n\n")
	count := 0
	for _, s := range historySessions {
		if count >= 10 {
			break
		}
		statusLabel := "○"
		if s.ID == activeID {
			statusLabel = "●"
		}
		sb.WriteString(fmt.Sprintf("%s `%s` %s\n\n", statusLabel, s.ID, s.FirstPrompt))
		count++
	}
	b.sendWechat(msg, sb.String())
}

func (b *Bridge) handleStatus(msg wechat.Message) {
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil {
		b.sendWechat(msg, "当前无活跃会话")
		return
	}
	b.sendWechat(msg, fmt.Sprintf("**会话状态**\n> 名称: %s\n> 状态: %s\n> 目录: %s", sess.Name, sess.Status, sess.WorkDir))
}

func (b *Bridge) handleStop(msg wechat.Message) {
	b.mu.Lock()
	sess := b.activeSess
	b.mu.Unlock()

	if sess == nil {
		b.sendWechat(msg, "当前无活跃会话")
		return
	}
	b.StopSession()
	b.sendWechat(msg, "会话已停止")
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
		b.sendWechat(msg, "用法: /y <数量> 或 /y all")
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
		b.sendWechat(msg, "用法: /n <数量> 或 /n all")
		return
	}
	b.handlePermissionResponse(false, n, msg)
}

func (b *Bridge) handlePermissionResponse(allow bool, count int, msg wechat.Message) {
	b.mu.Lock()
	if len(b.pendingPerms) == 0 {
		b.mu.Unlock()
		b.sendWechat(msg, "当前没有待处理的权限请求")
		return
	}
	if count > len(b.pendingPerms) {
		count = len(b.pendingPerms)
	}
	toProcess := b.pendingPerms[:count]
	b.pendingPerms = b.pendingPerms[count:]
	sess := b.activeSess
	remaining := len(b.pendingPerms)
	b.mu.Unlock()
	if sess == nil {
		b.sendWechat(msg, "会话已结束，无法响应权限请求")
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
		b.sendWechat(msg, fmt.Sprintf("%s `%s`", action, toProcess[0].ToolName))
	} else {
		b.sendWechat(msg, fmt.Sprintf("批量%s %d 个权限请求", action, count))
	}

	if remaining > 0 {
		// Check if there's any AskUserQuestion pending
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
		b.sendWechat(msg, hint)
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

	logger, _ := claude.NewSessionLogger(sessionID)
	b.activeLogger = logger

	// Store session metadata for resume sessions.
	// New sessions get their ID from the system event.
	if sessionID != "" {
		b.store.InsertSession(&store.Session{
			ID:      sessionID,
			Name:    opts.Name,
			WorkDir: opts.WorkDir,
			Model:   opts.Model,
			Status:  "active",
		})
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

	b.emit(WSEvent{Event: "session_status_changed", SessionID: sessionID, Status: "active"})

	if b.wechatClient != nil && b.config.IsPushEnabled("session_status") {
		ct := b.wechatClient.LastContact()
		if ct.FromID != "" {
			b.wechatClient.SendMessage(ct.FromID, ct.ContextToken,
				fmt.Sprintf("✅ **已接管会话**\n\n**名称：** %s\n\n**会话ID：** `%s`\n\n**目录：** %s",
					opts.Name, sessionID, opts.WorkDir))
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
					b.store.InsertSession(b.newSession)
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
					b.sendWechatToLast(evt.Text)
				}
			}
			for _, block := range evt.Content {
				if block.Type == "tool_use" {
					logger.ToolUse(block.Name, fmt.Sprintf("%v", truncateInput(block.Input)))
					if b.config.IsPushEnabled("tool_use") {
						b.sendWechatToLast(fmt.Sprintf("**工具调用** `%s`\n> %v", block.Name, truncateInput(block.Input)))
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
			b.sendWechatToLast(permMsg)
			b.emit(WSEvent{
				Event:     "permission_request",
				SessionID: b.activeSessID,
				RequestID: evt.RequestID,
				Tool:      evt.ToolName,
				Data:      evt.ToolInput,
			})

		case claude.EventResult:
			logger.Result(evt.StopReason, evt.DurationMs, evt.NumTurns)
			if evt.StopReason != "tool_use" {
				b.stopTyping()
			}
			if b.config.IsPushEnabled("session_status") {
				b.sendWechatToLast(fmt.Sprintf("✅ **完成** %s (耗时 %dms, %d 轮)",
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
			b.sendWechatToLast(fmt.Sprintf("权限请求已取消，剩余 %d 个", remaining))
			b.emit(WSEvent{
				Event:     "permission_cancel",
				SessionID: b.activeSessID,
				Data:      evt.RequestID,
			})
		}
	}
}

func (b *Bridge) formatPermissionWeChat(count int, toolName string, input map[string]interface{}) string {
	// Check if this is AskUserQuestion
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

	// Regular tool permission — show command preview
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

func (b *Bridge) resumeSession(id string, msg wechat.Message) {
	var workDir, name string
	sessMeta, err := b.store.GetSession(id)
	if err == nil {
		workDir = sessMeta.WorkDir
		name = sessMeta.Name
	} else if hs := claude.FindSession(id); hs != nil {
		workDir = hs.ProjectPath
		name = hs.FirstPrompt
	}
	if workDir == "" {
		b.sendWechat(msg, fmt.Sprintf("找不到会话: %s", id))
		return
	}
	err = b.StartSession(claude.StartOptions{
		WorkDir:  workDir,
		Name:     name,
		ResumeID: id,
	})
	if err != nil {
		b.sendWechat(msg, fmt.Sprintf("恢复会话失败: %v", err))
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
	var chunks []string
	runes := []rune(text)
	total := (len(runes) + maxLen - 1) / maxLen
	for i := 0; i < len(runes); i += maxLen {
		end := i + maxLen
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