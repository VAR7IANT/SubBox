import { useEffect, useState, type ReactNode } from 'react'
import { QRCodeSVG } from 'qrcode.react'

const MOCK_SUBSCRIPTION_URL = 'https://example.com/sub/q8F3mXk29L'
const MOCK_INITIAL_UPDATED_AT = '2026-09-09 08:54'
const MOCK_NODE_COUNT = 3

type Feedback = {
  tone: 'success' | 'error' | 'neutral'
  message: string
}

type DialogKind = 'qr' | 'rotate' | null

type MockNode = {
  id: string
  name: string
  protocol: string
  endpoint: string
}

const mockNodes: MockNode[] = [
  { id: 'tokyo-edge', name: '东京边缘节点', protocol: 'VLESS Reality', endpoint: 'Tokyo · 443' },
  { id: 'singapore-core', name: '新加坡核心节点', protocol: 'Hysteria2', endpoint: 'Singapore · 8443' },
  { id: 'los-angeles-west', name: '洛杉矶西部节点', protocol: 'AnyTLS', endpoint: 'Los Angeles · 443' },
]

function advanceMockTimestamp(timestamp: string) {
  const [date, time] = timestamp.split(' ')
  const [hours, minutes] = time.split(':').map(Number)
  const nextMinutes = hours * 60 + minutes + 1
  const nextHours = Math.floor(nextMinutes / 60) % 24
  const nextMinute = nextMinutes % 60

  return `${date} ${String(nextHours).padStart(2, '0')}:${String(nextMinute).padStart(2, '0')}`
}

async function copyText(text: string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(text)
    return
  }

  const textArea = document.createElement('textarea')
  textArea.value = text
  textArea.setAttribute('readonly', '')
  textArea.style.position = 'fixed'
  textArea.style.opacity = '0'
  document.body.appendChild(textArea)
  textArea.select()

  const copied = document.execCommand('copy')
  document.body.removeChild(textArea)

  if (!copied) {
    throw new Error('Clipboard unavailable')
  }
}

function CopyIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-4 w-4">
      <rect x="6.5" y="6.5" width="9" height="10" rx="1.8" stroke="currentColor" strokeWidth="1.5" />
      <path d="M13.5 6.5V5.3A1.8 1.8 0 0 0 11.7 3.5H5.3a1.8 1.8 0 0 0-1.8 1.8v7.4a1.8 1.8 0 0 0 1.8 1.8h1.2" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  )
}

function QrIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-4 w-4">
      <path d="M3.5 3.5h5v5h-5zM11.5 3.5h5v5h-5zM3.5 11.5h5v5h-5z" stroke="currentColor" strokeWidth="1.5" />
      <path d="M13 12v1.5h1.5V15H16.5M11.5 16.5h2M16.5 11.5v2M11.5 11.5h1.5" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.5" />
    </svg>
  )
}

function RefreshIcon({ spinning = false }: { spinning?: boolean }) {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className={`h-4 w-4 ${spinning ? 'animate-spin' : ''}`}>
      <path d="M16 8.5A6 6 0 1 0 16.2 12" stroke="currentColor" strokeLinecap="round" strokeWidth="1.6" />
      <path d="M16 4.5v4h-4" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.6" />
    </svg>
  )
}

function RotateIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-4 w-4">
      <path d="M15.5 7.5A5.8 5.8 0 0 0 5 5.5L3.5 7M3.5 7v-3M3.5 7h3" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.5" />
      <path d="M4.5 12.5A5.8 5.8 0 0 0 15 14.5l1.5-1.5M16.5 13v3M16.5 13h-3" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.5" />
    </svg>
  )
}

function CloseIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-5 w-5">
      <path d="m5 5 10 10M15 5 5 15" stroke="currentColor" strokeLinecap="round" strokeWidth="1.6" />
    </svg>
  )
}

