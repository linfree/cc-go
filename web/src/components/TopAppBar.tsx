import { useLocation } from 'react-router-dom'

const routeLabels: Record<string, string> = {
  '': '仪表板',
  sessions: '会话管理',
  wechat: '微信连接',
  settings: '系统设置',
  log: '系统日志',
}

export default function TopAppBar() {
  const location = useLocation()
  const segments = location.pathname.split('/').filter(Boolean)

  const crumbs: { label: string; path: string }[] = []
  let path = ''
  for (const seg of segments) {
    path += '/' + seg
    if (routeLabels[seg]) {
      crumbs.push({ label: routeLabels[seg], path })
    } else {
      // Session ID or dynamic segment
      crumbs.push({ label: seg.slice(0, 8) + '...', path })
    }
  }

  if (crumbs.length === 0) {
    crumbs.push({ label: '仪表板', path: '/' })
  }

  return (
    <header className="flex justify-between items-center h-14 px-4 w-full border-b border-outline-variant backdrop-blur-md bg-surface/80 sticky top-0 z-40">
      <div className="flex items-center gap-4">
        <nav className="flex items-center gap-2 text-[14px]">
          {crumbs.map((crumb, i) => (
            <span key={crumb.path} className="flex items-center gap-2">
              {i > 0 && (
                <span className="material-symbols-outlined text-on-surface-variant text-[16px]">
                  chevron_right
                </span>
              )}
              <span
                className={
                  i === crumbs.length - 1
                    ? 'text-on-surface-variant'
                    : 'text-primary font-semibold hover:text-primary transition-colors'
                }
              >
                {crumb.label}
              </span>
            </span>
          ))}
        </nav>
      </div>
      <div className="flex items-center gap-4">
        <div className="flex items-center gap-2 px-3 py-1 rounded bg-surface-container border border-outline-variant">
          <span className="w-2 h-2 rounded-full bg-secondary" />
          <span className="font-mono text-[11px] font-medium text-on-surface-variant">
            WebSocket 已连接
          </span>
        </div>
        <button className="w-8 h-8 flex items-center justify-center rounded hover:bg-surface-variant text-on-surface-variant transition-colors">
          <span className="material-symbols-outlined">notifications</span>
        </button>
        <button className="w-8 h-8 flex items-center justify-center rounded hover:bg-surface-variant text-on-surface-variant transition-colors">
          <span className="material-symbols-outlined">account_circle</span>
        </button>
      </div>
    </header>
  )
}
