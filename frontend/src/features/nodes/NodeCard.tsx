import { useState, type ReactNode } from 'react'
import { ChangePortDialog, type PortEditValues } from './ChangePortDialog'
import { getMockClientLink } from './mockNodes'
import { ProtocolBadge } from './ProtocolBadge'
import type { NodeFixture } from './types'

type Feedback = {
  tone: 'success' | 'neutral'
  message: string
}

type NodeCardProps = {
  node: NodeFixture
  onPortChange: (nodeId: string, ports: PortEditValues) => void
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

function EditIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-4 w-4">
      <path d="m12.8 4.2 3 3M4 16l.7-3.4L13.8 3.5a1.7 1.7 0 0 1 2.4 2.4l-9.1 9.1L4 16Z" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.45" />
    </svg>
  )
}

function PortIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-4 w-4">
      <path d="M4 6.25h12M4 13.75h12" stroke="currentColor" strokeLinecap="round" strokeWidth="1.5" />
      <circle cx="7" cy="6.25" r="1.6" fill="currentColor" />
      <circle cx="13" cy="13.75" r="1.6" fill="currentColor" />
    </svg>
  )
}

function ActionButton({
  children,
  onClick,
  variant = 'outline',
  ariaLabel,
}: {
  children: ReactNode
  onClick: () => void
  variant?: 'soft' | 'outline'
  ariaLabel: string
}) {
  const variantClass = variant === 'soft'
    ? 'border-apricot/20 bg-apricot-soft text-apricot-strong hover:border-apricot/40 hover:bg-[#ffe8d4]'
    : 'border-line bg-surface text-ink hover:border-apricot/40 hover:bg-cream'

  return (
    <button
      type="button"
      aria-label={ariaLabel}
      onClick={onClick}
      className={`inline-flex min-h-10 items-center justify-center gap-2 rounded-xl border px-3 py-2 text-sm font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apricot focus-visible:ring-offset-2 focus-visible:ring-offset-surface ${variantClass}`}
    >
      {children}
    </button>
  )
}

function StatusChip({ label, tone }: { label: string; tone: 'success' | 'apricot' | 'muted' }) {
  const toneClass = {
    success: 'border-success/15 bg-success-soft text-success',
    apricot: 'border-apricot/20 bg-apricot-soft text-apricot-strong',
    muted: 'border-line bg-cream text-warm-muted',
  }[tone]

  return (
    <span className={`inline-flex items-center gap-1.5 rounded-full border px-2.5 py-1 text-[11px] font-semibold ${toneClass}`}>
      <span className={`h-1.5 w-1.5 rounded-full ${tone === 'success' ? 'bg-success' : tone === 'apricot' ? 'bg-apricot' : 'bg-warm-muted/60'}`} aria-hidden="true" />
      {label}
    </span>
  )
}

function PortValue({ label, value, description }: { label: string; value: number; description: string }) {
  return (
    <div className="rounded-2xl border border-line bg-cream/60 p-3.5">
      <dt className="font-mono text-[11px] font-semibold text-warm-muted">{label}</dt>
      <dd className="mt-2 text-xl font-semibold tracking-tight text-ink">{value}</dd>
      <p className="mt-1 text-[11px] leading-5 text-warm-muted">{description}</p>
    </div>
  )
}

