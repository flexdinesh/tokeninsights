import { describe, expect, it } from 'vitest'
import { dashboardSchema } from '@tokeninsights/contracts'
import { mockDashboard, mockTabs } from './data'

describe('mock API data', () => {
  it('provides every dashboard view through generated contracts', () => {
    for (const tab of mockTabs) {
      expect(dashboardSchema.safeParse(mockDashboard(tab)).success).toBe(true)
      expect(mockDashboard(tab).rows.length).toBeGreaterThan(0)
    }
  })

  it('clamps pagination to available mock rows', () => {
    const dashboard = mockDashboard('sessions', '999', '1')
    expect(dashboard.page).toBe(dashboard.rowCount)
    expect(dashboard.rows).toHaveLength(1)
  })
})
