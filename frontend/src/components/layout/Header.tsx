function ThemePlaceholderIcon() {
  return (
    <svg viewBox="0 0 20 20" fill="none" aria-hidden="true" className="h-4 w-4">
      <path
        d="M10 2.75v1.5m0 11.5v1.5M17.25 10h-1.5m-11.5 0h-1.5m12.64-5.14-1.06 1.06M4.67 15.33l-1.06 1.06m0-11.53 1.06 1.06m10.66 9.41 1.06 1.06"
        stroke="currentColor"
        strokeLinecap="round"
        strokeWidth="1.5"
      />
      <circle cx="10" cy="10" r="3.25" stroke="currentColor" strokeWidth="1.5" />
    </svg>
  )
}

type HeaderProps = {
  title: string
  description: string
}

export function Header({ title, description }: HeaderProps) {
  return (
    <header className="border-b border-line/80 bg-cream/90 backdrop-blur-sm">
      <div className="mx-auto flex max-w-[1440px] flex-col gap-6 px-5 py-6 sm:px-8 sm:py-7 md:flex-row md:items-center md:justify-between xl:px-10">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.2em] text-apricot-strong">
            SubBox / Workspace
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-0.03em] text-ink sm:text-[34px]">
            {title}
          </h1>
          <p className="mt-2 text-sm leading-6 text-warm-muted">{description}</p>
        </div>

        <div className="flex flex-wrap items-center gap-3">
          <div
            className="inline-flex items-center gap-2 rounded-full border border-success/15 bg-success-soft px-3.5 py-2 text-sm font-medium text-success"
            role="status"
            aria-label="Sing-box Running"
          >
            <span className="h-2 w-2 rounded-full bg-success" aria-hidden="true" />
            Sing-box Running
          </div>

          <button
            type="button"
            disabled
            aria-label="主题切换（即将支持）"
            title="主题切换即将支持"
            className="flex h-10 w-10 items-center justify-center rounded-full border border-line bg-surface text-warm-muted transition-colors disabled:cursor-not-allowed disabled:opacity-70"
          >
            <ThemePlaceholderIcon />
          </button>

          <div className="flex items-center gap-3 rounded-full border border-line bg-surface py-1.5 pl-1.5 pr-3">
            <span className="flex h-8 w-8 items-center justify-center rounded-full bg-apricot-soft text-xs font-bold text-apricot-strong">
              AD
            </span>
            <div className="leading-tight">
              <p className="text-sm font-semibold text-ink">admin</p>
              <p className="mt-0.5 text-[11px] text-warm-muted">管理员</p>
            </div>
          </div>
        </div>
      </div>
    </header>
  )
}
