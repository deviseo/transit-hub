<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertTriangle, CheckCircle2, Copy, Download, ExternalLink, Loader2, RefreshCw, Sparkles, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  applySystemUpdate,
  checkSystemUpdate,
  getSystemUpdateProgress,
  type SystemUpdateCheck,
  type SystemUpdateProgress,
} from '../../api/system'

// 检查更新：对比当前版本和 GitHub 上最新的正式版。系统管理员可以直接在线更新：服务端在后台下载、校验、安装，
// 然后重启到新版本，页面等新版本起来后自动刷新。不能在线更新时说明原因，并给出 Docker 的更新方法。
const emit = defineEmits<{ close: []; checked: [result: SystemUpdateCheck] }>()
const { t, locale } = useI18n()

const result = ref<SystemUpdateCheck | null>(null)
const checking = ref(false)
const error = ref('')
const copied = ref(false)

// 在线更新的阶段：confirm 等确认；updating 按服务端进度显示（下载、安装、重启）；reloading 新版本已经起来，
// 马上刷新页面；stalled 重启后很久连不上；reverted 新版本没能启动，服务退回了原来的版本。
type Phase = 'idle' | 'confirm' | 'updating' | 'reloading' | 'stalled' | 'reverted'
const phase = ref<Phase>('idle')
const target = ref('')
const progress = ref<SystemUpdateProgress | null>(null)
const offline = ref(false)
const busy = computed(() => phase.value === 'updating' || phase.value === 'reloading')

const pollInterval = 2000
const restartTimeout = 3 * 60 * 1000
let pollTimer: number | undefined
let offlineSince = 0
let disposed = false

const check = async (force: boolean) => {
  checking.value = true
  error.value = ''
  try {
    result.value = await checkSystemUpdate(force)
    emit('checked', result.value)
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.update.errors.checkFailed'
  } finally {
    checking.value = false
  }
}

const publishedAt = computed(() => {
  if (!result.value?.publishedAt) return ''
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium' }).format(new Date(result.value.publishedAt))
})

const reasonKey = computed(() => (result.value?.reason ? `admin.update.reasons.${result.value.reason}` : ''))

const megabytes = (bytes: number) => (bytes / 1024 / 1024).toFixed(1)

const percent = computed(() => {
  const current = progress.value
  if (offline.value || current?.status !== 'downloading' || current.total <= 0) return null
  return Math.min(100, Math.round((current.downloaded / current.total) * 100))
})

const stepText = computed(() => {
  const version = target.value || progress.value?.target || ''
  if (phase.value === 'reloading') return t('admin.update.progress.reloading', { version })
  if (offline.value || progress.value?.status === 'restarting') return t('admin.update.progress.restarting', { version })
  if (progress.value?.status === 'installing') return t('admin.update.progress.installing')
  return t('admin.update.progress.downloading', { version })
})

const schedulePoll = (delay = pollInterval) => {
  if (disposed) return
  pollTimer = window.setTimeout(() => { void poll() }, delay)
}

// 每隔两秒查一次进度：连不上是在重启；正在运行的就是目标版本时刷新页面；
// 进度回到 idle 却还是旧版本，说明服务重启过但新版本没能启动。
const poll = async () => {
  try {
    const current = await getSystemUpdateProgress()
    offline.value = false
    offlineSince = 0
    progress.value = current
    if (!target.value && current.target) target.value = current.target
    if (target.value && current.version === target.value) {
      phase.value = 'reloading'
      window.setTimeout(reloadPage, 800)
      return
    }
    if (current.status === 'failed') {
      phase.value = 'idle'
      error.value = current.error || 'admin.update.errors.failed'
      return
    }
    if (current.status === 'idle') {
      phase.value = 'reverted'
      return
    }
  } catch {
    offline.value = true
    if (!offlineSince) offlineSince = Date.now()
    if (Date.now() - offlineSince > restartTimeout) {
      phase.value = 'stalled'
      return
    }
  }
  schedulePoll()
}

const reloadPage = () => window.location.reload()

const follow = (version: string) => {
  phase.value = 'updating'
  target.value = version
  offline.value = false
  offlineSince = 0
  schedulePoll(500)
}

