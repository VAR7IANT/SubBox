import { useEffect, useRef, useState, type FormEvent } from 'react'
import type { NodeFixture } from './types'

export type PortEditValues = Pick<NodeFixture, 'listen_port' | 'public_port'>

type PortField = keyof PortEditValues
type PortErrors = Partial<Record<PortField, string>>
type ChangeKind = 'none' | 'public-only' | 'listen-only' | 'both'

type ChangePortDialogProps = {
  node: NodeFixture
  onCancel: () => void
  onSave: (ports: PortEditValues) => void
}

function validatePort(value: string, label: string) {
  const trimmedValue = value.trim()

  if (!trimmedValue) {
    return `${label}不能为空，请输入 1–65535 的整数。`
  }

  if (!/^-?\d+$/.test(trimmedValue)) {
    return `${label}必须是整数，范围为 1–65535。`
  }

  const port = Number(trimmedValue)
  if (port < 1) {
    return `${label}不能小于 1。`
  }

  if (port > 65535) {
    return `${label}不能超过 65535。`
  }

  return null
}

function getErrors(listenPort: string, publicPort: string): PortErrors {
  const errors: PortErrors = {}
  const listenError = validatePort(listenPort, '监听端口 / listen_port')
  const publicError = validatePort(publicPort, '对外端口 / public_port')

  if (listenError) errors.listen_port = listenError
  if (publicError) errors.public_port = publicError

  return errors
}

function getChangeKind(node: NodeFixture, ports: PortEditValues): ChangeKind {
  const listenChanged = ports.listen_port !== node.listen_port
  const publicChanged = ports.public_port !== node.public_port

  if (listenChanged && publicChanged) return 'both'
  if (listenChanged) return 'listen-only'
  if (publicChanged) return 'public-only'
  return 'none'
}

function parsePort(value: string) {
  return Number(value.trim())
}

function CloseIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-5 w-5">
      <path d="m5 5 10 10M15 5 5 15" stroke="currentColor" strokeLinecap="round" strokeWidth="1.6" />
    </svg>
  )
}

function getSummary(node: NodeFixture, ports: PortEditValues, hasErrors: boolean) {
  if (hasErrors) {
    return {
      title: '等待有效端口',
      detail: '修正具体字段的错误后，这里会显示本次保存将影响的端口范围。',
    }
  }

  switch (getChangeKind(node, ports)) {
    case 'public-only':
      return {
        title: 'Subscription-only mapping change',
        detail: `仅修改对外映射：public_port ${node.public_port} → ${ports.public_port}。listen_port 保持 ${node.listen_port}，不改变 Sing-box listener。`,
      }
    case 'listen-only':
      return {
        title: 'Sing-box listener change',
        detail: `Sing-box listener 将从 listen_port ${node.listen_port} 改为 ${ports.listen_port}；public_port 保持 ${node.public_port}。Task 005 仅更新 Mock 状态。`,
      }
    case 'both':
      return {
        title: 'Listener + public mapping change',
        detail: `listen_port ${node.listen_port} → ${ports.listen_port}，public_port ${node.public_port} → ${ports.public_port}；本机 listener 与对外映射都会改变。Task 005 仅更新 Mock 状态。`,
      }
    case 'none':
      return {
        title: 'No change',
        detail: '两个端口都保持当前值，不会改变节点卡片或客户端 endpoint。',
      }
  }
}

