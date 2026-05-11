import { BrowserRouter, Routes, Route, Navigate, Outlet } from 'react-router-dom'
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
  const [newSessionOpen, setNewSessionOpen] = useState(false)
  return (
    <div className="h-screen bg-background text-on-surface overflow-hidden">
      <SideNavBar onNewSession={() => setNewSessionOpen(true)} />
      <div className="ml-sidebar h-full flex flex-col overflow-hidden">
        <TopAppBar />
        <main className="flex-1 overflow-y-auto">
          <Outlet context={{ newSessionOpen, setNewSessionOpen }} />
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
