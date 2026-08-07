import { describe, expect, it } from 'vitest'

import { preservePriorityMaxPendingAge } from '../src/modules/admin/utils/connectionHealthPolicy'

describe('connection health priority sync preset', () => {
  it('preserves a hidden non-default max pending age during ordinary edits', () => {
    expect(preservePriorityMaxPendingAge(900, 30)).toBe(900)
  })

  it('keeps max pending age valid when the write interval is raised', () => {
    expect(preservePriorityMaxPendingAge(300, 600)).toBe(600)
  })

  it('uses the B-phase default for legacy policies without the field', () => {
    expect(preservePriorityMaxPendingAge(undefined, 30)).toBe(300)
  })
})
