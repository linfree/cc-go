import { useState, useEffect, useRef, useCallback } from 'react'
import { api } from '../api'
import { cn } from '../lib/cn'

interface LogEntry {
  type: string
  role?: string
  content: string
  tool?: string
  input?: string
  detail?: string
  error?: string
}

const levels = ['INFO', 'WARN', 'ERROR', 'DEBUG'] as const

const levelColors: Record<string, { bg: string; border: string; text: string; dot: string }> = {
  INFO: { bg: 'bg-secondary/10', border: 'border-secondary/30', text: 'text-secondary', dot: 'bg-secondary' },
  WARN: { bg: 'bg-tertiary/10', border: 'border-tertiary/30', text: 'text-tertiary', dot: 'bg-tertiary' },
  ERROR: { bg: 'bg-error/10', border: 'border-error/30', text: 'text-error', dot: 'bg-error' },
  DEBUG: { bg: 'bg-on-surface-variant/10', border: 'border-on-surface-variant/30', text: 'text-on-surface-variant', dot: 'bg-on-surface-variant' },
}

function getLevel(type: string): string {
  if (type === 'error') return 'ERROR'
  if (type === 'permission' || type === 'tool_call') return 'WARN'
  if (type === 'thinking') return 'DEBUG'
  return 'INFO'
}

const typeLabels: Record<string, string> = {
  user: '输入',
  assistant: '输出',
  tool_call: '工具',
  thinking: '思考',
  permission: '权限',
  result: '完成',
  error: '错误',
}