const startUpdate = async () => {
  error.value = ''
  progress.value = null
  phase.value = 'updating'
  try {
    const started = await applySystemUpdate()
    follow(started.version)
  } catch (caught) {
    const key = caught instanceof Error ? caught.message : 'admin.update.errors.failed'
    if (key === 'admin.update.errors.inProgress') {
      // 别人（或者刷新前的这个页面）已经开始更新了：跟着显示进度。
      follow('')
      return
    }
    phase.value = 'idle'
    error.value = key
  }
}

// 打开时如果已经在更新（例如更新途中刷新了页面），接着显示进度。
const resumeIfUpdating = async () => {
  try {
    const current = await getSystemUpdateProgress()
    if (['downloading', 'installing', 'restarting'].includes(current.status)) {
      progress.value = current
      follow(current.target ?? '')
    }
  } catch {
    // 查不到进度不影响检查更新。
  }
}

const updateCommand = 'docker compose -f deploy/docker-compose.prod.yml pull && docker compose -f deploy/docker-compose.prod.yml up -d'

const copyCommand = async () => {
  await navigator.clipboard.writeText(updateCommand)
  copied.value = true
  window.setTimeout(() => { copied.value = false }, 1500)
}

// 更新途中不能关闭，免得错过重启后的自动刷新。
const close = () => {
  if (!busy.value) emit('close')
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') close()
}

