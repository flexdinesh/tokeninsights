/** TokenInsights Split T mark and wordmark in its single brand weight. */
export function Brand() {
  return (
    <div
      data-slot="brand"
      className="flex items-center gap-1.5 text-title font-bold tracking-tight whitespace-nowrap"
    >
      <svg viewBox="0 0 32 32" aria-hidden="true" className="-ml-0.5 size-5 shrink-0 fill-current">
        <rect x="4" y="4" width="10" height="6" rx="1.5" />
        <rect x="16" y="4" width="12" height="6" rx="1.5" />
        <rect x="12" y="12" width="8" height="6" rx="1.5" />
        <rect x="12" y="20" width="8" height="8" rx="1.5" className="fill-action" />
      </svg>
      <span>TokenInsights</span>
    </div>
  )
}
