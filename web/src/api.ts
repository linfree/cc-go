const BASE = '/api/v1'

async function request(path: string, options?: RequestInit) {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    throw new Error(`API error: ${res.status}`)
  }
  return res.json()
}

export const api = {
  getStats: () => request('/stats'),
  getQRCode: () => request('/wechat/qrcode'),
  getWechatStatus: () => request('/wechat/status'),
  updateWechatSettings: (data: Record<string, unknown>) =>
    request('/wechat/settings', { method: 'PUT', body: JSON.stringify(data) }),
  disconnectWechat: () => request('/wechat/disconnect', { method: 'POST' }),
  getClaudePath: () => request('/claude/path'),
  setClaudePath: (path: string) =>
    request('/claude/path', { method: 'POST', body: JSON.stringify({ path }) }),
  autoDetectClaude: () => request('/claude/auto-detect', { method: 'POST' }),
  getSessions: () => request('/sessions'),
  syncSessions: () => request('/sync', { method: 'POST' }),
  getActiveSession: () => request('/sessions/active'),
  getSession: (id: string) => request(`/sessions/${id}`),
  getSessionHistory: (id: string) => request(`/sessions/${id}/history`),
  getActiveSessionLog: () => request('/sessions/active/log'),
  startSession: (data: { work_dir: string; model?: string; name?: string }) =>
    request('/sessions/start', { method: 'POST', body: JSON.stringify(data) }),
  resumeSession: (id: string) => request(`/sessions/${id}/resume`, { method: 'POST' }),
  sendMessage: (id: string, content: string) =>
    request(`/sessions/${id}/message`, { method: 'POST', body: JSON.stringify({ content }) }),
  respondPermission: (id: string, requestID: string, allow: boolean, answer?: string) =>
    request(`/sessions/${id}/permission`, { method: 'POST', body: JSON.stringify({ request_id: requestID, allow, ...(answer ? { answer } : {}) }) }),
  pendingPermissions: (id: string) => request(`/sessions/${id}/permissions/pending`),
  stopSession: (id: string) => request(`/sessions/${id}/stop`, { method: 'POST' }),
  renameSession: (id: string, name: string) =>
    request(`/sessions/${id}`, { method: 'PATCH', body: JSON.stringify({ name }) }),
  deleteSession: (id: string) => request(`/sessions/${id}`, { method: 'DELETE' }),
  getPushTypes: () => request('/push/types'),
  getPushSettings: () => request('/push/settings'),
  updatePushSettings: (types: string[]) =>
    request('/push/settings', { method: 'PUT', body: JSON.stringify({ push_types: types }) }),
  getSettings: () => request('/settings'),
  updateSettings: (data: Record<string, unknown>) =>
    request('/settings', { method: 'PUT', body: JSON.stringify(data) }),
  getBotCommands: () => request('/bot-commands'),
  updateBotCommands: (commands: unknown[]) =>
    request('/bot-commands', { method: 'PUT', body: JSON.stringify({ commands }) }),
  getSkills: () => request('/skills'),
  updateSkills: (skills: unknown[]) =>
    request('/skills', { method: 'PUT', body: JSON.stringify({ skills }) }),
  deleteSkill: (name: string) => request(`/skills/${name}`, { method: 'DELETE' }),
  getAvailableSkills: () => request('/skills/available'),
  importSkills: (names: string[]) =>
    request('/skills/import', { method: 'POST', body: JSON.stringify({ names }) }),
}