export function ChangePortDialog({ node, onCancel, onSave }: ChangePortDialogProps) {
  const listenInputId = `listen-port-${node.id}`
  const publicInputId = `public-port-${node.id}`
  const listenHelpId = `${listenInputId}-help`
  const publicHelpId = `${publicInputId}-help`
  const listenErrorId = `${listenInputId}-error`
  const publicErrorId = `${publicInputId}-error`
  const dialogTitleId = `change-port-title-${node.id}`
  const firstInputRef = useRef<HTMLInputElement>(null)
  const [listenPort, setListenPort] = useState(String(node.listen_port))
  const [publicPort, setPublicPort] = useState(String(node.public_port))
  const [touched, setTouched] = useState<Partial<Record<PortField, boolean>>>({})
  const [submitted, setSubmitted] = useState(false)

  useEffect(() => {
    firstInputRef.current?.focus()

    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onCancel()
    }

    document.addEventListener('keydown', handleEscape)
    return () => document.removeEventListener('keydown', handleEscape)
  }, [onCancel])

  const errors = getErrors(listenPort, publicPort)
  const hasErrors = Boolean(errors.listen_port || errors.public_port)
  const ports = {
    listen_port: parsePort(listenPort),
    public_port: parsePort(publicPort),
  }
  const summary = getSummary(node, ports, hasErrors)
  const showListenError = (touched.listen_port || submitted) ? errors.listen_port : undefined
  const showPublicError = (touched.public_port || submitted) ? errors.public_port : undefined

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSubmitted(true)
    setTouched({ listen_port: true, public_port: true })

    if (hasErrors) return
    onSave(ports)
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-ink/35 p-4 backdrop-blur-[2px]"
      role="presentation"
      onMouseDown={(event) => {
        if (event.target === event.currentTarget) onCancel()
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={dialogTitleId}
        className="max-h-[calc(100vh-2rem)] w-full max-w-xl overflow-y-auto rounded-[28px] border border-orange-100 bg-surface p-5 shadow-[0_24px_70px_-28px_rgba(84,54,34,0.42)] sm:p-7"
        onMouseDown={(event) => event.stopPropagation()}
      >
        <div className="flex items-start justify-between gap-4">
          <div>
            <p className="text-xs font-semibold uppercase tracking-[0.16em] text-apricot-strong">Node ports</p>
            <h2 id={dialogTitleId} className="mt-2 text-xl font-semibold tracking-tight text-ink">Change Port</h2>
            <p className="mt-1.5 text-sm leading-6 text-warm-muted">编辑 {node.name} 的本机监听端口与对外映射端口。</p>
          </div>
          <button
            type="button"
            aria-label="关闭端口编辑对话框"
            onClick={onCancel}
            className="inline-flex min-h-10 min-w-10 items-center justify-center rounded-xl border border-line bg-cream text-warm-muted transition-colors hover:border-apricot/40 hover:text-ink focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apricot focus-visible:ring-offset-2 focus-visible:ring-offset-surface"
          >
            <CloseIcon />
          </button>
        </div>

        <div className="mt-5 rounded-2xl border border-apricot/25 bg-apricot-soft/70 p-4">
          <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-apricot-strong">Port relationship</p>
          {hasErrors ? (
            <p className="mt-2 text-sm font-semibold text-ink">修正端口后查看映射预览</p>
          ) : (
            <p className="mt-2 font-mono text-base font-semibold text-apricot-strong">
              Public {ports.public_port} <span className="px-1 text-warm-muted">→</span> Local {ports.listen_port}
            </p>
          )}
          <p className="mt-2 text-xs leading-5 text-warm-muted">public_port 是客户端经过 NAT / 端口转发后使用的对外端口；listen_port 是 Sing-box 本机监听端口。</p>
        </div>

        <form className="mt-5" onSubmit={handleSubmit} noValidate>
          <div className="grid gap-4 sm:grid-cols-2">
            <div>
              <label htmlFor={listenInputId} className="text-sm font-semibold text-ink">
                监听端口 <span className="font-mono text-xs font-medium text-warm-muted">/ listen_port</span>
              </label>
              <p id={listenHelpId} className="mt-1 text-xs leading-5 text-warm-muted">Sing-box 本机监听端口</p>
              <input
                ref={firstInputRef}
                id={listenInputId}
                name="listen_port"
                type="text"
                inputMode="numeric"
                value={listenPort}
                onChange={(event) => {
                  setListenPort(event.target.value)
                  setTouched((current) => ({ ...current, listen_port: true }))
                }}
                onBlur={() => setTouched((current) => ({ ...current, listen_port: true }))}
                aria-invalid={Boolean(showListenError)}
                aria-describedby={showListenError ? `${listenHelpId} ${listenErrorId}` : listenHelpId}
                className={`mt-3 block min-h-11 w-full rounded-xl border bg-cream px-3.5 py-2.5 font-mono text-sm font-semibold text-ink outline-none transition-colors placeholder:text-warm-muted/70 focus-visible:border-apricot focus-visible:ring-2 focus-visible:ring-apricot/30 ${showListenError ? 'border-red-300' : 'border-line'}`}
              />
              {showListenError ? <p id={listenErrorId} role="alert" className="mt-2 text-xs font-medium leading-5 text-red-700">{showListenError}</p> : null}
            </div>

            <div>
              <label htmlFor={publicInputId} className="text-sm font-semibold text-ink">
                对外端口 <span className="font-mono text-xs font-medium text-warm-muted">/ public_port</span>
              </label>
              <p id={publicHelpId} className="mt-1 text-xs leading-5 text-warm-muted">客户端经过 NAT / 转发后使用</p>
              <input
                id={publicInputId}
                name="public_port"
                type="text"
                inputMode="numeric"
                value={publicPort}
                onChange={(event) => {
                  setPublicPort(event.target.value)
                  setTouched((current) => ({ ...current, public_port: true }))
                }}
                onBlur={() => setTouched((current) => ({ ...current, public_port: true }))}
                aria-invalid={Boolean(showPublicError)}
                aria-describedby={showPublicError ? `${publicHelpId} ${publicErrorId}` : publicHelpId}
                className={`mt-3 block min-h-11 w-full rounded-xl border bg-cream px-3.5 py-2.5 font-mono text-sm font-semibold text-ink outline-none transition-colors placeholder:text-warm-muted/70 focus-visible:border-apricot focus-visible:ring-2 focus-visible:ring-apricot/30 ${showPublicError ? 'border-red-300' : 'border-line'}`}
              />
              {showPublicError ? <p id={publicErrorId} role="alert" className="mt-2 text-xs font-medium leading-5 text-red-700">{showPublicError}</p> : null}
            </div>
          </div>

          <div className={`mt-5 rounded-2xl border p-4 ${hasErrors ? 'border-red-200 bg-red-50/70' : 'border-line bg-cream/55'}`} aria-live="polite">
            <p className="text-[11px] font-semibold uppercase tracking-[0.14em] text-warm-muted">Change summary</p>
            <p className={`mt-2 text-sm font-semibold ${hasErrors ? 'text-red-800' : 'text-ink'}`}>{summary.title}</p>
            <p className="mt-1.5 text-xs leading-5 text-warm-muted">{summary.detail}</p>
          </div>

          <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
            <button
              type="button"
              onClick={onCancel}
              className="inline-flex min-h-11 items-center justify-center rounded-xl border border-line bg-surface px-4 py-2.5 text-sm font-semibold text-ink transition-colors hover:border-apricot/40 hover:bg-cream focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apricot focus-visible:ring-offset-2 focus-visible:ring-offset-surface"
            >
              Cancel
            </button>
            <button
              type="submit"
              className="inline-flex min-h-11 items-center justify-center rounded-xl border border-apricot bg-apricot px-4 py-2.5 text-sm font-semibold text-ink transition-colors hover:bg-[#f39b60] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apricot focus-visible:ring-offset-2 focus-visible:ring-offset-surface"
            >
              Save ports
            </button>
          </div>
          <p className="mt-3 text-center text-[11px] text-warm-muted">Mock state only · 不调用后端或 API</p>
        </form>
      </div>
    </div>
  )
}
