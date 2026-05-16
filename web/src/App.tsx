import { BrowserRouter, Routes, Route, Navigate, Outlet, useNavigate, useLocation } from 'react-router-dom'
import { useState } from 'react'
import SideNavBar from './components/SideNavBar'
import TopAppBar from './components/TopAppBar'
import Dashboard from './pages/Dashboard'
import SessionList from './pages/SessionList'
import SessionChat from './pages/SessionChat'
import LogViewer from './pages/LogViewer'
import WechatBind from './pages/WechatBind'
import Settings from './pages/Settings'

function Shell() {
  const navigate = useNavigate()
  const location = useLocation()
  const [newSessionSignal, setNewSessionSignal] = useState(0)

  const handleNewSession = () => {
    if (location.pathname !== '/sessions') {
      navigate('/sessions', { state: { newSession: true } })
    } else {
      setNewSessionSignal(prev => prev + 1)
    }
  }

  return (
    <div className="h-screen bg-background text-on-surface overflow-hidden">
      <SideNavBar onNewSession={handleNewSession} />
      <div className="ml-sidebar h-full flex flex-col overflow-hidden">
        <TopAppBar />
        <main className="flex-1 overflow-y-auto">
          <Outlet context={{ newSessionSignal, setNewSessionSignal }} />
        </main>
      </div>
    </div>
  )
}

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Shell />}>
          <Route index element={<Dashboard />} />
          <Route path="sessions" element={<SessionList />} />
          <Route path="sessions/:id" element={<SessionChat />} />
          <Route path="log" element={<LogViewer />} />
          <Route path="wechat" element={<WechatBind />} />
          <Route path="settings" element={<Settings />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
