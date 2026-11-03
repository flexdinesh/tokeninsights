import { describe, expect, it } from 'vitest'
import { cn } from './cn'

describe('cn', () => {
  it('keeps token type sizes beside text colors', () => {
    expect(cn('text-label', 'text-ink-muted')).toBe('text-label text-ink-muted')
  })

  it('resolves conflicts within the token type scale', () => {
    expect(cn('text-body', 'text-label')).toBe('text-label')
  })

  it('resolves conflicts within token colors', () => {
    expect(cn('bg-surface', 'bg-action')).toBe('bg-action')
  })

  it('keeps token radius and shadow utilities distinct', () => {
    expect(cn('rounded-lg shadow-overlay', 'rounded-sm')).toBe('shadow-overlay rounded-sm')
  })
})
