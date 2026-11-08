<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Check, Loader2, Search, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { searchVipCandidates, watchVip } from '../../api/vip'
import { formatCny } from '../../utils/dashboard'
import type { VipCandidate, VipRow } from '../../types/vip'

// 添加关注：搜站点用户（邮箱、用户名或 ID），选一个，写个备注。传了 row 时是改这个大户的备注
// （系统推荐的人写了备注就变成手动关注，不会因为排名变化掉出名单）。
const props = defineProps<{
  row?: VipRow
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saved'): void
}>()

const { t, te } = useI18n()

const query = ref('')
const results = ref<VipCandidate[]>([])
const picked = ref<VipCandidate | null>(null)
const note = ref(props.row?.source.note ?? '')
const isSearching = ref(false)
const isSaving = ref(false)
const errorKey = ref('')
let searchTimer: number | undefined
let searchSequence = 0

const isEdit = computed(() => Boolean(props.row))
const target = computed(() => props.row?.siteUserId ?? picked.value?.siteUserId ?? '')
const canSave = computed(() => !isSaving.value && target.value !== '')
const errorText = (key: string): string => (te(key) ? t(key) : t('admin.vip.errors.request'))
const candidateName = (item: VipCandidate): string => item.email || item.username || `#${item.siteUserId}`

watch(query, (value) => {
  window.clearTimeout(searchTimer)
  if (!value.trim()) {
    results.value = []
    return
  }
  searchTimer = window.setTimeout(async () => {
    const sequence = ++searchSequence
    isSearching.value = true
    errorKey.value = ''
    try {
      const items = await searchVipCandidates(value.trim())
      if (sequence === searchSequence) results.value = items
    } catch (error) {
      if (sequence === searchSequence) errorKey.value = error instanceof Error ? error.message : 'admin.vip.errors.request'
    } finally {
      if (sequence === searchSequence) isSearching.value = false
    }
  }, 300)
})

const save = async () => {
  if (!canSave.value) return
  isSaving.value = true
  errorKey.value = ''
  try {
    await watchVip(target.value, note.value)
    emit('saved')
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.vip.errors.request'
  } finally {
    isSaving.value = false
  }
}

const close = () => {
  if (!isSaving.value) emit('close')
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') close()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => {
  window.clearTimeout(searchTimer)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="vip-watch-title">
    <form class="flex max-h-[calc(100dvh-2rem)] w-full max-w-lg flex-col rounded-lg border border-border/60 bg-card shadow-xl" @submit.prevent="save">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <div class="min-w-0">
          <h2 id="vip-watch-title" class="text-lg font-semibold text-foreground">{{ t(isEdit ? 'admin.vip.watch.editTitle' : 'admin.vip.watch.addTitle') }}</h2>
          <p v-if="row" class="mt-0.5 truncate text-sm text-muted-foreground">{{ row.email || row.username || `#${row.siteUserId}` }}</p>
        </div>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.vip.actions.close')" :disabled="isSaving" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <template v-if="!isEdit">
          <div class="relative">
            <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <label class="sr-only" for="vip-watch-search">{{ t('admin.vip.watch.search') }}</label>
            <input
              id="vip-watch-search"
              v-model="query"
              type="search"
              autocomplete="off"
              autofocus
              :placeholder="t('admin.vip.watch.search')"
              class="h-10 w-full rounded-lg border border-border/60 bg-surface pl-9 pr-3 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
            >
          </div>
          <div v-if="isSearching" class="flex justify-center py-4 text-muted-foreground"><Loader2 class="h-4 w-4 animate-spin" /></div>
          <p v-else-if="query.trim() && results.length === 0" class="py-2 text-center text-sm text-muted-foreground">{{ t('admin.vip.watch.noResults') }}</p>
          <ul v-else-if="results.length > 0" class="max-h-64 divide-y divide-border/50 overflow-y-auto rounded-lg border border-border/60">
            <li v-for="item in results" :key="item.siteUserId">
              <button
                type="button"
                class="flex w-full items-center gap-3 px-3 py-2 text-left text-sm transition-colors hover:bg-surface focus-visible:bg-surface focus-visible:outline-none"
                :class="picked?.siteUserId === item.siteUserId ? 'bg-primary/10' : ''"
                :aria-pressed="picked?.siteUserId === item.siteUserId"
                @click="picked = item"
              >
                <span class="min-w-0 flex-1">
                  <span class="block truncate font-medium text-foreground">{{ candidateName(item) }}</span>
                  <span class="block truncate text-xs text-muted-foreground">
                    {{ t('admin.vip.watch.candidate', { balance: item.balance == null ? '—' : formatCny(item.balance), daily: formatCny(item.dailyAverage) }) }}
                  </span>
                </span>
                <span v-if="item.watched" class="shrink-0 rounded-md bg-primary/10 px-1.5 py-0.5 text-[11px] text-primary">{{ t('admin.vip.watch.watched') }}</span>
                <span v-else-if="item.dismissed" class="shrink-0 rounded-md bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ t('admin.vip.watch.dismissed') }}</span>
                <Check v-if="picked?.siteUserId === item.siteUserId" class="h-4 w-4 shrink-0 text-primary" />
              </button>
            </li>
          </ul>
        </template>

        <label class="block space-y-1 text-sm font-medium text-foreground">
          <span>{{ t('admin.vip.watch.note') }}</span>
          <textarea
            v-model="note"
            rows="2"
            maxlength="200"
            class="w-full resize-none rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
            :placeholder="t('admin.vip.watch.notePlaceholder')"
          />
        </label>
        <p v-if="isEdit && row && !row.source.manual" class="text-xs text-muted-foreground">{{ t('admin.vip.watch.pinHint') }}</p>

        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
      </div>

      <div class="flex shrink-0 flex-col-reverse gap-2 border-t border-border/50 p-4 sm:flex-row sm:justify-end">
        <Button type="button" variant="secondary" class="rounded-lg" :disabled="isSaving" @click="close">{{ t('admin.vip.actions.cancel') }}</Button>
        <Button type="submit" class="rounded-lg" :disabled="!canSave">
          <Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" />
          {{ t(isEdit ? 'admin.vip.actions.save' : 'admin.vip.actions.watch') }}
        </Button>
      </div>
    </form>
  </div>
</template>
