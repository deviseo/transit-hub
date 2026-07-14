<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CheckCircle2, Code2, Eye, FilePlus2, Loader2, MailCheck, Save, Send, Sparkles, Trash2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  createEmailTemplate,
  deleteEmailTemplate,
  getEmailTemplates,
  testEmailTemplate,
  updateEmailTemplate,
} from '../../api/settings'
import type { EmailTemplate, SaveEmailTemplatePayload } from '../../types/settings'

const { t } = useI18n()

const templates = ref<EmailTemplate[]>([])
const selectedId = ref('')
const name = ref('')
const subject = ref('')
const htmlBody = ref('')
const baseline = ref<SaveEmailTemplatePayload | null>(null)
const testRecipient = ref('')
const previewMode = ref<'preview' | 'code'>('preview')

const isLoading = ref(false)
const isSaving = ref(false)
const isDeleting = ref(false)
const isTesting = ref(false)
const errorMessage = ref('')
const successMessage = ref('')

const selectedTemplate = computed(() => templates.value.find((template) => template.id === selectedId.value) ?? null)
const customCount = computed(() => templates.value.filter((template) => !template.isBuiltIn).length)
const htmlByteLength = computed(() => new TextEncoder().encode(htmlBody.value).length)
const previewDocument = computed(() => {
  const policy = "default-src 'none'; style-src 'unsafe-inline'; img-src data: cid:; font-src data:; form-action 'none'; frame-src 'none'; connect-src 'none'"
  const meta = `<meta http-equiv="Content-Security-Policy" content="${policy}"><meta name="referrer" content="no-referrer">`
  if (/<head(?:\s[^>]*)?>/i.test(htmlBody.value)) {
    return htmlBody.value.replace(/<head(?:\s[^>]*)?>/i, (head) => `${head}${meta}`)
  }
  return `<!doctype html><html><head>${meta}</head><body>${htmlBody.value}</body></html>`
})
const currentPayload = computed<SaveEmailTemplatePayload>(() => ({
  name: name.value.trim(),
  subject: subject.value.trim(),
  htmlBody: htmlBody.value,
}))
const isValid = computed(() => (
  currentPayload.value.name.length > 0 &&
  currentPayload.value.name.length <= 120 &&
  currentPayload.value.subject.length > 0 &&
  currentPayload.value.subject.length <= 255 &&
  !/[\r\n]/.test(currentPayload.value.subject) &&
  currentPayload.value.htmlBody.trim().length > 0 &&
  htmlByteLength.value <= 102400
))
const isDirty = computed(() => {
  if (!baseline.value) return true
  const current = currentPayload.value
  return current.name !== baseline.value.name || current.subject !== baseline.value.subject || current.htmlBody !== baseline.value.htmlBody
})

const clearFeedback = () => {
  errorMessage.value = ''
  successMessage.value = ''
}

const applyTemplate = (template: EmailTemplate) => {
  selectedId.value = template.id
  name.value = template.name
  subject.value = template.subject
  htmlBody.value = template.htmlBody
  baseline.value = { name: template.name, subject: template.subject, htmlBody: template.htmlBody }
  clearFeedback()
}

const selectTemplate = (template: EmailTemplate) => {
  if (isDirty.value && selectedTemplate.value && !window.confirm(t('admin.settings.emailTemplates.discardConfirm'))) return
  applyTemplate(template)
}

const loadTemplates = async (preferredId = '') => {
  isLoading.value = true
  clearFeedback()
  try {
    templates.value = await getEmailTemplates()
    const target = templates.value.find((template) => template.id === preferredId)
      ?? templates.value.find((template) => template.id === selectedId.value)
      ?? templates.value[0]
    if (target) applyTemplate(target)
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isLoading.value = false
  }
}

const addTemplate = async () => {
  if (customCount.value >= 50) {
    errorMessage.value = 'admin.settings.emailTemplates.errors.limitReached'
    return
  }
  if (isDirty.value && selectedTemplate.value && !window.confirm(t('admin.settings.emailTemplates.discardConfirm'))) return
  isSaving.value = true
  clearFeedback()
  try {
    const created = await createEmailTemplate({
      name: t('admin.settings.emailTemplates.newTemplateName'),
      subject: t('admin.settings.emailTemplates.newTemplateSubject'),
      htmlBody: t('admin.settings.emailTemplates.newTemplateHtml'),
    })
    templates.value = [...templates.value, created]
    applyTemplate(created)
    successMessage.value = 'admin.settings.emailTemplates.createSuccess'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isSaving.value = false
  }
}

