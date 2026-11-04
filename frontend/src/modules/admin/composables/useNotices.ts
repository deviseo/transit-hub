import { ref } from 'vue'

// 页面右下角的提醒（全局，任何页面都能看到）。
export interface NoticeAction {
  label: string
  to: string
}

export interface Notice {
  id: number
  tone: 'success' | 'error'
  title: string
  message?: string
  actions: NoticeAction[]
}

const notices = ref<Notice[]>([])
const timers = new Map<number, number>()
let nextId = 0
const MAX_NOTICES = 4

const dismissNotice = (id: number) => {
  const timer = timers.get(id)
  if (timer !== undefined) window.clearTimeout(timer)
  timers.delete(id)
  notices.value = notices.value.filter((notice) => notice.id !== id)
}

// 成功的提醒 12 秒后自动消失；失败的提醒停留 60 秒，也可以手动关闭。
const pushNotice = (notice: Omit<Notice, 'id'>) => {
  const id = ++nextId
  const next = [...notices.value, { ...notice, id }]
  // 最多同时显示 MAX_NOTICES 条，最早的先收起。
  while (next.length > MAX_NOTICES) {
    const dropped = next.shift()
    if (dropped) dismissNotice(dropped.id)
  }
  notices.value = next
  timers.set(id, window.setTimeout(() => dismissNotice(id), notice.tone === 'success' ? 12_000 : 60_000))
  return id
}

export function useNotices() {
  return { notices, pushNotice, dismissNotice }
}
