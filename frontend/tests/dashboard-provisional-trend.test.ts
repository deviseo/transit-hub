import { describe, expect, it } from 'vitest'
import { selectDashboardTrendValue } from '@/modules/admin/utils/dashboard'

describe('dashboard provisional trend values', () => {
  it('uses confirmed cost for a partial day without changing the formal field', () => {
    expect(selectDashboardTrendValue({
      formalValue: null,
      provisionalValue: 256.04,
      status: 'partial',
      provisionalQuality: 'confirmed',
    })).toEqual({ value: 256.04, quality: 'confirmed' })
  })

  it('uses the provisional profit ceiling for a partial day', () => {
    expect(selectDashboardTrendValue({
      formalValue: null,
      provisionalValue: 227.93,
      status: 'partial',
      provisionalQuality: 'ceiling',
    })).toEqual({ value: 227.93, quality: 'ceiling' })
  })

  it('keeps unavailable data as a chart gap instead of zero', () => {
    expect(selectDashboardTrendValue({
      formalValue: null,
      provisionalValue: null,
      status: 'unavailable',
      provisionalQuality: 'ceiling',
    })).toEqual({ value: null, quality: 'unavailable' })
  })
})