const saveTemplate = async () => {
  if (!selectedTemplate.value || !isValid.value || !isDirty.value) return
  isSaving.value = true
  clearFeedback()
  try {
    const saved = await updateEmailTemplate(selectedTemplate.value.id, currentPayload.value)
    templates.value = templates.value.map((template) => template.id === saved.id ? saved : template)
    applyTemplate(saved)
    successMessage.value = 'admin.settings.emailTemplates.saveSuccess'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isSaving.value = false
  }
}

const removeTemplate = async () => {
  const template = selectedTemplate.value
  if (!template || template.isBuiltIn || !window.confirm(t('admin.settings.emailTemplates.deleteConfirm', { name: template.name }))) return
  isDeleting.value = true
  clearFeedback()
  try {
    await deleteEmailTemplate(template.id)
    templates.value = templates.value.filter((item) => item.id !== template.id)
    const next = templates.value[0]
    if (next) applyTemplate(next)
    successMessage.value = 'admin.settings.emailTemplates.deleteSuccess'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isDeleting.value = false
  }
}

const sendTest = async () => {
  const template = selectedTemplate.value
  if (!template || isDirty.value || !testRecipient.value.trim()) return
  isTesting.value = true
  clearFeedback()
  try {
    await testEmailTemplate(template.id, { recipientEmail: testRecipient.value.trim() })
    successMessage.value = 'admin.settings.emailTemplates.testEmailSuccess'
  } catch (error) {
    errorMessage.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isTesting.value = false
  }
}

onMounted(() => { void loadTemplates() })
</script>

