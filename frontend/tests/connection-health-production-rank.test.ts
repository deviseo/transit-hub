import { readFileSync } from 'node:fs'

import { describe, expect, it } from 'vitest'

const detailSource = readFileSync(
  new URL('../src/modules/admin/components/dashboard/AdminGroupHealthDetail.vue', import.meta.url),
  'utf8',
)
const viewSource = readFileSync(
  new URL('../src/modules/admin/views/ConnectionHealthView.vue', import.meta.url),
  'utf8',
)

describe('connection health production rank display', () => {
  it('uses the backend global account rank for the default detail order', () => {
    expect(detailSource).toContain('productionSortOrder')
    expect(detailSource).toContain('!customSortActive.value')
  })

  it('orders the group overview by the minimum global rank', () => {
    expect(viewSource).toContain('minProductionRank')
  })

  it('does not expose the removed capacity warning', () => {
    expect(detailSource).not.toContain('priorityCapacityLimited')
  })
})
