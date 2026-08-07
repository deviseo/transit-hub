import type {
  AuthTokenResponse,
  LoginRequest,
} from '../types/auth'
import { resetWorkspaceCheck } from '@/lib/workspaceGuard'

export const authTokenStorageKey = 'transithub.auth.accessToken'
export const authUnauthorizedErrorKey = 'auth.errors.unauthorized'

type ApiErrorPayload = {
  message?: string
}

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'

const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

const requestJson = async <T>(path: string, options: RequestInit = {}, errorKey = 'auth.errors.unknown'): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...(options.headers ?? {}),
      },
    })
  } catch (error) {
    throw new Error('auth.errors.network')
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) as T & { message?: string } : ({} as T & { message?: string })

  if (!response.ok) {
    throw new Error(payload.message ?? errorKey)
  }

  return payload
}

export const loginWithEmail = async (form: LoginRequest): Promise<AuthTokenResponse> => (
  requestJson<AuthTokenResponse>('/auth/login', {
    method: 'POST',
    body: JSON.stringify(form),
  }, 'auth.login.errors.login')
)

export const storeAccessToken = (accessToken: string): void => {
  localStorage.setItem(authTokenStorageKey, accessToken)
}

export const getAccessToken = (): string | null => localStorage.getItem(authTokenStorageKey)

export const clearAccessToken = (): void => {
  localStorage.removeItem(authTokenStorageKey)
  // 清除 token 时同步重置 workspace 路由守卫缓存，
  // 防止下次登录（可能是不同用户）复用旧 workspace 状态。
  resetWorkspaceCheck()
}

export const isUnauthorizedApiResponse = (status: number, payload: ApiErrorPayload): boolean => (
  status === 401 || payload.message === authUnauthorizedErrorKey
)

// 登录状态过期时的统一处理：清除本地登录态并跳转登录页。
// 所有检测到 401 / authUnauthorizedErrorKey 的请求层都应调用它，
// 避免在各个页面/组件里各自重复实现跳转逻辑。
// 使用整页跳转（而非 router.push）以同时重置内存中的组件/store 状态，
// 并避免 auth.ts 与 router.ts 之间产生循环依赖。
export const handleAuthExpired = (): void => {
  clearAccessToken()
  if (typeof window === 'undefined') return
  if (window.location.pathname === '/login') return
  window.location.href = '/login'
}
