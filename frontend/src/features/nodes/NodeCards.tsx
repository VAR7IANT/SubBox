import { NodeCard } from './NodeCard'
import { mockNodes } from './mockNodes'

type NodeCardsProps = {
  layout?: 'overview' | 'page'
}

export function NodeCards({ layout = 'page' }: NodeCardsProps) {
  const isOverview = layout === 'overview'

  return (
    <section aria-labelledby="nodes-section-title">
      <div className="mb-4 flex items-end justify-between gap-4">
        <div>
          <p className="text-xs font-semibold uppercase tracking-[0.16em] text-apricot-strong">Nodes</p>
          <h2 id="nodes-section-title" className="mt-1.5 text-xl font-semibold tracking-tight text-ink">节点</h2>
          <p className="mt-1.5 text-sm leading-6 text-warm-muted">客户端使用 public_port，Sing-box 本机监听 listen_port。</p>
        </div>
        <span className="hidden rounded-full bg-cream px-3 py-1.5 text-xs font-semibold text-warm-muted sm:inline-flex">{mockNodes.length} 个节点</span>
      </div>
      <div className={isOverview ? 'space-y-4' : 'grid gap-5 md:grid-cols-2 xl:grid-cols-3'}>
        {mockNodes.map((node) => <NodeCard key={node.id} node={node} />)}
      </div>
    </section>
  )
}
