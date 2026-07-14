<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Check, Copy, Globe2, Link2, Loader2, RefreshCw, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  getLeaderboardEmbedConfig,
  rotateLeaderboardEmbedToken,
} from '../api/leaderboard'
import type { LeaderboardEmbedConfig } from '../types'

const props = defineProps<{ open: boolean }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()

const config = ref<LeaderboardEmbedConfig | null>(null)
const loading = ref(false)
const rotating = ref(false)
const copied = ref(false)
const errorKey = ref<string | null>(null)

const embedUrl = computed(() => {
  if (!config.value) return ''
  const url = new URL('/embed/leaderboard', window.location.origin)
  url.searchParams.set('embed_token', config.value.embedToken)
  return url.toString()
})

const applyConfig = (value: LeaderboardEmbedConfig) => {
  config.value = value
}

const load = async () => {
  loading.value = true
  errorKey.value = null
  try {
    applyConfig(await getLeaderboardEmbedConfig())
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.leaderboard.errors.unknown'
  } finally {
    loading.value = false
  }
}

watch(() => props.open, (open) => {
  if (open) void load()
})

const rotate = async () => {
  if (!window.confirm(t('admin.leaderboard.embed.confirmRotate'))) return
  rotating.value = true
  errorKey.value = null
  try {
    applyConfig(await rotateLeaderboardEmbedToken())
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.leaderboard.errors.unknown'
  } finally {
    rotating.value = false
  }
}

const copy = async () => {
  if (!embedUrl.value) return
  try {
    await navigator.clipboard.writeText(embedUrl.value)
    copied.value = true
    setTimeout(() => { copied.value = false }, 1500)
  } catch {
    errorKey.value = 'admin.leaderboard.embed.copyFailed'
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition enter-active-class="transition duration-150" enter-from-class="opacity-0" leave-active-class="transition duration-100" leave-to-class="opacity-0">
      <div v-if="open" class="fixed inset-0 z-[160] flex items-center justify-center bg-background/70 p-4 backdrop-blur-sm" @click.self="emit('close')">
        <div role="dialog" aria-modal="true" :aria-label="t('admin.leaderboard.embed.title')" class="w-full max-w-xl overflow-hidden rounded-lg border border-border bg-card shadow-2xl">
          <header class="flex items-center justify-between border-b border-border px-5 py-4">
            <div class="flex items-center gap-3">
              <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary"><Link2 class="h-4 w-4" /></span>
              <div><h2 class="text-base font-semibold">{{ t('admin.leaderboard.embed.title') }}</h2><p class="text-xs text-muted-foreground">{{ t('admin.leaderboard.embed.subtitle') }}</p></div>
            </div>
            <button type="button" class="rounded-md p-2 text-muted-foreground hover:bg-surface hover:text-foreground" :aria-label="t('admin.leaderboard.embed.close')" @click="emit('close')"><X class="h-4 w-4" /></button>
          </header>

          <div class="space-y-5 p-5">
            <div v-if="loading" class="flex min-h-48 items-center justify-center text-muted-foreground"><Loader2 class="h-5 w-5 animate-spin" /></div>
            <template v-else>
              <p v-if="errorKey" class="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">{{ t(errorKey) }}</p>

              <div class="space-y-2">
                <p class="text-sm font-medium">{{ t('admin.leaderboard.embed.sourceOrigin') }}</p>
                <div class="flex min-h-11 items-center gap-3 rounded-lg border border-border/70 bg-surface px-4 py-2.5">
                  <Globe2 class="h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
                  <span class="min-w-0 break-all text-sm font-medium text-foreground">{{ config?.sub2apiSourceOrigin }}</span>
                </div>
                <p class="text-xs text-muted-foreground">{{ t('admin.leaderboard.embed.sourceOriginHint') }}</p>
              </div>

              <div class="space-y-2">
                <label for="leaderboard-embed-url" class="text-sm font-medium">{{ t('admin.leaderboard.embed.url') }}</label>
                <div class="flex gap-2">
                  <Input id="leaderboard-embed-url" :model-value="embedUrl" readonly />
                  <Button variant="secondary" :title="t('admin.leaderboard.embed.copy')" :aria-label="t('admin.leaderboard.embed.copy')" @click="copy">
                    <Check v-if="copied" class="h-4 w-4 text-signal" />
                    <Copy v-else class="h-4 w-4" />
                  </Button>
                </div>
                <p class="text-xs text-muted-foreground">{{ t('admin.leaderboard.embed.urlHint') }}</p>
              </div>
            </template>
          </div>

          <footer class="flex flex-col-reverse gap-2 border-t border-border bg-surface/60 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
            <Button variant="ghost" :disabled="loading || rotating" @click="rotate">
              <Loader2 v-if="rotating" class="h-4 w-4 animate-spin" />
              <RefreshCw v-else class="h-4 w-4" />
              {{ t('admin.leaderboard.embed.rotate') }}
            </Button>
            <Button :disabled="loading" @click="emit('close')">{{ t('admin.leaderboard.embed.close') }}</Button>
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
