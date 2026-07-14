<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle,
  CalendarClock,
  Check,
  Copy,
  Eye,
  Gift,
  History,
  Loader2,
  Play,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Settings2,
  Shuffle,
  SquarePen,
  Ticket,
  Trophy,
  X,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import Tooltip from '@/components/ui/tooltip/Tooltip.vue'
import {
  cancelLotteryCampaign,
  closeLotteryCampaign,
  completeManualLotteryReward,
  createLotteryCampaign,
  drawLotteryCampaign,
  getLotteryCampaign,
  getLotteryEmbedConfig,
  listLotteryAudit,
  listLotteryCampaigns,
  listLotteryEntries,
  listLotterySubscriptionGroups,
  publishLotteryCampaign,
  retryLotteryReward,
  rotateLotteryEmbedToken,
  updateLotteryCampaign,
} from './api/lottery'
import type {
  LotteryAuditLog,
  LotteryCampaign,
  LotteryCampaignRequest,
  LotteryCampaignStatus,
  LotteryDrawMode,
  LotteryEmbedConfig,
  LotteryEntry,
  LotteryPrizeRequest,
  LotteryPrizeDeliveryMode,
  LotteryPrizeType,
  LotteryRewardStatus,
  LotterySubscriptionGroup,
} from './types'

const { t, locale } = useI18n()

const campaigns = ref<LotteryCampaign[]>([])
const selectedCampaign = ref<LotteryCampaign | null>(null)
const entries = ref<LotteryEntry[]>([])
const auditLogs = ref<LotteryAuditLog[]>([])
const embedConfig = ref<LotteryEmbedConfig | null>(null)
const statusFilter = ref('')
const activeTab = ref<'overview' | 'entries' | 'rewards' | 'audit' | 'embed'>('overview')
const isLoading = ref(false)
const isDetailLoading = ref(false)
const isSaving = ref(false)
const isActing = ref(false)
const isEmbedLoading = ref(false)
const isSubscriptionGroupsLoading = ref(false)
const isCopied = ref(false)
const editorOpen = ref(false)
const editingId = ref<string | null>(null)
const errorKey = ref('')
const formErrorKey = ref('')
const subscriptionGroupsErrorKey = ref('')
const subscriptionGroups = ref<LotterySubscriptionGroup[]>([])

const campaignStatuses: LotteryCampaignStatus[] = ['draft', 'scheduled', 'open', 'closed', 'drawing', 'drawn', 'fulfilling', 'completed', 'partial', 'cancelled']
const drawModes: LotteryDrawMode[] = ['manual', 'scheduled']
const prizeTypes: LotteryPrizeType[] = ['balance', 'subscription']
const deliveryModes: LotteryPrizeDeliveryMode[] = ['sub2api_auto', 'voucher', 'manual']

interface PrizeForm {
  type: LotteryPrizeType
  name: string
  quantity: string | number
  balanceAmount: string | number
  groupId: string
  groupName: string
  multiplier: string | number
  validityDays: string | number
  deliveryMode: LotteryPrizeDeliveryMode
  manualContact: string
  voucherCodes: string
}

interface CampaignForm {
  name: string
  description: string
  registrationStart: string
  registrationEnd: string
  drawAt: string
  drawMode: LotteryDrawMode
  publicWinners: boolean
  prizes: PrizeForm[]
}

const blankPrize = (): PrizeForm => ({
  type: 'balance',
  name: '',
  quantity: '1',
  balanceAmount: '',
  groupId: '',
  groupName: '',
  multiplier: '',
  validityDays: '',
  deliveryMode: 'sub2api_auto',
  manualContact: '',
  voucherCodes: '',
})

const blankForm = (): CampaignForm => ({
  name: '',
  description: '',
  registrationStart: '',
  registrationEnd: '',
  drawAt: '',
  drawMode: 'manual',
  publicWinners: true,
  prizes: [blankPrize()],
})

const form = ref<CampaignForm>(blankForm())

const filteredCampaigns = computed(() => (
  statusFilter.value ? campaigns.value.filter((campaign) => campaign.status === statusFilter.value) : campaigns.value
))

const selectedRewards = computed(() => selectedCampaign.value?.rewardStatuses ?? [])
const selectedWinners = computed(() => selectedCampaign.value?.winners ?? [])
const selectedPrizes = computed(() => selectedCampaign.value?.prizes ?? [])

const embedUrl = computed(() => {
  if (!embedConfig.value) return ''
  const url = new URL('/embed/lottery', window.location.origin)
  url.searchParams.set('embed_token', embedConfig.value.embedToken)
  return url.toString()
})