function ActionButton({
  children,
  onClick,
  variant = 'soft',
  disabled = false,
  ariaLabel,
}: {
  children: ReactNode
  onClick: () => void
  variant?: 'soft' | 'outline' | 'danger'
  disabled?: boolean
  ariaLabel: string
}) {
  const variantClass = {
    soft: 'border-apricot/20 bg-apricot-soft text-apricot-strong hover:border-apricot/40 hover:bg-[#ffe8d4]',
    outline: 'border-line bg-surface text-ink hover:border-apricot/40 hover:bg-cream',
    danger: 'border-[#e6b5a5] bg-[#fff5f1] text-[#aa5d46] hover:border-[#d98e76] hover:bg-[#ffebe4]',
  }[variant]

  return (
    <button
      type="button"
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={onClick}
      className={`inline-flex min-h-10 items-center justify-center gap-2 rounded-xl border px-3.5 py-2 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apricot focus-visible:ring-offset-2 focus-visible:ring-offset-surface disabled:cursor-wait disabled:opacity-65 ${variantClass}`}
    >
      {children}
    </button>
  )
}

function LocalQrCode({ value }: { value: string }) {
  return (
    <div className="flex aspect-square w-full max-w-[220px] items-center justify-center rounded-[26px] border border-[#f2ddcb] bg-[#fffaf4] p-4 shadow-[0_16px_34px_-28px_rgba(166,92,42,0.75)]">
      <QRCodeSVG
        value={value}
        size={220}
        level="M"
        marginSize={4}
        fgColor="#27313a"
        bgColor="#ffffff"
        role="img"
        aria-label="订阅 URL 二维码"
        title="订阅 URL 二维码"
        className="h-full w-full max-w-[220px] rounded-xl"
      />
    </div>
  )
}

function DialogFrame({
  title,
  description,
  onClose,
  children,
}: {
  title: string
  description: string
  onClose: () => void
  children: ReactNode
}) {
  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-[#4a3529]/25 p-4 backdrop-blur-[2px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="subscription-dialog-title"
        aria-describedby="subscription-dialog-description"
        className="max-h-[calc(100vh-2rem)] w-full max-w-[520px] overflow-y-auto rounded-[28px] border border-orange-100 bg-surface p-5 shadow-[0_28px_70px_-30px_rgba(75,47,30,0.45)] sm:p-7"
      >
        <div className="flex items-start justify-between gap-5">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-apricot-strong">Subscription</p>
            <h2 id="subscription-dialog-title" className="mt-2 text-xl font-semibold tracking-tight text-ink">
              {title}
            </h2>
            <p id="subscription-dialog-description" className="mt-2 text-sm leading-6 text-warm-muted">
              {description}
            </p>
          </div>
          <button
            type="button"
            aria-label="关闭对话框"
            onClick={onClose}
            className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-line text-warm-muted transition-colors hover:border-apricot/40 hover:bg-cream hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apricot focus-visible:ring-offset-2"
          >
            <CloseIcon />
          </button>
        </div>
        <div className="mt-6">{children}</div>
      </div>
    </div>
  )
}

function SubscriptionQrDialog({
  feedback,
  onClose,
  onCopy,
}: {
  feedback: Feedback | null
  onClose: () => void
  onCopy: () => void
}) {
  return (
    <DialogFrame
      title="订阅二维码"
      description="在客户端扫描下方二维码，或复制稳定订阅地址。"
      onClose={onClose}
    >
      <div className="flex flex-col items-center gap-5">
        <LocalQrCode value={MOCK_SUBSCRIPTION_URL} />
        <p className="rounded-full bg-cream px-3 py-1.5 text-xs font-medium text-warm-muted">本地生成 · 扫码导入订阅</p>
        <div className="w-full rounded-2xl border border-line bg-cream/60 p-4">
          <p className="text-xs font-semibold text-warm-muted">Subscription URL</p>
          <p className="mt-2 break-all font-mono text-sm leading-6 text-ink">{MOCK_SUBSCRIPTION_URL}</p>
        </div>
        {feedback ? (
          <p className={`w-full rounded-xl px-3 py-2.5 text-sm ${feedback.tone === 'error' ? 'bg-[#fff3ef] text-[#a45b46]' : 'bg-success-soft text-success'}`} role="status">
            {feedback.message}
          </p>
        ) : null}
        <div className="flex w-full flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <ActionButton ariaLabel="关闭订阅二维码对话框" variant="outline" onClick={onClose}>
            关闭
          </ActionButton>
          <ActionButton ariaLabel="复制订阅 URL" onClick={onCopy}>
            <CopyIcon />
            复制
          </ActionButton>
        </div>
      </div>
    </DialogFrame>
  )
}

