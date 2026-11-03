<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, Send, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { getEmailTemplates } from '../../api/settings'
import { createMassEmailBatch } from '../../api/massEmail'
import { defaultUnsubscribeBaseUrl, isLikelyPrivateUrl, previewUserActivityRecipients } from '../../api/userActivity'
import { useUserActivityFormat } from '../../composables/useUserActivityFormat'
import type { EmailTemplate } from '../../types/settings'
import type { MassEmailBatch } from '../../types/massEmail'
import type { UserActivityFilter, UserActivityRecipientPreview } from '../../types/userActivity'

// 按当前筛选条件群发：收件人在后端按筛选条件从本地同步的用户快照解析（有邮箱、不在退订名单中）。
const props = defineProps<{
  filter: UserActivityFilter
  filterSummary: string
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'created', batch: MassEmailBatch): void
}>()

const { t, te } = useI18n()
const { formatCount } = useUserActivityFormat()

const templates = ref<EmailTemplate[]>([])
const selectedTemplateId = ref('')
const preview = ref<UserActivityRecipientPreview | null>(null)
const includeUnsubscribe = ref(true)
const publicBaseUrl = ref(defaultUnsubscribeBaseUrl())
const isLoading = ref(true)
const isSubmitting = ref(false)
const errorKey = ref('')
const requestId = ref(crypto.randomUUID())

const unsubscribePlaceholder = '{{unsubscribe_url}}'
const selectedTemplate = computed(() => templates.value.find((template) => template.id === selectedTemplateId.value) ?? null)
const templateHasPlaceholder = computed(() => (selectedTemplate.value?.htmlBody ?? '').includes(unsubscribePlaceholder))
const showPrivateUrlWarning = computed(() => includeUnsubscribe.value && Boolean(publicBaseUrl.value) && isLikelyPrivateUrl(publicBaseUrl.value))
const canSubmit = computed(() => (
  !isLoading.value && !isSubmitting.value && Boolean(selectedTemplateId.value) && (preview.value?.sendable ?? 0) > 0
  && (!includeUnsubscribe.value || Boolean(publicBaseUrl.value.trim()))
))

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.userActivity.errors.request'))

const load = async () => {
  isLoading.value = true
  errorKey.value = ''
  try {
    const [templateList, recipientPreview] = await Promise.all([
      getEmailTemplates(),
      previewUserActivityRecipients(props.filter),
    ])
    templates.value = templateList
    preview.value = recipientPreview
    if (!selectedTemplateId.value && templateList.length > 0) selectedTemplateId.value = templateList[0].id
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
  } finally {
    isLoading.value = false
  }
}

const submit = async () => {
  if (!canSubmit.value) return
  isSubmitting.value = true
  errorKey.value = ''
  try {
    const batch = await createMassEmailBatch({
      templateId: selectedTemplateId.value,
      selectionMode: 'segment',
      filters: {
        status: props.filter.status ?? '',
        role: props.filter.role ?? '',
        search: props.filter.search ?? '',
        segment: props.filter.segment,
        inactiveDays: props.filter.segment === 'inactive' ? props.filter.inactiveDays : undefined,
      },
      requestId: requestId.value,
      publicBaseUrl: includeUnsubscribe.value ? publicBaseUrl.value.trim() : '',
    })
    emit('created', batch)
  } catch (error) {
    // 保留同一个 requestId：如果请求其实已在后端创建成功（只是响应丢失），重试会拿回同一个批次而不是重复创建。
    errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
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

watch(() => props.filter, () => { void load() }, { deep: true })

onMounted(() => {
  void load()
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="user-activity-email-title">
    <div class="flex max-h-[calc(100dvh-2rem)] w-full max-w-xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <div class="min-w-0">
          <h2 id="user-activity-email-title" class="text-lg font-semibold text-foreground">{{ t('admin.userActivity.email.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ filterSummary }}</p>
        </div>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.userActivity.actions.close')" :disabled="isSubmitting" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <div v-if="isLoading" class="flex items-center justify-center py-8 text-sm text-muted-foreground" role="status">
          <Loader2 class="mr-2 h-4 w-4 animate-spin" />
          {{ t('admin.userActivity.email.loading') }}
        </div>
        <template v-else>
          <dl v-if="preview" class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.email.matched') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ formatCount(preview.matched) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.email.withEmail') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ formatCount(preview.withEmail) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.email.suppressed') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ formatCount(preview.suppressed) }}</dd>
            </div>
            <div class="rounded-lg bg-primary/10 p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.email.sendable') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ formatCount(preview.sendable) }}</dd>
            </div>
          </dl>
          <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.email.previewHint') }}</p>

          <label class="block space-y-1 text-sm font-medium text-foreground">
            <span>{{ t('admin.userActivity.email.template') }}</span>
            <select v-model="selectedTemplateId" class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary">
              <option value="">{{ t('admin.userActivity.email.templatePlaceholder') }}</option>
              <option v-for="template in templates" :key="template.id" :value="template.id">{{ template.name }}</option>
            </select>
          </label>
          <p v-if="templates.length === 0" class="text-xs text-muted-foreground">{{ t('admin.userActivity.email.noTemplates') }}</p>
          <div v-if="selectedTemplate" class="rounded-lg border border-border/50 bg-surface p-3 text-sm">
            <span class="text-muted-foreground">{{ t('admin.userActivity.email.subject') }}</span>
            <span class="ml-1 font-medium text-foreground">{{ selectedTemplate.subject }}</span>
          </div>

          <div class="space-y-2 rounded-lg border border-border/50 p-3">
            <label class="flex items-start gap-2 text-sm text-foreground">
              <input v-model="includeUnsubscribe" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-border bg-surface text-primary focus:ring-2 focus:ring-primary">
              <span>
                <span class="font-medium">{{ t('admin.userActivity.email.unsubscribe') }}</span>
                <span class="mt-0.5 block text-xs text-muted-foreground">
                  {{ templateHasPlaceholder ? t('admin.userActivity.email.unsubscribePlaceholderFound', { placeholder: unsubscribePlaceholder }) : t('admin.userActivity.email.unsubscribeHint', { placeholder: unsubscribePlaceholder }) }}
                </span>
              </span>
            </label>
            <label v-if="includeUnsubscribe" class="block space-y-1 text-xs font-medium text-foreground">
              <span>{{ t('admin.userActivity.email.publicBaseUrl') }}</span>
              <input
                v-model="publicBaseUrl"
                type="url"
                class="h-9 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                placeholder="https://transithub.example.com/api"
              >
              <span class="block font-normal text-muted-foreground">{{ t('admin.userActivity.email.publicBaseUrlHint') }}</span>
            </label>
            <p v-if="showPrivateUrlWarning" class="flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300">
              <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{{ t('admin.userActivity.email.privateUrlWarning') }}</span>
            </p>
          </div>
        </template>

        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
      </div>

      <div class="flex shrink-0 flex-col-reverse gap-2 border-t border-border/50 p-4 sm:flex-row sm:justify-end">
        <Button variant="secondary" class="rounded-lg" :disabled="isSubmitting" @click="close">{{ t('admin.userActivity.actions.cancel') }}</Button>
        <Button class="rounded-lg" :disabled="!canSubmit" @click="submit">
          <Loader2 v-if="isSubmitting" class="h-4 w-4 animate-spin" />
          <Send v-else class="h-4 w-4" />
          {{ t('admin.userActivity.email.submit', { count: formatCount(preview?.sendable ?? 0) }) }}
        </Button>
      </div>
    </div>
  </div>
</template>
