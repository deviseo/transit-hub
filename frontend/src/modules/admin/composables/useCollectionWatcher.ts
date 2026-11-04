import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getCollectionStatus, type CollectionKind, type CollectionStatus } from '../api/collections'
import { useNotices, type NoticeAction } from './useNotices'
import { useUserActivityFormat } from './useUserActivityFormat'

// 后台采集提醒（只在页面内）：轮询当前工作区「利润分析」「用户分析」的采集状态——有采集在进行时每 5 秒一次，
// 否则每 60 秒一次——某次采集结束时在页面右下角提醒：
//   - 手动开始、后台自动开始的首次采集、服务重启后自动继续的采集，完成时提醒；每小时的例行采集完成不提醒；
//   - 任何采集失败都提醒，同一个原因连续失败只提醒一次；被服务重启打断的不提醒（会在后台自动继续）。
// 已经提醒过的采集按工作区记在 localStorage：页面关着的时候完成的采集，下次打开时补一次提醒。

const KINDS: CollectionKind[] = ['profit', 'users']
const ACTIVE_POLL = 5_000
const IDLE_POLL = 60_000
const NOTIFY_STARTERS = new Set(['manual', 'auto', 'resume'])
const INTERRUPTED: Record<CollectionKind, string> = {
  profit: 'admin.profit.errors.interrupted',
  users: 'admin.userActivity.errors.interrupted',
}
const AUTH_ERRORS = new Set(['admin.profit.errors.authRequired', 'admin.userActivity.errors.authRequired'])

export const COLLECTION_PAGES: Record<CollectionKind, string> = {
  profit: '/admin/profit',
  users: '/admin/user-activity',
}

interface Marker {
  startedAt: string
  errorKey: string
}

const markerKey = (workspaceId: string, kind: CollectionKind) => `transithub.collections.${workspaceId}.${kind}`

const readMarker = (key: string): Marker | null => {
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return null
    const parsed = JSON.parse(raw) as Partial<Marker>
    return { startedAt: String(parsed.startedAt ?? ''), errorKey: String(parsed.errorKey ?? '') }
  } catch {
    return null
  }
}

const writeMarker = (key: string, marker: Marker) => {
  try {
    localStorage.setItem(key, JSON.stringify(marker))
  } catch {
    // 无法写入（隐私模式等）时只是少一次补提醒，不影响页面。
  }
}

// 页面上手动开始采集后调用，让顶部的「后台采集中」立刻出现。
const refreshers = new Set<() => void>()
export const notifyCollectionStarted = () => {
  refreshers.forEach((refresh) => refresh())
}

export function useCollectionWatcher(workspaceId: Ref<string | null | undefined>) {
  const { t, te } = useI18n()
  const { formatDay, formatCount } = useUserActivityFormat()
  const { pushNotice } = useNotices()
  const statuses = ref<Partial<Record<CollectionKind, CollectionStatus>>>({})
  const running = computed(() => KINDS.filter((kind) => statuses.value[kind]?.status === 'running'))
  let timer: number | undefined
  let stopped = false
  let generation = 0

  const notifySuccess = (kind: CollectionKind, status: CollectionStatus) => {
    const variant = status.startedBy === 'auto' ? 'firstDone' : status.startedBy === 'resume' ? 'resumed' : 'done'
    const actions: NoticeAction[] = [{ label: t('admin.collections.view'), to: COLLECTION_PAGES[kind] }]
    let message: string | undefined
    if (kind === 'profit') {
      message = status.syncedTo ? t('admin.collections.profit.doneDetail', { date: formatDay(status.syncedTo) }) : undefined
    } else {
      message = t('admin.collections.users.doneDetail', { count: formatCount(status.userCount ?? 0) }, status.userCount ?? 0)
      actions.push({ label: t('admin.collections.users.viewSales'), to: '/admin/sales' })
    }
    pushNotice({ tone: 'success', title: t(`admin.collections.${kind}.${variant}`), message, actions })
  }

  const notifyFailure = (kind: CollectionKind, errorKey: string) => {
    const auth = AUTH_ERRORS.has(errorKey)
    pushNotice({
      tone: 'error',
      title: t(`admin.collections.${kind}.failed`),
      message: errorKey && te(errorKey) ? t(errorKey) : t('admin.collections.unknownError'),
      actions: [auth
        ? { label: t('admin.collections.goDashboard'), to: '/admin' }
        : { label: t('admin.collections.view'), to: COLLECTION_PAGES[kind] }],
    })
  }

  const evaluate = (id: string, kind: CollectionKind, status: CollectionStatus | null) => {
    if (!status || !status.status) return
    const key = markerKey(id, kind)
    const marker = readMarker(key)
    if (status.status === 'running') {
      // 第一次看到这个工作区时采集正在进行：结束时要提醒。
      if (!marker) writeMarker(key, { startedAt: '', errorKey: '' })
      return
    }
    const startedAt = status.lastStartedAt ?? ''
    if (!marker) {
      // 第一次看到这个工作区：只记下当前状态，不补提醒更早的采集。
      writeMarker(key, { startedAt, errorKey: status.status === 'success' ? '' : status.errorKey ?? '' })
      return
    }
    if (marker.startedAt === startedAt) return
    if (status.status === 'interrupted' || status.errorKey === INTERRUPTED[kind]) {
      writeMarker(key, { startedAt, errorKey: marker.errorKey })
      return
    }
    if (status.status === 'success') {
      if (NOTIFY_STARTERS.has(status.startedBy ?? '')) notifySuccess(kind, status)
      writeMarker(key, { startedAt, errorKey: '' })
      return
    }
    const errorKey = status.errorKey ?? ''
    if (errorKey !== marker.errorKey) notifyFailure(kind, errorKey)
    writeMarker(key, { startedAt, errorKey })
  }

  const schedule = (delay: number) => {
    if (stopped) return
    window.clearTimeout(timer)
    timer = window.setTimeout(() => { void poll() }, delay)
  }

  const poll = async () => {
    const current = ++generation
    const id = workspaceId.value
    if (!id) {
      statuses.value = {}
      schedule(IDLE_POLL)
      return
    }
    const results = await Promise.all(KINDS.map((kind) => getCollectionStatus(kind)))
    if (stopped || current !== generation || id !== workspaceId.value) return
    const next: Partial<Record<CollectionKind, CollectionStatus>> = {}
    KINDS.forEach((kind, index) => {
      const status = results[index]
      if (status) next[kind] = status
      evaluate(id, kind, status)
    })
    statuses.value = next
    schedule(running.value.length > 0 ? ACTIVE_POLL : IDLE_POLL)
  }

  const refresh = () => {
    window.clearTimeout(timer)
    void poll()
  }

  const onVisibilityChange = () => {
    if (document.visibilityState === 'visible') refresh()
  }

  onMounted(() => {
    refreshers.add(refresh)
    document.addEventListener('visibilitychange', onVisibilityChange)
    refresh()
  })

  onBeforeUnmount(() => {
    stopped = true
    window.clearTimeout(timer)
    refreshers.delete(refresh)
    document.removeEventListener('visibilitychange', onVisibilityChange)
  })

  watch(workspaceId, () => {
    statuses.value = {}
    refresh()
  })

  return { statuses, running, refresh }
}