export default function LogViewer() {
  const [entries, setEntries] = useState<LogEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [activeLevels, setActiveLevels] = useState<Set<string>>(new Set(levels))
  const [searchQuery, setSearchQuery] = useState('')
  const [paused, setPaused] = useState(false)
  const [showScrollBtn, setShowScrollBtn] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)
  const entriesRef = useRef<LogEntry[]>([])

  const fetchLog = useCallback(async () => {
    if (paused) return
    try {
      const data = await api.getActiveSessionLog()
      const arr = data || []
      if (arr.length !== entriesRef.current.length) {
        entriesRef.current = arr
        setEntries(arr)
      }
    } catch {
      // silently fail
    } finally {
      setLoading(false)
    }
  }, [paused])

  useEffect(() => {
    fetchLog()
    const interval = setInterval(fetchLog, 2000)
    return () => clearInterval(interval)
  }, [fetchLog])

  useEffect(() => {
    if (!loading && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [loading])

  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const handleScroll = () => {
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
      setShowScrollBtn(!atBottom)
    }
    el.addEventListener('scroll', handleScroll)
    return () => el.removeEventListener('scroll', handleScroll)
  }, [])

  const scrollToBottom = () => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }

  const toggleLevel = (level: string) => {
    setActiveLevels(prev => {
      const next = new Set(prev)
      if (next.has(level)) next.delete(level)
      else next.add(level)
      return next
    })
  }

  const filtered = entries.filter(e => {
    const level = getLevel(e.type)
    if (!activeLevels.has(level)) return false
    if (searchQuery) {
      const q = searchQuery.toLowerCase()
      return (
        e.content?.toLowerCase().includes(q) ||
        e.tool?.toLowerCase().includes(q) ||
        e.detail?.toLowerCase().includes(q) ||
        e.error?.toLowerCase().includes(q)
      )
    }
    return true
  })

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="px-8 pt-8 pb-4">
        <h2 className="text-[30px] font-bold text-primary mb-1">系统日志</h2>
        <p className="text-[14px] text-on-surface-variant">实时追踪活跃会话的 Claude Code 事件流。</p>
      </div>

      {/* Toolbar */}
      <div className="mx-8 mb-4 bg-surface-container rounded-lg p-3 flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="relative">
            <span className="material-symbols-outlined absolute left-3 top-1/2 -translate-y-1/2 text-on-surface-variant text-[18px]">
              search
            </span>
            <input
              type="text"
              value={searchQuery}
              onChange={e => setSearchQuery(e.target.value)}
              placeholder="搜索日志..."
              className="bg-surface-container-lowest border border-outline-variant rounded pl-9 pr-3 py-2 font-mono text-[13px] text-on-surface placeholder:text-on-surface-variant/50 w-64 focus:outline-none focus:border-primary"
            />
          </div>
          <div className="flex items-center gap-1">
            {levels.map(level => {
              const c = levelColors[level]
              const active = activeLevels.has(level)
              return (
                <button
                  key={level}
                  onClick={() => toggleLevel(level)}
                  className={cn(
                    'flex items-center gap-1.5 px-2.5 py-1 rounded border text-[12px] font-mono font-medium transition-colors',
                    active ? `${c.bg} ${c.border} ${c.text}` : 'bg-transparent border-transparent text-on-surface-variant/50'
                  )}
                >
                  <span className={cn('w-1.5 h-1.5 rounded-full', active ? c.dot : 'bg-on-surface-variant/30')} />
                  {level}
                </button>
              )
            })}
          </div>
        </div>
        <div className="flex items-center gap-2">
          <button
            onClick={() => setPaused(!paused)}
            className={cn(
              'flex items-center gap-1.5 px-3 py-1.5 rounded border text-[13px] transition-colors',
              paused
                ? 'bg-tertiary/10 border-tertiary/30 text-tertiary'
                : 'bg-surface-container-lowest border-outline-variant text-on-surface-variant hover:text-primary'
            )}
          >
            <span className="material-symbols-outlined text-[16px]">{paused ? 'play_arrow' : 'pause'}</span>
            {paused ? '恢复' : '暂停'}
          </button>
          <button
            onClick={() => setEntries([])}
            className="flex items-center gap-1.5 px-3 py-1.5 rounded border border-outline-variant bg-surface-container-lowest text-on-surface-variant hover:text-error text-[13px] transition-colors"
          >
            <span className="material-symbols-outlined text-[16px]">delete</span>
            清空
          </button>
        </div>
      </div>

      {/* Terminal viewer */}
      <div className="flex-1 mx-8 mb-8 bg-surface-container-lowest rounded-lg border border-outline-variant flex flex-col overflow-hidden">
        {/* Terminal header */}
        <div className="flex items-center gap-2 px-4 py-2 bg-surface-container border-b border-outline-variant">
          <span className="w-2.5 h-2.5 rounded-full bg-error/60" />
          <span className="w-2.5 h-2.5 rounded-full bg-tertiary/60" />
          <span className="w-2.5 h-2.5 rounded-full bg-secondary/60" />
          <span className="ml-3 font-mono text-[11px] text-on-surface-variant">tty1 -- syslog</span>
          {paused && (
            <span className="ml-auto font-mono text-[11px] text-tertiary">已暂停</span>
          )}
        </div>

        {/* Log entries */}
        <div
          ref={scrollRef}
          className="flex-1 overflow-y-auto p-4 font-mono text-[13px] leading-relaxed terminal-scroll"
        >
          {loading ? (
            <div className="flex items-center gap-2 text-on-surface-variant">
              <span className="material-symbols-outlined animate-spin text-[16px]">progress_activity</span>
              加载中...
            </div>
          ) : filtered.length === 0 ? (
            <div className="text-on-surface-variant/50 py-8 text-center">
              {entries.length === 0 ? '暂无活跃会话日志' : '没有匹配的日志条目'}
            </div>
          ) : (
            filtered.map((entry, i) => {
              const level = getLevel(entry.type)
              const c = levelColors[level]
              const label = typeLabels[entry.type] || entry.type
              const isError = level === 'ERROR'
              const isWarn = level === 'WARN'

              return (
                <div
                  key={i}
                  className={cn(
                    'py-1.5 border-l-2 pl-3 mb-1',
                    isError && 'border-l-error/50 bg-error/5',
                    isWarn && !isError && 'border-l-tertiary/50 bg-tertiary/5',
                    !isError && !isWarn && 'border-l-transparent'
                  )}
                >
                  <div className="flex items-start gap-2">
                    <span className="font-mono text-[11px] text-on-surface-variant opacity-70 whitespace-nowrap mt-0.5">
                      {new Date().toLocaleTimeString('zh-CN', { hour12: false })}
                    </span>
                    <span className={cn('px-1.5 py-0.5 rounded text-[10px] font-mono font-medium border', c.bg, c.border, c.text)}>
                      {label}
                    </span>
                    <span className="text-on-surface-variant/70 text-[11px]">[{entry.type}]</span>
                  </div>
                  <div className="mt-1 text-on-surface whitespace-pre-wrap break-all">
                    {entry.type === 'tool_call' && (
                      <>
                        <span className="text-secondary">{entry.tool}</span>
                        {entry.input && <span className="text-on-surface-variant"> {entry.input}</span>}
                      </>
                    )}
                    {entry.type === 'permission' && (
                      <>
                        <span className="text-tertiary">{entry.detail}</span>
                        {entry.tool && <span className="text-secondary"> {entry.tool}</span>}
                        {entry.input && <span className="text-on-surface-variant"> {entry.input}</span>}
                      </>
                    )}
                    {entry.type === 'result' && (
                      <span className="text-secondary">{entry.detail || entry.content}</span>
                    )}
                    {entry.type === 'error' && (
                      <span className="text-error">{entry.error || entry.content}</span>
                    )}
                    {entry.type === 'thinking' && (
                      <span className="text-on-surface-variant">{entry.content}</span>
                    )}
                    {(entry.type === 'user' || entry.type === 'assistant') && (
                      <span>{entry.content}</span>
                    )}
                  </div>
                </div>
              )
            })
          )}
          {/* Cursor */}
          <div className="flex items-center gap-1 text-on-surface-variant/30 mt-2">
            <span className="w-2 h-4 bg-secondary/50 animate-pulse" />
          </div>
        </div>
      </div>

      {/* Auto-scroll FAB */}
      {showScrollBtn && (
        <button
          onClick={scrollToBottom}
          className="fixed bottom-6 right-8 flex items-center gap-2 px-4 py-2 rounded-full bg-surface-container border border-outline-variant backdrop-blur-md text-[13px] text-on-surface-variant hover:text-secondary transition-colors shadow-lg"
        >
          <span className="material-symbols-outlined text-[16px]">keyboard_arrow_down</span>
          跟进最新日志
        </button>
      )}
    </div>
  )
}
