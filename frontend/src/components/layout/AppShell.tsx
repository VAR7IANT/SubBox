import type { NavigationLabel } from './Sidebar'
import { Header } from './Header'
import { Sidebar } from './Sidebar'

type AppShellProps = {
  activeItem: NavigationLabel
  onNavigate: (label: NavigationLabel) => void
  pageTitle: string
  pageDescription: string
}

function AreaPlaceholder({
  eyebrow,
  title,
  description,
}: {
  eyebrow: string
  title: string
  description: string
}) {
  return (
    <div className="rounded-[26px] border border-dashed border-line bg-surface/75 p-6 sm:p-7">
      <div className="flex items-start gap-4">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-2xl bg-apricot-soft text-apricot-strong">
          <span className="h-2.5 w-2.5 rounded-full bg-apricot" aria-hidden="true" />
        </div>
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-warm-muted">
            {eyebrow}
          </p>
          <h2 className="mt-2 text-lg font-semibold tracking-tight text-ink">{title}</h2>
          <p className="mt-2 text-sm leading-6 text-warm-muted">{description}</p>
        </div>
      </div>
    </div>
  )
}

function MainSection({ activeItem }: { activeItem: NavigationLabel }) {
  const isOverview = activeItem === '总览'

  return (
    <main className="mx-auto w-full max-w-[1440px] px-5 pb-10 pt-6 sm:px-8 sm:pt-8 xl:px-10">
      <section aria-labelledby="workspace-title">
        <div className="flex flex-col gap-4 rounded-[28px] border border-orange-100/80 bg-surface p-6 shadow-card sm:flex-row sm:items-center sm:justify-between sm:p-8">
          <div className="max-w-2xl">
            <div className="flex items-center gap-2 text-sm font-medium text-success">
              <span className="h-2 w-2 rounded-full bg-success" aria-hidden="true" />
              {isOverview ? '工作区已准备就绪' : `${activeItem} 页面骨架`}
            </div>
            <h2 id="workspace-title" className="mt-4 text-2xl font-semibold tracking-tight text-ink sm:text-[28px]">
              {isOverview ? '轻松管理你的 SubBox 工作区' : `${activeItem} 功能即将加入`}
            </h2>
            <p className="mt-3 text-sm leading-7 text-warm-muted sm:text-base">
              {isOverview
                ? '这里将集中呈现订阅与节点管理功能，保持清晰、轻量的操作体验。'
                : '当前仅提供页面布局，具体操作会在后续任务中逐步加入。'}
            </p>
          </div>
          <div className="inline-flex w-fit items-center rounded-full bg-cream px-4 py-2 text-xs font-medium text-warm-muted">
            Task 002
          </div>
        </div>
      </section>

      <section aria-label="后续功能区域" className="mt-6 grid gap-5 md:grid-cols-2">
        <AreaPlaceholder
          eyebrow="Coming next"
          title="订阅区域"
          description="为稳定订阅 URL 与相关操作保留布局位置。"
        />
        <AreaPlaceholder
          eyebrow="Coming next"
          title="节点区域"
          description="为节点列表与 NAT 端口关系保留布局位置。"
        />
      </section>
    </main>
  )
}

export function AppShell({
  activeItem,
  onNavigate,
  pageTitle,
  pageDescription,
}: AppShellProps) {
  return (
    <div className="min-h-screen bg-cream text-ink">
      <Sidebar activeItem={activeItem} onNavigate={onNavigate} />
      <div className="min-h-screen lg:pl-[248px]">
        <Header title={pageTitle} description={pageDescription} />
        <MainSection activeItem={activeItem} />
      </div>
    </div>
  )
}
