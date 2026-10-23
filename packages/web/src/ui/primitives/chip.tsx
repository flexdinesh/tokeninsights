import { X } from 'lucide-react'
import type { ReactNode } from 'react'
import { Button } from './button'

/** Active filter: dimension, value and a remove action in the selected treatment. */
function Chip({
  dimension,
  value,
  title,
  removeLabel,
  onRemove,
}: {
  dimension?: ReactNode
  value: ReactNode
  title?: string
  removeLabel?: string
  onRemove: () => void
}) {
  return (
    <Button
      variant="secondary"
      size="compact"
      selected
      className="max-w-full"
      aria-label={removeLabel}
      onClick={onRemove}
    >
      {dimension && <span className="text-ink-subtle">{dimension}</span>}
      <span className="max-w-(--chip-value-width) min-w-0 truncate" title={title}>
        {value}
      </span>
      <X aria-hidden="true" />
    </Button>
  )
}

export { Chip }
