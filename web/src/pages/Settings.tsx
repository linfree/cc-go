import { useState, useEffect } from 'react'
import { api } from '../api'
import { cn } from '../lib/cn'

interface PushType {
  key: string
  label: string
  required: boolean
}

interface BotCommand {
  key: string
  keyword: string
  description: string
  enabled: boolean
}

interface SkillConfig {
  name: string
  description: string
  enabled: boolean
  builtin?: boolean
}

interface AvailableSkill {
  name: string
  description: string
  exists: boolean
}

interface Settings {
  language?: string
  web_port?: number
  actual_port?: number
  auto_open_browser?: boolean
  auto_resume_latest?: boolean
  auto_find_claude?: boolean
  permission_mode?: string
  claude_env_vars?: string
  wechat?: {
    send_budget_limit: number
    max_buffered_messages: number
    activation_warning_hours: number
    activation_reminder_minutes: number
  }
}

interface ClaudeInfo {
  path: string
  version: string
  valid: boolean
}

const tabs = [
  { id: 'general', icon: 'tune', label: '通用设置' },
  { id: 'cli', icon: 'terminal', label: 'Claude CLI' },
  { id: 'wechat', icon: 'chat', label: '微信' },
  { id: 'notifications', icon: 'notifications', label: '通知' },
  { id: 'commands', icon: 'smart_toy', label: '机器人指令' },
  { id: 'skills', icon: 'extension', label: 'Skills' },
]

function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <div
      className={cn('toggle-switch', checked && 'active')}
      onClick={() => onChange(!checked)}
    />
  )
}