onMounted(() => {
  void check(false)
  void resumeIfUpdating()
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  disposed = true
  window.clearTimeout(pollTimer)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="system-update-title" @click.self="close">
    <div class="flex max-h-[calc(100dvh-2rem)] w-full max-w-xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <h2 id="system-update-title" class="text-lg font-semibold text-foreground">{{ t('admin.update.title') }}</h2>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-40" :disabled="busy" :aria-label="t('admin.update.close')" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4 text-sm">
        <p v-if="checking && !result" class="flex items-center gap-2 text-muted-foreground">
          <Loader2 class="h-4 w-4 animate-spin" />{{ t('admin.update.checking') }}
        </p>
        <p v-if="error" class="rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-destructive" role="alert">{{ t(error) }}</p>

        <template v-if="result">
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2">
            <dt class="text-muted-foreground">{{ t('admin.update.current') }}</dt>
            <dd class="font-medium text-foreground">{{ result.current }}</dd>
            <dt class="text-muted-foreground">{{ t('admin.update.latest') }}</dt>
            <dd class="font-medium text-foreground">
              {{ result.latest }}
              <span v-if="publishedAt" class="ml-1 text-xs font-normal text-muted-foreground">{{ t('admin.update.publishedAt', { date: publishedAt }) }}</span>
            </dd>
          </dl>

          <p v-if="!result.hasUpdate" class="flex items-center gap-2 rounded-lg bg-green-500/10 px-3 py-2 text-green-700 dark:text-green-400">
            <CheckCircle2 class="h-4 w-4 shrink-0" />{{ t('admin.update.upToDate') }}
          </p>

          <template v-else>
            <p class="flex items-center gap-2 rounded-lg bg-primary/10 px-3 py-2 font-medium text-primary">
              <Sparkles class="h-4 w-4 shrink-0" />{{ t('admin.update.available', { version: result.latest }) }}
            </p>
            <div v-if="result.notes" class="grid grid-cols-1 gap-1.5">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.update.notes') }}</span>
              <pre class="max-h-60 overflow-y-auto whitespace-pre-wrap break-words rounded-lg border border-border/60 bg-surface/40 p-3 font-sans text-sm text-foreground">{{ result.notes }}</pre>
            </div>

            <section v-if="result.canUpdate || phase !== 'idle'" class="grid grid-cols-1 gap-3 rounded-lg border border-border/60 p-3">
              <div class="flex items-start gap-2">
                <Download class="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                <div class="grid gap-1">
                  <p class="font-medium text-foreground">{{ t('admin.update.online.title') }}</p>
                  <p class="text-muted-foreground">{{ t('admin.update.online.description', { version: result.latest }) }}</p>
                </div>
              </div>

              <div v-if="phase === 'idle'" class="flex justify-end">
                <Button size="sm" @click="phase = 'confirm'">
                  <Download class="h-4 w-4" />{{ t('admin.update.online.start', { version: result.latest }) }}
                </Button>
              </div>

              <div v-else-if="phase === 'confirm'" class="grid gap-3 rounded-lg bg-amber-500/10 p-3">
                <p class="flex items-start gap-2 text-amber-800 dark:text-amber-300">
                  <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" />{{ t('admin.update.online.confirm', { version: result.latest }) }}
                </p>
                <div class="flex justify-end gap-2">
                  <Button variant="secondary" size="sm" @click="phase = 'idle'">{{ t('admin.update.online.cancel') }}</Button>
                  <Button size="sm" @click="startUpdate">{{ t('admin.update.online.confirmButton') }}</Button>
                </div>
              </div>

              <div v-else-if="busy" class="grid gap-2" role="status" aria-live="polite">
                <p class="flex items-center gap-2 font-medium text-foreground">
                  <CheckCircle2 v-if="phase === 'reloading'" class="h-4 w-4 shrink-0 text-green-500" />
                  <Loader2 v-else class="h-4 w-4 shrink-0 animate-spin" />
                  {{ stepText }}
                </p>
                <template v-if="percent !== null && progress">
                  <div class="h-1.5 overflow-hidden rounded-full bg-surface-line" role="progressbar" :aria-valuenow="percent" aria-valuemin="0" aria-valuemax="100" :aria-label="t('admin.update.progress.downloadLabel')">
                    <div class="h-full rounded-full bg-primary transition-[width] duration-500" :style="{ width: `${percent}%` }" />
                  </div>
                  <p class="text-xs tabular-nums text-muted-foreground">
                    {{ t('admin.update.progress.downloaded', { done: megabytes(progress.downloaded), total: megabytes(progress.total), percent }) }}
                  </p>
                </template>
                <p class="text-xs text-muted-foreground">{{ t('admin.update.progress.keepOpen') }}</p>
              </div>

              <div v-else-if="phase === 'stalled'" class="grid gap-3 rounded-lg bg-amber-500/10 p-3">
                <p class="flex items-start gap-2 text-amber-800 dark:text-amber-300">
                  <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" />{{ t('admin.update.progress.stalled') }}
                </p>
                <div class="flex justify-end">
                  <Button size="sm" @click="reloadPage">
                    <RefreshCw class="h-4 w-4" />{{ t('admin.update.progress.reload') }}
                  </Button>
                </div>
              </div>

              <p v-else-if="phase === 'reverted'" class="flex items-start gap-2 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-destructive" role="alert">
                <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" />{{ t('admin.update.progress.reverted', { version: progress?.version ?? result.current }) }}
              </p>
            </section>

            <p v-else-if="reasonKey" class="flex items-start gap-2 rounded-lg bg-surface/60 px-3 py-2 text-muted-foreground">
              <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" />{{ t(reasonKey) }}
            </p>

            <details :open="!result.canUpdate || phase === 'reverted'">
              <summary class="cursor-pointer select-none text-xs font-medium text-muted-foreground hover:text-foreground">{{ t('admin.update.howTo') }}</summary>
              <div class="mt-2 grid grid-cols-1 gap-2">
                <ol class="list-decimal space-y-1.5 pl-5 text-foreground">
                  <li>{{ t('admin.update.step1', { version: result.latest }) }}</li>
                  <li>{{ t('admin.update.step2') }}</li>
                </ol>
                <div class="flex items-center gap-2 rounded-lg bg-surface/60 py-1 pl-3 pr-1">
                  <code class="min-w-0 flex-1 truncate font-mono text-xs text-foreground">{{ updateCommand }}</code>
                  <Button variant="ghost" size="sm" :aria-label="t('admin.update.copy')" @click="copyCommand">
                    <CheckCircle2 v-if="copied" class="h-4 w-4 text-green-500" />
                    <Copy v-else class="h-4 w-4" />
                  </Button>
                </div>
              </div>
            </details>
          </template>

          <a :href="result.releaseUrl" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 text-primary hover:underline">
            <ExternalLink class="h-3.5 w-3.5" />{{ t('admin.update.viewRelease') }}
          </a>
        </template>
      </div>

      <div class="flex shrink-0 justify-end gap-2 border-t border-border/50 p-3">
        <Button variant="secondary" size="sm" :disabled="checking || busy" @click="check(true)">
          <Loader2 v-if="checking" class="h-4 w-4 animate-spin" />
          <RefreshCw v-else class="h-4 w-4" />
          {{ t('admin.update.recheck') }}
        </Button>
        <Button size="sm" :disabled="busy" @click="close">{{ t('admin.update.close') }}</Button>
      </div>
    </div>
  </div>
</template>
