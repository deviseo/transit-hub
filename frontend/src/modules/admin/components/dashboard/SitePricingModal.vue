<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, Tag, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { getDashboardPricing, saveDashboardPricing } from '../../api/dashboardAdmin'

// 我的站点售价：站点余额 1 个单位（通常显示为 1 美元）卖多少元。营收、站点用户余额按此换算成人民币，
// 与按「充值倍率」换算的上游成本使用同一单位，净利润与利润率才有意义。
const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saved'): void
}>()

const { t, te } = useI18n()

const loading = ref(false)
const saving = ref(false)
const errorKey = ref<string | null>(null)
const draft = ref('')

const parsed = computed(() => Number(draft.value))
const valid = computed(() => draft.value.trim() !== '' && Number.isFinite(parsed.value) && parsed.value > 0 && parsed.value <= 10000)

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.dashboard.pricing.errors.request'))

const load = async () => {
  loading.value = true
  errorKey.value = null
  try {
    const config = await getDashboardPricing()
    draft.value = String(config.siteUnitPrice ?? 1)
  } catch (err) {
    errorKey.value = err instanceof Error ? err.message : 'admin.dashboard.pricing.errors.request'
  } finally {
    loading.value = false
  }
}

const save = async () => {
  if (!valid.value || saving.value) return
  saving.value = true
  errorKey.value = null
  try {
    await saveDashboardPricing({ siteUnitPrice: parsed.value })
    emit('saved')
    emit('close')
  } catch (err) {
    errorKey.value = err instanceof Error ? err.message : 'admin.dashboard.pricing.errors.request'
  } finally {
    saving.value = false
  }
}

watch(() => props.open, (isOpen) => {
  if (isOpen) void load()
})
</script>

<template>
  <Teleport defer to="body">
    <div v-if="open" class="fixed inset-0 z-[100] flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-background/80 backdrop-blur-sm" @click="emit('close')"></div>
      <div role="dialog" aria-modal="true" aria-labelledby="dashboard-pricing-title" class="relative w-full max-w-lg overflow-hidden rounded-2xl border border-border/60 bg-card text-card-foreground shadow-2xl">
        <div class="flex items-start justify-between gap-4 px-6 pt-6">
          <div class="flex items-center gap-3">
            <div class="flex h-10 w-10 items-center justify-center rounded-full bg-primary/10 text-primary">
              <Tag class="h-5 w-5" />
            </div>
            <div>
              <h2 id="dashboard-pricing-title" class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.pricing.title') }}</h2>
              <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.pricing.subtitle') }}</p>
            </div>
          </div>
          <button type="button" class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground" :title="t('admin.dashboard.pricing.close')" @click="emit('close')">
            <X class="h-5 w-5" />
          </button>
        </div>

        <div class="space-y-4 px-6 py-5">
          <div v-if="loading" class="flex items-center justify-center py-8">
            <Loader2 class="h-5 w-5 animate-spin text-primary/60" />
          </div>
          <template v-else>
            <label class="block space-y-1.5 text-sm font-medium text-foreground">
              <span>{{ t('admin.dashboard.pricing.label') }}</span>
              <span class="relative block">
                <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                <input
                  v-model="draft"
                  type="number"
                  inputmode="decimal"
                  min="0.01"
                  max="10000"
                  step="0.01"
                  class="h-10 w-full rounded-lg border border-border/60 bg-surface pl-7 pr-16 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                  @keydown.enter.prevent="save"
                >
                <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{{ t('admin.dashboard.pricing.unit') }}</span>
              </span>
            </label>
            <ul class="space-y-1 rounded-lg bg-surface p-3 text-xs leading-5 text-muted-foreground">
              <li>{{ t('admin.dashboard.pricing.helpRevenue') }}</li>
              <li>{{ t('admin.dashboard.pricing.helpExample') }}</li>
              <li>{{ t('admin.dashboard.pricing.helpCost') }}</li>
              <li>{{ t('admin.dashboard.pricing.helpHistory') }}</li>
            </ul>
            <p v-if="draft.trim() !== '' && !valid" class="text-xs text-amber-700 dark:text-amber-300">{{ t('admin.dashboard.pricing.errors.invalid') }}</p>
          </template>
          <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
            <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
            <span>{{ errorText(errorKey) }}</span>
          </div>
        </div>

        <div class="flex justify-end gap-2 border-t border-border/50 px-6 py-4">
          <Button variant="secondary" class="rounded-lg" :disabled="saving" @click="emit('close')">{{ t('admin.dashboard.pricing.cancel') }}</Button>
          <Button class="rounded-lg" :disabled="!valid || saving || loading" @click="save">
            <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
            {{ t('admin.dashboard.pricing.save') }}
          </Button>
        </div>
      </div>
    </div>
  </Teleport>
</template>
