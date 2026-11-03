<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CheckCircle2, Globe, Info, Loader2, Save, ShieldAlert, Wifi, XCircle } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { getOutboundProxy, saveOutboundProxy, testOutboundProxy } from '../../api/settings'
import type { OutboundProxySettings, TestOutboundProxyResponse } from '../../types/settings'

const { t } = useI18n()

const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const saveError = ref('')
const saved = ref(false)
const testing = ref(false)
const testError = ref('')
const testResult = ref<TestOutboundProxyResponse | null>(null)

const current = ref<OutboundProxySettings | null>(null)
const enabled = ref(false)
const proxyUrl = ref('')
const bypass = ref('')
const testTarget = ref('')

const canEdit = computed(() => current.value?.canEdit ?? false)
const formInvalid = computed(() => enabled.value && proxyUrl.value.trim() === '')
const canSave = computed(() => canEdit.value && !saving.value && !formInvalid.value)
const canTest = computed(() => canEdit.value && !testing.value && !formInvalid.value && testTarget.value.trim() !== '')

// 当前实际生效的出站方式（与表单里尚未保存的修改无关）。
const modeLabel = computed(() => {
  const settings = current.value
  if (!settings) return ''
  if (settings.mode === 'environment') {
    return settings.environment.proxyUrl
      ? t('admin.settings.proxy.mode.environment', { proxy: settings.environment.proxyUrl })
      : t('admin.settings.proxy.mode.environmentDirect')
  }
  if (settings.mode === 'proxy') {
    return t('admin.settings.proxy.mode.proxy', { proxy: settings.proxyUrl })
  }
  return t('admin.settings.proxy.mode.direct')
})

const testResultText = computed(() => {
  const result = testResult.value
  if (!result) return ''
  const route = t(`admin.settings.proxy.test.routes.${result.route}`)
  if (result.ok) {
    return t('admin.settings.proxy.test.success', { route, status: result.statusCode ?? '-', ms: result.durationMs })
  }
  const reason = t(`admin.settings.proxy.test.failures.${result.failure ?? 'network'}`)
  return t('admin.settings.proxy.test.failure', { route, reason, ms: result.durationMs })
})

const errorKey = (error: unknown, fallback: string) => (error instanceof Error && error.message ? error.message : fallback)

// 后端保存为逗号分隔，展示时加空格便于阅读；提交时逗号、空格、换行都可以作为分隔符。
const applySettings = (settings: OutboundProxySettings) => {
  current.value = settings
  enabled.value = settings.enabled
  proxyUrl.value = settings.proxyUrl
  bypass.value = settings.bypass.split(',').filter(Boolean).join(', ')
}

const formPayload = () => ({
  enabled: enabled.value,
  proxyUrl: proxyUrl.value.trim(),
  bypass: bypass.value,
})

const load = async () => {
  loading.value = true
  loadError.value = ''
  try {
    applySettings(await getOutboundProxy())
  } catch (error) {
    loadError.value = errorKey(error, 'admin.settings.proxy.errors.request')
  } finally {
    loading.value = false
  }
}

let savedTimer: ReturnType<typeof setTimeout> | undefined
const save = async () => {
  if (!canSave.value) return
  saving.value = true
  saveError.value = ''
  saved.value = false
  try {
    applySettings(await saveOutboundProxy(formPayload()))
    saved.value = true
    if (savedTimer) clearTimeout(savedTimer)
    savedTimer = setTimeout(() => { saved.value = false }, 3000)
  } catch (error) {
    saveError.value = errorKey(error, 'admin.settings.proxy.errors.request')
  } finally {
    saving.value = false
  }
}

