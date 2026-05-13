import { useState, useEffect, useRef, useCallback } from 'react'
import { useParams } from 'react-router-dom'
import { api } from '../api'

interface HistoryMessage {
  type: string
  role: string
  content: string
  thinking?: string
  tool_use?: { name: string; id?: string; input: Record<string, unknown> }
  tool_result?: string
  tool_use_id?: string
  subtype?: string
  attachment?: string
  timestamp: string
}

interface Session {
  id: string
  name: string
  work_dir: string
  status: string
  message_count: number
  created: string
  modified: string
  history_path: string
  git_branch: string
}

function mergeToolResults(msgs: HistoryMessage[]): HistoryMessage[] {
  const idMap = new Map<string, string>()
  for (const m of msgs) {
    if (m.role === 'assistant' && m.tool_use?.id) {
      idMap.set(m.tool_use.id, '')
    }
  }
  const result: HistoryMessage[] = []
  for (const m of msgs) {
    if (m.role === 'tool_result' && m.tool_use_id && idMap.has(m.tool_use_id)) {
      const target = result.find(r =>
        r.role === 'assistant' && r.tool_use?.id === m.tool_use_id && !r.tool_result
      )
      if (target) {
        target.tool_result = m.tool_result
        continue
      }
    }
    result.push(m)
  }
  return result
}

function statusBadge(status: string) {
  const cfg: Record<string, { bg: string; text: string; label: string }> = {
    active: { bg: 'bg-secondary/20', text: 'text-secondary', label: 'running' },
    idle: { bg: 'bg-tertiary/20', text: 'text-tertiary', label: 'idle' },
    stopped: { bg: 'bg-on-surface-variant/20', text: 'text-on-surface-variant', label: 'stopped' },
    error: { bg: 'bg-error/20', text: 'text-error', label: 'error' },
  }
  const c = cfg[status] || cfg.stopped
  return (
    <span className={`${c.bg} ${c.text} px-2.5 py-0.5 rounded-full text-[12px] font-medium`}>
      {c.label}
    </span>
  )
}

function formatTime(ts: string) {
  if (!ts) return null
  return new Date(ts).toLocaleString('zh-CN', { hour12: false })
}

interface PendingPermission {
  requestID: string
  toolName: string
  toolInput: Record<string, unknown>
}

