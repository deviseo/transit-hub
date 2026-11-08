<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, Send, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { getEmailTemplates } from '../../api/settings'
import { previewLeadSend, sendToLeads } from '../../api/leads'
import { defaultUnsubscribeBaseUrl, isLikelyPrivateUrl } from '../../api/userActivity'
import type { EmailTemplate } from '../../types/settings'
import type { MassEmailBatch } from '../../types/massEmail'
import type { LeadSelection, LeadSendPreview } from '../../types/leads'

// 给线索发邮件：走群发邮件的发送队列。线索邮件一定带退订链接；标题默认加「【广告】」；
// 最近 24 小时的发送量不能超过上限（线索页面上可以改）。
const props = defineProps<{
  selection: LeadSelection
  summary: string
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'created', batch: MassEmailBatch): void
}>()

const { t, te } = useI18n()

const adLabel = '【广告】'
const unsubscribePlaceholder = '{{unsubscribe_url}}'
const templates = ref<EmailTemplate[]>([])
const selectedTemplateId = ref('')
const preview = ref<LeadSendPreview | null>(null)
const withAdLabel = ref(true)
const publicBaseUrl = ref(defaultUnsubscribeBaseUrl())
const isLoading = ref(true)
const isSubmitting = ref(false)
const errorKey = ref('')
const requestId = ref(crypto.randomUUID())

const selectedTemplate = computed(() => templates.value.find((template) => template.id === selectedTemplateId.value) ?? null)
const subjectPreview = computed(() => {
  const subject = selectedTemplate.value?.subject ?? ''
  if (!withAdLabel.value || /^(【广告】|【AD】|\[AD\]|AD[\s:：])/i.test(subject.trim())) return subject
  return adLabel + subject.trim()
})
const templateHasPlaceholder = computed(() => (selectedTemplate.value?.htmlBody ?? '').includes(unsubscribePlaceholder))
const overLimit = computed(() => Boolean(preview.value && preview.value.sendable > preview.value.remaining))
const showPrivateUrlWarning = computed(() => Boolean(publicBaseUrl.value.trim()) && isLikelyPrivateUrl(publicBaseUrl.value))
const canSubmit = computed(() => (
  !isLoading.value && !isSubmitting.value && Boolean(selectedTemplateId.value) && Boolean(publicBaseUrl.value.trim())
  && (preview.value?.sendable ?? 0) > 0 && !overLimit.value
))

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.leads.errors.request'))

const load = async () => {
  isLoading.value = true
  errorKey.value = ''
  try {
    const [templateList, sendPreview] = await Promise.all([getEmailTemplates(), previewLeadSend(props.selection)])
    templates.value = templateList
    preview.value = sendPreview
    if (!selectedTemplateId.value && templateList.length > 0) selectedTemplateId.value = templateList[0].id
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.leads.errors.request'
  } finally {
    isLoading.value = false
  }
}

