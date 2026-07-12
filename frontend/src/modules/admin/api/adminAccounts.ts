import {
  type AdminAccount,
  type DeleteAdminAccountResponse,
  type WorkspaceDeleteConfirmation,
} from '../types/adminAccounts'
import {
  authUnauthorizedErrorKey,
  getAccessToken,
  handleAuthExpired,
  isUnauthorizedApiResponse,
} from '@/modules/auth/api/auth'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'

const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

const authHeaders = (): HeadersInit => {
  const token = getAccessToken()
  if (!token) return {}
  return { Authorization: `Bearer ${token}` }
}

type AdminErrorPayload = {
  message?: string
}

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...authHeaders(),
        ...(options.headers ?? {}),
      },
    })
  } catch {
    throw new Error('admin.adminAccounts.errors.network')
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) as T & AdminErrorPayload : ({} as T & AdminErrorPayload)

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }

    const key = payload.message || 'admin.adminAccounts.errors.request'
    throw new Error(key)
  }

  return payload
}

export const listAdminAccounts = async (): Promise<AdminAccount[]> =>
  requestJson<AdminAccount[]>('/admin-accounts')

export const getCurrentAdminAccount = async (): Promise<AdminAccount> =>
  requestJson<AdminAccount>('/admin-accounts/current')

export const switchAdminAccount = async (id: string): Promise<AdminAccount> =>
  requestJson<AdminAccount>('/admin-accounts/current', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })

export const updateAdminAccount = async (id: string, displayName: string): Promise<AdminAccount> =>
  requestJson<AdminAccount>(`/admin-accounts/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ displayName }),
  })

export const deleteAdminAccount = async (
  id: string,
  confirmation: WorkspaceDeleteConfirmation,
): Promise<DeleteAdminAccountResponse> =>
  requestJson<DeleteAdminAccountResponse>(`/admin-accounts/${id}`, {
    method: 'DELETE',
    body: JSON.stringify({ confirmation }),
  })
