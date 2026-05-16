import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../api'

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
  model: string
}

interface LogEntry {
  type: string
  role?: string
  content: string
  tool?: string
  input?: string
  detail?: string
  error?: string
  timestamp?: string
}

const statusDotColor: Record<string, string> = {
  active: 'bg-secondary',
  idle: 'bg-tertiary',
  computing: 'bg-tertiary animate-pulse',
  stopped: 'bg-outline-variant',
  error: 'bg-error',
}

const statusBadgeColor: Record<string, string> = {
  active: 'text-secondary bg-secondary/10 border-secondary/20',
  idle: 'text-tertiary bg-tertiary/10 border-tertiary/20',
  computing: 'text-tertiary bg-tertiary/10 border-tertiary/20',
  stopped: 'text-on-surface-variant bg-surface-variant border-outline-variant',
  error: 'text-error bg-error/10 border-error/20',
}

const statusLabel: Record<string, string> = {
  active: '运行',
  idle: '停止',
  computing: '计算中',
  stopped: '停止',
  error: '错误',
}

const logTypeBadge: Record<string, { color: string; label: string }> = {
  user: { color: 'text-on-surface bg-on-surface/10', label: '输入' },
  assistant: { color: 'text-secondary bg-secondary/10', label: '输出' },
  tool_call: { color: 'text-tertiary bg-tertiary/10', label: '工具' },
  thinking: { color: 'text-on-surface-variant bg-surface-variant', label: '思考' },
  permission: { color: 'text-tertiary bg-tertiary/10', label: '权限' },
  result: { color: 'text-secondary bg-secondary/10', label: '完成' },
  error: { color: 'text-error bg-error/10', label: '错误' },
}