const submit = async () => {
  if (!canSubmit.value) return
  isSubmitting.value = true
  errorKey.value = ''
  try {
    const batch = await sendToLeads({
      templateId: selectedTemplateId.value,
      selection: props.selection,
      publicBaseUrl: publicBaseUrl.value.trim(),
      adLabel: withAdLabel.value,
      requestId: requestId.value,
    })
    emit('created', batch)
  } catch (error) {
    // 保留同一个 requestId：如果其实已经建好了（只是响应丢了），重试会拿回同一个批次。
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

onMounted(() => {
  void load()
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="lead-send-title">
    <div class="flex max-h-[calc(100dvh-2rem)] w-full max-w-xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <div class="min-w-0">
          <h2 id="lead-send-title" class="text-lg font-semibold text-foreground">{{ t('admin.leads.send.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ summary }}</p>
        </div>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.leads.actions.close')" :disabled="isSubmitting" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <div v-if="isLoading" class="flex items-center justify-center py-8 text-sm text-muted-foreground" role="status">
          <Loader2 class="mr-2 h-4 w-4 animate-spin" />
          {{ t('admin.leads.send.loading') }}
        </div>
        <template v-else>
          <dl v-if="preview" class="grid grid-cols-3 gap-2">
            <div class="rounded-lg bg-primary/10 p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.leads.send.sendable') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ preview.sendable }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.leads.send.skipped') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ preview.skipped }}</dd>
            </div>
            <div class="rounded-lg p-3" :class="overLimit ? 'bg-warning/10' : 'bg-surface'">
              <dt class="text-xs text-muted-foreground">{{ t('admin.leads.send.remaining') }}</dt>
              <dd class="mt-1 text-base font-semibold text-foreground">{{ preview.remaining }} / {{ preview.dailyLimit }}</dd>
            </div>
          </dl>
          <p v-if="overLimit && preview" class="flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300">
            <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>{{ t('admin.leads.send.overLimit', { sendable: preview.sendable, remaining: preview.remaining }) }}</span>
          </p>
          <p v-else-if="preview && preview.sendable === 0" class="text-xs text-muted-foreground">{{ t('admin.leads.send.nothingToSend') }}</p>

          <label class="block space-y-1 text-sm font-medium text-foreground">
            <span>{{ t('admin.leads.send.template') }}</span>
            <select v-model="selectedTemplateId" class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary">
              <option value="">{{ t('admin.leads.send.templatePlaceholder') }}</option>
              <option v-for="template in templates" :key="template.id" :value="template.id">{{ template.name }}</option>
            </select>
          </label>
          <p v-if="templates.length === 0" class="text-xs text-muted-foreground">{{ t('admin.leads.send.noTemplates') }}</p>
          <div v-if="selectedTemplate" class="rounded-lg border border-border/50 bg-surface p-3 text-sm">
            <span class="text-muted-foreground">{{ t('admin.leads.send.subject') }}</span>
            <span class="ml-1 font-medium text-foreground">{{ subjectPreview }}</span>
          </div>

          <label class="flex items-start gap-2 text-sm text-foreground">
            <input v-model="withAdLabel" type="checkbox" class="mt-0.5 h-4 w-4 rounded border-border bg-surface text-primary focus:ring-2 focus:ring-primary">
            <span>
              <span class="font-medium">{{ t('admin.leads.send.adLabel') }}</span>
              <span class="mt-0.5 block text-xs text-muted-foreground">{{ t('admin.leads.send.adLabelHint') }}</span>
            </span>
          </label>

          <div class="space-y-2 rounded-lg border border-border/50 p-3">
            <label class="block space-y-1 text-sm font-medium text-foreground">
              <span>{{ t('admin.leads.send.publicBaseUrl') }}</span>
              <input
                v-model="publicBaseUrl"
                type="url"
                class="h-9 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                placeholder="https://transithub.example.com/api"
              >
              <span class="block text-xs font-normal text-muted-foreground">
                {{ templateHasPlaceholder ? t('admin.leads.send.unsubscribePlaceholderFound', { placeholder: unsubscribePlaceholder }) : t('admin.leads.send.unsubscribeHint', { placeholder: unsubscribePlaceholder }) }}
              </span>
            </label>
            <p v-if="showPrivateUrlWarning" class="flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300">
              <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{{ t('admin.leads.send.privateUrlWarning') }}</span>
            </p>
          </div>
        </template>

        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
      </div>

      <div class="flex shrink-0 flex-col-reverse gap-2 border-t border-border/50 p-4 sm:flex-row sm:justify-end">
        <Button variant="secondary" class="rounded-lg" :disabled="isSubmitting" @click="close">{{ t('admin.leads.actions.cancel') }}</Button>
        <Button class="rounded-lg" :disabled="!canSubmit" @click="submit">
          <Loader2 v-if="isSubmitting" class="h-4 w-4 animate-spin" />
          <Send v-else class="h-4 w-4" />
          {{ t('admin.leads.send.submit', { count: preview?.sendable ?? 0 }) }}
        </Button>
      </div>
    </div>
  </div>
</template>
