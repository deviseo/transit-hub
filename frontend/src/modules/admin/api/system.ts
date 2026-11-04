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
  } catch (error) {
    throw new Error('admin.system.errors.network')
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) as T & AdminErrorPayload : ({} as T & AdminErrorPayload)

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.system.errors.request')
  }

  return payload
}

export interface SystemVersionResponse {
  version: string
}

export const getSystemVersion = async (): Promise<SystemVersionResponse> => (
  requestJson<SystemVersionResponse>('/system/version')
)

/** 不能在线更新的原因：不是系统管理员、部署环境不支持、这个版本没有更新包。 */
export type SystemUpdateBlocker = 'notAdmin' | 'unsupported' | 'noPackage'

/** 检查更新的结果：对比当前版本和 GitHub 上最新的正式版，并说明能不能在线更新。 */
export interface SystemUpdateCheck {
  current: string
  latest: string
  hasUpdate: boolean
  notes: string
  publishedAt: string | null
  releaseUrl: string
  checkedAt: string
  /** 可以在线更新（有新版本、是系统管理员、部署环境支持、Release 里有更新包）。 */
  canUpdate: boolean
  reason?: SystemUpdateBlocker
  /** 数据目录是持久卷（在线更新的版本和凭据密钥重建容器后还在）。 */
  persistent: boolean
  /** 保存的登录凭据是加密的。 */
  credentialsEncrypted: boolean
}

/** 检查更新；force 为真时不用服务端缓存（服务端默认缓存 10 分钟）。 */
export const checkSystemUpdate = async (force = false): Promise<SystemUpdateCheck> => (
  requestJson<SystemUpdateCheck>(`/system/update${force ? '?force=1' : ''}`)
)

/** 开始在线更新：服务端在后台下载、校验并安装最新版本，然后重启到新版本。返回目标版本。 */
export const applySystemUpdate = async (): Promise<{ version: string }> => (
  requestJson<{ version: string }>('/system/update', { method: 'POST' })
)

/** 在线更新的进度。服务重启后从 idle 重新开始。 */
export type SystemUpdateStatus = 'idle' | 'downloading' | 'installing' | 'restarting' | 'failed'

export interface SystemUpdateProgress {
  /** 正在运行的版本。 */
  version: string
  status: SystemUpdateStatus
  target?: string
  downloaded: number
  total: number
  error?: string
}

export const getSystemUpdateProgress = async (): Promise<SystemUpdateProgress> => (
  requestJson<SystemUpdateProgress>('/system/update/progress')
)