export default function Dashboard() {
  const navigate = useNavigate()
  const [sessions, setSessions] = useState<Session[]>([])
  const [activeSessions, setActiveSessions] = useState<Session[]>([])
  const [logEntries, setLogEntries] = useState<LogEntry[]>([])
  const [wechatStatus, setWechatStatus] = useState<{ connected: boolean }>({ connected: false })
  const [syncing, setSyncing] = useState(false)

  const fetchAll = async () => {
    try {
      const [sessData, _activeData, logData, wechatData] = await Promise.allSettled([
        api.getSessions(),
        api.getActiveSession(),
        api.getActiveSessionLog(),
        api.getWechatStatus(),
      ])

      if (sessData.status === 'fulfilled') {
        const all = (sessData.value || []) as Session[]
        setSessions(all)
        setActiveSessions(all.filter(s => s.status === 'active'))
      }

      if (logData.status === 'fulfilled') {
        setLogEntries((logData.value || []) as LogEntry[])
      }

      if (wechatData.status === 'fulfilled') {
        setWechatStatus(wechatData.value as { connected: boolean })
      }
    } catch {
      // silently fail
    }
  }

  useEffect(() => {
    fetchAll()
    const interval = setInterval(fetchAll, 10000)
    return () => clearInterval(interval)
  }, [])

  // Derived stats
  const totalMessages = sessions.reduce((sum, s) => sum + (s.message_count || 0), 0)
  const idleCount = activeSessions.filter(s => s.status === 'idle').length
  const computingCount = activeSessions.filter(s => s.status === 'computing').length
  const activeSubText = idleCount > 0 || computingCount > 0
    ? `${idleCount} 空闲, ${computingCount} 计算中`
    : `${activeSessions.length} 个活跃`

  const statCards = [
    {
      icon: 'terminal',
      value: activeSessions.length,
      label: '活跃会话',
      sub: activeSubText,
    },
    {
      icon: 'forum',
      value: totalMessages,
      label: '消息总量',
      sub: `共 ${sessions.length} 个会话`,
    },
    {
      icon: 'chat',
      value: wechatStatus.connected ? '已连接' : '未连接',
      label: '微信状态',
      sub: wechatStatus.connected ? '通信正常' : '等待连接',
    },
    {
      icon: 'schedule',
      value: '运行中',
      label: '系统状态',
      sub: 'cc-go manager',
    },
  ]

  return (
    <main className="flex-1 p-8 max-w-[1440px] mx-auto w-full flex flex-col gap-8">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-[30px] font-bold text-primary">系统概览</h1>
          <p className="text-on-surface-variant text-[14px] mt-1">监控 cc-go 进程及活跃代理会话。</p>
        </div>
        <button
          onClick={async () => {
            setSyncing(true)
            try { await api.syncSessions() } catch {}
            await fetchAll()
            setSyncing(false)
          }}
          disabled={syncing}
          className="flex items-center gap-1.5 px-4 py-2 text-[13px] font-medium text-secondary border border-secondary/20 rounded hover:bg-secondary/10 transition disabled:opacity-50"
        >
          <span className="material-symbols-outlined text-[16px]">sync</span>
          {syncing ? '同步中...' : '同步会话'}
        </button>
      </div>

      {/* Stats Grid */}
      <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
        {statCards.map((card, i) => (
          <div
            key={i}
            className="bg-surface-container border border-outline-variant rounded-lg p-4 flex flex-col gap-2 relative overflow-hidden group hover:border-secondary transition-colors"
          >
            {/* Decorative blur circle */}
            <div className="absolute -right-4 -top-4 w-16 h-16 bg-primary/5 rounded-full blur-xl group-hover:bg-secondary/10 transition-colors" />
            {/* Icon */}
            <div className="flex items-center justify-between">
              <span className="font-mono text-[11px] font-medium text-on-surface-variant">{card.label}</span>
              <span className="material-symbols-outlined text-secondary text-[20px]">{card.icon}</span>
            </div>
            {/* Value */}
            <div className="text-[28px] font-bold text-primary">{card.value}</div>
            {/* Sub text */}
            <div className="font-mono text-[11px] font-medium text-on-surface-variant">{card.sub}</div>
          </div>
        ))}
      </div>

      {/* Two-column section */}
      <div className="grid grid-cols-12 gap-6">
        {/* Left: System log */}
        <div className="col-span-12 lg:col-span-8 flex flex-col gap-4">
          <h2 className="text-[18px] font-semibold text-primary">系统日志</h2>
          <div className="bg-surface-container border border-outline-variant rounded-lg overflow-hidden flex flex-col">
            {/* Terminal header */}
            <div className="flex items-center gap-2 px-4 py-2.5 bg-surface-container-high border-b border-outline-variant">
              <div className="w-2.5 h-2.5 rounded-full bg-error/70" />
              <div className="w-2.5 h-2.5 rounded-full bg-tertiary/70" />
              <div className="w-2.5 h-2.5 rounded-full bg-secondary/70" />
              <span className="ml-2 font-mono text-[11px] text-on-surface-variant">syslog</span>
            </div>

            {/* Scrollable log entries */}
            <div className="max-h-[300px] overflow-y-auto terminal-scroll p-3 flex flex-col gap-1.5 font-mono text-[12px]">
              {logEntries.length === 0 ? (
                <div className="flex items-center justify-center py-8 text-on-surface-variant">
                  <span className="text-[12px]">暂无日志</span>
                </div>
              ) : (
                logEntries.slice(-50).map((entry, i) => {
                  const badge = logTypeBadge[entry.type] || { color: 'text-on-surface-variant bg-surface-variant', label: entry.type }
                  return (
                    <div key={i} className="flex gap-2 items-start">
                      <span className="text-on-surface-variant/50 shrink-0 text-[10px] leading-[18px]">
                        {entry.timestamp ? new Date(entry.timestamp).toLocaleTimeString('zh-CN', { hour12: false }) : ''}
                      </span>
                      <span className={`shrink-0 text-[10px] px-1 py-0 rounded leading-[18px] ${badge.color}`}>
                        {badge.label}
                      </span>
                      <span className="text-on-surface-variant leading-[18px] break-all">
                        {entry.type === 'tool_call' && entry.tool && (
                          <>
                            <span className="text-secondary">{entry.tool}</span>
                            {entry.input && <span className="text-on-surface-variant/50"> {entry.input}</span>}
                          </>
                        )}
                        {entry.type === 'error' && entry.error && (
                          <span className="text-error">{entry.error}</span>
                        )}
                        {entry.type === 'result' && entry.detail && (
                          <span className="text-secondary">{entry.detail}</span>
                        )}
                        {entry.type === 'permission' && (
                          <>
                            {entry.detail && <span className="text-tertiary">{entry.detail}</span>}
                            {entry.tool && <span className="text-secondary"> {entry.tool}</span>}
                          </>
                        )}
                        {(entry.type === 'user' || entry.type === 'assistant' || entry.type === 'thinking') && entry.content && (
                          <span className="line-clamp-2">{entry.content}</span>
                        )}
                        {!['tool_call', 'error', 'result', 'permission', 'user', 'assistant', 'thinking'].includes(entry.type) && (
                          <span>{entry.content}</span>
                        )}
                      </span>
                    </div>
                  )
                })
              )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between px-4 py-2 border-t border-outline-variant bg-surface-container-high">
              <div className="flex items-center gap-2">
                <span className="material-symbols-outlined text-secondary text-[14px] animate-pulse">circle</span>
                <span className="font-mono text-[11px] text-on-surface-variant">正在追踪日志...</span>
              </div>
              <button
                onClick={() => navigate('/log')}
                className="flex items-center gap-1 text-[12px] text-secondary hover:text-secondary/80 transition-colors"
              >
                <span className="material-symbols-outlined text-[14px]">open_in_new</span>
                查看完整日志
              </button>
            </div>
          </div>
        </div>

        {/* Right: Active Sessions */}
        <div className="col-span-12 lg:col-span-4 flex flex-col gap-4">
          <h2 className="text-[18px] font-semibold text-primary">活跃会话</h2>
          {activeSessions.length === 0 ? (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-8 flex flex-col items-center justify-center gap-3">
              <span className="material-symbols-outlined text-on-surface-variant text-[40px]">terminal</span>
              <p className="text-on-surface-variant text-[14px]">暂无活跃会话</p>
            </div>
          ) : (
            <div className="grid grid-cols-1 gap-4">
              {activeSessions.map(session => (
                <div
                  key={session.id}
                  className="bg-surface-container border border-outline-variant rounded-lg p-4 flex flex-col gap-3 hover:border-secondary/50 transition-colors"
                >
                  {/* Header: name + status */}
                  <div className="flex items-center gap-2">
                    <div className={`w-2 h-2 rounded-full ${statusDotColor[session.status] || 'bg-outline-variant'}`} />
                    <span className="text-[13px] text-primary truncate">{session.name || '(无标题)'}</span>
                    <span className={`ml-auto text-[10px] font-mono px-1.5 py-0.5 rounded border ${statusBadgeColor[session.status] || 'text-on-surface-variant bg-surface-variant border-outline-variant'}`}>
                      {statusLabel[session.status] || session.status}
                    </span>
                  </div>

                  {/* Work dir */}
                  <div className="font-mono text-[12px] bg-surface-container-lowest rounded px-2 py-1 text-on-surface-variant truncate">
                    {session.work_dir}
                  </div>

					{/* Footer: model + git branch + message count */}
					<div className="flex items-center gap-2 text-[11px] flex-wrap">
						{session.model && (
							<span className="flex items-center gap-1 font-mono text-on-surface-variant bg-surface-container-lowest rounded px-1.5 py-0.5">
								<span className="material-symbols-outlined text-[12px] text-tertiary">smart_toy</span>
								{session.model}
							</span>
						)}
						{session.git_branch && (
							<span className="flex items-center gap-1 font-mono text-on-surface-variant bg-surface-container-lowest rounded px-1.5 py-0.5">
								<span className="material-symbols-outlined text-[12px] text-secondary">fork_right</span>
								{session.git_branch}
							</span>
						)}
						<span className="flex items-center gap-1 font-mono text-on-surface-variant">
							<span className="material-symbols-outlined text-[12px]">chat_bubble</span>
							{session.message_count} 条消息
						</span>
					</div>

                  {/* Action button */}
                  <button
                    onClick={() => navigate(`/sessions/${session.id}`)}
                    className="mt-auto flex items-center justify-center gap-1.5 py-1.5 px-3 text-[12px] font-medium text-secondary border border-secondary/20 rounded hover:bg-secondary/10 transition-colors"
                  >
                    <span className="material-symbols-outlined text-[14px]">visibility</span>
                    查看详情
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </main>
  )
}
