<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import LeaderboardBoard from '@/modules/leaderboard/components/LeaderboardBoard.vue'
import { leaderboardDateRange } from '@/modules/leaderboard/utils/period'
import type { LeaderboardPeriod, LeaderboardRow } from '@/modules/leaderboard/types'
import { createLeaderboardEmbedSession, getEmbedLeaderboard } from './api'

const route = useRoute()
const { locale } = useI18n()
const rows = ref<LeaderboardRow[]>([])
const period = ref<LeaderboardPeriod>('today')
const loading = ref(true)
const errorKey = ref<string | null>(null)
const updatedAt = ref<Date | null>(null)
const ready = ref(false)

const queryString = (value: unknown): string => {
  if (Array.isArray(value)) return typeof value[0] === 'string' ? value[0] : ''
  return typeof value === 'string' ? value : ''
}

const applyTheme = (theme: string) => {
  if (theme === 'dark') document.documentElement.classList.add('dark')
  if (theme === 'light') document.documentElement.classList.remove('dark')
}

const stripViewerToken = () => {
  const params = new URLSearchParams(window.location.search)
  params.delete('token')
  const query = params.toString()
  window.history.replaceState(window.history.state, '', query ? `${window.location.pathname}?${query}` : window.location.pathname)
}

const load = async () => {
  if (!ready.value) return
  loading.value = true
  errorKey.value = null
  try {
    const response = await getEmbedLeaderboard(leaderboardDateRange(period.value))
    rows.value = response.rows
    updatedAt.value = new Date()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'embed.leaderboard.errors.request'
  } finally {
    loading.value = false
  }
}

const setPeriod = (value: LeaderboardPeriod) => {
  if (period.value === value) return
  period.value = value
  void load()
}

onMounted(async () => {
  applyTheme(queryString(route.query.theme))
  const lang = queryString(route.query.lang).toLowerCase()
  locale.value = lang.startsWith('zh') ? 'zh-CN' : 'en-US'

  const embedToken = queryString(route.query.embed_token)
  const sub2apiToken = queryString(route.query.token)
  const srcHost = queryString(route.query.src_host)
  const userId = queryString(route.query.user_id)
  // Keep the token in this function for the one-time exchange, but remove it
  // from browser history before any request can fail or the URL can be shared.
  if (sub2apiToken) stripViewerToken()
  if (!embedToken || !sub2apiToken || !srcHost) {
    errorKey.value = 'embed.leaderboard.errors.missingParams'
    loading.value = false
    return
  }

  try {
    await createLeaderboardEmbedSession({ embedToken, sub2apiToken, srcHost, userId })
    ready.value = true
    await load()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'embed.leaderboard.errors.request'
    loading.value = false
  }
})
</script>

<template>
  <main class="min-h-screen w-full bg-background px-3 py-4 text-foreground sm:px-5 sm:py-6 lg:px-8">
    <LeaderboardBoard
      :rows="rows"
      :loading="loading"
      :error-key="errorKey"
      :period="period"
      :updated-at="updatedAt"
      i18n-prefix="embed.leaderboard"
      @refresh="load"
      @update:period="setPeriod"
    />
  </main>
</template>