const runTest = async () => {
  if (!canTest.value) return
  testing.value = true
  testError.value = ''
  testResult.value = null
  try {
    testResult.value = await testOutboundProxy({ ...formPayload(), targetUrl: testTarget.value.trim() })
  } catch (error) {
    testError.value = errorKey(error, 'admin.settings.proxy.errors.request')
  } finally {
    testing.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-5">
    <div>
      <h3 class="text-lg font-semibold text-foreground">{{ t('admin.settings.proxy.title') }}</h3>
      <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.settings.proxy.subtitle') }}</p>
    </div>

    <div v-if="loading" class="flex items-center gap-2 rounded-2xl border border-border/50 bg-card p-5 text-sm text-muted-foreground">
      <Loader2 class="h-4 w-4 animate-spin" />
      {{ t('admin.settings.proxy.loading') }}
    </div>

    <div v-else-if="loadError" class="flex items-start gap-2 rounded-xl border border-destructive/20 bg-destructive/10 p-4 text-sm text-destructive" role="alert">
      <XCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ t(loadError) }}</span>
    </div>

    <template v-else-if="current">
      <!-- 当前生效方式 -->
      <div class="flex items-start gap-3 rounded-xl border border-border/50 bg-surface p-4 text-sm" role="status">
        <Info class="mt-0.5 h-4 w-4 shrink-0 text-primary" />
        <div class="space-y-1">
          <p class="font-medium text-foreground">{{ modeLabel }}</p>
          <p v-if="!current.configured" class="text-xs text-muted-foreground">{{ t('admin.settings.proxy.unconfiguredHint') }}</p>
        </div>
      </div>

      <div v-if="!canEdit" class="flex items-start gap-3 rounded-xl border border-amber-500/30 bg-amber-500/10 p-4 text-sm text-amber-700 dark:text-amber-400">
        <ShieldAlert class="mt-0.5 h-4 w-4 shrink-0" />
        <span>{{ t('admin.settings.proxy.readOnly') }}</span>
      </div>

      <!-- 代理配置 -->
      <div class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
        <div class="flex items-start justify-between gap-4 p-5">
          <div class="flex items-start gap-3">
            <div class="mt-0.5 shrink-0 rounded-xl bg-blue-500/10 p-2 text-blue-500">
              <Globe class="h-5 w-5" />
            </div>
            <div>
              <h4 class="text-sm font-semibold text-foreground">{{ t('admin.settings.proxy.enable') }}</h4>
              <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.settings.proxy.enableHelp') }}</p>
            </div>
          </div>
          <label class="relative mt-1 inline-flex shrink-0 items-center" :class="canEdit ? 'cursor-pointer' : 'cursor-not-allowed opacity-60'">
            <input v-model="enabled" type="checkbox" class="peer sr-only" :disabled="!canEdit" :aria-label="t('admin.settings.proxy.enable')">
            <div class="peer h-6 w-11 rounded-full bg-surface-elevated peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-border after:bg-white after:transition-transform after:content-[''] peer-checked:after:translate-x-full peer-checked:after:border-white"></div>
          </label>
        </div>

        <div class="space-y-4 px-5 pb-5 sm:pl-[4.25rem]">
          <div class="space-y-1.5">
            <label for="outbound-proxy-url" class="text-sm font-medium text-foreground">{{ t('admin.settings.proxy.url') }}</label>
            <Input
              id="outbound-proxy-url"
              v-model="proxyUrl"
              name="outboundProxyUrl"
              :placeholder="t('admin.settings.proxy.urlPlaceholder')"
              :disabled="!canEdit"
              autocomplete="off"
              spellcheck="false"
            />
            <p v-if="formInvalid" class="text-xs text-destructive" role="alert">{{ t('admin.settings.proxy.urlRequired') }}</p>
            <p v-else class="text-xs text-muted-foreground">{{ t('admin.settings.proxy.urlHelp') }}</p>
          </div>

          <div class="space-y-1.5">
            <label for="outbound-proxy-bypass" class="text-sm font-medium text-foreground">{{ t('admin.settings.proxy.bypass') }}</label>
            <textarea
              id="outbound-proxy-bypass"
              v-model="bypass"
              name="outboundProxyBypass"
              rows="3"
              spellcheck="false"
              :disabled="!canEdit"
              :placeholder="t('admin.settings.proxy.bypassPlaceholder')"
              class="w-full rounded-lg border border-border/70 bg-surface px-4 py-3 text-sm text-foreground outline-none transition-[color,background-color,border-color,box-shadow] placeholder:text-muted-foreground focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/30 disabled:cursor-not-allowed disabled:opacity-60"
            ></textarea>
            <p class="text-xs text-muted-foreground">{{ t('admin.settings.proxy.bypassHelp') }}</p>
          </div>
        </div>
      </div>

      <!-- 连接测试：使用表单里尚未保存的配置 -->
      <div class="space-y-3 rounded-2xl border border-border/50 bg-card p-5 shadow-sm">
        <div class="flex items-start gap-3">
          <div class="mt-0.5 shrink-0 rounded-xl bg-emerald-500/10 p-2 text-emerald-500">
            <Wifi class="h-5 w-5" />
          </div>
          <div>
            <h4 class="text-sm font-semibold text-foreground">{{ t('admin.settings.proxy.test.title') }}</h4>
            <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.settings.proxy.test.help') }}</p>
          </div>
        </div>
        <div class="flex flex-col gap-2 sm:flex-row sm:pl-[3.25rem]">
          <Input
            v-model="testTarget"
            name="outboundProxyTestTarget"
            class="sm:flex-1"
            :placeholder="t('admin.settings.proxy.test.targetPlaceholder')"
            :disabled="!canEdit || testing"
            autocomplete="off"
            spellcheck="false"
            :aria-label="t('admin.settings.proxy.test.target')"
            @keydown.enter.prevent="runTest"
          />
          <Button variant="secondary" class="h-11 shrink-0" :disabled="!canTest" @click="runTest">
            <Loader2 v-if="testing" class="h-4 w-4 animate-spin" />
            {{ testing ? t('admin.settings.proxy.test.running') : t('admin.settings.proxy.test.run') }}
          </Button>
        </div>
        <div
          v-if="testResult"
          class="flex items-start gap-2 text-sm sm:pl-[3.25rem]"
          :class="testResult.ok ? 'text-emerald-600 dark:text-emerald-400' : 'text-destructive'"
          role="status"
        >
          <CheckCircle2 v-if="testResult.ok" class="mt-0.5 h-4 w-4 shrink-0" />
          <XCircle v-else class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ testResultText }}</span>
        </div>
        <p v-if="testError" class="text-sm text-destructive sm:pl-[3.25rem]" role="alert">{{ t(testError) }}</p>
      </div>

      <div class="flex flex-wrap items-center justify-end gap-3">
        <span v-if="saved" class="flex items-center gap-1 text-sm text-emerald-600 dark:text-emerald-400" role="status">
          <CheckCircle2 class="h-4 w-4" />
          {{ t('admin.settings.proxy.saved') }}
        </span>
        <span v-if="saveError" class="text-sm text-destructive" role="alert">{{ t(saveError) }}</span>
        <Button :disabled="!canSave" @click="save">
          <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
          <Save v-else class="h-4 w-4" />
          {{ t('admin.settings.proxy.save') }}
        </Button>
      </div>
    </template>
  </div>
</template>