export default function Settings() {
  const [activeTab, setActiveTab] = useState('general')
  const [settings, setSettings] = useState<Settings>({})
  const [saving, setSaving] = useState(false)
  const [claudeInfo, setClaudeInfo] = useState<ClaudeInfo | null>(null)
  const [cliLoading, setCliLoading] = useState(false)
  const [pushTypes, setPushTypes] = useState<PushType[]>([])
  const [enabledTypes, setEnabledTypes] = useState<string[]>([])
  const [commands, setCommands] = useState<BotCommand[]>([])
  const [skills, setSkills] = useState<SkillConfig[]>([])
  const [importDialogOpen, setImportDialogOpen] = useState(false)
  const [availableSkills, setAvailableSkills] = useState<AvailableSkill[]>([])
  const [importLoading, setImportLoading] = useState(false)
  const [selectedImport, setSelectedImport] = useState<string[]>([])
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  useEffect(() => {
    api.getSettings().then(data => setSettings(data))
    api.getClaudePath().then(data => setClaudeInfo(data)).catch(() => {})
    api.getPushTypes().then(data => setPushTypes(data.types || []))
    api.getPushSettings().then(data => setEnabledTypes(data.push_types || []))
    api.getBotCommands().then(data => setCommands(data.commands || []))
    api.getSkills().then(data => setSkills(data.skills || []))
  }, [])

  const handleSave = async () => {
    setSaving(true)
    try {
      await api.updateSettings(settings as Record<string, unknown>)
      if (settings.wechat) {
        await api.updateWechatSettings(settings.wechat as Record<string, unknown>)
      }
    } catch {
      // ignore
    } finally {
      setSaving(false)
    }
  }

  const handleAutoDetect = async () => {
    setCliLoading(true)
    try {
      const data = await api.autoDetectClaude()
      setClaudeInfo(data)
    } catch {
      // ignore
    } finally {
      setCliLoading(false)
    }
  }

  const handleValidate = async () => {
    if (!claudeInfo?.path) return
    setCliLoading(true)
    try {
      const data = await api.setClaudePath(claudeInfo.path)
      setClaudeInfo(prev => prev ? { ...prev, valid: data.valid, version: data.version } : data)
    } catch {
      // ignore
    } finally {
      setCliLoading(false)
    }
  }

  const handleTogglePush = async (key: string, enabled: boolean) => {
    const newTypes = enabled ? [...enabledTypes, key] : enabledTypes.filter(t => t !== key)
    setEnabledTypes(newTypes)
    try {
      await api.updatePushSettings(newTypes)
    } catch {
      // revert
      api.getPushSettings().then(data => setEnabledTypes(data.push_types || []))
    }
  }

  const handleCmdToggle = async (cmdKey: string, enabled: boolean) => {
    const updated = commands.map(c => c.key === cmdKey ? { ...c, enabled } : c)
    setCommands(updated)
    try {
      await api.updateBotCommands(updated)
    } catch {
      api.getBotCommands().then(data => setCommands(data.commands || []))
    }
  }

  const handleKeywordChange = (cmdKey: string, keyword: string) => {
    const clean = keyword.startsWith('/') ? keyword : '/' + keyword
    setCommands(commands.map(c => c.key === cmdKey ? { ...c, keyword: clean } : c))
  }

  const handleKeywordSave = async (cmdKey: string) => {
    const cmd = commands.find(c => c.key === cmdKey)
    if (!cmd || !cmd.keyword.startsWith('/') || cmd.keyword.length <= 1) {
      api.getBotCommands().then(data => setCommands(data.commands || []))
      return
    }
    try {
      await api.updateBotCommands(commands)
    } catch {
      api.getBotCommands().then(data => setCommands(data.commands || []))
    }
  }

  const handleSkillToggle = async (name: string, enabled: boolean) => {
    const updated = skills.map(s => s.name === name ? { ...s, enabled } : s)
    setSkills(updated)
    try {
      await api.updateSkills(updated)
    } catch {
      api.getSkills().then(data => setSkills(data.skills || []))
    }
  }

  const handleSkillDelete = async (name: string) => {
    try {
      const data = await api.deleteSkill(name)
      setSkills(data.skills || [])
    } catch {
      api.getSkills().then(data => setSkills(data.skills || []))
    } finally {
      setDeleteConfirm(null)
    }
  }

  const confirmDelete = (name: string) => {
    setDeleteConfirm(name)
  }

  const handleOpenImport = async () => {
    setImportLoading(true)
    try {
      const data = await api.getAvailableSkills()
      setAvailableSkills(data.skills || [])
      setSelectedImport([])
      setImportDialogOpen(true)
    } catch {
      // ignore
    } finally {
      setImportLoading(false)
    }
  }

  const handleImport = async () => {
    if (selectedImport.length === 0) return
    setImportLoading(true)
    try {
      const data = await api.importSkills(selectedImport)
      setSkills(data.skills || [])
      setImportDialogOpen(false)
    } catch {
      api.getSkills().then(data => setSkills(data.skills || []))
    } finally {
      setImportLoading(false)
    }
  }

  return (
    <div className="p-8 max-w-[1440px] mx-auto w-full">
      <div className="mb-8">
        <h2 className="text-[30px] font-bold text-primary mb-1">系统设置</h2>
        <p className="text-[14px] text-on-surface-variant">管理 cc-go 的全局配置和偏好设置。</p>
      </div>

      <div className="grid grid-cols-12 gap-6">
        {/* Left nav */}
        <div className="col-span-12 md:col-span-3">
          <nav className="sticky top-24 flex flex-col gap-1">
            {tabs.map(tab => (
              <button
                key={tab.id}
                onClick={() => setActiveTab(tab.id)}
                className={cn(
                  'flex items-center gap-3 px-4 py-3 rounded text-[14px] transition-colors w-full text-left',
                  activeTab === tab.id
                    ? 'bg-surface-container-high text-secondary'
                    : 'text-on-surface-variant hover:bg-surface-variant/30'
                )}
              >
                <span className="material-symbols-outlined text-[20px]">{tab.icon}</span>
                {tab.label}
              </button>
            ))}
          </nav>
        </div>

        {/* Settings panels */}
        <div className="col-span-12 md:col-span-9">
          {/* General */}
          {activeTab === 'general' && (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-6">
              <h3 className="text-[20px] font-semibold text-primary mb-6">通用设置</h3>
              <div className="space-y-6">
                <div className="grid grid-cols-2 gap-4">
                  <div className="min-w-0 flex-1">
                    <label className="block text-[13px] text-on-surface-variant mb-2">端口</label>
                    <input
                      type="number"
                      value={settings.web_port || ''}
                      onChange={e => setSettings({ ...settings, web_port: parseInt(e.target.value) || 0 })}
                      className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface focus:outline-none focus:border-primary"
                    />
                    {settings.actual_port && settings.actual_port !== settings.web_port && (
                      <p className="mt-1 text-[12px] text-tertiary">
                        实际监听端口: {settings.actual_port}（{settings.web_port} 被占用）
                      </p>
                    )}
                  </div>
                  <div className="min-w-0 flex-1">
                    <label className="block text-[13px] text-on-surface-variant mb-2">语言</label>
                    <select
                      value={settings.language || 'zh-CN'}
                      onChange={e => setSettings({ ...settings, language: e.target.value })}
                      className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 text-[13px] text-on-surface focus:outline-none focus:border-primary"
                    >
                      <option value="zh-CN">中文</option>
                      <option value="en">English</option>
                    </select>
                  </div>
                </div>

                <div className="space-y-4">
                  <div className="flex items-center justify-between p-3 bg-surface-container-low rounded-lg">
                    <div className="min-w-0 flex-1">
                      <p className="text-[14px] text-on-surface">启动时自动打开浏览器</p>
                      <p className="text-[12px] text-on-surface-variant">服务启动后自动在默认浏览器中打开 Web 界面</p>
                    </div>
                    <div className="shrink-0"><Toggle
                      checked={settings.auto_open_browser || false}
                      onChange={v => setSettings({ ...settings, auto_open_browser: v })}
                    /></div>
                  </div>
                  <div className="flex items-center justify-between p-3 bg-surface-container-low rounded-lg">
                    <div className="min-w-0 flex-1">
                      <p className="text-[14px] text-on-surface">启动后自动接管最新会话</p>
                      <p className="text-[12px] text-on-surface-variant">启动时自动恢复上次最近使用的会话</p>
                    </div>
                    <div className="shrink-0"><Toggle
                      checked={settings.auto_resume_latest || false}
                      onChange={v => setSettings({ ...settings, auto_resume_latest: v })}
                    /></div>
                  </div>
                  <div className="flex items-center justify-between p-3 bg-surface-container-low rounded-lg">
                    <div className="min-w-0 flex-1">
                      <p className="text-[14px] text-on-surface">自动查找 Claude CLI</p>
                      <p className="text-[12px] text-on-surface-variant">启动时自动在系统 PATH 中查找 Claude CLI</p>
                    </div>
                    <div className="shrink-0"><Toggle
                      checked={settings.auto_find_claude || false}
                      onChange={v => setSettings({ ...settings, auto_find_claude: v })}
                    /></div>
                  </div>
                  <div className="p-3 bg-surface-container-low rounded-lg">
                    <label className="block text-[14px] text-on-surface mb-2">权限模式</label>
                    <select
                      value={settings.permission_mode || 'default'}
                      onChange={e => setSettings({ ...settings, permission_mode: e.target.value })}
                      className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 text-[13px] text-on-surface focus:outline-none focus:border-primary"
                    >
                      <option value="default">逐一询问</option>
                      <option value="acceptEdits">接受编辑（半自动）</option>
                      <option value="auto">自动批准（低风险自动）</option>
                      <option value="bypassPermissions">跳过权限检查（全自动，不推荐日常使用）</option>
                      <option value="plan">只读模式（不允许写入和执行）</option>
                    </select>
                    <p className="text-[11px] text-on-surface-variant mt-2">权限模式控制 Claude Code 会话中权限请求的处理方式，通过 <code className="bg-surface-container-lowest px-1 rounded font-mono">--permission-mode</code> 参数传入启动命令。</p>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* Claude CLI */}
          {activeTab === 'cli' && (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-6">
              <div className="flex items-center justify-between mb-6">
                <h3 className="text-[20px] font-semibold text-primary">Claude CLI 配置</h3>
                {claudeInfo?.valid && (
                  <span className="flex items-center gap-1.5 px-2.5 py-1 rounded border font-mono text-[11px] font-medium bg-secondary/10 border-secondary/30 text-secondary">
                    <span className="w-1.5 h-1.5 rounded-full bg-secondary" />
                    已连接
                  </span>
                )}
              </div>
              <div className="space-y-4">
                <div className="min-w-0 flex-1">
                  <label className="block text-[13px] text-on-surface-variant mb-2">CLI 路径</label>
                  <div className="flex gap-2">
                    <input
                      type="text"
                      value={claudeInfo?.path || ''}
                      onChange={e => setClaudeInfo(prev => prev ? { ...prev, path: e.target.value, valid: false, version: '' } : { path: e.target.value, valid: false, version: '' })}
                      placeholder="输入或自动检测 Claude CLI 路径"
                      className="flex-1 bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface focus:outline-none focus:border-primary placeholder:text-on-surface-variant/40"
                    />
                    <button
                      onClick={handleAutoDetect}
                      disabled={cliLoading}
                      className="px-4 py-2 bg-surface-variant border border-outline-variant rounded text-[13px] text-primary hover:bg-surface-bright transition-colors flex items-center gap-2 disabled:opacity-50"
                    >
                      <span className={cn('material-symbols-outlined text-[16px]', cliLoading && 'animate-spin')}>
                        sync
                      </span>
                      自动检测
                    </button>
                  </div>
                </div>
                {claudeInfo?.version && (
                  <div className="p-3 bg-surface-container-low rounded-lg">
                    <p className="font-mono text-[11px] text-on-surface-variant mb-1">版本</p>
                    <p className="font-mono text-[13px] text-on-surface">{claudeInfo.version}</p>
                  </div>
                )}
                <button
                  onClick={handleValidate}
                  disabled={cliLoading || !claudeInfo?.path}
                  className="px-4 py-2 bg-secondary/10 border border-secondary/30 rounded text-secondary hover:bg-secondary/20 transition-colors text-[13px] flex items-center gap-2 disabled:opacity-50"
                >
                  <span className={cn('material-symbols-outlined text-[16px]', cliLoading && 'animate-spin')}>
                    verified
                  </span>
                  验证 CLI
                </button>

                {/* Environment Variables */}
                <div className="mt-6 pt-6 border-t border-outline-variant">
                  <div className="flex items-center gap-2 mb-2">
                    <span className="material-symbols-outlined text-on-surface-variant text-[18px]">key</span>
                    <label className="text-[14px] font-medium text-on-surface">环境变量</label>
                  </div>
                  <p className="text-[12px] text-on-surface-variant mb-3">
                    每行一个 KEY=VALUE，以 # 开头的行为注释。这些变量将在启动 Claude 会话时注入到子进程环境。
                  </p>
                  <textarea
                    value={settings.claude_env_vars || ''}
                    onChange={e => setSettings({ ...settings, claude_env_vars: e.target.value })}
                    placeholder={'# 例如:\n# ANTHROPIC_BASE_URL=https://aigw.example.com\n# ANTHROPIC_AUTH_TOKEN=dcm_xxx\n# ANTHROPIC_MODEL=claude-sonnet-4-20250514\n# CLAUDE_CODE_MAX_OUTPUT_TOKENS=16384'}
                    rows={10}
                    spellCheck={false}
                    className="w-full bg-surface-container-lowest border border-outline-variant rounded-lg px-4 py-3 font-mono text-[13px] text-on-surface placeholder:text-on-surface-variant/30 focus:outline-none focus:border-primary resize-y leading-relaxed"
                  />
                </div>
              </div>
            </div>
          )}

          {/* WeChat */}
          {activeTab === 'wechat' && (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-6">
              <h3 className="text-[20px] font-semibold text-primary mb-2">微信消息设置</h3>
              <p className="text-[13px] text-on-surface-variant mb-6">
                管理微信机器人的消息发送限制、缓存策略和激活机制。
              </p>
              <div className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <div className="min-w-0 flex-1">
                    <label className="block text-[13px] text-on-surface-variant mb-2">每轮消息上限</label>
                    <input
                      type="number"
                      min={4}
                      max={7}
                      value={settings.wechat?.send_budget_limit ?? 7}
                      onChange={e => setSettings({
                        ...settings,
                        wechat: { ...(settings.wechat || {} as any), send_budget_limit: parseInt(e.target.value) || 7 }
                      })}
                      className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface focus:outline-none focus:border-primary"
                    />
                    <p className="text-[11px] text-on-surface-variant mt-1">用户每次发消息后，机器人可回复的最大消息条数 (4-7)</p>
                  </div>
                  <div className="min-w-0 flex-1">
                    <label className="block text-[13px] text-on-surface-variant mb-2">最大缓存消息</label>
                    <input
                      type="number"
                      min={100}
                      max={1000}
                      value={settings.wechat?.max_buffered_messages ?? 100}
                      onChange={e => setSettings({
                        ...settings,
                        wechat: { ...(settings.wechat || {} as any), max_buffered_messages: parseInt(e.target.value) || 100 }
                      })}
                      className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface focus:outline-none focus:border-primary"
                    />
                    <p className="text-[11px] text-on-surface-variant mt-1">超出上限后缓存的消息数量上限，超过后旧消息会被驱逐 (100-1000)</p>
                  </div>
                </div>
                <div className="grid grid-cols-2 gap-4">
                  <div className="min-w-0 flex-1">
                    <label className="block text-[13px] text-on-surface-variant mb-2">登录提醒 (小时)</label>
                    <input
                      type="number"
                      min={1}
                      max={22}
                      value={settings.wechat?.activation_warning_hours ?? 21}
                      onChange={e => setSettings({
                        ...settings,
                        wechat: { ...(settings.wechat || {} as any), activation_warning_hours: parseInt(e.target.value) || 21 }
                      })}
                      className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface focus:outline-none focus:border-primary"
                    />
                    <p className="text-[11px] text-on-surface-variant mt-1">登录后多少小时开始发送登录提醒 (1-22)</p>
                  </div>
                  <div className="min-w-0 flex-1">
                    <label className="block text-[13px] text-on-surface-variant mb-2">提醒间隔 (分钟)</label>
                    <input
                      type="number"
                      min={1}
                      max={60}
                      value={settings.wechat?.activation_reminder_minutes ?? 60}
                      onChange={e => setSettings({
                        ...settings,
                        wechat: { ...(settings.wechat || {} as any), activation_reminder_minutes: parseInt(e.target.value) || 60 }
                      })}
                      className="w-full bg-surface-container-lowest border border-outline-variant rounded px-3 py-2 font-mono text-[13px] text-on-surface focus:outline-none focus:border-primary"
                    />
                    <p className="text-[11px] text-on-surface-variant mt-1">登录提醒的重复间隔 (1-60)</p>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* Notifications */}
          {activeTab === 'notifications' && (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-6">
              <h3 className="text-[20px] font-semibold text-primary mb-6">通知设置</h3>
              <div className="space-y-3">
                {pushTypes.map(pt => (
                  <div key={pt.key} className="flex items-center justify-between p-3 bg-surface-container-low rounded-lg">
                    <div className="min-w-0 flex-1">
                      <p className="text-[14px] text-on-surface">
                        {pt.label}
                        {pt.required && (
                          <span className="ml-2 px-1.5 py-0.5 rounded bg-error/10 border border-error/30 text-error font-mono text-[10px]">
                            强制开启
                          </span>
                        )}
                      </p>
                    </div>
                    <div className="shrink-0"><Toggle
                      checked={enabledTypes.includes(pt.key)}
                      onChange={v => handleTogglePush(pt.key, v)}
                    /></div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Bot Commands */}
          {activeTab === 'commands' && (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-6">
              <h3 className="text-[20px] font-semibold text-primary mb-2">机器人指令</h3>
              <p className="text-[13px] text-on-surface-variant mb-6">
                管理微信机器人可用的指令。指令关键字必须以 <code className="bg-surface-container-lowest px-1 rounded font-mono">/</code> 开头。
              </p>
              <div className="border border-outline-variant rounded-lg overflow-hidden">
                <table className="w-full">
                  <thead>
                    <tr className="bg-surface-container-high">
                      <th className="px-4 py-2 text-left font-mono text-[11px] text-on-surface-variant uppercase tracking-wider">指令</th>
                      <th className="px-4 py-2 text-left font-mono text-[11px] text-on-surface-variant uppercase tracking-wider">说明</th>
                      <th className="px-4 py-2 text-center font-mono text-[11px] text-on-surface-variant uppercase tracking-wider w-20">启用</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-outline-variant/30">
                    {commands.map(cmd => (
                      <tr key={cmd.key} className="hover:bg-surface-container-high/50">
                        <td className="px-4 py-2">
                          <input
                            type="text"
                            value={cmd.keyword}
                            onChange={e => handleKeywordChange(cmd.key, e.target.value)}
                            onBlur={() => handleKeywordSave(cmd.key)}
                            className="bg-transparent border-b border-outline-variant font-mono text-[13px] text-on-surface w-full focus:outline-none focus:border-primary px-1"
                          />
                        </td>
                        <td className="px-4 py-2">
                          <input
                            type="text"
                            value={cmd.description}
                            onChange={e => setCommands(commands.map(c => c.key === cmd.key ? { ...c, description: e.target.value } : c))}
                            onBlur={() => api.updateBotCommands(commands).catch(() => api.getBotCommands().then(d => setCommands(d.commands || [])))}
                            className="bg-transparent border-b border-outline-variant text-[13px] text-on-surface-variant w-full focus:outline-none focus:border-primary px-1"
                          />
                        </td>
                        <td className="px-4 py-2 text-center">
                          <Toggle checked={cmd.enabled} onChange={v => handleCmdToggle(cmd.key, v)} />
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}

          {/* Skills */}
          {activeTab === 'skills' && (
            <div className="bg-surface-container border border-outline-variant rounded-lg p-6">
              <h3 className="text-[20px] font-semibold text-primary mb-2">Skills 管理</h3>
              <p className="text-[13px] text-on-surface-variant mb-6">
                管理 Claude Code 会话中可用的 Skills。启用后，Skill 会在接管会话时自动注入到项目的 <code className="bg-surface-container-lowest px-1 rounded font-mono">[项目]/.claude/skills/</code> 目录。
              </p>
              <div className="space-y-3">
                {skills.map(skill => (
                  <div key={skill.name} className="relative flex items-center justify-between gap-4 p-4 bg-surface-container-low rounded-lg group">
                    {!skill.builtin && (
                      <button
                        onClick={() => confirmDelete(skill.name)}
                        className="absolute top-1 right-1 p-1 rounded text-on-surface-variant/30 hover:text-error hover:bg-error/10 transition-colors opacity-0 group-hover:opacity-100"
                        title="删除"
                      >
                        <span className="material-symbols-outlined text-[16px]">close</span>
                      </button>
                    )}
                    <div className="min-w-0 flex-1">
                      <p className="text-[14px] font-medium text-primary">
                        {skill.name}
                        {skill.builtin && (
                          <span className="ml-2 px-1.5 py-0.5 rounded bg-secondary/10 border border-secondary/30 text-secondary font-mono text-[10px]">内置</span>
                        )}
                      </p>
                      <p className="text-[12px] text-on-surface-variant mt-0.5 line-clamp-2">{skill.description}</p>
                    </div>
                    <div className="shrink-0">
                      <Toggle
                        checked={skill.enabled}
                        onChange={v => handleSkillToggle(skill.name, v)}
                      />
                    </div>
                  </div>
                ))}
                {skills.length === 0 && (
                  <p className="text-[13px] text-on-surface-variant text-center py-6">暂无 Skills，点击下方导入或手动添加到 ~/.cc-go/skills/</p>
                )}
              </div>

              {/* Delete confirmation */}
              {deleteConfirm && (
                <div className="mt-6 p-4 bg-error/5 rounded-lg border border-error/30">
                  <p className="text-[14px] text-on-surface mb-3">
                    确定要删除 skill <strong className="text-primary">{deleteConfirm}</strong> 吗？该文件将从磁盘永久移除。
                  </p>
                  <div className="flex justify-end gap-3">
                    <button
                      onClick={() => setDeleteConfirm(null)}
                      className="px-4 py-2 rounded border border-outline-variant text-on-surface-variant hover:text-primary transition-colors text-[13px]"
                    >
                      取消
                    </button>
                    <button
                      onClick={() => handleSkillDelete(deleteConfirm)}
                      className="px-4 py-2 rounded bg-error/10 border border-error/30 text-error hover:bg-error/20 transition-colors text-[13px] font-medium"
                    >
                      删除
                    </button>
                  </div>
                </div>
              )}

              {/* Import Dialog */}
              {importDialogOpen && (
                <div className="mt-6 p-4 bg-surface-container-low rounded-lg border border-outline-variant">
                  <h4 className="text-[16px] font-semibold text-primary mb-3">从 Claude Code 导入 Skills</h4>
                  <p className="text-[12px] text-on-surface-variant mb-4">
                    以下是从 <code className="bg-surface-container-lowest px-1 rounded font-mono">~/.claude/skills/</code> 发现的 Skills，选择要导入的项目：
                  </p>
                  {availableSkills.length === 0 ? (
                    <p className="text-[13px] text-on-surface-variant text-center py-4">未发现可导入的 Skills</p>
                  ) : (
                    <div className="space-y-2">
                      {availableSkills.map(s => (
                        <div key={s.name} className={cn(
                          'flex items-center justify-between gap-3 p-3 rounded border transition-colors cursor-pointer',
                          selectedImport.includes(s.name)
                            ? 'bg-tertiary/10 border-tertiary/30'
                            : 'bg-surface-container-lowest border-outline-variant/50 hover:border-outline-variant',
                          s.exists && 'opacity-50'
                        )}
                          onClick={() => {
                            if (s.exists) return
                            setSelectedImport(prev =>
                              prev.includes(s.name) ? prev.filter(n => n !== s.name) : [...prev, s.name]
                            )
                          }}
                        >
                          <div className="min-w-0 flex-1">
                            <p className="text-[13px] font-medium text-primary">{s.name}</p>
                            <p className="text-[11px] text-on-surface-variant mt-0.5 line-clamp-1">{s.description}</p>
                          </div>
                          <div className="shrink-0">
                            {s.exists ? (
                              <span className="text-[11px] font-mono text-on-surface-variant">已存在</span>
                            ) : (
                              <span className={cn(
                                'w-4 h-4 rounded border-2 flex items-center justify-center',
                                selectedImport.includes(s.name) ? 'border-tertiary bg-tertiary' : 'border-outline-variant'
                              )}>
                                {selectedImport.includes(s.name) && <span className="text-white text-[10px]">✓</span>}
                              </span>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}
                  <div className="flex justify-end gap-3 mt-4">
                    <button
                      onClick={() => setImportDialogOpen(false)}
                      className="px-4 py-2 rounded border border-outline-variant text-on-surface-variant hover:text-primary transition-colors text-[13px]"
                    >
                      取消
                    </button>
                    <button
                      onClick={handleImport}
                      disabled={selectedImport.length === 0 || importLoading}
                      className="px-4 py-2 rounded bg-tertiary/10 border border-tertiary/30 text-tertiary hover:bg-tertiary/20 transition-colors text-[13px] font-medium disabled:opacity-50 flex items-center gap-2"
                    >
                      {importLoading && <span className="material-symbols-outlined text-[16px] animate-spin">progress_activity</span>}
                      导入 ({selectedImport.length})
                    </button>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Action footer */}
          <div className="sticky bottom-0 mt-6 flex items-center justify-end gap-3 py-4 bg-background/80 backdrop-blur-md">
            {activeTab === 'skills' && (
              <button
                onClick={handleOpenImport}
                disabled={importLoading && !importDialogOpen}
                className="flex items-center gap-2 px-4 py-2 rounded bg-tertiary/10 border border-tertiary/30 text-tertiary hover:bg-tertiary/20 transition-colors text-[14px] font-medium disabled:opacity-50"
              >
                <span className={cn('material-symbols-outlined text-[18px]', importLoading && !importDialogOpen && 'animate-spin')}>download</span>
                从 Claude Code 导入
              </button>
            )}
            <button
              onClick={() => api.getSettings().then(data => setSettings(data))}
              className="px-4 py-2 rounded border border-outline-variant text-on-surface-variant hover:text-primary hover:border-primary transition-colors text-[14px]"
            >
              重置为默认
            </button>
            <button
              onClick={handleSave}
              disabled={saving}
              className="flex items-center gap-2 px-6 py-2 rounded bg-secondary/10 border border-secondary/30 text-secondary hover:bg-secondary/20 transition-colors text-[14px] font-medium disabled:opacity-50"
            >
              <span className="material-symbols-outlined text-[18px]">save</span>
              {saving ? '保存中...' : '保存更改'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}