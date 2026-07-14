<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle,
  ArrowLeft,
  CalendarClock,
  CheckCircle2,
  Clock3,
  Gift,
  Copy,
  Loader2,
  RefreshCw,
  ShieldCheck,
  Sparkles,
  Ticket,
  Trophy,
  Undo2,
  Users,
  XCircle,
} from 'lucide-vue-next'
import type { LotteryCampaign, LotteryCampaignStatus, LotteryPrize, LotteryWinner } from '@/modules/lottery/types'
import {
  createLotteryEmbedSession,
  enterEmbedLotteryCampaign,
  getEmbedLotteryCampaign,
  listEmbedLotteryCampaigns,
  withdrawEmbedLotteryEntry,
} from './api'
import { useLotteryDrawReveal, type LotteryDrawRevealOutcome } from './composables/useLotteryDrawReveal'

const route = useRoute()
const { t, locale } = useI18n()

type PageState = 'loading' | 'error' | 'ready'
type ResultState = 'none' | 'pending' | 'won' | 'lost' | 'withdrawn'

const pageState = ref<PageState>('loading')
const errorKey = ref<string | null>(null)
const listErrorKey = ref<string | null>(null)
const actionErrorKey = ref<string | null>(null)
const campaigns = ref<LotteryCampaign[]>([])
const selectedCampaign = ref<LotteryCampaign | null>(null)
const selectedCampaignId = ref<string | null>(null)
const isListLoading = ref(false)
const isDetailLoading = ref(false)
const isActing = ref(false)
const showCampaignList = ref(false)
const copiedVoucher = ref(false)
const now = ref(Date.now())
const pendingRevealCampaign = ref<LotteryCampaign | null>(null)
const drawReveal = useLotteryDrawReveal()

let clock: number | undefined
let campaignPoller: number | undefined
let isPollingCampaign = false
let lastCampaignPollAt = 0
let revealStartedCampaignId: string | null = null

const drawnStatuses = new Set<LotteryCampaignStatus>(['drawn', 'fulfilling', 'completed', 'partial'])
const activeDrawStatuses = new Set<LotteryCampaignStatus>(['closed', 'drawing'])
const revealableStatuses = new Set<LotteryCampaignStatus>(['scheduled', 'open', 'closed', 'drawing'])

const queryString = (value: unknown): string => {
  if (Array.isArray(value)) {
    const first = value[0]
    return typeof first === 'string' ? first : ''
  }
  return typeof value === 'string' ? value : ''
}

const applyTheme = (theme: string) => {
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else if (theme === 'light') {
    document.documentElement.classList.remove('dark')
  }
}

const applyLocale = (lang: string) => {
  locale.value = lang.toLowerCase().startsWith('zh') ? 'zh-CN' : 'en-US'
}

const stripTokenFromUrl = () => {
  const params = new URLSearchParams(window.location.search)
  params.delete('token')
  const query = params.toString()
  window.history.replaceState(window.history.state, '', query ? `${window.location.pathname}?${query}` : window.location.pathname)
}