function RotateCredentialsDialog({
  onClose,
  onConfirm,
}: {
  onClose: () => void
  onConfirm: (node: MockNode) => void
}) {
  const [selectedNodeId, setSelectedNodeId] = useState('')
  const selectedNode = mockNodes.find((node) => node.id === selectedNodeId)

  return (
    <DialogFrame
      title="选择需要轮换的节点"
      description="轮换凭证是 node-scoped 操作，请先选择一个节点再确认。"
      onClose={onClose}
    >
      <div className="space-y-5">
        <div>
          <label htmlFor="rotate-node" className="text-sm font-semibold text-ink">
            目标节点
          </label>
          <select
            id="rotate-node"
            value={selectedNodeId}
            onChange={(event) => setSelectedNodeId(event.target.value)}
            className="mt-2 h-12 w-full rounded-xl border border-line bg-cream px-3.5 text-sm text-ink outline-none transition-colors focus:border-apricot focus:ring-2 focus:ring-apricot/20"
          >
            <option value="">请选择一个节点</option>
            {mockNodes.map((node) => (
              <option key={node.id} value={node.id}>
                {node.name} · {node.protocol}
              </option>
            ))}
          </select>
          <p className="mt-2 text-xs leading-5 text-warm-muted">必须选择且仅选择一个节点；此入口不会自动轮换全部节点。</p>
        </div>

        <div className="rounded-2xl border border-[#e9c0b2] bg-[#fff6f2] p-4">
          <p className="text-sm font-semibold text-[#9f543f]">请确认凭证变更</p>
          <p className="mt-2 text-sm leading-6 text-[#9f6655]">轮换凭证会使对应节点旧凭证失效。</p>
          <p className="mt-2 text-xs leading-5 text-warm-muted">当前为 Mock UI 流程，不会生成 UUID、Password 或 Reality Key，也不会调用 API。</p>
        </div>

        {selectedNode ? (
          <div className="rounded-2xl border border-success/15 bg-success-soft/70 p-4">
            <p className="text-xs font-semibold uppercase tracking-[0.14em] text-success">Selected node</p>
            <p className="mt-2 text-sm font-semibold text-ink">{selectedNode.name}</p>
            <p className="mt-1 text-xs text-warm-muted">{selectedNode.protocol} · {selectedNode.endpoint}</p>
          </div>
        ) : null}

        <div className="flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <ActionButton ariaLabel="取消轮换凭证" variant="outline" onClick={onClose}>
            取消
          </ActionButton>
          <ActionButton
            ariaLabel="确认所选节点的模拟凭证轮换"
            variant="danger"
            disabled={!selectedNode}
            onClick={() => {
              if (selectedNode) onConfirm(selectedNode)
            }}
          >
            <RotateIcon />
            确认轮换（模拟）
          </ActionButton>
        </div>
      </div>
    </DialogFrame>
  )
}

