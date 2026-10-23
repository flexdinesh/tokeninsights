import { Search } from 'lucide-react'
import type { ComponentProps } from 'react'
import { cn } from '../cn'
import { fieldText } from './input'

/** Search input with a leading icon; the wrapper owns the field edge and focus. */
function SearchField({ className, id, ...props }: Omit<ComponentProps<'input'>, 'type'>) {
  return (
    <label
      htmlFor={id}
      data-slot="search-field"
      className={cn(
        'flex min-h-(--control-height) items-center gap-2 rounded-md border border-line-strong bg-surface-field px-3 text-ink-subtle focus-ring-within [&_svg]:size-(--icon-size) [&_svg]:shrink-0',
        className,
      )}
    >
      <Search aria-hidden="true" />
      <input
        id={id}
        type="search"
        className={cn(
          'w-full min-w-0 border-0 bg-transparent py-1 text-ink caret-action outline-none placeholder:text-ink-subtle [&::-webkit-search-cancel-button]:hidden',
          fieldText,
        )}
        {...props}
      />
    </label>
  )
}

export { SearchField }
