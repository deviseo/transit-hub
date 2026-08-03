import assert from 'node:assert/strict'
import test from 'node:test'

import * as dashboard from '../src/modules/admin/utils/dashboard.ts'

function requireFunction(name) {
  assert.equal(typeof dashboard[name], 'function', `${name} must be exported`)
  return dashboard[name]
}

test('calculates an exact margin when revenue and cost are complete', () => {
  const calculateProfitMargin = requireFunction('calculateProfitMargin')

  const result = calculateProfitMargin({
    revenue: 301.69,
    netProfit: 147.10,
    costComplete: true,
    confirmedCost: 154.59,
    collectedSites: 23,
  })

  assert.equal(result.mode, 'exact')
  assert.ok(Math.abs(result.value - (147.10 / 301.69 * 100)) < 1e-12)
})

test('returns a ceiling and never a formal delta when only 20 of 23 sites are available', () => {
  const calculateProfitMargin = requireFunction('calculateProfitMargin')

  const result = calculateProfitMargin({
    revenue: 301.69,
    netProfit: null,
    costComplete: false,
    confirmedCost: 154.59,
    collectedSites: 20,
  })

  assert.equal(result.mode, 'ceiling')
  assert.ok(Math.abs(result.value - ((301.69 - 154.59) / 301.69 * 100)) < 1e-12)
})

test('returns unavailable instead of 100 percent when all site costs are unavailable', () => {
  const calculateProfitMargin = requireFunction('calculateProfitMargin')

  assert.deepEqual(calculateProfitMargin({
    revenue: 301.69,
    netProfit: null,
    costComplete: false,
    confirmedCost: 0,
    collectedSites: 0,
  }), {
    value: null,
    mode: 'unavailable',
  })
})

test('keeps a missing profit as null in the margin series', () => {
  const buildProfitMarginSeries = requireFunction('buildProfitMarginSeries')

  const series = buildProfitMarginSeries(
    [
      { label: '8/2', date: '2026-08-02', value: 200 },
      { label: '8/3', date: '2026-08-03', value: 301.69 },
    ],
    [
      { label: '8/2', date: '2026-08-02', value: 88.4 },
      { label: '8/3', date: '2026-08-03', value: null },
    ],
  )

  assert.deepEqual(series, [
    { label: '8/2', date: '2026-08-02', value: 44.2 },
    { label: '8/3', date: '2026-08-03', value: null },
  ])
  assert.equal(dashboard.computeDelta(series).unavailable, true)
})

test('marks a one-point series as unavailable instead of a flat zero delta', () => {
  assert.deepEqual(dashboard.computeDelta([
    { label: '8/3', date: '2026-08-03', value: 48.8 },
  ]), {
    amount: 0,
    direction: 'flat',
    unavailable: true,
    reason: 'missing_data',
  })
})