export function SubscriptionCard() {
  const [lastUpdated, setLastUpdated] = useState(MOCK_INITIAL_UPDATED_AT)
  const [isRefreshing, setIsRefreshing] = useState(false)
  const [dialog, setDialog] = useState<DialogKind>(null)
  const [feedback, setFeedback] = useState<Feedback | null>(null)

  useEffect(() => {
    if (!dialog) return undefined

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setDialog(null)
    }

    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [dialog])

  const handleCopy = async () => {
    setFeedback({ tone: 'neutral', message: '正在复制订阅 URL…' })

    try {
      await copyText(MOCK_SUBSCRIPTION_URL)
      setFeedback({ tone: 'success', message: '订阅 URL 已复制到剪贴板。' })
    } catch {
      setFeedback({ tone: 'error', message: '复制失败，请手动选择并复制订阅 URL。' })
    }
  }

  const handleRefresh = () => {
    if (isRefreshing) return

    setIsRefreshing(true)
    setFeedback({ tone: 'neutral', message: '正在刷新订阅内容（模拟）…' })

    window.setTimeout(() => {
      setLastUpdated((current) => advanceMockTimestamp(current))
      setIsRefreshing(false)
      setFeedback({ tone: 'success', message: '订阅内容已刷新，URL 保持不变。' })
    }, 850)
  }

  const handleRotateConfirm = (node: MockNode) => {
    setDialog(null)
    setFeedback({ tone: 'success', message: `已提交「${node.name}」的轮换模拟操作，凭证未实际变更。` })
  }

  return (
    <>
      <article aria-labelledby="subscription-card-title" className="rounded-[28px] border border-orange-100/90 bg-surface p-5 shadow-card sm:p-7 lg:p-8">
        <div className="flex flex-col gap-5">
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div>
              <div className="flex items-center gap-2 text-sm font-medium text-apricot-strong">
                <span className="h-2 w-2 rounded-full bg-apricot" aria-hidden="true" />
                Stable subscription
              </div>
              <h2 id="subscription-card-title" className="mt-2 text-2xl font-semibold tracking-tight text-ink">订阅</h2>
              <p className="mt-2 text-sm leading-6 text-warm-muted">使用下方链接在客户端中订阅节点</p>
            </div>
            <span className="inline-flex w-fit items-center rounded-full bg-apricot-soft px-3 py-1.5 text-xs font-semibold text-apricot-strong">稳定 URL</span>
          </div>

          <div className="rounded-2xl border border-line bg-cream/70 p-4 sm:p-5">
            <p className="text-xs font-semibold uppercase tracking-[0.15em] text-warm-muted">Subscription URL</p>
            <p className="mt-2 break-all font-mono text-sm leading-6 text-ink sm:text-[15px]">{MOCK_SUBSCRIPTION_URL}</p>
          </div>

          <div className="flex flex-wrap items-center gap-2.5" aria-label="订阅操作">
            <ActionButton ariaLabel="复制订阅 URL" onClick={handleCopy}>
              <CopyIcon />
              复制
            </ActionButton>
            <ActionButton ariaLabel="打开订阅二维码" variant="outline" onClick={() => setDialog('qr')}>
              <QrIcon />
              QR
            </ActionButton>
            <ActionButton ariaLabel="刷新订阅内容" variant="outline" disabled={isRefreshing} onClick={handleRefresh}>
              <RefreshIcon spinning={isRefreshing} />
              {isRefreshing ? 'Refreshing…' : 'Refresh'}
            </ActionButton>
            <ActionButton ariaLabel="打开节点凭证轮换确认" variant="danger" onClick={() => setDialog('rotate')}>
              <RotateIcon />
              轮换凭证
            </ActionButton>
          </div>

          {feedback ? (
            <p
              role="status"
              aria-live="polite"
              className={`rounded-xl px-3.5 py-3 text-sm ${feedback.tone === 'error' ? 'bg-[#fff3ef] text-[#a45b46]' : feedback.tone === 'neutral' ? 'bg-cream text-warm-muted' : 'bg-success-soft text-success'}`}
            >
              {feedback.message}
            </p>
          ) : null}

          <div className="flex flex-col gap-4 border-t border-line pt-5 sm:flex-row sm:items-end sm:justify-between">
            <dl className="grid grid-cols-2 gap-x-8 gap-y-3 sm:gap-x-12">
              <div>
                <dt className="text-xs font-medium text-warm-muted">最后更新</dt>
                <dd className="mt-1.5 text-sm font-semibold text-ink">{lastUpdated}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium text-warm-muted">节点数</dt>
                <dd className="mt-1.5 text-sm font-semibold text-ink">{MOCK_NODE_COUNT}</dd>
              </div>
            </dl>
            <p className="text-xs leading-5 text-warm-muted">Refresh 只更新订阅内容，不会轮换凭证。</p>
          </div>
        </div>
      </article>

      {dialog === 'qr' ? <SubscriptionQrDialog feedback={feedback} onClose={() => setDialog(null)} onCopy={handleCopy} /> : null}
      {dialog === 'rotate' ? <RotateCredentialsDialog onClose={() => setDialog(null)} onConfirm={handleRotateConfirm} /> : null}
    </>
  )
}
