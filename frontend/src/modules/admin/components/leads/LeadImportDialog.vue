<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, CheckCircle2, FileUp, Info, Loader2, Upload, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { importLeads } from '../../api/leads'
import type { LeadImportResult } from '../../types/leads'

// 批量导入：粘贴一堆邮箱（换行、逗号、空格分隔都行），或上传 TXT / CSV。这一批可以统一填一个来源。
// 后端会去掉格式不对的、重复的和已退订的，导完显示结果。
defineProps<{
  sources: string[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'imported', result: LeadImportResult): void
}>()

const { t, te } = useI18n()

const maxFileBytes = 1024 * 1024
const text = ref('')
const source = ref('')
const fileName = ref('')
const fileInput = ref<HTMLInputElement | null>(null)
const isSubmitting = ref(false)
const errorKey = ref('')
const result = ref<LeadImportResult | null>(null)

const emailCount = computed(() => (text.value.match(/@/g) ?? []).length)
const canSubmit = computed(() => !isSubmitting.value && emailCount.value > 0)
const errorText = (key: string): string => (te(key) ? t(key) : t('admin.leads.errors.request'))

const pickFile = () => fileInput.value?.click()

const onFile = async (event: Event) => {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file) return
  errorKey.value = ''
  if (file.size > maxFileBytes) {
    errorKey.value = 'admin.leads.errors.fileTooLarge'
    return
  }
  try {
    text.value = await file.text()
    fileName.value = file.name
  } catch {
    errorKey.value = 'admin.leads.errors.fileRead'
  }
}

const submit = async () => {
  if (!canSubmit.value) return
  isSubmitting.value = true
  errorKey.value = ''
  try {
    result.value = await importLeads(text.value, source.value)
    emit('imported', result.value)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.leads.errors.request'
  } finally {
    isSubmitting.value = false
  }
}

const close = () => {
  if (!isSubmitting.value) emit('close')
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') close()
}

onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="lead-import-title">
    <div class="flex max-h-[calc(100dvh-2rem)] w-full max-w-xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <div class="min-w-0">
          <h2 id="lead-import-title" class="text-lg font-semibold text-foreground">{{ t('admin.leads.import.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.leads.import.description') }}</p>
        </div>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.leads.actions.close')" :disabled="isSubmitting" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <template v-if="result">
          <div class="flex items-start gap-2 rounded-lg border border-emerald-400/20 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300" role="status">
            <CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0" />
            <span>{{ t('admin.leads.import.done') }}</span>
          </div>
          <dl class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div class="rounded-lg bg-primary/10 p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.leads.import.added') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ result.added }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.leads.import.duplicates') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ result.duplicates }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.leads.import.invalid') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ result.invalid }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.leads.import.suppressed') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ result.suppressed }}</dd>
            </div>
          </dl>
        </template>
        <template v-else>
          <label class="block space-y-1 text-sm font-medium text-foreground">
            <span>{{ t('admin.leads.fields.source') }}</span>
            <input
              v-model="source"
              type="text"
              list="lead-import-sources"
              maxlength="40"
              class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
              :placeholder="t('admin.leads.import.sourcePlaceholder')"
            >
            <datalist id="lead-import-sources">
              <option v-for="item in sources" :key="item" :value="item" />
            </datalist>
          </label>
          <div class="space-y-1">
            <div class="flex items-center justify-between gap-2">
              <label for="lead-import-text" class="text-sm font-medium text-foreground">{{ t('admin.leads.import.emails') }}</label>
              <Button type="button" variant="secondary" size="sm" class="rounded-lg" @click="pickFile">
                <FileUp class="h-4 w-4" />
                {{ t('admin.leads.import.chooseFile') }}
              </Button>
              <input ref="fileInput" type="file" accept=".txt,.csv,text/plain,text/csv" class="hidden" @change="onFile">
            </div>
            <textarea
              id="lead-import-text"
              v-model="text"
              rows="9"
              class="w-full resize-y rounded-lg border border-border/60 bg-surface px-3 py-2 font-mono text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
              :placeholder="t('admin.leads.import.placeholder')"
            />
            <p class="text-xs text-muted-foreground">
              <span v-if="fileName">{{ t('admin.leads.import.fromFile', { name: fileName }) }} · </span>
              {{ t('admin.leads.import.found', { count: emailCount }) }}
            </p>
          </div>
          <p class="flex items-start gap-2 text-xs text-muted-foreground">
            <Info class="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{{ t('admin.leads.import.hint') }}</span>
          </p>
        </template>

        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
      </div>

      <div class="flex shrink-0 flex-col-reverse gap-2 border-t border-border/50 p-4 sm:flex-row sm:justify-end">
        <template v-if="result">
          <Button class="rounded-lg" @click="close">{{ t('admin.leads.actions.done') }}</Button>
        </template>
        <template v-else>
          <Button variant="secondary" class="rounded-lg" :disabled="isSubmitting" @click="close">{{ t('admin.leads.actions.cancel') }}</Button>
          <Button class="rounded-lg" :disabled="!canSubmit" @click="submit">
            <Loader2 v-if="isSubmitting" class="h-4 w-4 animate-spin" />
            <Upload v-else class="h-4 w-4" />
            {{ t('admin.leads.import.submit') }}
          </Button>
        </template>
      </div>
    </div>
  </div>
</template>
