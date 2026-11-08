<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { addLead, updateLead } from '../../api/leads'
import type { Lead } from '../../types/leads'

// 添加一个邮箱（带来源和备注），或者修改已有线索的来源、备注（邮箱不能改）。
const props = defineProps<{
  lead?: Lead | null
  sources: string[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saved', lead: Lead): void
}>()

const { t, te } = useI18n()

const isEdit = computed(() => Boolean(props.lead))
const email = ref(props.lead?.email ?? '')
const source = ref(props.lead?.source ?? '')
const note = ref(props.lead?.note ?? '')
const isSubmitting = ref(false)
const errorKey = ref('')

const canSubmit = computed(() => !isSubmitting.value && (isEdit.value || email.value.trim() !== ''))
const errorText = (key: string): string => (te(key) ? t(key) : t('admin.leads.errors.request'))

const submit = async () => {
  if (!canSubmit.value) return
  isSubmitting.value = true
  errorKey.value = ''
  try {
    const saved = props.lead
      ? await updateLead(props.lead.id, { source: source.value, note: note.value })
      : await addLead({ email: email.value.trim(), source: source.value, note: note.value })
    emit('saved', saved)
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
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="lead-form-title">
    <form class="flex max-h-[calc(100dvh-2rem)] w-full max-w-md flex-col rounded-lg border border-border/60 bg-card shadow-xl" @submit.prevent="submit">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <h2 id="lead-form-title" class="text-lg font-semibold text-foreground">{{ t(isEdit ? 'admin.leads.form.editTitle' : 'admin.leads.form.addTitle') }}</h2>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.leads.actions.close')" :disabled="isSubmitting" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <label class="block space-y-1 text-sm font-medium text-foreground">
          <span>{{ t('admin.leads.fields.email') }}</span>
          <input
            v-model="email"
            type="email"
            autocomplete="off"
            :disabled="isEdit"
            maxlength="254"
            class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary disabled:opacity-70"
            placeholder="name@example.com"
          >
        </label>
        <label class="block space-y-1 text-sm font-medium text-foreground">
          <span>{{ t('admin.leads.fields.source') }}</span>
          <input
            v-model="source"
            type="text"
            list="lead-form-sources"
            maxlength="40"
            class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
            :placeholder="t('admin.leads.form.sourcePlaceholder')"
          >
          <datalist id="lead-form-sources">
            <option v-for="item in sources" :key="item" :value="item" />
          </datalist>
        </label>
        <label class="block space-y-1 text-sm font-medium text-foreground">
          <span>{{ t('admin.leads.fields.note') }}</span>
          <textarea
            v-model="note"
            rows="3"
            maxlength="200"
            class="w-full resize-none rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
            :placeholder="t('admin.leads.form.notePlaceholder')"
          />
        </label>

        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
      </div>

      <div class="flex shrink-0 flex-col-reverse gap-2 border-t border-border/50 p-4 sm:flex-row sm:justify-end">
        <Button type="button" variant="secondary" class="rounded-lg" :disabled="isSubmitting" @click="close">{{ t('admin.leads.actions.cancel') }}</Button>
        <Button type="submit" class="rounded-lg" :disabled="!canSubmit">
          <Loader2 v-if="isSubmitting" class="h-4 w-4 animate-spin" />
          {{ t(isEdit ? 'admin.leads.actions.save' : 'admin.leads.actions.add') }}
        </Button>
      </div>
    </form>
  </div>
</template>
