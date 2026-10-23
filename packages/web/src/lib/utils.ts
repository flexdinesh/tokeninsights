import { clsx } from 'clsx'
import type { ClassValue } from 'clsx'
import { extendTailwindMerge } from 'tailwind-merge'

// Teach tailwind-merge the TokenInsights scales so size utilities such as
// text-label are not mistaken for text colors and dropped during merges.
const twMerge = extendTailwindMerge({
  override: {
    theme: {
      text: ['label', 'body', 'title', 'subheading', 'readout', 'page'],
      'font-weight': ['normal', 'medium', 'semibold', 'bold'],
      leading: ['tight', 'normal'],
      tracking: ['tight', 'heading'],
      radius: ['sm', 'md', 'lg'],
      shadow: ['overlay'],
      breakpoint: ['compact', 'nav', 'wide'],
    },
  },
})

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