export function NodeCard({ node, onPortChange }: NodeCardProps) {
  const [feedback, setFeedback] = useState<Feedback | null>(null)
  const [isPortDialogOpen, setIsPortDialogOpen] = useState(false)
  const hasNatMapping = node.listen_port !== node.public_port
  const clientLink = getMockClientLink(node)

  const handleCopyLink = async () => {
    try {
      await copyText(clientLink)
      setFeedback({ tone: 'success', message: `客户端链接已复制：${node.host}:${node.public_port}` })
    } catch {
      setFeedback({ tone: 'neutral', message: '复制失败，请检查浏览器剪贴板权限。' })
    }
  }

  const handlePortSave = (ports: PortEditValues) => {
    const listenChanged = ports.listen_port !== node.listen_port
    const publicChanged = ports.public_port !== node.public_port
    onPortChange(node.id, ports)
    setIsPortDialogOpen(false)

    if (listenChanged && publicChanged) {
      setFeedback({ tone: 'success', message: `端口已更新：listen_port 与 public_port 均已更新。客户端 endpoint 使用 ${node.host}:${ports.public_port}。` })
    } else if (listenChanged) {
      setFeedback({ tone: 'success', message: `端口已更新：Sing-box listener 改为 ${ports.listen_port}，public_port 保持 ${ports.public_port}，客户端 endpoint 不变。` })
    } else if (publicChanged) {
      setFeedback({ tone: 'success', message: `端口已更新：仅修改对外映射为 ${ports.public_port}，listen_port 保持 ${ports.listen_port}。客户端 endpoint 已更新为 ${node.host}:${ports.public_port}。` })
    } else {
      setFeedback({ tone: 'success', message: '端口未变化，Mock 状态保持不变。' })
    }
  }

  return (
    <article aria-labelledby={`${node.id}-title`} className="flex h-full flex-col rounded-[26px] border border-orange-100/90 bg-surface p-5 shadow-card sm:p-6">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <ProtocolBadge protocol={node.protocol} />
            <span className="font-mono text-[10px] font-medium text-warm-muted">{node.protocol}</span>
          </div>
          <h3 id={`${node.id}-title`} className="mt-4 truncate text-lg font-semibold tracking-tight text-ink">{node.name}</h3>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1.5">
          <StatusChip label={node.enabled ? 'Enabled' : 'Disabled'} tone={node.enabled ? 'apricot' : 'muted'} />
          <StatusChip label={node.running ? 'Running' : 'Stopped'} tone={node.running ? 'success' : 'muted'} />
        </div>
      </div>

      <dl className="mt-5 space-y-3 border-t border-line pt-4">
        <div className="flex items-start justify-between gap-4">
          <dt className="text-xs font-medium text-warm-muted">host</dt>
          <dd className="max-w-[68%] break-all text-right font-mono text-xs font-semibold text-ink">{node.host}</dd>
        </div>
        <div>
          <dt className="text-xs font-medium text-warm-muted">protocol</dt>
          <dd className="mt-1 text-xs font-mono text-ink">{node.protocol}</dd>
        </div>
      </dl>

      <dl className="mt-5 grid grid-cols-2 gap-3">
        <PortValue label="listen_port" value={node.listen_port} description="Sing-box 本机监听" />
        <PortValue label="public_port" value={node.public_port} description="客户端使用" />
      </dl>

      <div className={`mt-4 rounded-2xl border p-4 ${hasNatMapping ? 'border-apricot/25 bg-apricot-soft/70' : 'border-line bg-cream/70'}`}>
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-warm-muted">Port mapping</p>
            <p className="mt-1 text-sm font-semibold text-ink">{hasNatMapping ? 'NAT 映射' : '同端口直连'}</p>
          </div>
          <span className="rounded-full bg-surface/80 px-2 py-1 text-[10px] font-semibold text-warm-muted">
            {hasNatMapping ? 'NAT' : 'Same port'}
          </span>
        </div>
        <p className="mt-3 font-mono text-sm font-semibold text-apricot-strong">
          Public {node.public_port} <span className="px-1 text-warm-muted">{hasNatMapping ? '→' : '='}</span> Local {node.listen_port}
        </p>
        <p className="mt-2 text-xs leading-5 text-warm-muted">
          {hasNatMapping ? '客户端连接对外端口，Sing-box 继续监听本机端口。' : '对外端口与 Sing-box 本机监听端口一致。'}
        </p>
      </div>

      <div className="mt-4 rounded-2xl border border-line bg-cream/45 px-3.5 py-3">
        <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-warm-muted">Client endpoint</p>
        <p className="mt-1 break-all font-mono text-xs font-semibold text-ink">{node.host}:{node.public_port}</p>
      </div>

      {feedback ? (
        <p role="status" aria-live="polite" className={`mt-4 rounded-xl px-3 py-2.5 text-xs leading-5 ${feedback.tone === 'success' ? 'bg-success-soft text-success' : 'bg-cream text-warm-muted'}`}>
          {feedback.message}
        </p>
      ) : null}

      <div className="mt-auto flex flex-wrap gap-2 pt-5" aria-label={`${node.name} 操作`}>
        <ActionButton ariaLabel={`复制 ${node.name} 客户端链接`} variant="soft" onClick={handleCopyLink}>
          <CopyIcon />
          Copy Link
        </ActionButton>
        <ActionButton ariaLabel={`编辑 ${node.name}（模拟入口）`} onClick={() => setFeedback({ tone: 'neutral', message: 'Edit 为 Mock 占位入口，Task 004 不打开编辑表单。' })}>
          <EditIcon />
          Edit
        </ActionButton>
        <ActionButton ariaLabel={`修改 ${node.name} 端口`} onClick={() => setIsPortDialogOpen(true)}>
          <PortIcon />
          Change Port
        </ActionButton>
      </div>

      {isPortDialogOpen ? <ChangePortDialog node={node} onCancel={() => setIsPortDialogOpen(false)} onSave={handlePortSave} /> : null}
    </article>
  )
}
