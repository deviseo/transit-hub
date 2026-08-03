<script setup lang="ts">
import { computed } from 'vue'
import { useDark } from '@vueuse/core'
import { marked } from 'marked'
import { Code2, Eye } from 'lucide-vue-next'
import type { NotificationTemplateFormat } from '../../types/settings'

type TemplateVariable = {
  token: string
  label: string
}

const content = defineModel<string>({ required: true })
const format = defineModel<NotificationTemplateFormat>('format', { required: true })
const props = defineProps<{
  variables: TemplateVariable[]
  previewValues: Record<string, string>
  placeholder: string
}>()

import { t } from '@/locales'
const isDark = useDark({
  selector: 'html',
  attribute: 'class',
  valueDark: 'dark',
  valueLight: '',
})
const formatOptions: Array<{ value: NotificationTemplateFormat, labelKey: string }> = [
  { value: 'text', labelKey: 'admin.settings.templateEditor.formats.text' },
  { value: 'markdown', labelKey: 'admin.settings.templateEditor.formats.markdown' },
  { value: 'html', labelKey: 'admin.settings.templateEditor.formats.html' },
]

const previewSource = computed(() => {
  let result = content.value
  for (const [token, value] of Object.entries(props.previewValues)) {
    result = result.replaceAll(token, value)
  }
  return result
})

const escapeHTML = (value: string) => value
  .replaceAll('&', '&amp;')
  .replaceAll('<', '&lt;')
  .replaceAll('>', '&gt;')
  .replaceAll('"', '&quot;')
  .replaceAll("'", '&#039;')

const previewBody = computed(() => {
  if (format.value === 'html') return previewSource.value
  if (format.value === 'markdown') {
    try {
      return marked.parse(previewSource.value, { async: false, breaks: true, gfm: true })
    } catch {
      return `<p class="plain">${escapeHTML(previewSource.value).replaceAll('\n', '<br>')}</p>`
    }
  }
  return `<p class="plain">${escapeHTML(previewSource.value).replaceAll('\n', '<br>')}</p>`
})

const previewDocument = computed(() => {
  const policy = "default-src 'none'; style-src 'unsafe-inline'; img-src data: https:; font-src data:; form-action 'none'; frame-src 'none'; connect-src 'none'; media-src 'none'; object-src 'none'; base-uri 'none'"
  const palette = isDark.value
    ? { background: '#18181b', foreground: '#e4e4e7', heading: '#fafafa', link: '#60a5fa', muted: '#a1a1aa', subtle: '#27272a', border: '#52525b' }
    : { background: '#ffffff', foreground: '#18181b', heading: '#09090b', link: '#2563eb', muted: '#52525b', subtle: '#f4f4f5', border: '#d4d4d8' }
  return `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <meta http-equiv="Content-Security-Policy" content="${policy}">
    <meta name="referrer" content="no-referrer">
    <style>
      :root { color-scheme: ${isDark.value ? 'dark' : 'light'}; }
      * { box-sizing: border-box; }
      body { margin: 0; padding: 18px; background: ${palette.background}; color: ${palette.foreground}; font: 14px/1.65 ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; overflow-wrap: anywhere; }
      h1, h2, h3, h4, h5, h6 { margin: 0 0 12px; color: ${palette.heading}; line-height: 1.3; }
      h1 { font-size: 22px; } h2 { font-size: 19px; } h3 { font-size: 16px; }
      p, ul, ol, blockquote, pre { margin: 0 0 12px; }
      p:last-child, ul:last-child, ol:last-child, blockquote:last-child, pre:last-child { margin-bottom: 0; }
      ul, ol { padding-left: 24px; }
      a { color: ${palette.link}; }
      code { border-radius: 4px; background: ${palette.subtle}; padding: 2px 5px; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 12px; }
      pre { overflow: auto; border-radius: 6px; background: ${palette.subtle}; padding: 12px; }
      pre code { padding: 0; background: transparent; }
      blockquote { border-left: 3px solid ${palette.border}; padding-left: 12px; color: ${palette.muted}; }
      img { max-width: 100%; height: auto; }
      .plain { white-space: normal; }
    </style>
  </head>
  <body>${previewBody.value}</body>
</html>`
})
</script>

<template>
  <div class="space-y-3">
    <div class="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.customTemplate') }}</label>
      <div class="inline-flex w-fit rounded-md border border-border/60 bg-surface/30 p-1" role="group" :aria-label="t('admin.settings.templateEditor.formatLabel')">
        <button
          v-for="option in formatOptions"
          :key="option.value"
          type="button"
          class="rounded px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          :class="format === option.value ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
          :aria-pressed="format === option.value"
          @click="format = option.value"
        >
          {{ t(option.labelKey) }}
        </button>
      </div>
    </div>

    <p class="text-xs text-muted-foreground">{{ t('admin.settings.templateEditor.formatHelp') }}</p>

    <div class="flex flex-wrap gap-x-3 gap-y-1.5">
      <span v-for="variable in variables" :key="variable.token" class="inline-flex items-center gap-1.5">
        <code class="rounded bg-primary/10 px-1.5 py-0.5 font-mono text-xs text-primary">{{ variable.token }}</code>
        <span class="text-xs text-muted-foreground">{{ variable.label }}</span>
      </span>
    </div>

    <div class="grid gap-4 xl:grid-cols-2">
      <div class="min-w-0 space-y-2">
        <div class="flex h-6 items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <Code2 class="h-3.5 w-3.5" />
          {{ t('admin.settings.templateEditor.editor') }}
        </div>
        <textarea
          v-model="content"
          :placeholder="placeholder"
          class="flex min-h-[220px] w-full resize-y rounded-lg border border-input bg-background px-3 py-3 font-mono text-sm leading-6 text-foreground ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        ></textarea>
      </div>

      <div class="min-w-0 space-y-2">
        <div class="flex h-6 items-center gap-1.5 text-xs font-medium text-muted-foreground">
          <Eye class="h-3.5 w-3.5" />
          {{ t('admin.settings.templateEditor.preview') }}
        </div>
        <iframe
          :srcdoc="previewDocument"
          sandbox=""
          :title="t('admin.settings.templateEditor.previewTitle')"
          class="h-[220px] w-full rounded-lg border border-input bg-background"
        ></iframe>
      </div>
    </div>
  </div>
</template>
