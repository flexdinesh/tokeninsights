import { expect, it } from 'vitest'
import { categoricalChartColor } from './UsageChart'

it('assigns a distinct color to every displayed category', () => {
  const colors = Array.from({ length: 12 }, (_, index) => categoricalChartColor(index))

  expect(new Set(colors).size).toBe(12)
  expect(categoricalChartColor(12)).toBe(colors[0])
})
