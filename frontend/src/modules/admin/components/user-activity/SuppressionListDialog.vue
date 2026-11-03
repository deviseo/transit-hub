<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, CheckCircle2, Loader2, Plus, Search, Trash2, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { addMassEmailSuppressions, listMassEmailSuppressions, removeMassEmailSuppression } from '../../api/massEmail'
import { useUserActivityFormat } from '../../composables/useUserActivityFormat'
import type { MassEmailSuppression } from '../../types/massEmail'

// 退订名单（按工作区）：收件人点击邮件里的退订链接后自动加入；也可以手动添加或移除。
const emit = defineEmits<{
  (event: 'close'): void
  (event: 'changed'): void
}>()

const { t, te } = useI18n()
const { formatCount, formatDateTime } = useUserActivityFormat()

const items = ref<MassEmailSuppression[]>([])
const page = ref(1)
const totalPages = ref(1)
const total = ref(0)
const searchDraft = ref('')
const search = ref('')
const addDraft = ref('')
const isLoading = ref(false)
const isSaving = ref(false)
const errorKey = ref('')
const successText = ref('')

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.userActivity.errors.request'))

const load = async (targetPage = page.value) => {
  isLoading.value = true
  errorKey.value = ''
  try {
    const result = await listMassEmailSuppressions(targetPage, 20, search.value)
    items.value = result.items
    page.value = result.page
    totalPages.value = Math.max(1, result.totalPages)
    total.value = result.total
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
  } finally {
    isLoading.value = false
  }
}

const applySearch = () => {
  search.value = searchDraft.value.trim()
  void load(1)
}

const add = async () => {
  const emails = addDraft.value.split(/[\s,;，；]+/).map((value) => value.trim()).filter(Boolean)
  if (emails.length === 0) return
  isSaving.value = true
  errorKey.value = ''
  successText.value = ''
  try {
    const result = await addMassEmailSuppressions(emails)
    successText.value = t('admin.userActivity.suppressions.added', { added: formatCount(result.added), invalid: formatCount(result.invalid) })
    addDraft.value = ''
    emit('changed')
    await load(1)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
  } finally {
    isSaving.value = false
  }
}

const remove = async (email: string) => {
  if (!window.confirm(t('admin.userActivity.suppressions.removeConfirm', { email }))) return
  isSaving.value = true
  errorKey.value = ''
  successText.value = ''
  try {
    await removeMassEmailSuppression(email)
    successText.value = t('admin.userActivity.suppressions.removed', { email })
    emit('changed')
    await load(items.value.length === 1 && page.value > 1 ? page.value - 1 : page.value)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
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

onMounted(() => {
  void load(1)
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="user-activity-suppressions-title">
    <div class="flex max-h-[calc(100dvh-2rem)] w-full max-w-2xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <div class="min-w-0">
          <h2 id="user-activity-suppressions-title" class="text-lg font-semibold text-foreground">{{ t('admin.userActivity.suppressions.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.userActivity.suppressions.description') }}</p>
        </div>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.userActivity.actions.close')" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
        <form class="space-y-2" @submit.prevent="add">
          <label for="user-activity-suppression-add" class="block text-sm font-medium text-foreground">{{ t('admin.userActivity.suppressions.addLabel') }}</label>
          <textarea
            id="user-activity-suppression-add"
            v-model="addDraft"
            rows="2"
            class="w-full rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
            :placeholder="t('admin.userActivity.suppressions.addPlaceholder')"
          />
          <div class="flex justify-end">
            <Button type="submit" size="sm" class="rounded-lg" :disabled="isSaving || !addDraft.trim()">
              <Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" />
              <Plus v-else class="h-4 w-4" />
              {{ t('admin.userActivity.suppressions.add') }}
            </Button>
          </div>
        </form>

        <form class="flex gap-2" @submit.prevent="applySearch">
          <div class="relative min-w-0 flex-1">
            <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              v-model="searchDraft"
              type="search"
              class="h-9 w-full rounded-lg border border-border/60 bg-surface py-2 pl-9 pr-3 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
              :placeholder="t('admin.userActivity.suppressions.searchPlaceholder')"
              :aria-label="t('admin.userActivity.suppressions.searchPlaceholder')"
            >
          </div>
          <Button type="submit" variant="secondary" size="sm" class="rounded-lg">{{ t('admin.userActivity.actions.search') }}</Button>
        </form>

        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
        <div v-if="successText" class="flex items-start gap-2 rounded-lg border border-emerald-400/20 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300" role="status">
          <CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ successText }}</span>
        </div>

        <div class="overflow-hidden rounded-lg border border-border/50" :class="{ 'opacity-60': isLoading }">
          <table class="w-full text-left text-sm">
            <thead class="bg-surface text-xs text-muted-foreground">
              <tr>
                <th class="px-3 py-2 font-medium">{{ t('admin.userActivity.suppressions.email') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.userActivity.suppressions.reason') }}</th>
                <th class="px-3 py-2 font-medium">{{ t('admin.userActivity.suppressions.createdAt') }}</th>
                <th class="px-3 py-2" />
              </tr>
            </thead>
            <tbody class="divide-y divide-border/40">
              <tr v-if="items.length === 0">
                <td colspan="4" class="px-3 py-6 text-center text-sm text-muted-foreground">
                  {{ isLoading ? t('admin.userActivity.status.loading') : t('admin.userActivity.suppressions.empty') }}
                </td>
              </tr>
              <tr v-for="item in items" :key="item.email">
                <td class="break-all px-3 py-2 text-foreground">{{ item.email }}</td>
                <td class="px-3 py-2 text-muted-foreground">
                  {{ te(`admin.userActivity.suppressions.reasons.${item.reason}`) ? t(`admin.userActivity.suppressions.reasons.${item.reason}`) : item.reason }}
                </td>
                <td class="whitespace-nowrap px-3 py-2 text-muted-foreground tabular-nums">{{ formatDateTime(item.createdAt) }}</td>
                <td class="px-3 py-2 text-right">
                  <button
                    type="button"
                    class="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground hover:bg-surface hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                    :title="t('admin.userActivity.suppressions.remove')"
                    :aria-label="t('admin.userActivity.suppressions.removeAria', { email: item.email })"
                    :disabled="isSaving"
                    @click="remove(item.email)"
                  >
                    <Trash2 class="h-4 w-4" />
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="flex shrink-0 items-center justify-between gap-2 border-t border-border/50 p-4 text-sm text-muted-foreground">
        <span>{{ t('admin.userActivity.pagination.total', { total: formatCount(total) }) }}</span>
        <div class="flex items-center gap-2">
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="page <= 1 || isLoading" @click="load(page - 1)">{{ t('admin.userActivity.pagination.previous') }}</Button>
          <span class="tabular-nums">{{ page }} / {{ totalPages }}</span>
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="page >= totalPages || isLoading" @click="load(page + 1)">{{ t('admin.userActivity.pagination.next') }}</Button>
        </div>
      </div>
    </div>
  </div>
</template>
