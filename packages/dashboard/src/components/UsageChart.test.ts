import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { expect, it } from 'vitest'
import { z } from 'zod'
import { categoricalChartColor, formatTokenShare } from './UsageChart'

// Shared with packages/core/dashboardview so native shares stay identical.
const { tokenShare } = z
  .object({
    tokenShare: z.array(
      z.strictObject({ tokens: z.number(), total: z.number(), expected: z.string() }),
    ),
  })
  .parse(
    JSON.parse(
      readFileSync(
        join(import.meta.dirname, '../../../contracts/fixtures/dashboard-view/format.json'),
        'utf8',
      ),
    ),
  )

it('assigns a distinct color to every displayed category', () => {
  const colors = Array.from({ length: 12 }, (_, index) => categoricalChartColor(index))

  expect(new Set(colors).size).toBe(12)
  expect(categoricalChartColor(12)).toBe(colors[0])
})

it('rounds shares of the full filtered total and keeps tiny nonzero usage distinguishable', () => {
  for (const { tokens, total, expected } of tokenShare) {
    expect(formatTokenShare(tokens, total), `${tokens}/${total}`).toBe(expected)
  }
})