export default function SessionChat() {
  const { id } = useParams<{ id: string }>()
  const [session, setSession] = useState<Session | null>(null)
  const [messages, setMessages] = useState<HistoryMessage[]>([])
  const [pendingPerms, setPendingPerms] = useState<PendingPermission[]>([])
  const [permAnswers, setPermAnswers] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [inputValue, setInputValue] = useState('')
  const [sending, setSending] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const nearBottomRef = useRef(true)
  const forceScrollRef = useRef(false)
  const msgCountRef = useRef(0)
  const pollingRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const [showScrollBtn, setShowScrollBtn] = useState(false)

  // Track scroll position
  const handleScroll = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const threshold = 80
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < threshold
    nearBottomRef.current = near
    setShowScrollBtn(!near)
  }, [])

  // Auto-scroll when near bottom or forced (user sent a message)
  const scrollToBottom = useCallback(() => {
    if (!scrollRef.current) return
    if (!nearBottomRef.current && !forceScrollRef.current) return
    requestAnimationFrame(() => {
      if (!scrollRef.current) return
      if (nearBottomRef.current || forceScrollRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight
        forceScrollRef.current = false
        setShowScrollBtn(false)
      }
    })
  }, [])

  const scrollToBottomManual = useCallback(() => {
    if (!scrollRef.current) return
    nearBottomRef.current = true
    forceScrollRef.current = true
    scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    setShowScrollBtn(false)
  }, [])

  // Fetch session info + history
  useEffect(() => {
    if (!id) return

    Promise.all([
      api.getSession(id).catch(() => null),
      api.getSessionHistory(id).then(d => d || []).catch(() => []),
      api.pendingPermissions(id).catch(() => []),
    ]).then(([s, data, perms]) => {
      setSession(s)
      setMessages(data)
      const mapped = (perms as Array<{request_id?: string; tool_name?: string; tool_input?: Record<string, unknown>}> || []).map(p => ({
        requestID: p.request_id || '',
        toolName: p.tool_name || '',
        toolInput: p.tool_input || {},
      }))
      setPendingPerms(mapped)
      msgCountRef.current = data.length
      setLoading(false)
    })

    return () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current)
        pollingRef.current = null
      }
    }
  }, [id])

  // WebSocket for real-time permission events (with auto reconnect)
  useEffect(() => {
    if (!id) return
    let ws: WebSocket | null = null
    let timer: ReturnType<typeof setTimeout> | null = null

    function connect() {
      const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
      ws = new WebSocket(`${proto}//${location.host}/ws/events`)
      ws.onmessage = (e) => {
        try {
          const evt = JSON.parse(e.data)
          if (evt.event === 'permission_request' && evt.session_id === id) {
            setPendingPerms(prev => [...prev, {
              requestID: evt.request_id || '',
              toolName: evt.tool || '',
              toolInput: evt.data || {},
            }])
            scrollToBottom()
          }
          if (evt.event === 'permission_cancel' && evt.session_id === id) {
            setPendingPerms(prev => prev.filter(p => p.requestID !== evt.data))
          }
        } catch { /* ignore */ }
      }
      ws.onclose = () => {
        timer = setTimeout(connect, 3000)
      }
    }
    connect()
    return () => {
      if (timer) clearTimeout(timer)
      if (ws) ws.close()
    }
  }, [id, scrollToBottom])

  // Polling: start after initial load when session is active
  useEffect(() => {
    if (!id || !session) return

    if (session.status !== 'active') return

    // Refresh session status periodically
    const interval = setInterval(() => {
      api.getSessionHistory(id).then(data => {
        const arr = data || []
        if (arr.length !== msgCountRef.current) {
          msgCountRef.current = arr.length
          forceScrollRef.current = true
          setMessages(arr)
        }
      }).catch(() => {})

      // Poll pending permissions (handles both initial load and WS miss)
      api.pendingPermissions(id).then(perms => {
        const mapped = (perms as Array<{request_id?: string; tool_name?: string; tool_input?: Record<string, unknown>}> || []).map(p => ({
          requestID: p.request_id || '',
          toolName: p.tool_name || '',
          toolInput: p.tool_input || {},
        }))
        setPendingPerms(prev => {
          // Merge: keep existing ones not yet resolved + add any new ones from server
          const merged = [...prev]
          for (const m of mapped) {
            if (!merged.find(p => p.requestID === m.requestID)) {
              merged.push(m)
            }
          }
          return merged
        })
      }).catch(() => {})

      // Refresh session status (only trigger re-render on actual changes)
      api.getSession(id).then(s => {
        setSession(prev => {
          if (!prev || prev.status !== s.status) return s
          return prev
        })
      }).catch(() => {})
    }, 3000)

    pollingRef.current = interval
    return () => {
      clearInterval(interval)
      pollingRef.current = null
    }
  }, [id, session?.status])

  // Auto-scroll on message changes
  useEffect(() => {
    scrollToBottom()
  }, [messages, scrollToBottom])

  // Auto-scroll on initial load
  useEffect(() => {
    if (!loading) {
      nearBottomRef.current = true
      scrollToBottom()
    }
  }, [loading, scrollToBottom])

  const handleSend = async () => {
    const content = inputValue.trim()
    if (!content || !id || sending) return

    setSending(true)
    setInputValue('')
    if (textareaRef.current) {
      textareaRef.current.style.height = '24px'
    }

    try {
      await api.sendMessage(id, content)
      const data = await api.getSessionHistory(id)
      const arr = data || []
      msgCountRef.current = arr.length
      nearBottomRef.current = true
      forceScrollRef.current = true
      setMessages(arr)
    } catch (err) {
      console.error('Failed to send message:', err)
    } finally {
      setSending(false)
    }
  }

  const handlePermission = async (requestID: string, allow: boolean) => {
    if (!id) return
    setPendingPerms(prev => prev.filter(p => p.requestID !== requestID))
    try {
      await api.respondPermission(id, requestID, allow)
    } catch {
      // revert on error
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const handleTextareaInput = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    setInputValue(e.target.value)
    const el = e.target
    el.style.height = '24px'
    el.style.height = Math.min(el.scrollHeight, 120) + 'px'
  }

  const chatMessages = mergeToolResults(messages.filter(m =>
    m.role === 'user' || m.role === 'assistant' || m.role === 'system' || m.role === 'tool_result'
  ))

  function renderMessage(msg: HistoryMessage, i: number) {
    // System messages
    if (msg.role === 'system') {
      return (
        <div key={i} className="flex justify-center mb-3">
          <div className="bg-surface-container border border-outline-variant rounded-full px-4 py-2 text-[13px] text-on-surface-variant mx-auto flex items-center gap-2">
            <span className="material-symbols-outlined text-[14px]">info</span>
            {msg.subtype && <span className="opacity-70">[{msg.subtype}] </span>}
            {msg.content && <span>{msg.content}</span>}
            {msg.attachment && (
              <span className="opacity-70 flex items-center gap-1">
                <span className="material-symbols-outlined text-[14px]">attach_file</span>
                {msg.attachment}
              </span>
            )}
            {msg.timestamp && (
              <span className="opacity-50 text-[11px] ml-2">{formatTime(msg.timestamp)}</span>
            )}
          </div>
        </div>
      )
    }

    const isUser = msg.role === 'user'

    // Permission request cards (assistant messages with tool_use that look like permission requests)
    if (
      msg.role === 'assistant' &&
      msg.tool_use &&
      (msg.tool_use.name === 'permission_request' ||
        msg.tool_use.name === 'ask_permission' ||
        msg.content?.includes('permission'))
    ) {
      return (
        <div key={i} className="mb-3">
          <div className="border border-tertiary/50 rounded-lg p-4 bg-surface-container shadow-[0_0_15px_rgba(255,185,95,0.1)]">
            <div className="flex items-center gap-2 mb-2">
              <span className="material-symbols-outlined text-tertiary text-[18px]">warning</span>
              <span className="text-tertiary font-medium text-[14px]">
                权限请求: {msg.tool_use.name}
              </span>
            </div>
            {msg.content && (
              <p className="text-on-surface-variant text-[13px] mb-3 whitespace-pre-wrap">{msg.content}</p>
            )}
            {msg.tool_use.input && (
              <div className="bg-surface-container-lowest rounded p-3 font-mono text-[13px] overflow-auto max-h-[200px] text-on-surface-variant mb-3">
                <pre className="whitespace-pre-wrap">{JSON.stringify(msg.tool_use.input, null, 2)}</pre>
              </div>
            )}
            <div className="flex gap-2 justify-end">
              <button className="px-4 py-1.5 rounded-lg bg-error/20 text-error text-[13px] hover:bg-error/30 transition">
                拒绝
              </button>
              <button className="px-4 py-1.5 rounded-lg bg-secondary/20 text-secondary text-[13px] hover:bg-secondary/30 transition">
                批准
              </button>
            </div>
          </div>
          {msg.timestamp && (
            <div className="text-[11px] text-on-surface-variant/50 mt-1 text-center">{formatTime(msg.timestamp)}</div>
          )}
        </div>
      )
    }

    // User messages
    if (isUser) {
      return (
        <div key={i} className="mb-4">
          <div className="bg-surface-variant rounded-xl rounded-tr-none px-4 py-3 max-w-[70%] ml-auto">
            <p className="text-on-surface text-[14px] whitespace-pre-wrap break-words">{msg.content}</p>
          </div>
          {msg.timestamp && (
            <div className="text-[11px] text-on-surface-variant/50 mt-1 text-right">{formatTime(msg.timestamp)}</div>
          )}
        </div>
      )
    }

    // Assistant messages
    return (
      <div key={i} className="flex gap-3 mb-4">
        {/* Avatar */}
        <div className="w-8 h-8 rounded-full bg-secondary/20 flex items-center justify-center flex-shrink-0">
          <span className="material-symbols-outlined text-secondary text-[18px]">smart_toy</span>
        </div>

        <div className="min-w-0 max-w-[75%]">
          {/* Thinking block */}
          {msg.thinking && (
            <details className="mb-3">
              <summary className="cursor-pointer flex items-center gap-1.5 text-secondary text-[13px] hover:opacity-80 select-none">
                <span className="material-symbols-outlined text-[16px]">psychology</span>
                思考过程
              </summary>
              <div className="bg-surface-dim rounded p-3 font-mono text-[13px] text-on-surface-variant whitespace-pre-wrap mt-2">
                {msg.thinking}
              </div>
            </details>
          )}

          {/* Tool call card */}
          {msg.tool_use && (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-4 mb-3">
              <div className="flex items-center gap-2 mb-2">
                <span className="material-symbols-outlined text-on-surface-variant text-[16px]">terminal</span>
                <span className="text-on-surface-variant text-[13px] font-medium">
                  工具调用: {msg.tool_use.name}
                </span>
              </div>
              <div className="bg-surface-container-lowest rounded p-3 font-mono text-[13px] overflow-auto max-h-[200px]">
                <pre className="text-on-surface-variant whitespace-pre-wrap">{JSON.stringify(msg.tool_use.input, null, 2)}</pre>
              </div>
            </div>
          )}

          {/* Tool result card */}
          {msg.tool_result && (
            <div className="bg-surface-container border border-secondary/30 rounded-lg p-4 mb-3">
              <div className="flex items-center gap-2 mb-2">
                <span className="material-symbols-outlined text-secondary text-[16px]">check_circle</span>
                <span className="text-secondary text-[13px] font-medium">Tool Result</span>
              </div>
              <div className="bg-surface-container-lowest rounded p-3 font-mono text-[13px] overflow-auto max-h-[200px]">
                <pre className="text-on-surface-variant whitespace-pre-wrap break-words">{msg.tool_result}</pre>
              </div>
            </div>
          )}

          {/* Content text */}
          {msg.content && (
            <div className="bg-surface-container rounded-xl rounded-tl-none px-4 py-3">
              <p className="text-on-surface text-[14px] whitespace-pre-wrap break-words">{msg.content}</p>
            </div>
          )}

          {/* Timestamp */}
          {msg.timestamp && (
            <div className="text-[11px] text-on-surface-variant/50 mt-1">{formatTime(msg.timestamp)}</div>
          )}
        </div>
      </div>
    )
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-[60vh]">
        <div className="flex flex-col items-center gap-3">
          <span className="material-symbols-outlined text-secondary text-[32px] animate-spin">progress_activity</span>
          <span className="text-on-surface-variant text-[14px]">加载中...</span>
        </div>
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full">
      {/* Session header */}
      <div className="bg-surface-container-high px-6 py-3 border-b border-outline-variant flex-shrink-0">
        <div className="flex items-center gap-3">
          <h1 className="text-[20px] font-semibold text-primary truncate">
            {session?.name || `会话 ${id?.slice(0, 8)}`}
          </h1>
          {session && statusBadge(session.status)}
        </div>
        <div className="flex items-center gap-3 mt-1.5">
          {session?.work_dir && (
            <span className="font-mono text-[12px] text-on-surface-variant bg-surface-container-lowest rounded px-2 py-1 truncate max-w-[50%]">
              {session.work_dir}
            </span>
          )}
          {session?.git_branch && (
            <span className="flex items-center gap-1 text-[12px] text-secondary bg-secondary/10 rounded-full px-2.5 py-1">
              <span className="material-symbols-outlined text-[14px]">fork_right</span>
              {session.git_branch}
            </span>
          )}
        </div>
      </div>

      {/* Chat area */}
      <div className="flex-1 relative bg-surface-container-lowest overflow-hidden">
        <div
          ref={scrollRef}
          onScroll={handleScroll}
          className="h-full overflow-y-auto p-6"
        >
        {chatMessages.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-full text-on-surface-variant/50 gap-2">
            <span className="material-symbols-outlined text-[48px]">chat_bubble_outline</span>
            <span className="text-[14px]">暂无聊天记录</span>
          </div>
        ) : (
          chatMessages.map((msg, i) => renderMessage(msg, i))
        )}
        {/* Pending permission cards */}
        {pendingPerms.length > 0 && (
          <div className="mt-4 space-y-4">
            {pendingPerms.map(perm => (
              <div key={perm.requestID} className="border border-tertiary/50 rounded-lg p-4 bg-surface-container shadow-[0_0_15px_rgba(255,185,95,0.1)]">
                <div className="flex items-center gap-2 mb-3">
                  <span className="material-symbols-outlined text-tertiary text-[18px]">warning</span>
                  <span className="text-tertiary font-medium text-[14px]">
                    权限请求: {perm.toolName}
                  </span>
                </div>
                {/* Questions (AskUserQuestion type) */}
                {Array.isArray(perm.toolInput?.questions) && (perm.toolInput.questions as Array<{ question?: string; header?: string; options?: Array<{ label: string; description: string }> }>).map((q, qi) => (
                  <div key={qi} className="mb-3">
                    <p className="text-on-surface text-[13px] mb-2">{q.question || q.header}</p>
                    {q.options && q.options.length > 0 ? (
                      <div className="grid grid-cols-1 gap-2">
                        {q.options.map((opt, oi) => (
                          <button
                            key={oi}
                            onClick={async () => {
                              setPendingPerms(prev => prev.filter(p => p.requestID !== perm.requestID))
                              try {
                                await api.respondPermission(id!, perm.requestID, true, opt.label)
                              } catch {}
                            }}
                            className="w-full text-left p-3 rounded-lg bg-surface-container-low border border-outline-variant hover:border-secondary/50 hover:bg-surface-container transition-colors"
                          >
                            <div className="text-[13px] font-medium text-primary">{opt.label}</div>
                            {opt.description && (
                              <div className="text-[12px] text-on-surface-variant mt-0.5">{opt.description}</div>
                            )}
                          </button>
                        ))}
                      </div>
                    ) : (
                      <div className="flex gap-2">
                        <input
                          type="text"
                          className="flex-1 bg-surface-container-lowest border border-outline-variant rounded-lg px-3 py-2 text-[13px] text-on-surface outline-none placeholder:text-on-surface-variant/50"
                          placeholder="输入回答..."
                          value={permAnswers[perm.requestID] || ''}
                          onChange={e => setPermAnswers(prev => ({ ...prev, [perm.requestID]: e.target.value }))}
                          onKeyDown={async e => {
                            if (e.key === 'Enter' && !e.shiftKey) {
                              e.preventDefault()
                              const answer = permAnswers[perm.requestID]?.trim()
                              if (!answer) return
                              setPendingPerms(prev => prev.filter(p => p.requestID !== perm.requestID))
                              try {
                                await api.respondPermission(id!, perm.requestID, true, answer)
                              } catch {}
                            }
                          }}
                        />
                        <button
                          onClick={async () => {
                            const answer = permAnswers[perm.requestID]?.trim()
                            if (!answer) return
                            setPendingPerms(prev => prev.filter(p => p.requestID !== perm.requestID))
                            try {
                              await api.respondPermission(id!, perm.requestID, true, answer)
                            } catch {}
                          }}
                          className="px-3 py-2 rounded-lg bg-secondary/20 text-secondary text-[13px] hover:bg-secondary/30 transition"
                        >
                          发送
                        </button>
                      </div>
                    )}
                  </div>
                ))}
                {/* Regular tool permission */}
                {(!perm.toolInput?.questions) && (
                  <>
                    {perm.toolInput && Object.keys(perm.toolInput).length > 0 && (
                      <div className="bg-surface-container-lowest rounded p-3 font-mono text-[13px] overflow-auto max-h-[200px] text-on-surface-variant mb-3">
                        <pre className="whitespace-pre-wrap">{JSON.stringify(perm.toolInput, null, 2)}</pre>
                      </div>
                    )}
                    <div className="flex gap-2 justify-end">
                      <button
                        onClick={() => handlePermission(perm.requestID, false)}
                        className="px-4 py-1.5 rounded-lg bg-error/20 text-error text-[13px] hover:bg-error/30 transition"
                      >
                        拒绝
                      </button>
                      <button
                        onClick={() => handlePermission(perm.requestID, true)}
                        className="px-4 py-1.5 rounded-lg bg-secondary/20 text-secondary text-[13px] hover:bg-secondary/30 transition"
                      >
                        批准
                      </button>
                    </div>
                  </>
                )}
              </div>
            ))}
          </div>
        )}
        </div>

        {/* Scroll-to-bottom floating button */}
        {showScrollBtn && (
          <button
            onClick={scrollToBottomManual}
            className="absolute bottom-4 right-6 w-10 h-10 rounded-full bg-secondary/90 hover:bg-secondary text-on-secondary shadow-lg flex items-center justify-center transition-all z-10"
          >
            <span className="material-symbols-outlined text-[20px]">keyboard_arrow_down</span>
          </button>
        )}
      </div>

      {/* Chat input */}
      {session?.status === 'active' ? (
        <div className="bg-surface-container/90 backdrop-blur-md border-t border-outline-variant px-6 py-4 flex-shrink-0">
          <div className="bg-surface-container-lowest border border-outline-variant rounded-xl flex items-end gap-3 px-4 py-3">
            <textarea
              ref={textareaRef}
              value={inputValue}
              onChange={handleTextareaInput}
              onKeyDown={handleKeyDown}
              placeholder="输入消息... (Enter 发送, Shift+Enter 换行)"
              rows={1}
              className="flex-1 bg-transparent text-on-surface resize-none outline-none text-[14px] min-h-[24px] max-h-[120px] placeholder:text-on-surface-variant/50"
            />
            <button
              onClick={handleSend}
              disabled={!inputValue.trim() || sending}
              className="w-8 h-8 rounded-full bg-secondary/20 flex items-center justify-center text-secondary hover:bg-secondary/30 transition disabled:opacity-30 disabled:cursor-not-allowed flex-shrink-0"
            >
              <span className="material-symbols-outlined text-[18px]">
                {sending ? 'progress_activity' : 'send'}
              </span>
            </button>
          </div>
        </div>
      ) : (
        <div className="bg-surface-container/90 backdrop-blur-md border-t border-outline-variant px-6 py-4 flex-shrink-0">
          <div className="bg-surface-container-lowest border border-outline-variant rounded-xl flex items-center justify-center gap-2 px-4 py-3 opacity-50">
            <span className="material-symbols-outlined text-on-surface-variant text-[18px]">lock</span>
            <span className="text-on-surface-variant text-[13px]">会话已结束，无法发送消息</span>
          </div>
        </div>
      )}
    </div>
  )
}