const formatDateTime = (value?: string): string => {
  if (!value) return t('embed.lottery.common.empty')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('embed.lottery.common.empty')
  return new Intl.DateTimeFormat(locale.value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const isFutureTime = (value?: string): boolean => {
  if (!value) return false
  const timestamp = new Date(value).getTime()
  return !Number.isNaN(timestamp) && timestamp > now.value
}

const countdownTarget = computed(() => {
  const campaign = selectedCampaign.value
  if (!campaign) return null
  if (campaign.status === 'scheduled') {
    if (isFutureTime(campaign.registrationStart)) return campaign.registrationStart
    if (isFutureTime(campaign.registrationEnd)) return campaign.registrationEnd
    return campaign.drawAt ?? campaign.registrationEnd ?? campaign.registrationStart ?? null
  }
  if (campaign.status === 'open') {
    if (isFutureTime(campaign.registrationEnd)) return campaign.registrationEnd
    return campaign.drawAt ?? campaign.registrationEnd ?? null
  }
  if (campaign.status === 'closed' || campaign.status === 'drawing') return campaign.drawAt ?? campaign.registrationEnd ?? null
  return campaign.drawAt ?? campaign.registrationEnd ?? campaign.registrationStart ?? null
})

const countdown = computed(() => {
  if (!countdownTarget.value) return t('embed.lottery.common.empty')
  const target = new Date(countdownTarget.value).getTime()
  if (Number.isNaN(target)) return t('embed.lottery.common.empty')
  if (drawnStatuses.has(selectedCampaign.value?.status ?? 'draft')) return formatDateTime(countdownTarget.value)
  const totalSeconds = Math.max(0, Math.ceil((target - now.value) / 1000))
  const days = Math.floor(totalSeconds / 86400)
  const hours = Math.floor((totalSeconds % 86400) / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (days > 0) return t('embed.lottery.countdown.days', { days, hours, minutes, seconds })
  if (hours > 0) return t('embed.lottery.countdown.hours', { hours, minutes, seconds })
  if (minutes > 0) return t('embed.lottery.countdown.minutes', { minutes, seconds })
  return t('embed.lottery.countdown.seconds', { seconds })
})

const countdownLabel = computed(() => {
  const campaign = selectedCampaign.value
  if (campaign && drawnStatuses.has(campaign.status)) return t('embed.lottery.countdown.drawTime')
  const status = selectedCampaign.value?.status
  if (status === 'scheduled' && countdownTarget.value === campaign?.registrationStart) return t('embed.lottery.countdown.opensIn')
  if (status === 'scheduled' && countdownTarget.value === campaign?.registrationEnd) return t('embed.lottery.countdown.closesIn')
  if (status === 'open' && countdownTarget.value === campaign?.registrationEnd) return t('embed.lottery.countdown.closesIn')
  if (status === 'closed') return t('embed.lottery.countdown.drawsIn')
  if (countdownTarget.value === campaign?.drawAt) return t('embed.lottery.countdown.drawsIn')
  return t('embed.lottery.countdown.noTimer')
})

const selectedPrizes = computed(() => selectedCampaign.value?.prizes ?? [])
const selectedWinners = computed(() => selectedCampaign.value?.winners ?? [])
const publicEntries = computed(() => selectedCampaign.value?.entries ?? [])
const activeEntries = computed(() => publicEntries.value.filter((entry) => entry.status === 'active'))
const myEntry = computed(() => selectedCampaign.value?.myEntry ?? null)
const myWinner = computed(() => selectedCampaign.value?.myWinner ?? null)
const myRewardStatus = computed(() => selectedCampaign.value?.myRewardStatus ?? null)
const focusMode = computed(() => myEntry.value?.status === 'active' && !showCampaignList.value)

const registrationWindowOpen = computed(() => {
  const campaign = selectedCampaign.value
  if (!campaign || (campaign.status !== 'scheduled' && campaign.status !== 'open')) return false
  if (campaign.status === 'scheduled' && isFutureTime(campaign.registrationStart)) return false
  if (campaign.registrationEnd && !isFutureTime(campaign.registrationEnd)) return false
  return true
})

const canEnter = computed(() => registrationWindowOpen.value && !myEntry.value)
const canWithdraw = computed(() => registrationWindowOpen.value && myEntry.value?.status === 'active')

const resultState = computed<ResultState>(() => {
  const campaign = selectedCampaign.value
  if (!campaign || !myEntry.value) return 'none'
  if (myEntry.value.status === 'withdrawn') return 'withdrawn'
  if (myWinner.value) return 'won'
  if (['drawn', 'fulfilling', 'completed', 'partial'].includes(campaign.status)) return 'lost'
  return 'pending'
})

const winnerPrize = computed(() => {
  const winner = myWinner.value
  if (!winner) return null
  return selectedPrizes.value.find((prize) => prize.id === winner.prizeId) ?? null
})

const publicWinnersNote = computed(() => {
  if (!selectedCampaign.value?.publicWinners) return t('embed.lottery.winners.private')
  if (selectedWinners.value.length === 0) return t('embed.lottery.winners.empty')
  return t('embed.lottery.winners.count', { count: selectedWinners.value.length })
})

const algorithmTransparencyNote = computed(() => (
  selectedCampaign.value?.algorithmVersion === 'lottery-hmac-sha256-public-v2'
    ? t('embed.lottery.transparency.algorithmV2')
    : t('embed.lottery.transparency.algorithmLegacy')
))

const drawRevealPrize = computed(() => {
  const campaign = pendingRevealCampaign.value
  if (!campaign?.myWinner) return null
  return campaign.prizes.find((prize) => prize.id === campaign.myWinner?.prizeId) ?? null
})

const drawRevealTitle = computed(() => t(
  `embed.lottery.drawReveal.${drawReveal.outcome.value}.title`,
  { prize: drawRevealPrize.value?.name ?? t('embed.lottery.common.empty') },
))

const drawRevealDescription = computed(() => t(
  `embed.lottery.drawReveal.${drawReveal.outcome.value}.description`,
))

const particleColors = ['#f59e0b', '#10b981', '#ef4444', '#0ea5e9', '#a855f7', '#f97316']
const particleStyle = (index: number): Record<string, string> => ({
  '--draw-color': particleColors[(index - 1) % particleColors.length] ?? '#f59e0b',
  '--draw-radius': index % 3 === 0 ? '999px' : '2px',
})

const syncCampaignSummary = (campaign: LotteryCampaign) => {
  const index = campaigns.value.findIndex((item) => item.id === campaign.id)
  if (index === -1) return
  campaigns.value[index] = campaign
}

const campaignSortTime = (campaign: LotteryCampaign): number => {
  const timestamp = new Date(campaign.createdAt).getTime()
  return Number.isNaN(timestamp) ? 0 : timestamp
}

const revealOutcome = (campaign: LotteryCampaign): LotteryDrawRevealOutcome => {
  if (campaign.myWinner) return 'won'
  if (campaign.myEntry?.status === 'active') return 'lost'
  return 'spectator'
}

const waitForDrawnCampaign = async (campaign: LotteryCampaign): Promise<LotteryCampaign> => {
  let latest = campaign
  for (let attempt = 0; attempt < 8; attempt += 1) {
    if (attempt > 0) await new Promise<void>((resolve) => window.setTimeout(resolve, 500))
    try {
      latest = await getEmbedLotteryCampaign(campaign.id)
      if (drawnStatuses.has(latest.status)) return latest
    } catch {
      // Keep the reveal animation running while a transient poll fails.
    }
  }
  return latest
}

const startDrawReveal = (campaign: LotteryCampaign, waitForDraw = false) => {
  pendingRevealCampaign.value = campaign
  if (drawReveal.isVisible.value) return
  revealStartedCampaignId = campaign.id

  void drawReveal.play(waitForDraw ? 'spectator' : revealOutcome(campaign), async () => {
    const pendingCampaign = pendingRevealCampaign.value
    if (!pendingCampaign) return
    const revealedCampaign = waitForDraw ? await waitForDrawnCampaign(pendingCampaign) : pendingCampaign
    pendingRevealCampaign.value = revealedCampaign
    drawReveal.outcome.value = revealOutcome(revealedCampaign)
    if (!revealedCampaign) return
    if (selectedCampaignId.value === revealedCampaign.id) {
      selectedCampaign.value = revealedCampaign
    }
    syncCampaignSummary(revealedCampaign)
  })
}

const startDrawRevealAtDeadline = () => {
  const campaign = selectedCampaign.value
  if (!campaign || drawReveal.isVisible.value || revealStartedCampaignId === campaign.id) return
  if (campaign.drawMode !== 'scheduled' || !revealableStatuses.has(campaign.status) || !campaign.drawAt) return
  const drawAt = new Date(campaign.drawAt).getTime()
  if (Number.isNaN(drawAt) || drawAt > now.value) return
  startDrawReveal(campaign, true)
}

const closeDrawReveal = () => {
  drawReveal.close()
  if (!drawReveal.isVisible.value) pendingRevealCampaign.value = null
}

const applyCampaignUpdate = (campaign: LotteryCampaign) => {
  const previous = selectedCampaign.value
  const shouldReveal = previous?.id === campaign.id
    && !drawnStatuses.has(previous.status)
    && drawnStatuses.has(campaign.status)

  if (shouldReveal) {
    startDrawReveal(campaign)
    return
  }

  selectedCampaign.value = campaign
  syncCampaignSummary(campaign)
  startDrawRevealAtDeadline()
}

const loadList = async () => {
  isListLoading.value = true
  listErrorKey.value = null
  try {
    const response = await listEmbedLotteryCampaigns()
    campaigns.value = [...response.items].sort((left, right) => campaignSortTime(right) - campaignSortTime(left))
    const nextId = campaigns.value[0]?.id ?? null
    if (nextId) {
      await selectCampaign(nextId)
    } else {
      selectedCampaignId.value = null
      selectedCampaign.value = null
    }
  } catch (error) {
    listErrorKey.value = error instanceof Error ? error.message : 'embed.lottery.errors.request'
  } finally {
    isListLoading.value = false
  }
}

const selectCampaign = async (id: string, fromList = false) => {
  if (fromList) showCampaignList.value = true
  selectedCampaignId.value = id
  isDetailLoading.value = true
  actionErrorKey.value = null
  try {
    const campaign = await getEmbedLotteryCampaign(id)
    applyCampaignUpdate(campaign)
  } catch (error) {
    actionErrorKey.value = error instanceof Error ? error.message : 'embed.lottery.errors.request'
  } finally {
    isDetailLoading.value = false
  }
}

const pollSelectedCampaign = async () => {
  const id = selectedCampaignId.value
  if (!id || pageState.value !== 'ready' || isPollingCampaign || drawReveal.isVisible.value || document.visibilityState !== 'visible') return

  const pollInterval = selectedCampaign.value && activeDrawStatuses.has(selectedCampaign.value.status) ? 4000 : 12000
  const pollStartedAt = Date.now()
  if (pollStartedAt - lastCampaignPollAt < pollInterval) return

  isPollingCampaign = true
  lastCampaignPollAt = pollStartedAt
  try {
    const campaign = await getEmbedLotteryCampaign(id)
    if (selectedCampaignId.value === id) applyCampaignUpdate(campaign)
  } catch {
    // Background polling is best-effort; explicit refresh keeps the visible error path.
  } finally {
    isPollingCampaign = false
  }
}

const refreshSelected = async () => {
  if (!selectedCampaignId.value) return
  await selectCampaign(selectedCampaignId.value)
}

const runEntryAction = async (action: 'enter' | 'withdraw') => {
  const campaign = selectedCampaign.value
  if (!campaign) return
  if (action === 'enter' && !canEnter.value) return
  if (action === 'withdraw' && !canWithdraw.value) return
  isActing.value = true
  actionErrorKey.value = null
  try {
    if (action === 'enter') {
      await enterEmbedLotteryCampaign(campaign.id)
      showCampaignList.value = false
    } else {
      await withdrawEmbedLotteryEntry(campaign.id)
      showCampaignList.value = true
    }
    await loadList()
  } catch (error) {
    actionErrorKey.value = error instanceof Error ? error.message : 'embed.lottery.errors.request'
    await refreshSelected()
  } finally {
    isActing.value = false
  }
}

const statusBadgeClass = (status: LotteryCampaignStatus | string): string => {
  switch (status) {
    case 'open':
    case 'completed':
    case 'active':
    case 'fulfilled':
      return 'border-emerald-400/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
    case 'scheduled':
    case 'drawn':
    case 'processing':
      return 'border-sky-400/30 bg-sky-500/10 text-sky-700 dark:text-sky-300'
    case 'closed':
    case 'drawing':
    case 'fulfilling':
    case 'pending':
      return 'border-amber-400/30 bg-amber-500/10 text-amber-700 dark:text-amber-300'
    case 'partial':
    case 'cancelled':
    case 'withdrawn':
    case 'retryable_failed':
    case 'manual_attention':
    case 'failed':
      return 'border-destructive/30 bg-destructive/10 text-destructive'
    default:
      return 'border-border/70 bg-surface-elevated text-muted-foreground'
  }
}

const prizeValue = (prize: LotteryPrize): string => {
  if (prize.type === 'balance') {
    return t('embed.lottery.prizes.balanceValue', { amount: prize.balanceAmount ?? t('embed.lottery.common.empty') })
  }
  return t('embed.lottery.prizes.subscriptionValue', {
    group: prize.groupName || prize.groupId || t('embed.lottery.common.empty'),
    multiplier: prize.multiplier || t('embed.lottery.common.empty'),
    days: prize.validityDays ?? 0,
  })
}

const prizeName = (prizeId: string): string => (
  selectedPrizes.value.find((prize) => prize.id === prizeId)?.name ?? t('embed.lottery.common.empty')
)

const winnerLabel = (winner: LotteryWinner): string => (
  t('embed.lottery.winners.row', { email: winner.maskedEmail, prize: prizeName(winner.prizeId), slot: winner.prizeSlot })
)

const copyVoucherCode = async () => {
  const code = myRewardStatus.value?.voucherCode
  if (!code) return
  try {
    await navigator.clipboard.writeText(code)
    copiedVoucher.value = true
    window.setTimeout(() => {
      copiedVoucher.value = false
    }, 1600)
  } catch {
    actionErrorKey.value = 'embed.lottery.errors.copy'
  }
}

onMounted(async () => {
  applyTheme(queryString(route.query.theme))
  applyLocale(queryString(route.query.lang))
  now.value = Date.now()
  clock = window.setInterval(() => {
    now.value = Date.now()
    startDrawRevealAtDeadline()
  }, 1000)

  const embedToken = queryString(route.query.embed_token)
  const sub2apiToken = queryString(route.query.token)
  const srcHost = queryString(route.query.src_host)
  const srcUrl = queryString(route.query.src_url)
  const userId = queryString(route.query.user_id)

  if (sub2apiToken) stripTokenFromUrl()
  if (!embedToken || !sub2apiToken || !srcHost) {
    errorKey.value = 'embed.lottery.errors.missingParams'
    pageState.value = 'error'
    return
  }

  try {
    await createLotteryEmbedSession({ embedToken, sub2apiToken, srcHost, srcUrl, userId })
    await loadList()
    pageState.value = 'ready'
    campaignPoller = window.setInterval(() => {
      void pollSelectedCampaign()
    }, 4000)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'embed.lottery.errors.request'
    pageState.value = 'error'
  }
})

onBeforeUnmount(() => {
  if (clock !== undefined) window.clearInterval(clock)
  if (campaignPoller !== undefined) window.clearInterval(campaignPoller)
})
</script>

<template>
  <main
    class="min-h-screen bg-background px-3 py-3 text-foreground sm:px-5 sm:py-5"
    :inert="drawReveal.isVisible.value"
    :aria-hidden="drawReveal.isVisible.value ? 'true' : undefined"
  >
    <div class="mx-auto flex w-full max-w-6xl flex-col gap-4">
      <header class="flex flex-col gap-3 border-b border-border/70 pb-4 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0">
          <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t('embed.lottery.eyebrow') }}</p>
          <h1 class="mt-1 text-pretty text-2xl font-semibold tracking-normal text-foreground sm:text-3xl">{{ t('embed.lottery.title') }}</h1>
          <p class="mt-1 max-w-2xl text-sm text-muted-foreground">{{ t('embed.lottery.subtitle') }}</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            v-if="myEntry?.status === 'active'"
            type="button"
            class="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-border bg-surface px-3 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            @click="showCampaignList = !showCampaignList"
          >
            <Users v-if="focusMode" class="h-4 w-4" aria-hidden="true" />
            <ArrowLeft v-else class="h-4 w-4" aria-hidden="true" />
            <span>{{ t(focusMode ? 'embed.lottery.actions.browseCampaigns' : 'embed.lottery.actions.returnToDraw') }}</span>
          </button>
          <button
            type="button"
            class="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-border bg-surface px-3 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
            :disabled="pageState !== 'ready' || isListLoading || isDetailLoading"
            :aria-label="t('embed.lottery.actions.refresh')"
            @click="loadList"
          >
            <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': isListLoading }" aria-hidden="true" />
            <span>{{ t('embed.lottery.actions.refresh') }}</span>
          </button>
        </div>
      </header>

      <section v-if="pageState === 'loading'" class="flex min-h-80 items-center justify-center rounded-lg border border-border/70 bg-surface text-muted-foreground" aria-live="polite">
        <Loader2 class="mr-2 h-5 w-5 animate-spin" aria-hidden="true" />
        <span>{{ t('embed.lottery.page.loading') }}</span>
      </section>

      <section v-else-if="pageState === 'error'" class="rounded-lg border border-destructive/30 bg-destructive/10 p-4 text-sm text-destructive" aria-live="polite">
        <div class="flex gap-2">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          <div>
            <h2 class="font-semibold">{{ t('embed.lottery.errors.title') }}</h2>
            <p class="mt-1">{{ t(errorKey ?? 'embed.lottery.errors.request') }}</p>
          </div>
        </div>
      </section>

      <template v-else>
        <div v-if="listErrorKey" class="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" aria-live="polite">
          {{ t(listErrorKey) }}
        </div>

        <div class="min-h-[calc(100vh-11rem)] gap-4" :class="focusMode ? 'block' : 'grid lg:grid-cols-[20rem_minmax(0,1fr)]'">
          <aside v-if="!focusMode" class="min-w-0 rounded-lg border border-border/70 bg-surface p-3 lg:max-h-[calc(100vh-11rem)] lg:overflow-auto">
            <div class="mb-3 flex items-center justify-between gap-2">
              <h2 class="flex items-center gap-2 text-sm font-semibold">
                <Ticket class="h-4 w-4 text-primary" aria-hidden="true" />
                {{ t('embed.lottery.list.title') }}
              </h2>
              <span class="rounded-full border border-border/70 px-2 py-0.5 text-xs text-muted-foreground">{{ t('embed.lottery.list.count', { count: campaigns.length }) }}</span>
            </div>

            <div v-if="isListLoading && campaigns.length === 0" class="flex min-h-32 items-center justify-center text-sm text-muted-foreground" aria-live="polite">
              <Loader2 class="mr-2 h-4 w-4 animate-spin" aria-hidden="true" />
              <span>{{ t('embed.lottery.list.loading') }}</span>
            </div>
            <div v-else-if="campaigns.length === 0" class="rounded-md border border-dashed border-border/80 p-4 text-sm text-muted-foreground">
              {{ t('embed.lottery.list.empty') }}
            </div>
            <div v-else class="flex gap-2 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible lg:pb-0" role="list" :aria-label="t('embed.lottery.list.title')">
              <button
                v-for="campaign in campaigns"
                :key="campaign.id"
                type="button"
                class="min-w-64 rounded-md border p-3 text-left transition-colors hover:border-primary/50 hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring lg:min-w-0"
                :class="campaign.id === selectedCampaignId ? 'border-primary/60 bg-primary/5 text-foreground' : 'border-border/70 bg-background'"
                @click="selectCampaign(campaign.id, true)"
              >
                <div class="flex min-w-0 items-center justify-between gap-2">
                  <span class="truncate text-sm font-semibold">{{ campaign.name }}</span>
                  <span class="shrink-0 rounded-full border px-2 py-0.5 text-xs" :class="statusBadgeClass(campaign.status)">{{ t(`embed.lottery.status.${campaign.status}`) }}</span>
                </div>
                <div class="mt-2 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                  <span>{{ t('embed.lottery.metrics.entries', { count: campaign.entryCount }) }}</span>
                  <span>{{ formatDateTime(campaign.registrationEnd) }}</span>
                </div>
              </button>
            </div>
          </aside>

          <section class="min-w-0">
            <div v-if="isDetailLoading" class="flex min-h-96 items-center justify-center text-muted-foreground" aria-live="polite">
              <Loader2 class="mr-2 h-5 w-5 animate-spin" aria-hidden="true" />
              <span>{{ t('embed.lottery.detail.loading') }}</span>
            </div>

            <div v-else-if="!selectedCampaign" class="flex min-h-96 items-center justify-center text-sm text-muted-foreground">
              {{ t('embed.lottery.detail.empty') }}
            </div>

            <div v-else class="flex flex-col gap-5">
              <section class="flex flex-col gap-3 border-b border-primary/25 bg-primary/5 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <div class="flex min-w-0 items-start gap-3">
                  <span class="mt-0.5 grid h-9 w-9 shrink-0 place-items-center rounded-full border border-primary/25 bg-background text-primary">
                    <ShieldCheck class="h-5 w-5" aria-hidden="true" />
                  </span>
                  <div class="min-w-0">
                    <h2 class="text-sm font-semibold">{{ t('embed.lottery.transparency.title') }}</h2>
                    <p class="mt-1 text-sm text-muted-foreground">{{ t('embed.lottery.transparency.description') }}</p>
                  </div>
                </div>
                <div class="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
                  <Users class="h-4 w-4" aria-hidden="true" />
                  <span>{{ t('embed.lottery.transparency.activeEntries', { count: activeEntries.length }) }}</span>
                </div>
              </section>
              <div class="flex flex-col gap-4 border-b border-border/70 pb-4 xl:flex-row xl:items-start xl:justify-between">
                <div class="min-w-0">
                  <div class="mb-2 flex flex-wrap items-center gap-2">
                    <span class="rounded-full border px-2 py-0.5 text-xs font-medium" :class="statusBadgeClass(selectedCampaign.status)">{{ t(`embed.lottery.status.${selectedCampaign.status}`) }}</span>
                    <span class="rounded-full border border-border/70 px-2 py-0.5 text-xs text-muted-foreground">{{ t(`embed.lottery.drawMode.${selectedCampaign.drawMode}`) }}</span>
                  </div>
                  <h2 class="break-words text-pretty text-2xl font-semibold tracking-normal">{{ selectedCampaign.name }}</h2>
                  <p class="mt-2 max-w-3xl whitespace-pre-line break-words text-sm text-muted-foreground">{{ selectedCampaign.description || t('embed.lottery.common.noDescription') }}</p>
                </div>

                <div class="grid min-w-0 grid-cols-2 gap-2 text-sm sm:min-w-80">
                  <div class="rounded-md border border-border/70 bg-background p-3">
                    <div class="flex items-center gap-2 text-muted-foreground"><Clock3 class="h-4 w-4" aria-hidden="true" />{{ countdownLabel }}</div>
                    <div class="mt-1 font-semibold tabular-nums">{{ countdown }}</div>
                  </div>
                  <div class="rounded-md border border-border/70 bg-background p-3">
                    <div class="flex items-center gap-2 text-muted-foreground"><Trophy class="h-4 w-4" aria-hidden="true" />{{ t('embed.lottery.metrics.winnersLabel') }}</div>
                    <div class="mt-1 font-semibold tabular-nums">{{ t('embed.lottery.metrics.winners', { count: selectedCampaign.winnerCount }) }}</div>
                  </div>
                </div>
              </div>

              <div v-if="actionErrorKey" class="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" aria-live="polite">
                {{ t(actionErrorKey) }}
              </div>

              <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_18rem]">
                <div class="min-w-0 space-y-4">
                  <section class="rounded-lg border border-border/70 bg-background p-4">
                    <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><CalendarClock class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('embed.lottery.sections.schedule') }}</h3>
                    <dl class="grid gap-3 text-sm sm:grid-cols-3">
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.registrationStart') }}</dt><dd class="font-medium">{{ formatDateTime(selectedCampaign.registrationStart) }}</dd></div>
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.registrationEnd') }}</dt><dd class="font-medium">{{ formatDateTime(selectedCampaign.registrationEnd) }}</dd></div>
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.drawAt') }}</dt><dd class="font-medium">{{ formatDateTime(selectedCampaign.drawAt) }}</dd></div>
                    </dl>
                  </section>

                  <section class="rounded-lg border border-border/70 bg-background p-4">
                    <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><Gift class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('embed.lottery.sections.prizes') }}</h3>
                    <div v-if="selectedPrizes.length === 0" class="text-sm text-muted-foreground">{{ t('embed.lottery.prizes.empty') }}</div>
                    <div v-else class="grid gap-2 sm:grid-cols-2">
                      <div v-for="prize in selectedPrizes" :key="prize.id" class="rounded-md border border-border/70 p-3">
                        <div class="flex items-center justify-between gap-2">
                          <h4 class="min-w-0 truncate text-sm font-semibold">{{ prize.name }}</h4>
                          <span class="shrink-0 rounded-full border border-border/70 px-2 py-0.5 text-xs text-muted-foreground">{{ t('embed.lottery.prizes.quantity', { count: prize.quantity }) }}</span>
                        </div>
                        <p class="mt-2 text-sm text-muted-foreground">{{ t(`embed.lottery.prizeType.${prize.type}`) }}</p>
                        <p class="mt-1 break-words text-sm">{{ prizeValue(prize) }}</p>
                        <p v-if="prize.deliveryMode" class="mt-2 flex items-center gap-1.5 text-xs text-muted-foreground">
                          <Ticket class="h-3.5 w-3.5" aria-hidden="true" />
                          {{ t(`embed.lottery.deliveryMode.${prize.deliveryMode}`) }}
                        </p>
                      </div>
                    </div>
                  </section>

                  <section class="rounded-lg border border-border/70 bg-background p-4">
                    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
                      <div>
                        <h3 class="flex items-center gap-2 text-sm font-semibold"><Users class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('embed.lottery.sections.entries') }}</h3>
                        <p class="mt-1 text-xs text-muted-foreground">{{ t('embed.lottery.entries.description') }}</p>
                      </div>
                      <span class="rounded-full border border-border/70 px-2 py-0.5 text-xs text-muted-foreground">{{ t('embed.lottery.entries.count', { active: activeEntries.length, total: publicEntries.length }) }}</span>
                    </div>
                    <div v-if="publicEntries.length === 0" class="rounded-md border border-dashed border-border/80 p-4 text-sm text-muted-foreground">
                      {{ t('embed.lottery.entries.empty') }}
                    </div>
                    <ol v-else class="divide-y divide-border/60" :aria-label="t('embed.lottery.sections.entries')">
                      <li v-for="(entry, index) in publicEntries" :key="entry.id" class="grid gap-2 py-3 text-sm sm:grid-cols-[2.5rem_minmax(0,1fr)_auto] sm:items-start">
                        <span class="font-mono text-xs text-muted-foreground">{{ String(index + 1).padStart(2, '0') }}</span>
                        <div class="min-w-0">
                          <div class="flex flex-wrap items-center gap-2">
                            <span class="font-medium">{{ entry.maskedEmail }}</span>
                            <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusBadgeClass(entry.status)">{{ t(`embed.lottery.entryStatus.${entry.status}`) }}</span>
                          </div>
                          <div class="mt-1 break-all font-mono text-xs text-muted-foreground">{{ entry.receiptHash }}</div>
                        </div>
                        <time class="text-xs text-muted-foreground">{{ formatDateTime(entry.createdAt) }}</time>
                      </li>
                    </ol>
                  </section>

                  <section class="rounded-lg border border-border/70 bg-background p-4">
                    <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><Trophy class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('embed.lottery.sections.winners') }}</h3>
                    <p class="mb-3 text-sm text-muted-foreground">{{ publicWinnersNote }}</p>
                    <div v-if="selectedWinners.length > 0" class="grid gap-2 sm:grid-cols-2">
                      <div v-for="winner in selectedWinners" :key="winner.id" class="rounded-md border border-border/70 p-3 text-sm">
                        <div>{{ winnerLabel(winner) }}</div>
                        <div v-if="winner.entryId" class="mt-1 break-all font-mono text-xs text-muted-foreground">{{ t('embed.lottery.fields.entryId') }}: {{ winner.entryId }}</div>
                      </div>
                    </div>
                  </section>

                  <section class="rounded-lg border border-border/70 bg-background p-4">
                    <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><ShieldCheck class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('embed.lottery.sections.integrity') }}</h3>
                    <p class="mb-3 text-sm text-muted-foreground">{{ algorithmTransparencyNote }}</p>
                    <dl class="grid gap-3 text-sm">
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.seedCommitment') }}</dt><dd class="break-all font-mono text-xs">{{ selectedCampaign.seedCommitment || t('embed.lottery.common.empty') }}</dd></div>
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.entrySnapshotHash') }}</dt><dd class="break-all font-mono text-xs">{{ selectedCampaign.entrySnapshotHash || t('embed.lottery.common.empty') }}</dd></div>
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.revealedSeed') }}</dt><dd class="break-all font-mono text-xs">{{ selectedCampaign.revealedSeed || t('embed.lottery.common.empty') }}</dd></div>
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.algorithmVersion') }}</dt><dd class="break-all font-mono text-xs">{{ selectedCampaign.algorithmVersion }}</dd></div>
                    </dl>
                  </section>
                </div>

                <aside class="min-w-0 space-y-4">
                  <section class="rounded-lg border border-border/70 bg-background p-4">
                    <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><Ticket class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('embed.lottery.sections.myEntry') }}</h3>
                    <div v-if="myEntry" class="space-y-3 text-sm">
                      <div class="flex items-center justify-between gap-2">
                        <span class="text-muted-foreground">{{ t('embed.lottery.fields.entryStatus') }}</span>
                        <span class="rounded-full border px-2 py-0.5 text-xs" :class="statusBadgeClass(myEntry.status)">{{ t(`embed.lottery.entryStatus.${myEntry.status}`) }}</span>
                      </div>
                      <div><div class="text-muted-foreground">{{ t('embed.lottery.fields.receiptHash') }}</div><div class="mt-1 break-all font-mono text-xs">{{ myEntry.receiptHash }}</div></div>
                      <div><div class="text-muted-foreground">{{ t('embed.lottery.fields.enteredAt') }}</div><div class="mt-1 font-medium">{{ formatDateTime(myEntry.createdAt) }}</div></div>
                    </div>
                    <p v-else class="text-sm text-muted-foreground">{{ t('embed.lottery.entry.none') }}</p>

                    <div class="mt-4 flex flex-col gap-2">
                      <button
                        type="button"
                        class="inline-flex h-10 items-center justify-center gap-2 rounded-md bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                        :disabled="!canEnter || isActing"
                        :aria-label="t('embed.lottery.actions.enter')"
                        @click="runEntryAction('enter')"
                      >
                        <Loader2 v-if="isActing && canEnter" class="h-4 w-4 animate-spin" aria-hidden="true" />
                        <CheckCircle2 v-else class="h-4 w-4" aria-hidden="true" />
                        <span>{{ t('embed.lottery.actions.enter') }}</span>
                      </button>
                      <button
                        type="button"
                        class="inline-flex h-10 items-center justify-center gap-2 rounded-md border border-border bg-surface px-3 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:cursor-not-allowed disabled:opacity-60"
                        :disabled="!canWithdraw || isActing"
                        :aria-label="t('embed.lottery.actions.withdraw')"
                        @click="runEntryAction('withdraw')"
                      >
                        <Loader2 v-if="isActing && canWithdraw" class="h-4 w-4 animate-spin" aria-hidden="true" />
                        <Undo2 v-else class="h-4 w-4" aria-hidden="true" />
                        <span>{{ t('embed.lottery.actions.withdraw') }}</span>
                      </button>
                    </div>
                  </section>

                  <section class="rounded-lg border border-border/70 bg-background p-4">
                    <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><Trophy class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('embed.lottery.sections.myResult') }}</h3>
                    <div class="flex items-start gap-3 text-sm">
                      <CheckCircle2 v-if="resultState === 'won'" class="mt-0.5 h-5 w-5 text-emerald-600 dark:text-emerald-300" aria-hidden="true" />
                      <XCircle v-else-if="resultState === 'lost' || resultState === 'withdrawn'" class="mt-0.5 h-5 w-5 text-destructive" aria-hidden="true" />
                      <Clock3 v-else class="mt-0.5 h-5 w-5 text-muted-foreground" aria-hidden="true" />
                      <div class="min-w-0">
                        <div class="font-semibold">{{ t(`embed.lottery.result.${resultState}.title`) }}</div>
                        <p class="mt-1 text-muted-foreground">{{ t(`embed.lottery.result.${resultState}.description`) }}</p>
                      </div>
                    </div>
                    <dl v-if="myWinner" class="mt-4 space-y-3 text-sm">
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.prize') }}</dt><dd class="font-medium">{{ winnerPrize?.name ?? t('embed.lottery.common.empty') }}</dd></div>
                      <div><dt class="text-muted-foreground">{{ t('embed.lottery.fields.prizeSlot') }}</dt><dd class="font-medium tabular-nums">{{ myWinner.prizeSlot }}</dd></div>
                    </dl>
                    <dl v-if="myRewardStatus" class="mt-4 space-y-3 text-sm">
                      <div>
                        <dt class="text-muted-foreground">{{ t('embed.lottery.fields.rewardStatus') }}</dt>
                        <dd class="mt-1"><span class="rounded-full border px-2 py-0.5 text-xs" :class="statusBadgeClass(myRewardStatus.status)">{{ t(`embed.lottery.rewardStatus.${myRewardStatus.status}`) }}</span></dd>
                      </div>
                      <div v-if="myRewardStatus.errorKey"><dt class="text-muted-foreground">{{ t('embed.lottery.fields.rewardMessage') }}</dt><dd class="mt-1 break-words">{{ t(myRewardStatus.errorKey) }}</dd></div>
                      <div v-if="myRewardStatus.deliveryMode">
                        <dt class="text-muted-foreground">{{ t('embed.lottery.fields.deliveryMode') }}</dt>
                        <dd class="mt-1 font-medium">{{ t(`embed.lottery.deliveryMode.${myRewardStatus.deliveryMode}`) }}</dd>
                      </div>
                      <div v-if="myRewardStatus.voucherCode">
                        <dt class="text-muted-foreground">{{ t('embed.lottery.fields.voucherCode') }}</dt>
                        <dd class="mt-1 flex items-center gap-2 rounded-md border border-emerald-400/30 bg-emerald-500/10 p-2 text-emerald-800 dark:text-emerald-200">
                          <code class="min-w-0 flex-1 break-all font-mono text-sm font-semibold">{{ myRewardStatus.voucherCode }}</code>
                          <button type="button" class="grid h-8 w-8 shrink-0 place-items-center rounded-md hover:bg-emerald-500/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring" :aria-label="t('embed.lottery.actions.copyVoucher')" :title="t('embed.lottery.actions.copyVoucher')" @click="copyVoucherCode">
                            <CheckCircle2 v-if="copiedVoucher" class="h-4 w-4" aria-hidden="true" />
                            <Copy v-else class="h-4 w-4" aria-hidden="true" />
                          </button>
                        </dd>
                      </div>
                      <div v-if="myRewardStatus.manualContact">
                        <dt class="text-muted-foreground">{{ t('embed.lottery.fields.manualContact') }}</dt>
                        <dd class="mt-1 whitespace-pre-line break-words rounded-md border border-border/70 bg-surface p-2">{{ myRewardStatus.manualContact }}</dd>
                      </div>
                    </dl>
                  </section>
                </aside>
              </div>
            </div>
          </section>
        </div>
      </template>
    </div>
  </main>

  <Teleport to="body">
    <div
      v-if="drawReveal.isVisible.value"
      :ref="drawReveal.overlayRef"
      class="fixed inset-0 z-[100] grid min-h-screen place-items-center overflow-hidden bg-background/95 px-4 py-6 text-foreground"
      role="dialog"
      aria-modal="true"
      :aria-label="t('embed.lottery.drawReveal.ariaLabel')"
      @keydown.esc="closeDrawReveal"
    >
      <div class="pointer-events-none absolute inset-0" aria-hidden="true">
        <span
          v-for="index in 28"
          v-show="drawReveal.phase.value === 'result' && drawReveal.outcome.value === 'won'"
          :key="index"
          data-draw-particle
          class="draw-particle"
          :style="particleStyle(index)"
        />
      </div>

      <section data-draw-panel tabindex="-1" class="relative w-full max-w-lg rounded-lg border border-border bg-surface px-5 py-7 text-center shadow-2xl outline-none sm:px-8 sm:py-9">
        <div v-show="drawReveal.phase.value !== 'result'" data-draw-stage>
          <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {{ drawReveal.phase.value === 'countdown' ? t('embed.lottery.drawReveal.countdown.eyebrow') : t('embed.lottery.drawReveal.drawing.eyebrow') }}
          </p>

          <div class="mt-5 flex justify-center" aria-hidden="true">
            <div data-draw-wheel class="draw-wheel-shell">
              <span
                v-for="index in 12"
                :key="index"
                class="draw-wheel-dot"
                :style="{ '--draw-spoke': String(index - 1) }"
              />
              <span class="draw-wheel-core"><Sparkles class="h-8 w-8" /></span>
            </div>
          </div>

          <div class="mt-5 flex min-h-16 items-center justify-center" aria-live="assertive" aria-atomic="true">
            <span v-if="drawReveal.phase.value === 'countdown'" class="text-5xl font-semibold tabular-nums text-primary sm:text-6xl">{{ drawReveal.countdown.value }}</span>
            <span v-else class="text-xl font-semibold sm:text-2xl">{{ t('embed.lottery.drawReveal.drawing.title') }}</span>
          </div>
          <p class="mx-auto mt-2 max-w-sm text-sm text-muted-foreground">
            {{ drawReveal.phase.value === 'countdown' ? t('embed.lottery.drawReveal.countdown.description') : t('embed.lottery.drawReveal.drawing.description') }}
          </p>
        </div>

        <div v-if="drawReveal.phase.value === 'result'" data-draw-result aria-live="assertive" aria-atomic="true">
          <div
            class="mx-auto flex h-16 w-16 items-center justify-center rounded-full border"
            :class="drawReveal.outcome.value === 'won' ? 'border-amber-400/50 bg-amber-500/10 text-amber-600 dark:text-amber-300' : 'border-primary/35 bg-primary/10 text-primary'"
          >
            <Trophy v-if="drawReveal.outcome.value === 'won'" class="h-8 w-8" aria-hidden="true" />
            <Sparkles v-else class="h-8 w-8" aria-hidden="true" />
          </div>
          <p class="mt-5 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
            {{ t(`embed.lottery.drawReveal.${drawReveal.outcome.value}.eyebrow`) }}
          </p>
          <h2 class="mt-2 break-words text-2xl font-semibold tracking-normal sm:text-3xl">{{ drawRevealTitle }}</h2>
          <p class="mx-auto mt-3 max-w-sm text-sm text-muted-foreground">{{ drawRevealDescription }}</p>
          <button
            :ref="drawReveal.resultActionRef"
            type="button"
            class="mt-6 inline-flex h-10 items-center justify-center gap-2 rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            @click="closeDrawReveal"
          >
            <Trophy class="h-4 w-4" aria-hidden="true" />
            <span>{{ t('embed.lottery.drawReveal.viewResult') }}</span>
          </button>
        </div>
      </section>
    </div>
  </Teleport>
