import type { Protocol } from './types'
import { protocolLabels } from './protocol'

export function ProtocolBadge({ protocol }: { protocol: Protocol }) {
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-apricot/25 bg-apricot-soft px-3 py-1.5 text-xs font-semibold text-apricot-strong">
      <span className="h-1.5 w-1.5 rounded-full bg-apricot" aria-hidden="true" />
      {protocolLabels[protocol]}
    </span>
  )
}
