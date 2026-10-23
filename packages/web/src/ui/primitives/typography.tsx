import { cva } from 'class-variance-authority'
import type { VariantProps } from 'class-variance-authority'
import type { ComponentProps } from 'react'
import { cn } from '../cn'

const headingVariants = cva('', {
  variants: {
    variant: {
      /** Page title on startup, login and empty-route screens. */
      page: 'text-page leading-tight font-semibold tracking-tight',
      /** Section title: chart, table, settings sections, review headings. */
      section: 'text-title font-semibold tracking-heading',
      /** Sub-section title inside a section or overlay. */
      sub: 'text-body font-semibold',
      /** Current-view label in the page header. */
      view: 'text-label leading-tight font-semibold tracking-tight text-ink-subtle',
    },
  },
  defaultVariants: { variant: 'section' },
})

type HeadingLevel = 'h1' | 'h2' | 'h3'

function Heading({
  as: Level = 'h2',
  variant,
  className,
  ...props
}: ComponentProps<'h2'> & VariantProps<typeof headingVariants> & { as?: HeadingLevel }) {
  return (
    <Level data-slot="heading" className={cn(headingVariants({ variant }), className)} {...props} />
  )
}

const textVariants = cva('', {
  variants: {
    variant: {
      body: 'text-body',
      label: 'text-label',
      muted: 'text-label text-ink-subtle',
      secondary: 'text-body text-ink-muted',
      danger: 'text-label text-status-danger',
    },
    numeric: { true: 'tabular-nums' },
  },
  defaultVariants: { variant: 'body' },
})

type TextElement = 'p' | 'span' | 'div'

function Text({
  as: Element = 'p',
  variant,
  numeric,
  className,
  ...props
}: ComponentProps<'p'> & VariantProps<typeof textVariants> & { as?: TextElement }) {
  return (
    <Element
      data-slot="text"
      className={cn(textVariants({ variant, numeric }), className)}
      {...props}
    />
  )
}

export { Heading, headingVariants, Text, textVariants }
