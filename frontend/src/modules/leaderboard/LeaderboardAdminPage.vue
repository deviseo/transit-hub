<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Settings2 } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import LeaderboardBoard from './components/LeaderboardBoard.vue'
import LeaderboardEmbedConfigModal from './components/LeaderboardEmbedConfigModal.vue'
import { getLeaderboard } from './api/leaderboard'
import { leaderboardDateRange } from './utils/period'
import type { LeaderboardPeriod, LeaderboardRow } from './types'

const { t } = useI18n()
const rows = ref<LeaderboardRow[]>([])
const period = ref<LeaderboardPeriod>('today')
const loading = ref(false)
const errorKey = ref<string | null>(null)
const updatedAt = ref<Date | null>(null)
const embedConfigOpen = ref(false)

const load = async () => {
  loading.value = true
  errorKey.value = null
  try {
    const response = await getLeaderboard(leaderboardDateRange(period.value))
    rows.value = response.rows
    updatedAt.value = new Date()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.leaderboard.errors.unknown'
  } finally {
    loading.value = false
  }
}

const setPeriod = (value: LeaderboardPeriod) => {
  if (period.value === value) return
  period.value = value
  void load()
}

onMounted(() => { void load() })
</script>

<template>
  <div class="mx-auto w-full max-w-[1440px] p-4 sm:p-6 lg:p-8">
    <LeaderboardBoard
      :rows="rows"
      :loading="loading"
      :error-key="errorKey"
      :period="period"
      :updated-at="updatedAt"
      i18n-prefix="admin.leaderboard"
      @refresh="load"
      @update:period="setPeriod"
    >
      <template #actions>
        <Button variant="secondary" size="sm" @click="embedConfigOpen = true">
          <Settings2 class="h-4 w-4" />
          <span class="hidden sm:inline">{{ t('admin.leaderboard.embed.action') }}</span>
        </Button>
      </template>
    </LeaderboardBoard>
    <LeaderboardEmbedConfigModal :open="embedConfigOpen" @close="embedConfigOpen = false" />
  </div>
</template>
