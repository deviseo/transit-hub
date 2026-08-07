import { afterEach, describe, expect, it, vi } from 'vitest'
import * as authAPI from '../src/modules/auth/api/auth'

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('loginWithEmail', () => {
  it('sends credentials only to the real email-password login endpoint', async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({
      strategy: 'login',
      subject: 'user@example.com',
      accessToken: 'session-token',
    }), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    await expect(authAPI.loginWithEmail({
      email: 'user@example.com',
      password: 'secret',
    })).resolves.toEqual({
      strategy: 'login',
      subject: 'user@example.com',
      accessToken: 'session-token',
    })

    expect(fetchMock).toHaveBeenCalledWith('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ email: 'user@example.com', password: 'secret' }),
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
      },
    })
  })

  it('surfaces the backend error key for rejected credentials', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(JSON.stringify({
      message: 'auth.errors.invalidCredentials',
    }), { status: 401 })))

    await expect(authAPI.loginWithEmail({
      email: 'user@example.com',
      password: 'wrong-password',
    })).rejects.toThrow('auth.errors.invalidCredentials')
  })
})

it('does not expose removed public registration requests', () => {
  expect(authAPI).not.toHaveProperty('requestEmailCode')
  expect(authAPI).not.toHaveProperty('registerWithEmail')
})
