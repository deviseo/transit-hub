import {
  getAccessToken,
  handleAuthExpired,
  isUnauthorizedApiResponse,
} from '@/modules/auth/api/auth'

// 后台采集 / 同步的状态：利润分析（GET /api/profit/sync）和用户分析（GET /api/user-activity/sync）。
export type CollectionKind = 'profit' | 'users'

export interface CollectionStatus {
  status: string
  errorKey?: string
  // manual：手动开始；auto：后台自动开始的首次采集；resume：服务重启后自动继续；schedule：每小时例行。
  startedBy?: string
  lastStartedAt?: string | null
  lastSyncedAt?: string | null
  syncedTo?: string
  userCount?: number
}

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'

const paths: Record<CollectionKind, string> = {
  profit: '/profit/sync',
  users: '/user-activity/sync',
}

// 读取当前工作区的采集状态。没有选中工作区、请求失败时返回 null：后台提醒不打扰页面，下次轮询再试。
export const getCollectionStatus = async (kind: CollectionKind): Promise<CollectionStatus | null> => {
  const token = getAccessToken()
  if (!token) return null
  let response: Response
  try {
    response = await fetch(`${apiBaseUrl.replace(/\/$/, '')}${paths[kind]}`, {
      headers: { Accept: 'application/json', Authorization: `Bearer ${token}` },
    })
  } catch {
    return null
  }
  let payload: (CollectionStatus & { message?: string }) | null = null
  try {
    payload = await response.json() as CollectionStatus & { message?: string }
  } catch {
    payload = null
  }
  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload ?? {})) handleAuthExpired()
    return null
  }
  return payload
}