const formatDateTime = (value?: string): string => {
  if (!value) return t('admin.lottery.common.empty')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.lottery.common.empty')
  return new Intl.DateTimeFormat(locale.value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const toLocalInput = (value?: string): string => {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const offset = date.getTimezoneOffset() * 60000
  return new Date(date.getTime() - offset).toISOString().slice(0, 16)
}

const normalizeDateInput = (value: string): string => value.trim() ? `${value.trim()}:00` : ''

const normalizeFormText = (value: unknown): string => {
  if (typeof value === 'string') return value.trim()
  if (typeof value === 'number' && Number.isFinite(value)) return String(value)
  return ''
}

const isPositiveFiniteDecimal = (value: string): boolean => {
  const parsed = Number(value)
  return value !== '' && Number.isFinite(parsed) && parsed > 0
}

const isPositiveIntegerInRange = (value: string, min: number, max: number): boolean => {
  const parsed = Number(value)
  return Number.isInteger(parsed) && parsed >= min && parsed <= max
}

const statusBadgeClass = (status: string): string => {
  switch (status) {
    case 'open':
    case 'completed':
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
    case 'retryable_failed':
    case 'manual_attention':
    case 'failed':
      return 'border-destructive/30 bg-destructive/10 text-destructive'
    default:
      return 'border-border/70 bg-surface-elevated text-muted-foreground'
  }
}

const loadCampaigns = async () => {
  isLoading.value = true
  errorKey.value = ''
  try {
    const response = await listLotteryCampaigns()
    campaigns.value = response.items
    if (!selectedCampaign.value && response.items[0]) {
      await selectCampaign(response.items[0].id)
    } else if (selectedCampaign.value) {
      const next = response.items.find((campaign) => campaign.id === selectedCampaign.value?.id)
      if (next) selectedCampaign.value = next
    }
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isLoading.value = false
  }
}

const loadEmbedConfig = async () => {
  isEmbedLoading.value = true
  errorKey.value = ''
  try {
    embedConfig.value = await getLotteryEmbedConfig()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isEmbedLoading.value = false
  }
}

const subscriptionGroupForPrize = (prize: PrizeForm): LotterySubscriptionGroup | undefined => (
  subscriptionGroups.value.find((group) => group.id === prize.groupId)
)

const setPrizeSubscriptionGroup = (prize: PrizeForm) => {
  const selected = subscriptionGroupForPrize(prize)
  if (!selected) return
  prize.groupName = selected.name
}

const loadSubscriptionGroups = async () => {
  isSubscriptionGroupsLoading.value = true
  subscriptionGroupsErrorKey.value = ''
  subscriptionGroups.value = []
  try {
    const response = await listLotterySubscriptionGroups()
    subscriptionGroups.value = response.items
    if (editorOpen.value) {
      form.value.prizes
        .filter((prize) => prize.type === 'subscription')
        .forEach(setPrizeSubscriptionGroup)
    }
  } catch (error) {
    subscriptionGroupsErrorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isSubscriptionGroupsLoading.value = false
  }
}

async function selectCampaign(id: string) {
  isDetailLoading.value = true
  errorKey.value = ''
  try {
    const [campaign, entryResponse, auditResponse] = await Promise.all([
      getLotteryCampaign(id),
      listLotteryEntries(id),
      listLotteryAudit(id),
    ])
    selectedCampaign.value = campaign
    entries.value = entryResponse.items
    auditLogs.value = auditResponse.items
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isDetailLoading.value = false
  }
}

const openCreate = () => {
  editingId.value = null
  form.value = blankForm()
  formErrorKey.value = ''
  editorOpen.value = true
  void loadSubscriptionGroups()
}

const openEdit = (campaign: LotteryCampaign) => {
  editingId.value = campaign.id
  form.value = {
    name: campaign.name,
    description: campaign.description,
    registrationStart: toLocalInput(campaign.registrationStart),
    registrationEnd: toLocalInput(campaign.registrationEnd),
    drawAt: toLocalInput(campaign.drawAt),
    drawMode: campaign.drawMode,
    publicWinners: campaign.publicWinners,
    prizes: campaign.prizes.map((prize) => ({
      type: prize.type,
      name: prize.name,
      quantity: String(prize.quantity),
      balanceAmount: prize.balanceAmount ?? '',
      groupId: prize.groupId ?? '',
      groupName: prize.groupName ?? '',
      multiplier: prize.multiplier ?? '',
      validityDays: prize.validityDays == null ? '' : String(prize.validityDays),
      deliveryMode: prize.deliveryMode ?? 'sub2api_auto',
      manualContact: prize.manualContact ?? '',
      voucherCodes: (prize.voucherCodes ?? []).join('\n'),
    })),
  }
  if (form.value.prizes.length === 0) form.value.prizes = [blankPrize()]
  formErrorKey.value = ''
  editorOpen.value = true
  void loadSubscriptionGroups()
}

const addPrize = () => {
  form.value.prizes.push(blankPrize())
}

const removePrize = (index: number) => {
  if (form.value.prizes.length === 1) return
  form.value.prizes.splice(index, 1)
}

const setPrizeDeliveryMode = (prize: PrizeForm, mode: LotteryPrizeDeliveryMode) => {
  prize.deliveryMode = mode
  if (mode !== 'voucher') prize.voucherCodes = ''
  if (mode !== 'manual') prize.manualContact = ''
}

const voucherCodeCount = (prize: PrizeForm): number => (
  prize.voucherCodes.split(/\r?\n/).map((code) => code.trim()).filter(Boolean).length
)

const buildRequest = (): LotteryCampaignRequest | null => {
  const name = form.value.name.trim()
  if (!name || form.value.prizes.length === 0) {
    formErrorKey.value = 'admin.lottery.errors.validation'
    return null
  }
  const prizes: LotteryPrizeRequest[] = []
  for (const [index, prize] of form.value.prizes.entries()) {
    const prizeName = normalizeFormText(prize.name)
    const quantityText = normalizeFormText(prize.quantity)
    const balanceAmount = normalizeFormText(prize.balanceAmount)
    const groupId = normalizeFormText(prize.groupId)
    const groupName = normalizeFormText(prize.groupName)
    const multiplier = normalizeFormText(prize.multiplier)
    const validityDaysText = normalizeFormText(prize.validityDays)
    const quantity = Number.parseInt(quantityText, 10)
    const validityDays = validityDaysText ? Number.parseInt(validityDaysText, 10) : null
		const deliveryMode: LotteryPrizeDeliveryMode = prize.type === 'balance' ? prize.deliveryMode : 'sub2api_auto'
		const voucherCodes = prize.voucherCodes.split(/\r?\n/).map((code) => code.trim()).filter(Boolean)
		const manualContact = prize.manualContact.trim()
    if (!prizeName || !isPositiveIntegerInRange(quantityText, 1, 100000)) {
      formErrorKey.value = 'admin.lottery.errors.validation'
      return null
    }
    if (prize.type === 'balance' && !isPositiveFiniteDecimal(balanceAmount)) {
      formErrorKey.value = 'admin.lottery.errors.validation'
      return null
    }
		if (prize.type === 'balance' && deliveryMode === 'voucher' && (voucherCodes.length !== quantity || new Set(voucherCodes).size !== voucherCodes.length)) {
			formErrorKey.value = 'admin.lottery.errors.voucherQuantity'
			return null
		}
		if (prize.type === 'balance' && deliveryMode === 'manual' && !manualContact) {
			formErrorKey.value = 'admin.lottery.errors.manualContactRequired'
			return null
		}
    if (prize.type === 'subscription' && !isPositiveFiniteDecimal(multiplier)) {
      formErrorKey.value = 'admin.lottery.errors.validation'
      return null
    }
    if (prize.type === 'subscription' && (!groupId || !isPositiveIntegerInRange(validityDaysText, 1, 36500))) {
      formErrorKey.value = 'admin.lottery.errors.validation'
      return null
    }
    prizes.push({
      type: prize.type,
      name: prizeName,
      quantity,
      sortOrder: index + 1,
      balanceAmount: prize.type === 'balance' ? balanceAmount : '',
      groupId: prize.type === 'subscription' ? groupId : '',
      groupName: prize.type === 'subscription' ? groupName : '',
      multiplier: prize.type === 'subscription' ? multiplier : '',
      validityDays: prize.type === 'subscription' ? validityDays : null,
			deliveryMode,
			manualContact: deliveryMode === 'manual' ? manualContact : '',
			voucherCodes: deliveryMode === 'voucher' ? voucherCodes : [],
    })
  }
  return {
    name,
    description: form.value.description.trim(),
    registrationStart: normalizeDateInput(form.value.registrationStart),
    registrationEnd: normalizeDateInput(form.value.registrationEnd),
    drawAt: normalizeDateInput(form.value.drawAt),
    drawMode: form.value.drawMode,
    publicWinners: form.value.publicWinners,
    prizes,
  }
}

const saveCampaign = async () => {
  const request = buildRequest()
  if (!request) return
  isSaving.value = true
  formErrorKey.value = ''
  try {
    const campaign = editingId.value
      ? await updateLotteryCampaign(editingId.value, request)
      : await createLotteryCampaign(request)
    editorOpen.value = false
    selectedCampaign.value = campaign
    await loadCampaigns()
    await selectCampaign(campaign.id)
  } catch (error) {
    formErrorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isSaving.value = false
  }
}

const runAction = async (action: 'publish' | 'close' | 'draw' | 'cancel') => {
  if (!selectedCampaign.value) return
  if (!window.confirm(t(`admin.lottery.actions.confirm.${action}`))) return
  isActing.value = true
  errorKey.value = ''
  try {
    const id = selectedCampaign.value.id
    if (action === 'publish') await publishLotteryCampaign(id)
    if (action === 'close') await closeLotteryCampaign(id)
    if (action === 'draw') await drawLotteryCampaign(id)
    if (action === 'cancel') await cancelLotteryCampaign(id)
    await loadCampaigns()
    await selectCampaign(id)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isActing.value = false
  }
}

const retryReward = async (reward: LotteryRewardStatus) => {
  isActing.value = true
  errorKey.value = ''
  try {
    await retryLotteryReward(reward.id)
    if (selectedCampaign.value) await selectCampaign(selectedCampaign.value.id)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isActing.value = false
  }
}

const completeManualReward = async (reward: LotteryRewardStatus) => {
  if (reward.status !== 'manual_attention' || !selectedCampaign.value) return
  if (!window.confirm(t('admin.lottery.actions.confirm.completeManual'))) return
  isActing.value = true
  errorKey.value = ''
  try {
    await completeManualLotteryReward(reward.id)
    await selectCampaign(selectedCampaign.value.id)
    await loadCampaigns()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isActing.value = false
  }
}

const rotateEmbedToken = async () => {
  if (!window.confirm(t('admin.lottery.embed.confirmRotate'))) return
  isEmbedLoading.value = true
  errorKey.value = ''
  try {
    embedConfig.value = await rotateLotteryEmbedToken()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.lottery.errors.unknown'
  } finally {
    isEmbedLoading.value = false
  }
}

const copyEmbedUrl = async () => {
  if (!embedUrl.value) return
  try {
    await navigator.clipboard.writeText(embedUrl.value)
    isCopied.value = true
    window.setTimeout(() => { isCopied.value = false }, 1500)
  } catch {
    errorKey.value = 'admin.lottery.embed.copyFailed'
  }
}

const rewardPrizeName = (reward: LotteryRewardStatus): string => selectedPrizes.value.find((prize) => prize.id === reward.prizeId)?.name ?? reward.prizeId
const winnerEmail = (reward: LotteryRewardStatus): string => selectedWinners.value.find((winner) => winner.id === reward.winnerId)?.maskedEmail ?? t('admin.lottery.common.empty')
const rewardErrorMessage = (reward: LotteryRewardStatus): string => reward.errorKey ? t(reward.errorKey) : t('admin.lottery.errors.rewardSafeMessage')
const canEdit = computed(() => selectedCampaign.value?.status === 'draft')
const canPublish = computed(() => selectedCampaign.value?.status === 'draft')
const canClose = computed(() => selectedCampaign.value?.status === 'open')
const canDraw = computed(() => selectedCampaign.value?.status === 'closed')
const canCancel = computed(() => ['draft', 'scheduled', 'open', 'closed'].includes(selectedCampaign.value?.status ?? ''))

onMounted(async () => {
  await Promise.all([loadCampaigns(), loadEmbedConfig(), loadSubscriptionGroups()])
})
</script>

<template>
  <div class="mx-auto flex w-full max-w-[1480px] flex-col gap-4 p-4 sm:p-6 lg:h-[calc(100vh-8rem)] lg:p-8">
    <header class="flex flex-col gap-4 border-b border-border/60 pb-4 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <p class="text-xs font-semibold uppercase tracking-wide text-primary">{{ t('admin.lottery.eyebrow') }}</p>
        <h1 class="mt-1 text-2xl font-semibold text-foreground">{{ t('admin.lottery.title') }}</h1>
        <p class="mt-1 max-w-2xl text-sm text-muted-foreground">{{ t('admin.lottery.subtitle') }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <div class="relative">
          <select v-model="statusFilter" class="h-10 rounded-lg border border-border/70 bg-surface px-3 pr-9 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background" :aria-label="t('admin.lottery.filters.status')">
            <option value="">{{ t('admin.lottery.filters.all') }}</option>
            <option v-for="status in campaignStatuses" :key="status" :value="status">{{ t(`admin.lottery.status.${status}`) }}</option>
          </select>
        </div>
        <Tooltip :text="t('admin.lottery.actions.refresh')">
          <Button variant="secondary" :aria-label="t('admin.lottery.actions.refresh')" :disabled="isLoading" @click="loadCampaigns">
            <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" aria-hidden="true" />
            <RefreshCw v-else class="h-4 w-4" aria-hidden="true" />
          </Button>
        </Tooltip>
        <Button variant="secondary" :disabled="!selectedCampaign" @click="activeTab = 'embed'">
          <Settings2 class="h-4 w-4" aria-hidden="true" />
          {{ t('admin.lottery.embed.title') }}
        </Button>
        <Button @click="openCreate"><Plus class="h-4 w-4" aria-hidden="true" />{{ t('admin.lottery.actions.create') }}</Button>
      </div>
    </header>

    <div v-if="errorKey" class="flex items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" aria-live="polite">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <span>{{ t(errorKey) }}</span>
    </div>

    <div class="grid gap-4 lg:min-h-0 lg:flex-1 lg:grid-cols-[minmax(340px,420px)_1fr]">
      <section class="rounded-lg border border-border/70 bg-card lg:min-h-0 lg:overflow-hidden">
          <div class="flex items-center justify-between border-b border-border/60 px-4 py-3">
          <div class="flex items-center gap-2 text-sm font-semibold"><Gift class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('admin.lottery.list.title') }}</div>
          <span class="text-xs text-muted-foreground">{{ t('admin.lottery.list.count', { count: filteredCampaigns.length }) }}</span>
        </div>
        <div class="divide-y divide-border/60 lg:max-h-full lg:overflow-auto">
          <button
            v-for="campaign in filteredCampaigns"
            :key="campaign.id"
            type="button"
            class="flex w-full flex-col gap-2 px-4 py-3 text-left transition hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :class="selectedCampaign?.id === campaign.id ? 'border-l-2 border-primary bg-primary/5 text-foreground' : 'bg-card'"
            @click="selectCampaign(campaign.id)"
          >
            <div class="flex items-start justify-between gap-3">
              <span class="min-w-0 truncate text-sm font-semibold text-foreground">{{ campaign.name }}</span>
              <span class="shrink-0 rounded-full border px-2 py-0.5 text-[11px] font-semibold" :class="statusBadgeClass(campaign.status)">{{ t(`admin.lottery.status.${campaign.status}`) }}</span>
            </div>
            <div class="grid grid-cols-3 gap-2 text-xs text-muted-foreground">
              <span>{{ t('admin.lottery.metrics.entries', { count: campaign.entryCount }) }}</span>
              <span>{{ t('admin.lottery.metrics.winners', { count: campaign.winnerCount }) }}</span>
              <span>{{ t(`admin.lottery.drawMode.${campaign.drawMode}`) }}</span>
            </div>
          </button>
          <div v-if="!filteredCampaigns.length" class="p-8 text-center text-sm text-muted-foreground">{{ t('admin.lottery.list.empty') }}</div>
        </div>
      </section>

      <section class="lg:min-h-0 lg:overflow-hidden">
        <div v-if="!selectedCampaign" class="flex min-h-96 items-center justify-center text-sm text-muted-foreground lg:h-full">{{ t('admin.lottery.detail.empty') }}</div>
        <div v-else class="flex flex-col lg:h-full lg:min-h-0">
          <div class="border-b border-border/60 p-4">
            <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
              <div class="min-w-0">
                <div class="flex flex-wrap items-center gap-2">
                  <h2 class="truncate text-lg font-semibold text-foreground">{{ selectedCampaign.name }}</h2>
                  <span class="rounded-full border px-2 py-0.5 text-xs font-semibold" :class="statusBadgeClass(selectedCampaign.status)">{{ t(`admin.lottery.status.${selectedCampaign.status}`) }}</span>
                </div>
                <p class="mt-1 line-clamp-2 text-sm text-muted-foreground">{{ selectedCampaign.description || t('admin.lottery.common.noDescription') }}</p>
              </div>
              <div class="flex flex-wrap gap-2">
                <Tooltip :text="t('admin.lottery.actions.edit')"><Button variant="secondary" :disabled="!canEdit || isActing" :aria-label="t('admin.lottery.actions.edit')" @click="openEdit(selectedCampaign)"><SquarePen class="h-4 w-4" aria-hidden="true" /></Button></Tooltip>
                <Tooltip :text="t('admin.lottery.actions.publish')"><Button variant="secondary" :disabled="!canPublish || isActing" :aria-label="t('admin.lottery.actions.publish')" @click="runAction('publish')"><Play class="h-4 w-4" aria-hidden="true" /></Button></Tooltip>
                <Tooltip :text="t('admin.lottery.actions.close')"><Button variant="secondary" :disabled="!canClose || isActing" :aria-label="t('admin.lottery.actions.close')" @click="runAction('close')"><X class="h-4 w-4" aria-hidden="true" /></Button></Tooltip>
                <Tooltip :text="t('admin.lottery.actions.draw')"><Button variant="secondary" :disabled="!canDraw || isActing" :aria-label="t('admin.lottery.actions.draw')" @click="runAction('draw')"><Shuffle class="h-4 w-4" aria-hidden="true" /></Button></Tooltip>
                <Tooltip :text="t('admin.lottery.actions.cancel')"><Button variant="destructive" :disabled="!canCancel || isActing" :aria-label="t('admin.lottery.actions.cancel')" @click="runAction('cancel')"><X class="h-4 w-4" aria-hidden="true" /></Button></Tooltip>
              </div>
            </div>
            <div class="mt-4 flex flex-wrap gap-2 border-b border-border/60 text-sm">
              <button v-for="tab in ['overview', 'entries', 'rewards', 'audit', 'embed']" :key="tab" type="button" class="border-b-2 px-3 py-2 font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background" :class="activeTab === tab ? 'border-primary text-primary' : 'border-transparent text-muted-foreground hover:text-foreground'" @click="activeTab = tab as typeof activeTab">{{ t(`admin.lottery.tabs.${tab}`) }}</button>
            </div>
          </div>

          <div class="p-4 lg:min-h-0 lg:flex-1 lg:overflow-auto">
            <div v-if="isDetailLoading" class="flex min-h-80 items-center justify-center text-muted-foreground" aria-live="polite"><Loader2 class="h-5 w-5 animate-spin" aria-hidden="true" /></div>
            <template v-else>
              <div v-if="activeTab === 'overview'" class="grid gap-4 xl:grid-cols-2">
                <div class="rounded-lg border border-border/60 bg-surface p-4">
                  <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><CalendarClock class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('admin.lottery.sections.schedule') }}</h3>
                  <dl class="grid gap-3 text-sm sm:grid-cols-2">
                    <div><dt class="text-muted-foreground">{{ t('admin.lottery.fields.registrationStart') }}</dt><dd class="font-medium">{{ formatDateTime(selectedCampaign.registrationStart) }}</dd></div>
                    <div><dt class="text-muted-foreground">{{ t('admin.lottery.fields.registrationEnd') }}</dt><dd class="font-medium">{{ formatDateTime(selectedCampaign.registrationEnd) }}</dd></div>
                    <div><dt class="text-muted-foreground">{{ t('admin.lottery.fields.drawAt') }}</dt><dd class="font-medium">{{ formatDateTime(selectedCampaign.drawAt) }}</dd></div>
                    <div><dt class="text-muted-foreground">{{ t('admin.lottery.fields.publicWinners') }}</dt><dd class="font-medium">{{ selectedCampaign.publicWinners ? t('admin.lottery.common.yes') : t('admin.lottery.common.no') }}</dd></div>
                  </dl>
                </div>
                <div class="rounded-lg border border-border/60 bg-surface p-4">
                  <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><Ticket class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('admin.lottery.sections.integrity') }}</h3>
                  <dl class="space-y-3 text-sm">
                    <div><dt class="text-muted-foreground">{{ t('admin.lottery.fields.seedCommitment') }}</dt><dd class="break-all font-mono text-xs">{{ selectedCampaign.seedCommitment || t('admin.lottery.common.empty') }}</dd></div>
                    <div><dt class="text-muted-foreground">{{ t('admin.lottery.fields.entrySnapshotHash') }}</dt><dd class="break-all font-mono text-xs">{{ selectedCampaign.entrySnapshotHash || t('admin.lottery.common.empty') }}</dd></div>
                    <div><dt class="text-muted-foreground">{{ t('admin.lottery.fields.revealedSeed') }}</dt><dd class="break-all font-mono text-xs">{{ selectedCampaign.revealedSeed || t('admin.lottery.common.empty') }}</dd></div>
                  </dl>
                </div>
                <div class="rounded-lg border border-border/60 bg-surface p-4 xl:col-span-2">
                  <h3 class="mb-3 flex items-center gap-2 text-sm font-semibold"><Gift class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('admin.lottery.sections.prizes') }}</h3>
                  <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
                    <div v-for="prize in selectedPrizes" :key="prize.id" class="rounded-lg border border-border/60 bg-card p-3 text-sm">
                      <div class="flex items-center justify-between gap-2"><span class="font-semibold">{{ prize.name }}</span><span class="text-xs text-muted-foreground">{{ t(`admin.lottery.prizeType.${prize.type}`) }}</span></div>
                      <p class="mt-2 text-muted-foreground">{{ t('admin.lottery.fields.quantity') }}: {{ prize.quantity }}</p>
                      <p v-if="prize.type === 'balance'" class="text-muted-foreground">{{ t('admin.lottery.fields.balanceAmount') }}: {{ prize.balanceAmount }}</p>
                      <p v-else class="text-muted-foreground">{{ t('admin.lottery.prizes.subscriptionSummary', { group: prize.groupName || t('admin.lottery.common.empty'), id: prize.groupId || t('admin.lottery.common.empty'), multiplier: prize.multiplier || t('admin.lottery.common.empty'), days: prize.validityDays ?? t('admin.lottery.common.empty') }) }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <div v-if="activeTab === 'entries'" class="overflow-hidden rounded-lg border border-border/60">
                <table class="w-full min-w-[720px] text-left text-sm">
                  <thead class="bg-surface text-xs uppercase text-muted-foreground"><tr><th class="px-3 py-2">{{ t('admin.lottery.fields.maskedEmail') }}</th><th class="px-3 py-2">{{ t('admin.lottery.fields.receiptHash') }}</th><th class="px-3 py-2">{{ t('admin.lottery.fields.status') }}</th><th class="px-3 py-2">{{ t('admin.lottery.fields.createdAt') }}</th></tr></thead>
                  <tbody class="divide-y divide-border/60"><tr v-for="entry in entries" :key="entry.id"><td class="px-3 py-2 font-medium">{{ entry.maskedEmail }}</td><td class="px-3 py-2 font-mono text-xs">{{ entry.receiptHash }}</td><td class="px-3 py-2"><span class="rounded-full border px-2 py-0.5 text-xs" :class="statusBadgeClass(entry.status)">{{ t(`admin.lottery.entryStatus.${entry.status}`) }}</span></td><td class="px-3 py-2">{{ formatDateTime(entry.createdAt) }}</td></tr></tbody>
                </table>
                <div v-if="!entries.length" class="p-6 text-center text-sm text-muted-foreground">{{ t('admin.lottery.entries.empty') }}</div>
              </div>

              <div v-if="activeTab === 'rewards'" class="space-y-4">
                <div class="overflow-hidden rounded-lg border border-border/60">
                  <table class="w-full min-w-[760px] text-left text-sm">
                    <thead class="bg-surface text-xs uppercase text-muted-foreground"><tr><th class="px-3 py-2">{{ t('admin.lottery.fields.winner') }}</th><th class="px-3 py-2">{{ t('admin.lottery.fields.prize') }}</th><th class="px-3 py-2">{{ t('admin.lottery.fields.rewardStatus') }}</th><th class="px-3 py-2">{{ t('admin.lottery.fields.error') }}</th><th class="px-3 py-2">{{ t('admin.lottery.fields.actions') }}</th></tr></thead>
                    <tbody class="divide-y divide-border/60"><tr v-for="reward in selectedRewards" :key="reward.id"><td class="px-3 py-2">{{ winnerEmail(reward) }}</td><td class="px-3 py-2 font-medium">{{ rewardPrizeName(reward) }}</td><td class="px-3 py-2"><span class="rounded-full border px-2 py-0.5 text-xs" :class="statusBadgeClass(reward.status)">{{ t(`admin.lottery.rewardStatus.${reward.status}`) }}</span></td><td class="px-3 py-2 text-xs text-muted-foreground">{{ rewardErrorMessage(reward) }}</td><td class="px-3 py-2"><Button size="sm" variant="secondary" :disabled="reward.status !== 'retryable_failed' || isActing" @click="retryReward(reward)"><RotateCcw class="h-4 w-4" aria-hidden="true" />{{ t('admin.lottery.actions.retry') }}</Button></td></tr></tbody>
                  </table>
                  <div v-if="selectedRewards.some((reward) => reward.status === 'manual_attention')" class="border-t border-border/60 bg-surface/60 p-3">
                    <div class="mb-2 text-xs font-semibold text-muted-foreground">{{ t('admin.lottery.rewards.manualTitle') }}</div>
                    <div class="flex flex-col gap-2">
                      <div v-for="reward in selectedRewards.filter((item) => item.status === 'manual_attention')" :key="`manual-${reward.id}`" class="flex flex-col gap-2 rounded-md border border-border/70 bg-background p-3 sm:flex-row sm:items-center sm:justify-between">
                        <div class="min-w-0 text-sm">
                          <span class="font-medium">{{ winnerEmail(reward) }}</span>
                          <span class="mx-2 text-muted-foreground">·</span>
                          <span>{{ rewardPrizeName(reward) }}</span>
                        </div>
                        <Button size="sm" :disabled="isActing" @click="completeManualReward(reward)"><Check class="h-4 w-4" aria-hidden="true" />{{ t('admin.lottery.actions.completeManual') }}</Button>
                      </div>
                    </div>
                  </div>
                  <div v-if="!selectedRewards.length" class="p-6 text-center text-sm text-muted-foreground">{{ t('admin.lottery.rewards.empty') }}</div>
                </div>
                <div class="grid gap-3 md:grid-cols-2 xl:grid-cols-3"><div v-for="winner in selectedWinners" :key="winner.id" class="rounded-lg border border-border/60 bg-surface p-3 text-sm"><div class="flex items-center gap-2 font-semibold"><Trophy class="h-4 w-4 text-primary" aria-hidden="true" />{{ winner.maskedEmail }}</div><p class="mt-2 text-muted-foreground">{{ t('admin.lottery.fields.prizeSlot') }}: {{ winner.prizeSlot }}</p></div></div>
              </div>

              <div v-if="activeTab === 'audit'" class="space-y-3">
                <div v-for="log in auditLogs" :key="log.ID" class="rounded-lg border border-border/60 bg-surface p-3 text-sm">
                  <div class="flex flex-wrap items-center justify-between gap-2"><span class="flex items-center gap-2 font-semibold"><History class="h-4 w-4 text-primary" aria-hidden="true" />{{ t(`admin.lottery.audit.${log.Event}`, log.Event) }}</span><span class="text-xs text-muted-foreground">{{ formatDateTime(log.CreatedAt) }}</span></div>
                  <pre v-if="log.Detail" class="mt-2 overflow-auto rounded-lg bg-background p-2 text-xs text-muted-foreground">{{ JSON.stringify(log.Detail, null, 2) }}</pre>
                </div>
                <div v-if="!auditLogs.length" class="p-6 text-center text-sm text-muted-foreground">{{ t('admin.lottery.audit.empty') }}</div>
              </div>

              <div v-if="activeTab === 'embed'" class="max-w-3xl space-y-4">
                <div class="rounded-lg border border-border/60 bg-surface p-4">
                  <h3 class="flex items-center gap-2 text-sm font-semibold"><Settings2 class="h-4 w-4 text-primary" aria-hidden="true" />{{ t('admin.lottery.embed.title') }}</h3>
                  <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.lottery.embed.subtitle') }}</p>
                  <div v-if="isEmbedLoading" class="flex min-h-28 items-center justify-center" aria-live="polite"><Loader2 class="h-5 w-5 animate-spin text-muted-foreground" aria-hidden="true" /></div>
                  <div v-else class="mt-4 space-y-4">
                    <div><p class="text-sm font-medium">{{ t('admin.lottery.embed.sourceOrigin') }}</p><p class="mt-1 break-all rounded-lg border border-border/60 bg-card p-3 text-sm">{{ embedConfig?.sub2apiSourceOrigin || t('admin.lottery.common.empty') }}</p></div>
                    <div><label for="lottery-embed-url" class="text-sm font-medium">{{ t('admin.lottery.embed.url') }}</label><div class="mt-1 flex gap-2"><Input id="lottery-embed-url" :model-value="embedUrl" readonly /><Button variant="secondary" :aria-label="t('admin.lottery.embed.copy')" :title="t('admin.lottery.embed.copy')" @click="copyEmbedUrl"><Check v-if="isCopied" class="h-4 w-4" aria-hidden="true" /><Copy v-else class="h-4 w-4" aria-hidden="true" /></Button></div></div>
                    <Button variant="secondary" :disabled="isEmbedLoading" @click="rotateEmbedToken"><RefreshCw class="h-4 w-4" aria-hidden="true" />{{ t('admin.lottery.embed.rotate') }}</Button>
                  </div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </section>
    </div>

    <Teleport to="body">
      <div v-if="editorOpen" class="fixed inset-0 z-[160] flex items-center justify-center bg-background/70 p-4 backdrop-blur-sm" @click.self="editorOpen = false">
        <form role="dialog" aria-modal="true" :aria-label="t(editingId ? 'admin.lottery.form.editTitle' : 'admin.lottery.form.createTitle')" class="flex max-h-[90vh] w-full max-w-4xl flex-col overflow-hidden rounded-lg border border-border bg-card shadow-2xl" autocomplete="off" @submit.prevent="saveCampaign">
          <header class="flex items-center justify-between border-b border-border/60 px-5 py-4"><div><h2 class="text-base font-semibold">{{ t(editingId ? 'admin.lottery.form.editTitle' : 'admin.lottery.form.createTitle') }}</h2><p class="text-xs text-muted-foreground">{{ t('admin.lottery.form.subtitle') }}</p></div><button type="button" class="rounded-lg p-2 text-muted-foreground hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background" :aria-label="t('admin.lottery.actions.closeDialog')" @click="editorOpen = false"><X class="h-4 w-4" aria-hidden="true" /></button></header>
          <div class="min-h-0 flex-1 space-y-5 overflow-auto p-5">
            <div v-if="formErrorKey" class="rounded-lg border border-destructive/30 bg-destructive/10 p-3 text-sm text-destructive" aria-live="polite">{{ t(formErrorKey) }}</div>
            <div class="grid gap-4 md:grid-cols-2"><label class="space-y-1 text-sm font-medium">{{ t('admin.lottery.fields.name') }}<Input v-model="form.name" name="lottery-name" autocomplete="off" :placeholder="t('admin.lottery.form.namePlaceholder')" /></label><label class="space-y-1 text-sm font-medium">{{ t('admin.lottery.fields.drawMode') }}<select v-model="form.drawMode" name="lottery-draw-mode" autocomplete="off" class="h-11 w-full rounded-lg border border-border/70 bg-surface px-4 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background"><option v-for="mode in drawModes" :key="mode" :value="mode">{{ t(`admin.lottery.drawMode.${mode}`) }}</option></select></label></div>
            <label class="block space-y-1 text-sm font-medium">{{ t('admin.lottery.fields.description') }}<textarea v-model="form.description" name="lottery-description" autocomplete="off" rows="3" class="w-full resize-none rounded-lg border border-border/70 bg-surface px-4 py-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background" :placeholder="t('admin.lottery.form.descriptionPlaceholder')" /></label>
            <div class="grid gap-4 md:grid-cols-3"><label class="space-y-1 text-sm font-medium">{{ t('admin.lottery.fields.registrationStart') }}<Input v-model="form.registrationStart" name="lottery-registration-start" autocomplete="off" type="datetime-local" /></label><label class="space-y-1 text-sm font-medium">{{ t('admin.lottery.fields.registrationEnd') }}<Input v-model="form.registrationEnd" name="lottery-registration-end" autocomplete="off" type="datetime-local" /></label><label class="space-y-1 text-sm font-medium">{{ t('admin.lottery.fields.drawAt') }}<Input v-model="form.drawAt" name="lottery-draw-at" autocomplete="off" type="datetime-local" /></label></div>
            <label class="flex items-center gap-3 rounded-lg border border-border/60 bg-surface p-3 text-sm"><input v-model="form.publicWinners" name="lottery-public-winners" autocomplete="off" type="checkbox" class="h-4 w-4 rounded border-border text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background" />{{ t('admin.lottery.fields.publicWinners') }}</label>
            <section class="space-y-3">
              <div class="flex items-center justify-between">
                <h3 class="text-sm font-semibold">{{ t('admin.lottery.sections.prizes') }}</h3>
                <Button size="sm" variant="secondary" @click="addPrize"><Plus class="h-4 w-4" aria-hidden="true" />{{ t('admin.lottery.form.addPrize') }}</Button>
              </div>
              <div v-for="(prize, index) in form.prizes" :key="index" class="rounded-lg border border-border/60 bg-surface p-4">
                <div class="mb-3 flex items-center justify-between">
                  <span class="text-sm font-semibold">{{ t('admin.lottery.form.prizeNumber', { number: index + 1 }) }}</span>
                  <Button size="sm" variant="ghost" :disabled="form.prizes.length === 1" @click="removePrize(index)"><X class="h-4 w-4" aria-hidden="true" />{{ t('admin.lottery.form.removePrize') }}</Button>
                </div>
                <div class="grid gap-3 md:grid-cols-4">
                  <label class="space-y-1 text-sm font-medium">
                    {{ t('admin.lottery.fields.prizeType') }}
                    <select v-model="prize.type" :name="`lottery-prize-${index}-type`" autocomplete="off" class="h-11 w-full rounded-lg border border-border/70 bg-background px-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background">
                      <option v-for="type in prizeTypes" :key="type" :value="type">{{ t(`admin.lottery.prizeType.${type}`) }}</option>
                    </select>
                  </label>
                  <label class="space-y-1 text-sm font-medium md:col-span-2">
                    {{ t('admin.lottery.fields.prizeName') }}
                    <Input v-model="prize.name" :name="`lottery-prize-${index}-name`" autocomplete="off" :placeholder="t('admin.lottery.form.prizeNamePlaceholder')" />
                  </label>
                  <label class="space-y-1 text-sm font-medium">
                    {{ t('admin.lottery.fields.quantity') }}
                    <Input :model-value="normalizeFormText(prize.quantity)" :name="`lottery-prize-${index}-quantity`" autocomplete="off" type="number" inputmode="numeric" min="1" max="100000" step="1" @update:model-value="prize.quantity = $event" />
                  </label>
                </div>
                <div v-if="prize.type === 'balance'" class="mt-3 grid gap-3 md:grid-cols-2">
                  <label class="space-y-1 text-sm font-medium">
                    {{ t('admin.lottery.fields.balanceAmount') }}
                    <Input :model-value="normalizeFormText(prize.balanceAmount)" :name="`lottery-prize-${index}-balance`" autocomplete="off" type="number" inputmode="decimal" min="0" step="0.01" @update:model-value="prize.balanceAmount = $event" />
                  </label>
                </div>
                <div v-else class="mt-3 grid gap-3 md:grid-cols-4">
                  <div class="space-y-1 md:col-span-2">
                    <label :for="`lottery-prize-${index}-subscription-group`" class="text-sm font-medium">{{ t('admin.lottery.fields.subscriptionGroup') }}</label>
                    <select
                      :id="`lottery-prize-${index}-subscription-group`"
                      v-model="prize.groupId"
                      :name="`lottery-prize-${index}-subscription-group`"
                      autocomplete="off"
                      class="h-11 w-full rounded-lg border border-border/70 bg-background px-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-60"
                      :disabled="isSubscriptionGroupsLoading"
                      @change="setPrizeSubscriptionGroup(prize)"
                    >
                      <option value="" disabled>{{ t(isSubscriptionGroupsLoading ? 'admin.lottery.form.subscriptionGroupsLoading' : subscriptionGroups.length ? 'admin.lottery.form.subscriptionGroupPlaceholder' : 'admin.lottery.form.subscriptionGroupsEmpty') }}</option>
                      <option v-if="prize.groupId && !subscriptionGroupForPrize(prize)" :value="prize.groupId">
                        {{ t('admin.lottery.form.subscriptionGroupUnavailable', { name: prize.groupName || t('admin.lottery.common.empty'), id: prize.groupId, multiplier: prize.multiplier || t('admin.lottery.common.empty') }) }}
                      </option>
                      <option v-for="group in subscriptionGroups" :key="group.id" :value="group.id">
                        {{ t('admin.lottery.form.subscriptionGroupOption', { name: group.name, id: group.id, multiplier: group.multiplier }) }}
                      </option>
                    </select>
                    <div v-if="prize.groupId" class="flex min-h-6 items-center gap-2 text-xs text-muted-foreground">
                      <span>{{ t('admin.lottery.fields.currentMultiplier') }}: <strong class="font-semibold text-foreground">{{ subscriptionGroupForPrize(prize)?.multiplier || t('admin.lottery.common.empty') }}</strong></span>
                    </div>
                    <div v-if="subscriptionGroupsErrorKey" class="flex flex-wrap items-center gap-2 text-xs text-destructive" aria-live="polite">
                      <span>{{ t(subscriptionGroupsErrorKey) }}</span>
                      <Button type="button" size="sm" variant="ghost" :disabled="isSubscriptionGroupsLoading" @click="loadSubscriptionGroups">
                        <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': isSubscriptionGroupsLoading }" aria-hidden="true" />
                        {{ t('admin.lottery.form.refreshSubscriptionGroups') }}
                      </Button>
                    </div>
                  </div>
                  <label class="space-y-1 text-sm font-medium">
                    {{ t('admin.lottery.fields.rewardMultiplier') }}
                    <Input :model-value="normalizeFormText(prize.multiplier)" :name="`lottery-prize-${index}-reward-multiplier`" autocomplete="off" type="number" inputmode="decimal" min="0.0001" step="0.0001" required @update:model-value="prize.multiplier = $event" />
                  </label>
                  <label class="space-y-1 text-sm font-medium">
                    {{ t('admin.lottery.fields.validityDays') }}
                    <Input :model-value="normalizeFormText(prize.validityDays)" :name="`lottery-prize-${index}-validity-days`" autocomplete="off" type="number" inputmode="numeric" min="1" max="36500" step="1" @update:model-value="prize.validityDays = $event" />
                  </label>
                </div>
              </div>
            </section>
            <section v-if="form.prizes.some((prize) => prize.type === 'balance')" class="space-y-3 border-t border-border/60 pt-5">
              <div>
                <h3 class="text-sm font-semibold">{{ t('admin.lottery.delivery.title') }}</h3>
                <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.lottery.delivery.subtitle') }}</p>
              </div>
              <div
                v-for="(prize, index) in form.prizes"
                v-show="prize.type === 'balance'"
                :key="`delivery-${index}`"
                class="rounded-lg border border-border/60 bg-surface p-4"
              >
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="min-w-0">
                    <p class="truncate text-sm font-semibold">{{ prize.name || t('admin.lottery.form.prizeNumber', { number: index + 1 }) }}</p>
                    <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.lottery.delivery.quantityHint', { count: normalizeFormText(prize.quantity) || 0 }) }}</p>
                  </div>
                  <div class="grid w-full grid-cols-3 rounded-md border border-border/70 bg-background p-1 sm:w-auto" role="group" :aria-label="t('admin.lottery.fields.deliveryMode')">
                    <button
                      v-for="mode in deliveryModes"
                      :key="mode"
                      type="button"
                      class="min-h-9 px-3 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      :class="prize.deliveryMode === mode ? 'bg-primary text-primary-foreground' : 'text-muted-foreground hover:bg-accent hover:text-accent-foreground'"
                      @click="setPrizeDeliveryMode(prize, mode)"
                    >
                      {{ t(`admin.lottery.delivery.mode.${mode}`) }}
                    </button>
                  </div>
                </div>
                <div v-if="prize.deliveryMode === 'voucher'" class="mt-4">
                  <label class="text-sm font-medium" :for="`lottery-prize-${index}-vouchers`">{{ t('admin.lottery.fields.voucherCodes') }}</label>
                  <textarea
                    :id="`lottery-prize-${index}-vouchers`"
                    v-model="prize.voucherCodes"
                    :name="`lottery-prize-${index}-vouchers`"
                    rows="5"
                    class="mt-1 w-full resize-y rounded-md border border-border/70 bg-background px-3 py-2 font-mono text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    :placeholder="t('admin.lottery.delivery.voucherPlaceholder')"
                  />
                  <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.lottery.delivery.voucherCount', { current: voucherCodeCount(prize), required: normalizeFormText(prize.quantity) || 0 }) }}</p>
                </div>
                <div v-else-if="prize.deliveryMode === 'manual'" class="mt-4">
                  <label class="text-sm font-medium" :for="`lottery-prize-${index}-contact`">{{ t('admin.lottery.fields.manualContact') }}</label>
                  <textarea
                    :id="`lottery-prize-${index}-contact`"
                    v-model="prize.manualContact"
                    :name="`lottery-prize-${index}-contact`"
                    rows="3"
                    class="mt-1 w-full resize-y rounded-md border border-border/70 bg-background px-3 py-2 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    :placeholder="t('admin.lottery.delivery.manualPlaceholder')"
                  />
                  <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.lottery.delivery.manualHint') }}</p>
                </div>
                <p v-else class="mt-4 text-sm text-muted-foreground">{{ t('admin.lottery.delivery.autoHint') }}</p>
              </div>
            </section>
          </div>
          <footer class="flex justify-end gap-2 border-t border-border/60 px-5 py-4"><Button variant="secondary" type="button" @click="editorOpen = false">{{ t('admin.lottery.actions.closeDialog') }}</Button><Button type="submit" :disabled="isSaving"><Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" aria-hidden="true" /><Save v-else class="h-4 w-4" aria-hidden="true" />{{ t('admin.lottery.actions.save') }}</Button></footer>
        </form>
      </div>
    </Teleport>
  </div>
</template>
