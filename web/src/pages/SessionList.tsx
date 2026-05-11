import { useState, useEffect, useMemo, useCallback } from 'react'
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
}

const PAGE_SIZE = 15

type StatusFilter = 'all' | 'active' | 'stopped'

function formatTime(t: string): string {
  if (!t) return '-'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

function StatusBadge({ status }: { status: string }) {
  const s = status.toLowerCase()
  if (s === 'active') {
    return (
      <span className="inline-flex items-center gap-1.5 bg-secondary/10 border border-secondary/30 text-secondary rounded px-2 py-0.5">
        <span className="w-1.5 h-1.5 rounded-full bg-secondary" />
        <span className="font-mono text-[11px]">RUNNING</span>
      </span>
    )
  }
  if (s === 'idle') {
    return (
      <span className="inline-flex items-center gap-1.5 bg-primary/10 border border-primary/30 text-primary rounded px-2 py-0.5">
        <span className="w-1.5 h-1.5 rounded-full bg-primary" />
        <span className="font-mono text-[11px]">IDLE</span>
      </span>
    )
  }
  if (s === 'error') {
    return (
      <span className="inline-flex items-center gap-1.5 bg-error/10 border border-error/30 text-error rounded px-2 py-0.5">
        <span className="w-1.5 h-1.5 rounded-full bg-error" />
        <span className="font-mono text-[11px]">FAILED</span>
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5 bg-on-surface-variant/10 border border-on-surface-variant/30 text-on-surface-variant rounded px-2 py-0.5">
      <span className="w-1.5 h-1.5 rounded-full bg-on-surface-variant" />
      <span className="font-mono text-[11px]">STOPPED</span>
    </span>
  )
}

export default function SessionList() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [loading, setLoading] = useState(false)
  const [search, setSearch] = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [page, setPage] = useState(1)
  const [modalOpen, setModalOpen] = useState(false)
  const [formWorkDir, setFormWorkDir] = useState('')
  const [formName, setFormName] = useState('')
  const [formSubmitting, setFormSubmitting] = useState(false)
  const navigate = useNavigate()

  const fetchSessions = useCallback(async () => {
    setLoading(true)
    try {
      const data = await api.getSessions()
      setSessions(data || [])
    } catch {
      // silently fail
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchSessions()
    const interval = setInterval(fetchSessions, 10000)
    return () => clearInterval(interval)
  }, [fetchSessions])

  const filtered = useMemo(() => {
    let list = sessions
    if (statusFilter === 'active') {
      list = list.filter(s => s.status.toLowerCase() === 'active' || s.status.toLowerCase() === 'idle')
    } else if (statusFilter === 'stopped') {
      list = list.filter(s => s.status.toLowerCase() === 'stopped' || s.status.toLowerCase() === 'error')
    }
    if (search.trim()) {
      const q = search.toLowerCase()
      list = list.filter(
        s =>
          (s.name || '').toLowerCase().includes(q) ||
          s.work_dir.toLowerCase().includes(q) ||
          (s.git_branch || '').toLowerCase().includes(q) ||
          s.id.toLowerCase().includes(q)
      )
    }
    list.sort((a, b) => (b.modified || '').localeCompare(a.modified || ''))
    return list
  }, [sessions, statusFilter, search])

  const totalPages = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE))
  const safePage = Math.min(page, totalPages)
  const paged = filtered.slice((safePage - 1) * PAGE_SIZE, safePage * PAGE_SIZE)

  useEffect(() => {
    setPage(1)
  }, [search, statusFilter])

  const handleStart = async () => {
    if (!formWorkDir.trim()) return
    setFormSubmitting(true)
    try {
      await api.startSession({
        work_dir: formWorkDir.trim(),
        ...(formName.trim() ? { name: formName.trim() } : {}),
      })
      setModalOpen(false)
      setFormWorkDir('')
      setFormName('')
      fetchSessions()
    } catch {
      // silently fail
    } finally {
      setFormSubmitting(false)
    }
  }

  const handleResume = async (id: string) => {
    try {
      await api.resumeSession(id)
      fetchSessions()
    } catch {
      // silently fail
    }
  }

  const handleStop = async (id: string) => {
    try {
      await api.stopSession(id)
      fetchSessions()
    } catch {
      // silently fail
    }
  }

  const handleDelete = async (id: string) => {
    if (!window.confirm('确定要删除这个会话记录吗？')) return
    try {
      await api.deleteSession(id)
      setSelected(prev => {
        const next = new Set(prev)
        next.delete(id)
        return next
      })
      fetchSessions()
    } catch {
      // silently fail
    }
  }

  const handleBatchStop = async () => {
    const ids = Array.from(selected)
    if (!window.confirm(`确定要停止 ${ids.length} 个会话吗？`)) return
    await Promise.allSettled(ids.map(id => api.stopSession(id)))
    setSelected(new Set())
    fetchSessions()
  }

  const handleBatchDelete = async () => {
    const ids = Array.from(selected)
    if (!window.confirm(`确定要删除 ${ids.length} 个会话吗？`)) return
    await Promise.allSettled(ids.map(id => api.deleteSession(id)))
    setSelected(new Set())
    fetchSessions()
  }

  const filterPills: { key: StatusFilter; label: string }[] = [
    { key: 'all', label: '全部' },
    { key: 'active', label: '活跃' },
    { key: 'stopped', label: '已停止' },
  ]

  return (
    <div className="p-8 max-w-[1440px] mx-auto w-full flex flex-col gap-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-[20px] font-semibold text-primary">会话管理</h1>
        <button
          onClick={() => setModalOpen(true)}
          className="bg-surface-variant border border-outline-variant rounded px-4 py-2 text-primary hover:bg-surface-bright transition cursor-pointer"
        >
          <span className="material-symbols-outlined text-[16px] align-middle mr-1">add</span>
          新建会话
        </button>
      </div>

      {/* Filter bar */}
      <div className="bg-surface-container-high rounded-lg p-3 flex items-center gap-3 flex-wrap">
        <input
          type="text"
          placeholder="搜索会话..."
          value={search}
          onChange={e => setSearch(e.target.value)}
          className="bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface placeholder:text-on-surface-variant/50 w-64 focus:outline-none focus:border-primary"
        />
        <div className="flex items-center gap-1">
          {filterPills.map(p => (
            <button
              key={p.key}
              onClick={() => setStatusFilter(p.key)}
              className={`px-3 py-1.5 rounded text-[13px] transition cursor-pointer ${
                statusFilter === p.key
                  ? 'bg-surface-bright text-secondary'
                  : 'text-on-surface-variant hover:bg-surface-variant/30'
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
        {selected.size > 0 && (
          <>
            <span className="font-mono text-[12px] text-on-surface-variant ml-2">
              已选 {selected.size} 项
            </span>
            <div className="flex items-center gap-1 ml-auto">
              <button
                onClick={handleBatchStop}
                className="flex items-center gap-1 px-3 py-1.5 rounded text-[13px] text-on-surface-variant hover:bg-surface-variant/30 transition cursor-pointer"
              >
                <span className="material-symbols-outlined text-[16px]">stop</span>
                停止
              </button>
              <button
                onClick={handleBatchDelete}
                className="flex items-center gap-1 px-3 py-1.5 rounded text-[13px] text-error hover:bg-error/10 transition cursor-pointer"
              >
                <span className="material-symbols-outlined text-[16px]">delete</span>
                删除
              </button>
            </div>
          </>
        )}
      </div>

      {/* Data table */}
      <div className="bg-surface-container border border-outline-variant rounded-lg overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full">
            <thead>
              <tr className="bg-surface-container-high text-[12px] font-mono text-on-surface-variant uppercase tracking-wider">
                <th className="px-4 py-3 text-left">名称</th>
                <th className="px-4 py-3 text-left">工作目录</th>
                <th className="px-4 py-3 text-left">Git分支</th>
                <th className="px-4 py-3 text-left">状态</th>
                <th className="px-4 py-3 text-left">消息数</th>
                <th className="px-4 py-3 text-left">最后活跃</th>
                <th className="px-4 py-3 text-right">操作</th>
              </tr>
            </thead>
            <tbody>
              {loading && paged.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-on-surface-variant text-[14px]">
                    加载中...
                  </td>
                </tr>
              )}
              {!loading && paged.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-4 py-12 text-center text-on-surface-variant text-[14px]">
                    暂无会话记录
                  </td>
                </tr>
              )}
              {paged.map(session => (
                <tr
                  key={session.id}
                  className="group border-t border-outline-variant/30 hover:bg-surface-container-high/50 transition cursor-pointer"
                  onClick={() => navigate(`/sessions/${session.id}`)}
                >
                  <td className="px-4 py-3">
                    <span className="text-[13px] text-primary">
                      {session.name || '(无标题)'}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className="font-mono text-[13px] text-on-surface-variant">
                      {session.work_dir}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    {session.git_branch ? (
                      <span className="bg-surface-container-high rounded px-2 py-0.5 font-mono text-[11px] text-on-surface-variant">
                        {session.git_branch}
                      </span>
                    ) : (
                      <span className="text-on-surface-variant/40">-</span>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge status={session.status} />
                  </td>
                  <td className="px-4 py-3">
                    <span className="flex items-center gap-1 font-mono text-[12px] text-on-surface-variant">
                      <span className="material-symbols-outlined text-[14px]">chat_bubble</span>
                      {session.message_count}
                    </span>
                  </td>
                  <td className="px-4 py-3">
                    <span className="font-mono text-[12px] text-on-surface-variant">
                      {formatTime(session.modified)}
                    </span>
                  </td>
                  <td
                    className="px-4 py-3 text-right"
                    onClick={e => e.stopPropagation()}
                  >
                    <div className="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition">
                      {session.status.toLowerCase() !== 'active' && (
                        <button
                          onClick={() => handleResume(session.id)}
                          title="接管"
                          className="p-1.5 rounded hover:bg-surface-variant/50 text-on-surface-variant hover:text-secondary transition cursor-pointer"
                        >
                          <span className="material-symbols-outlined text-[18px]">play_arrow</span>
                        </button>
                      )}
                      {(session.status.toLowerCase() === 'active' || session.status.toLowerCase() === 'idle') && (
                        <button
                          onClick={() => handleStop(session.id)}
                          title="停止"
                          className="p-1.5 rounded hover:bg-surface-variant/50 text-on-surface-variant hover:text-tertiary transition cursor-pointer"
                        >
                          <span className="material-symbols-outlined text-[18px]">stop</span>
                        </button>
                      )}
                      <button
                        onClick={() => handleDelete(session.id)}
                        title="删除"
                        className="p-1.5 rounded hover:bg-surface-variant/50 text-on-surface-variant hover:text-error transition cursor-pointer"
                      >
                        <span className="material-symbols-outlined text-[18px]">delete</span>
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="flex items-center justify-between px-4 py-3 border-t border-outline-variant/30">
            <span className="font-mono text-[12px] text-on-surface-variant">
              共 {filtered.length} 条，第 {safePage}/{totalPages} 页
            </span>
            <div className="flex items-center gap-2">
              <button
                disabled={safePage <= 1}
                onClick={() => setPage(safePage - 1)}
                className="px-3 py-1.5 rounded text-[13px] text-on-surface-variant hover:bg-surface-variant/30 disabled:opacity-30 disabled:cursor-not-allowed transition cursor-pointer"
              >
                <span className="material-symbols-outlined text-[16px] align-middle">chevron_left</span>
                上一页
              </button>
              <button
                disabled={safePage >= totalPages}
                onClick={() => setPage(safePage + 1)}
                className="px-3 py-1.5 rounded text-[13px] text-on-surface-variant hover:bg-surface-variant/30 disabled:opacity-30 disabled:cursor-not-allowed transition cursor-pointer"
              >
                下一页
                <span className="material-symbols-outlined text-[16px] align-middle">chevron_right</span>
              </button>
            </div>
          </div>
        )}
      </div>

      {/* New session modal */}
      {modalOpen && (
        <div
          className="fixed inset-0 bg-black/50 flex items-center justify-center z-50"
          onClick={() => setModalOpen(false)}
        >
          <div
            className="bg-surface-container border border-outline-variant rounded-lg p-6 w-full max-w-md"
            onClick={e => e.stopPropagation()}
          >
            <h2 className="text-[18px] font-semibold text-primary mb-6">新建 Claude 会话</h2>
            <div className="flex flex-col gap-4">
              <div>
                <label className="block text-[13px] text-on-surface-variant mb-1.5">
                  工作目录 <span className="text-error">*</span>
                </label>
                <input
                  type="text"
                  placeholder="/path/to/project"
                  value={formWorkDir}
                  onChange={e => setFormWorkDir(e.target.value)}
                  className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface placeholder:text-on-surface-variant/50 focus:outline-none focus:border-primary"
                  autoFocus
                />
              </div>
              <div>
                <label className="block text-[13px] text-on-surface-variant mb-1.5">
                  会话名称
                </label>
                <input
                  type="text"
                  placeholder="可选"
                  value={formName}
                  onChange={e => setFormName(e.target.value)}
                  className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface placeholder:text-on-surface-variant/50 focus:outline-none focus:border-primary"
                />
              </div>
            </div>
            <div className="flex justify-end gap-3 mt-6">
              <button
                onClick={() => {
                  setModalOpen(false)
                  setFormWorkDir('')
                  setFormName('')
                }}
                className="px-4 py-2 rounded text-[14px] text-on-surface-variant hover:bg-surface-variant/30 transition cursor-pointer"
              >
                取消
              </button>
              <button
                onClick={handleStart}
                disabled={!formWorkDir.trim() || formSubmitting}
                className="px-4 py-2 rounded text-[14px] bg-surface-variant text-primary hover:bg-surface-bright transition disabled:opacity-50 disabled:cursor-not-allowed cursor-pointer"
              >
                {formSubmitting ? '创建中...' : '创建'}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