</template>

<style scoped>
.draw-wheel-shell {
  position: relative;
  width: 12rem;
  max-width: 58vw;
  aspect-ratio: 1;
  border: 1px solid hsl(var(--border));
  border-radius: 999px;
  background: hsl(var(--background));
  box-shadow: inset 0 0 0 0.75rem hsl(var(--primary) / 0.06);
}

.draw-wheel-shell::before,
.draw-wheel-shell::after {
  position: absolute;
  inset: 1.25rem;
  content: '';
  border: 1px dashed hsl(var(--primary) / 0.45);
  border-radius: 999px;
}

.draw-wheel-shell::after {
  inset: 2.35rem;
  border-style: solid;
  border-color: hsl(var(--border));
}

.draw-wheel-dot {
  position: absolute;
  top: 50%;
  left: 50%;
  z-index: 1;
  width: 0.72rem;
  height: 0.72rem;
  border: 2px solid hsl(var(--surface));
  border-radius: 999px;
  background: hsl(var(--primary));
  transform: translate(-50%, -50%) rotate(calc(var(--draw-spoke) * 30deg)) translateY(-4.65rem);
}

.draw-wheel-dot:nth-child(3n) {
  background: #f59e0b;
}

.draw-wheel-dot:nth-child(4n) {
  background: #10b981;
}

.draw-wheel-core {
  position: absolute;
  inset: 50% auto auto 50%;
  z-index: 2;
  display: grid;
  width: 4.5rem;
  aspect-ratio: 1;
  place-items: center;
  border: 1px solid hsl(var(--primary) / 0.4);
  border-radius: 999px;
  background: hsl(var(--surface));
  color: hsl(var(--primary));
  transform: translate(-50%, -50%);
}

.draw-particle {
  position: absolute;
  top: 46%;
  left: 50%;
  width: 0.55rem;
  height: 0.9rem;
  border-radius: var(--draw-radius);
  background: var(--draw-color);
  opacity: 0;
}

@media (max-width: 480px) {
  .draw-wheel-shell {
    width: 10rem;
  }

  .draw-wheel-dot {
    transform: translate(-50%, -50%) rotate(calc(var(--draw-spoke) * 30deg)) translateY(-3.9rem);
  }
}
</style>
