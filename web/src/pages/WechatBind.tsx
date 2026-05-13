import { useState, useEffect, useCallback } from 'react'
import QRCode from 'qrcode'
import { api } from '../api'

interface WechatStatus {
  connected: boolean
  status: string
  login_time?: string
  bot_name?: string
  wxid?: string
  masked_token?: string
  send_budget?: number
  budget_limit?: number
  buffer_mode?: boolean
  buffered_count?: number
  last_msg_time?: string
  next_reminder_time?: string
}

export default function WechatBind() {
  const [status, setStatus] = useState<WechatStatus>({ connected: false, status: 'unknown' })
  const [qrcodeDataUrl, setQrcodeDataUrl] = useState('')
  const [loading, setLoading] = useState(false)
  const [now, setNow] = useState(Date.now())

  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(t)
  }, [])

  const checkStatus = useCallback(async () => {
    try {
      const data = await api.getWechatStatus()
      setStatus(data)
      if (data.connected && qrcodeDataUrl) {
        setQrcodeDataUrl('')
      }
    } catch {
      // ignore
    }
  }, [qrcodeDataUrl])

  useEffect(() => {
    checkStatus()
    const interval = setInterval(checkStatus, 5000)
    return () => clearInterval(interval)
  }, [checkStatus])

  const handleGetQRCode = async () => {
    setLoading(true)
    try {
      const data = await api.getQRCode()
      const url = data.qrcode_img || data.qrcode
      if (url) {
        const dataUrl = await QRCode.toDataURL(url, {
          width: 256,
          margin: 2,
          color: { dark: '#d3e4fe', light: '#102034' },
        })
        setQrcodeDataUrl(dataUrl)
      }
    } catch (e) {
      console.error('Failed to get QR code:', e)
    } finally {
      setLoading(false)
    }
  }

  const handleDisconnect = async () => {
    if (!window.confirm('确定要断开微信连接吗？')) return
    try {
      await api.disconnectWechat()
      setStatus({ connected: false, status: 'disconnected' })
      setQrcodeDataUrl('')
    } catch {
      // ignore
    }
  }

  const connected = status.connected

  const formatCountdown = (targetISO: string) => {
    const diff = new Date(targetISO).getTime() - now
    if (diff <= 0) return '已过期'
    const h = Math.floor(diff / 3600000)
    const m = Math.floor((diff % 3600000) / 60000)
    const s = Math.floor((diff % 60000) / 1000)
    if (h > 0) return h + '小时' + m + '分' + s + '秒'
    return m + '分' + s + '秒'
  }

  return (
    <div className="p-8 max-w-[1440px] mx-auto w-full">
      {/* Page Header */}
      <div className="mb-8">
        <h2 className="text-[30px] font-bold text-primary mb-1">微信连接</h2>
        <p className="text-[14px] text-on-surface-variant">
          通过微信扫码绑定您的账户，实现移动端远程管理 Claude Code 会话。
        </p>
      </div>

      {/* Bento Grid */}
      <div className="grid grid-cols-12 gap-6">
        {/* Connection Status Card */}
        <div className="col-span-12 lg:col-span-8">
          <div className="bg-surface-container border border-outline-variant rounded-lg overflow-hidden">
            {/* Green gradient line */}
            <div className="h-1 bg-gradient-to-r from-secondary/50 to-transparent" />

            <div className="p-6">
              <div className="flex items-center justify-between mb-6">
                <h3 className="text-[20px] font-semibold text-primary">连接状态</h3>
                <span className={`flex items-center gap-1.5 px-2.5 py-1 rounded border font-mono text-[11px] font-medium ${
                  connected
                    ? 'bg-secondary/10 border-secondary/30 text-secondary'
                    : 'bg-on-surface-variant/10 border-on-surface-variant/30 text-on-surface-variant'
                }`}>
                  <span className={`w-1.5 h-1.5 rounded-full ${connected ? 'bg-secondary animate-pulse' : 'bg-on-surface-variant'}`} />
                  {connected ? '已连接' : '未连接'}
                </span>
              </div>

              {connected ? (
                <>
                  {/* Profile section */}
                  <div className="flex items-center gap-4 mb-6 p-4 bg-surface-container-low rounded-lg border border-outline-variant">
                    <div className="w-12 h-12 rounded-full bg-secondary/20 flex items-center justify-center">
                      <span className="material-symbols-outlined text-secondary">person</span>
                    </div>
                    <div>
                      <p className="text-[14px] font-semibold text-primary">
                        {status.bot_name || '微信助手'}
                      </p>
                      <p className="font-mono text-[12px] text-on-surface-variant">
                        {status.wxid || '未知'}
                      </p>
                      {status.masked_token && (
                        <p className="font-mono text-[11px] text-on-surface-variant/60 mt-0.5">
                          Token: {status.masked_token}
                        </p>
                      )}
                    </div>
                  </div>

                  {/* Detail grid */}
                  <div className="grid grid-cols-2 lg:grid-cols-3 gap-4 mb-6">
                    <div className="p-3 bg-surface-container-low rounded-lg">
                      <p className="font-mono text-[11px] text-on-surface-variant mb-1">登录时间</p>
                      <p className="text-[14px] text-on-surface">
                        {status.login_time ? new Date(status.login_time).toLocaleString('zh-CN', { hour12: false }) : '—'}
                      </p>
                    </div>
                    <div className="p-3 bg-surface-container-low rounded-lg">
                      <p className="font-mono text-[11px] text-on-surface-variant mb-1">最后消息</p>
                      <p className="text-[14px] text-on-surface">
                        {status.last_msg_time ? new Date(status.last_msg_time).toLocaleString('zh-CN', { hour12: false }) : '—'}
                      </p>
                    </div>
                    <div className="p-3 bg-surface-container-low rounded-lg">
                      <p className="font-mono text-[11px] text-on-surface-variant mb-1">消息通道</p>
                      <p className="text-[14px] text-on-surface">
                        剩余 {status.send_budget ?? '—'} / {status.budget_limit ?? '—'}
                      </p>
                    </div>
                    <div className="p-3 bg-surface-container-low rounded-lg">
                      <p className="font-mono text-[11px] text-on-surface-variant mb-1">缓存队列</p>
                      <p className="text-[14px] text-on-surface">
                        {status.buffer_mode
                          ? status.buffered_count != null ? status.buffered_count + ' 条' : '缓冲中'
                          : status.buffered_count != null ? status.buffered_count + ' 条' : '正常'}
                      </p>
                    </div>
                    <div className="p-3 bg-surface-container-low rounded-lg">
                      <p className="font-mono text-[11px] text-on-surface-variant mb-1">下次登录提醒</p>
                      <p className="text-[14px] text-on-surface">
                        {status.next_reminder_time ? formatCountdown(status.next_reminder_time) : '—'}
                      </p>
                    </div>
                  </div>

                  {/* Disconnect button */}
                  <button
                    onClick={handleDisconnect}
                    className="flex items-center gap-2 px-4 py-2 rounded border border-error/30 text-error bg-error/5 hover:bg-error/10 transition-colors text-[14px]"
                  >
                    <span className="material-symbols-outlined text-[18px]">link_off</span>
                    解除绑定
                  </button>
                </>
              ) : (
                <div className="py-8 text-center">
                  <span className="material-symbols-outlined text-on-surface-variant/30 text-[48px] mb-3 block">link_off</span>
                  <p className="text-on-surface-variant text-[14px]">微信未连接</p>
                  <p className="text-on-surface-variant/60 text-[12px] mt-1">请使用右侧二维码完成绑定</p>
                </div>
              )}
            </div>
          </div>
        </div>

        {/* New Binding Card */}
        <div className="col-span-12 lg:col-span-4">
          <div className="bg-surface-container border border-outline-variant rounded-lg p-6">
            <div className="flex items-center justify-between mb-6">
              <h3 className="text-[20px] font-semibold text-primary">新绑定</h3>
              <span className="flex items-center gap-1.5 px-2.5 py-1 rounded border font-mono text-[11px] font-medium bg-tertiary/10 border-tertiary/30 text-tertiary">
                <span className="w-1.5 h-1.5 rounded-full bg-tertiary" />
                待命
              </span>
            </div>

            {/* QR Code area */}
            <div className={`relative mb-6 ${connected ? 'opacity-50 cursor-not-allowed' : ''}`}>
              <div className="aspect-square bg-surface-container-lowest rounded-lg border-2 border-dashed border-outline-variant flex items-center justify-center overflow-hidden">
                {qrcodeDataUrl ? (
                  <img src={qrcodeDataUrl} alt="QR Code" className="w-full h-full p-4" />
                ) : (
                  <div className="flex flex-col items-center gap-3 text-on-surface-variant/40">
                    <span className="material-symbols-outlined text-[48px]">qr_code_2</span>
                    <span className="text-[13px]">点击下方按钮获取二维码</span>
                  </div>
                )}
              </div>
              {connected && (
                <div className="absolute inset-0 flex items-center justify-center bg-surface-container/60 rounded-lg">
                  <p className="text-[13px] text-on-surface-variant">请先断开当前连接</p>
                </div>
              )}
            </div>

            {/* Get QR button */}
            {!connected && (
              <button
                onClick={handleGetQRCode}
                disabled={loading}
                className="w-full py-2.5 px-4 bg-secondary/10 border border-secondary/30 rounded text-secondary hover:bg-secondary/20 transition-colors text-[14px] font-medium flex items-center justify-center gap-2 mb-6 disabled:opacity-50"
              >
                {loading ? (
                  <span className="material-symbols-outlined text-[18px] animate-spin">progress_activity</span>
                ) : (
                  <span className="material-symbols-outlined text-[18px]">qr_code_scanner</span>
                )}
                {loading ? '获取中...' : '获取二维码'}
              </button>
            )}

            {/* Binding steps */}
            <div>
              <p className="font-mono text-[11px] text-on-surface-variant mb-3">绑定流程</p>
              <ol className="space-y-2.5">
                {[
                  { icon: 'smartphone', text: '打开微信' },
                  { icon: 'qr_code_scanner', text: '扫描二维码' },
                  { icon: 'check_circle', text: '确认绑定' },
                ].map((step, i) => (
                  <li key={i} className="flex items-center gap-3 text-[13px] text-on-surface-variant">
                    <span className="w-6 h-6 rounded-full bg-surface-container-high flex items-center justify-center text-[11px] font-mono text-on-surface-variant">
                      {i + 1}
                    </span>
                    <span className="material-symbols-outlined text-[16px]">{step.icon}</span>
                    {step.text}
                  </li>
                ))}
              </ol>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