<template>
  <section class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
    <header class="flex flex-col gap-4 border-b border-border/50 bg-surface/30 p-6 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex items-center gap-3">
        <div class="rounded-xl bg-amber-500/10 p-2 text-amber-600 dark:text-amber-400"><Sparkles class="h-5 w-5" /></div>
        <div>
          <h3 class="text-lg font-semibold text-foreground">{{ t('admin.settings.emailTemplates.title') }}</h3>
          <p class="text-sm text-muted-foreground">{{ t('admin.settings.emailTemplates.description') }}</p>
        </div>
      </div>
      <Button size="sm" :disabled="isLoading || isSaving || customCount >= 50" @click="addTemplate">
        <FilePlus2 class="h-4 w-4" />{{ t('admin.settings.emailTemplates.add') }}
      </Button>
    </header>

    <div v-if="isLoading" class="flex items-center justify-center gap-2 p-12 text-sm text-muted-foreground">
      <Loader2 class="h-4 w-4 animate-spin" />{{ t('admin.settings.emailTemplates.loading') }}
    </div>

    <div v-else class="grid min-h-[620px] lg:grid-cols-[260px_minmax(0,1fr)]">
      <aside class="border-b border-border/50 bg-surface/20 p-4 lg:border-b-0 lg:border-r">
        <div class="mb-3 flex items-center justify-between px-1 text-xs font-medium text-muted-foreground">
          <span>{{ t('admin.settings.emailTemplates.library') }}</span>
          <span>{{ customCount }}/50</span>
        </div>
        <div class="space-y-2" role="listbox" :aria-label="t('admin.settings.emailTemplates.library')">
          <button
            v-for="template in templates"
            :key="template.id"
            type="button"
            role="option"
            :aria-selected="template.id === selectedId"
            class="w-full rounded-xl border p-3 text-left transition-colors"
            :class="template.id === selectedId ? 'border-primary/50 bg-primary/10' : 'border-transparent hover:border-border hover:bg-surface-elevated'"
            @click="selectTemplate(template)"
          >
            <span class="flex items-start justify-between gap-2">
              <span class="min-w-0">
                <span class="block truncate text-sm font-semibold text-foreground">{{ template.name }}</span>
                <span class="mt-1 block truncate text-xs text-muted-foreground">{{ template.subject }}</span>
              </span>
              <span v-if="template.isBuiltIn" class="shrink-0 rounded-full bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold text-amber-700 dark:text-amber-300">
                {{ t('admin.settings.emailTemplates.builtIn') }}
              </span>
            </span>
          </button>
        </div>
      </aside>

      <div v-if="selectedTemplate" class="min-w-0 p-5 sm:p-6">
        <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <p class="text-xs font-medium uppercase tracking-wider text-muted-foreground">{{ t('admin.settings.emailTemplates.editor') }}</p>
            <p v-if="isDirty" class="mt-1 text-xs text-amber-600 dark:text-amber-400">{{ t('admin.settings.emailTemplates.unsaved') }}</p>
          </div>
          <div class="flex gap-2">
            <Button v-if="!selectedTemplate.isBuiltIn" variant="destructive" size="sm" :disabled="isDeleting" @click="removeTemplate">
              <Loader2 v-if="isDeleting" class="h-4 w-4 animate-spin" /><Trash2 v-else class="h-4 w-4" />{{ t('admin.settings.emailTemplates.delete') }}
            </Button>
            <Button size="sm" :disabled="isSaving || !isDirty || !isValid" @click="saveTemplate">
              <Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" /><Save v-else class="h-4 w-4" />{{ t('admin.settings.emailTemplates.save') }}
            </Button>
          </div>
        </div>

        <div class="grid gap-4 xl:grid-cols-2">
          <div class="space-y-4">
            <div class="grid gap-2">
              <label for="email-template-name" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.emailTemplates.name') }}</label>
              <Input id="email-template-name" v-model="name" maxlength="120" />
            </div>
            <div class="grid gap-2">
              <label for="email-template-subject" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.emailTemplates.subject') }}</label>
              <Input id="email-template-subject" v-model="subject" maxlength="255" />
            </div>
            <div class="grid gap-2">
              <div class="flex items-center justify-between">
                <label for="email-template-html" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.emailTemplates.htmlBody') }}</label>
                <span class="text-[11px] text-muted-foreground">{{ htmlByteLength }}/102400</span>
              </div>
              <textarea id="email-template-html" v-model="htmlBody" spellcheck="false" class="min-h-[330px] w-full resize-y rounded-xl border border-border bg-background p-4 font-mono text-xs leading-5 text-foreground outline-none transition focus:border-primary focus:ring-2 focus:ring-primary/20" />
            </div>
          </div>

          <div class="overflow-hidden rounded-2xl border border-border bg-background">
            <div class="flex items-center justify-between border-b border-border bg-surface/40 px-3 py-2">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.emailTemplates.preview') }}</span>
              <div class="flex rounded-lg bg-surface-elevated p-1">
                <button type="button" class="rounded-md p-1.5" :class="previewMode === 'preview' ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground'" :aria-label="t('admin.settings.emailTemplates.preview')" @click="previewMode = 'preview'"><Eye class="h-4 w-4" /></button>
                <button type="button" class="rounded-md p-1.5" :class="previewMode === 'code' ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground'" :aria-label="t('admin.settings.emailTemplates.code')" @click="previewMode = 'code'"><Code2 class="h-4 w-4" /></button>
              </div>
            </div>
            <iframe v-if="previewMode === 'preview'" :srcdoc="previewDocument" sandbox="" referrerpolicy="no-referrer" :title="t('admin.settings.emailTemplates.previewTitle')" class="h-[470px] w-full bg-white" />
            <pre v-else class="h-[470px] overflow-auto whitespace-pre-wrap break-words p-4 text-xs leading-5 text-foreground">{{ htmlBody }}</pre>
          </div>
        </div>

        <div class="mt-5 rounded-2xl border border-border/70 bg-surface/30 p-4">
          <div class="flex flex-col gap-3 sm:flex-row sm:items-end">
            <div class="grid flex-1 gap-2">
              <label for="email-template-recipient" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.emailTemplates.testRecipient') }}</label>
              <Input id="email-template-recipient" v-model="testRecipient" type="email" :placeholder="t('admin.settings.emailTemplates.testRecipientPlaceholder')" />
            </div>
            <Button variant="secondary" :disabled="isTesting || isDirty || !testRecipient.trim()" @click="sendTest">
              <Loader2 v-if="isTesting" class="h-4 w-4 animate-spin" /><MailCheck v-else-if="successMessage === 'admin.settings.emailTemplates.testEmailSuccess'" class="h-4 w-4 text-green-500" /><Send v-else class="h-4 w-4" />{{ t('admin.settings.emailTemplates.test') }}
            </Button>
          </div>
          <p v-if="isDirty" class="mt-2 text-xs text-amber-600 dark:text-amber-400">{{ t('admin.settings.emailTemplates.dirtyBeforeTest') }}</p>
        </div>

        <p v-if="!isValid" class="mt-4 text-sm text-destructive">{{ t('admin.settings.emailTemplates.errors.validation') }}</p>
        <p v-if="errorMessage" class="mt-4 rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{{ t(errorMessage) }}</p>
        <p v-if="successMessage" class="mt-4 flex items-center gap-2 text-sm text-green-600 dark:text-green-400"><CheckCircle2 class="h-4 w-4" />{{ t(successMessage) }}</p>
      </div>
    </div>
  </section>
</template>
