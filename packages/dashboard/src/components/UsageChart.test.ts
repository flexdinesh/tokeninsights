import { expect, it } from 'vitest'
import { categoricalChartColor, formatTokenShare } from './UsageChart'

it('assigns a distinct color to every displayed category', () => {
  const colors = Array.from({ length: 12 }, (_, index) => categoricalChartColor(index))

  expect(new Set(colors).size).toBe(12)
  expect(categoricalChartColor(12)).toBe(colors[0])
})

it('uses the full filtered token total even when the chart shows only top groups', () => {
  expect(formatTokenShare(300, 1000)).toBe('30%')
  expect(formatTokenShare(200, 1000)).toBe('20%')
  expect(formatTokenShare(300, 300)).toBe('100%')
})

it('rounds shares and keeps tiny nonzero usage distinguishable from zero', () => {
  expect(formatTokenShare(1, 3)).toBe('33.3%')
  expect(formatTokenShare(1, 10000)).toBe('<0.1%')
  expect(formatTokenShare(0, 1000)).toBe('0%')
  expect(formatTokenShare(0, 0)).toBe('0%')
})
