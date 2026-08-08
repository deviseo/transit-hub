import { readFileSync } from 'node:fs'

import { afterEach, describe, expect, it, vi } from 'vitest'
import { manualProbeOnce } from '../src/modules/admin/api/connectionHealth'

const dialogSource = readFileSync(
  new URL('../src/modules/admin/components/dashboard/ManualOneTimeProbeDialog.vue', import.meta.url),
  'utf8',
)

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('manual probe cancellation', () => {
  it('passes the caller abort signal to the manual probe request', async () => {
    vi.stubGlobal('localStorage', { getItem: vi.fn().mockReturnValue(null) })
    const fetchMock = vi.fn().mockResolvedValue(new Response('[]', { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()

    await expect(manualProbeOnce('sub2api:ws1:account-1', ['gpt-5.6-sol'], controller.signal)).resolves.toEqual([])

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/connection-health/targets/sub2api%3Aws1%3Aaccount-1/manual-probe',
      expect.objectContaining({ signal: controller.signal }),
    )
  })

  it('closes and aborts even while a test is running', () => {
    const closeBody = dialogSource.match(/const close = \(\) => \{([\s\S]*?)\n\}/)?.[1] ?? ''
    expect(closeBody).toContain('cancelActiveRequest()')
    expect(closeBody).toContain("emit('close')")
    expect(closeBody).not.toContain("phase.value === 'testing'")
    expect(dialogSource).toContain('controller.signal')
  })
})
