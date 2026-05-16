import { useNavigate, useLocation } from 'react-router-dom'
import { cn } from '../lib/cn'
import { api } from '../api'
import { useState, useEffect } from 'react'

interface ActiveSession {
  id: string
  name: string
  work_dir: string
}

const navItems = [
  { path: '/', icon: 'dashboard', label: '仪表板' },
  { path: '/sessions', icon: 'terminal', label: '会话管理' },
  { path: '/wechat', icon: 'chat', label: '微信连接' },
  { path: '/settings', icon: 'settings', label: '系统设置' },
]

export default function SideNavBar({ onNewSession }: { onNewSession?: () => void }) {
  const navigate = useNavigate()
  const location = useLocation()
  const [activeSession, setActiveSession] = useState<ActiveSession | null>(null)

  useEffect(() => {
    const fetchActive = () => {
      api.getActiveSession().then(data => {
        setActiveSession(data?.active || null)
      }).catch(() => {})
    }
    fetchActive()
    const interval = setInterval(fetchActive, 10000)
    return () => clearInterval(interval)
  }, [])

  function isActive(path: string) {
    if (path === '/') return location.pathname === '/'
    return location.pathname.startsWith(path)
  }

  return (
    <nav className="fixed left-0 top-0 h-full w-sidebar bg-surface-container border-r border-outline-variant flex flex-col py-6 z-50">
      {/* Header */}
      <div className="px-4 mb-8">
        <div className="flex items-center gap-3">
          <img src="/cc-go.png" alt="cc-go" className="w-10 h-10 rounded" />
          <div>
            <h1 className="text-[20px] font-semibold text-primary">cc-go</h1>
          </div>
        </div>
      </div>

      {/* Navigation */}
      <div className="flex-1 overflow-y-auto flex flex-col gap-1 px-2">
        {navItems.map(item => {
          const active = isActive(item.path)
          return (
            <button
              key={item.path}
              onClick={() => navigate(item.path)}
              className={cn(
                'flex items-center gap-3 px-4 py-3 text-[14px] transition-colors w-full text-left',
                active
                  ? 'text-secondary border-l-2 border-secondary bg-surface-bright/50'
                  : 'text-on-surface-variant hover:bg-surface-variant/30 border-l-2 border-transparent'
              )}
            >
              <span
                className="material-symbols-outlined"
                style={{ fontVariationSettings: active ? "'FILL' 1" : undefined }}
              >
                {item.icon}
              </span>
              <span>{item.label}</span>
            </button>
          )
        })}
      </div>

      {/* Footer */}
      <div className="px-4 mt-auto flex flex-col gap-4">
        <button
          onClick={onNewSession}
          className="w-full py-2 px-4 bg-surface-variant border border-outline-variant rounded hover:bg-surface-bright transition-colors text-primary text-[14px] flex items-center justify-center gap-2"
        >
          <span className="material-symbols-outlined text-[18px]">add</span>
          新建会话
        </button>
        {activeSession && (
          <div className="pt-4 border-t border-outline-variant">
            <div className="flex items-center gap-3 px-2 py-2 text-on-surface-variant">
              <span
                className="material-symbols-outlined text-secondary animate-pulse"
                style={{ fontVariationSettings: "'FILL' 1" }}
              >
                radio_button_checked
              </span>
              <span className="font-mono text-[11px] font-medium text-secondary">当前会话</span>
            </div>
            <button
              onClick={() => navigate(`/sessions/${activeSession.id}`)}
              className="w-full text-left px-2 py-1 text-[13px] text-primary hover:text-secondary transition-colors truncate"
            >
              {activeSession.name || '(无标题)'}
            </button>
            <p className="px-2 text-[11px] text-on-surface-variant font-mono truncate">
              {activeSession.work_dir}
            </p>
          </div>
        )}
      </div>
    </nav>
  )
}
