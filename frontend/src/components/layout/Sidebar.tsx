import type { ComponentType } from 'react'

export type NavigationLabel = '总览' | '节点' | '部署' | '中转' | '设置'

type IconProps = {
  className?: string
}

type NavigationItem = {
  label: NavigationLabel
  description: string
  Icon: ComponentType<IconProps>
}

function OverviewIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
      <rect x="3" y="3" width="5.5" height="5.5" rx="1.2" stroke="currentColor" strokeWidth="1.5" />
      <rect x="11.5" y="3" width="5.5" height="5.5" rx="1.2" stroke="currentColor" strokeWidth="1.5" />
      <rect x="3" y="11.5" width="5.5" height="5.5" rx="1.2" stroke="currentColor" strokeWidth="1.5" />
      <rect x="11.5" y="11.5" width="5.5" height="5.5" rx="1.2" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  )
}

function NodesIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
      <rect x="3" y="3" width="14" height="5" rx="1.3" stroke="currentColor" strokeWidth="1.5" />
      <rect x="3" y="12" width="14" height="5" rx="1.3" stroke="currentColor" strokeWidth="1.5" />
      <path d="M5.5 5.5h.01M5.5 14.5h.01" stroke="currentColor" strokeLinecap="round" strokeWidth="2" />
      <path d="M8 5.5h5M8 14.5h5" stroke="currentColor" strokeLinecap="round" strokeWidth="1.5" />
    </svg>
  )
}

function DeployIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
      <path d="M10 3v9.5m0 0 3.5-3.5M10 12.5 6.5 9" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.6" />
      <path d="M4 14.5v1.25A1.25 1.25 0 0 0 5.25 17h9.5A1.25 1.25 0 0 0 16 15.75V14.5" stroke="currentColor" strokeLinecap="round" strokeWidth="1.5" />
    </svg>
  )
}

function RelayIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
      <path d="M3.25 6.5h10.5m0 0-2.5-2.5m2.5 2.5L11.25 9" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.5" />
      <path d="M16.75 13.5H6.25m0 0 2.5-2.5m-2.5 2.5 2.5 2.5" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round" strokeWidth="1.5" />
    </svg>
  )
}

function SettingsIcon({ className }: IconProps) {
  return (
    <svg viewBox="0 0 20 20" fill="none" className={className} aria-hidden="true">
      <path d="m8.55 3.42.33-.8a1.4 1.4 0 0 1 2.58 0l.33.8 1.02.43.84-.17a1.4 1.4 0 0 1 1.5 2.1l-.49.7.43 1.02.8.33a1.4 1.4 0 0 1 0 2.58l-.8.33-.43 1.02.49.7a1.4 1.4 0 0 1-1.5 2.1l-.84-.17-1.02.43-.33.8a1.4 1.4 0 0 1-2.58 0l-.33-.8-1.02-.43-.84.17a1.4 1.4 0 0 1-1.5-2.1l.49-.7-.43-1.02-.8-.33a1.4 1.4 0 0 1 0-2.58l.8-.33.43-1.02-.49-.7a1.4 1.4 0 0 1 1.5-2.1l.84.17 1.02-.43Z" stroke="currentColor" strokeLinejoin="round" strokeWidth="1.3" />
      <circle cx="10.17" cy="9.12" r="2.25" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  )
}

const navigationItems: NavigationItem[] = [
  { label: '总览', description: '工作区概览', Icon: OverviewIcon },
  { label: '节点', description: '节点管理', Icon: NodesIcon },
  { label: '部署', description: '部署 Sing-box', Icon: DeployIcon },
  { label: '中转', description: '端口与转发', Icon: RelayIcon },
  { label: '设置', description: '基础设置', Icon: SettingsIcon },
]

type SidebarProps = {
  activeItem: NavigationLabel
  onNavigate: (label: NavigationLabel) => void
}

function LogoMark() {
  return (
    <span className="relative flex h-10 w-10 items-center justify-center rounded-[14px] bg-apricot text-lg font-bold text-white shadow-[0_8px_20px_-10px_rgba(201,115,65,0.8)]">
      <span className="absolute -right-0.5 -top-0.5 h-2.5 w-2.5 rounded-full border-2 border-surface bg-white" aria-hidden="true" />
      S
    </span>
  )
}

function Brand() {
  return (
    <div className="flex items-center gap-3">
      <LogoMark />
      <div>
        <p className="text-[17px] font-semibold tracking-[-0.02em] text-ink">SubBox</p>
        <p className="mt-0.5 text-[11px] font-medium tracking-[0.08em] text-warm-muted">SING-BOX CONTROL</p>
      </div>
    </div>
  )
}

function Navigation({ activeItem, onNavigate }: SidebarProps) {
  return (
    <nav aria-label="主导航" className="flex gap-1 lg:block lg:space-y-1">
      {navigationItems.map(({ label, description, Icon }) => {
        const isActive = label === activeItem

        return (
          <button
            key={label}
            type="button"
            aria-label={label}
            aria-pressed={isActive}
            onClick={() => onNavigate(label)}
            className={`group flex min-w-[84px] flex-1 items-center gap-3 rounded-2xl px-3 py-2.5 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-apricot focus-visible:ring-offset-2 focus-visible:ring-offset-cream lg:w-full lg:px-3.5 ${
              isActive
                ? 'bg-apricot-soft text-apricot-strong shadow-[0_8px_24px_-18px_rgba(190,105,55,0.75)]'
                : 'text-warm-muted hover:bg-cream hover:text-ink'
            }`}
          >
            <Icon className="h-[19px] w-[19px] shrink-0" />
            <span className="min-w-0 lg:block">
              <span className="block text-sm font-semibold">{label}</span>
              <span className="mt-0.5 hidden truncate text-[11px] font-medium opacity-70 lg:block">{description}</span>
            </span>
          </button>
        )
      })}
    </nav>
  )
}

function ServiceStatus() {
  return (
    <div className="rounded-2xl border border-success/10 bg-success-soft/70 p-3.5">
      <div className="flex items-center gap-2 text-xs font-semibold text-success">
        <span className="h-2 w-2 rounded-full bg-success" aria-hidden="true" />
        服务运行中
      </div>
      <p className="mt-2 text-xs leading-5 text-warm-muted">Sing-box 状态正常</p>
    </div>
  )
}

export function Sidebar({ activeItem, onNavigate }: SidebarProps) {
  return (
    <>
      <aside className="fixed inset-y-0 left-0 z-20 hidden w-[248px] flex-col border-r border-line bg-surface px-5 py-6 lg:flex">
        <Brand />
        <div className="mt-12 flex-1">
          <p className="mb-3 px-3 text-[10px] font-bold uppercase tracking-[0.22em] text-warm-muted/80">Workspace</p>
          <Navigation activeItem={activeItem} onNavigate={onNavigate} />
        </div>
        <ServiceStatus />
      </aside>

      <div className="border-b border-line bg-surface px-5 py-4 lg:hidden sm:px-8">
        <div className="flex items-center justify-between gap-4">
          <Brand />
          <div className="hidden items-center gap-2 text-xs font-medium text-success sm:flex">
            <span className="h-2 w-2 rounded-full bg-success" aria-hidden="true" />
            Running
          </div>
        </div>
        <div className="-mx-1 mt-4 overflow-x-auto pb-1">
          <Navigation activeItem={activeItem} onNavigate={onNavigate} />
        </div>
      </div>
    </>
  )
}
