<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { CheckCircle2, Copy, ExternalLink, LifeBuoy, Link2, Loader2, Megaphone, Palette, Plus, RotateCcw, Save, Sparkles, Trash2, Upload } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { getClientConfig, getClientGroups, resetClientConfigCode, saveClientConfig } from '../api/clientConfig'
import type { ClientConfig, ClientConfigUpdate, ClientGroup, ClientSupportLink, ClientSupportType } from '../types/clientConfig'

const { t } = useI18n()

const maxSupportLinks = 8
const maxLogoBytes = 256 * 1024
const supportTypes: ClientSupportType[] = ['telegram', 'qq', 'wechat', 'discord', 'email', 'url', 'other']
const defaultColor = '#2563eb'

const loaded = ref<ClientConfig | null>(null)
const form = ref<ClientConfigUpdate | null>(null)
const loading = ref(true)
const loadError = ref('')
const saving = ref(false)
const saved = ref(false)
const saveError = ref('')
const resetting = ref(false)
const confirmingReset = ref(false)
const copied = ref(false)

const groups = ref<ClientGroup[]>([])
const groupsLoading = ref(false)
const groupsError = ref('')

const fromConfig = (config: ClientConfig): ClientConfigUpdate => ({
  enabled: config.enabled,
  // 对外地址要站长自己填：现在打开后台用的地址常常是内网或本机地址，不能直接发给用户。
  hubUrl: config.hubUrl,
  siteUrl: config.siteUrl,
  brand: { ...config.brand },
  recommend: { claudeCode: { ...config.recommend.claudeCode }, codex: { ...config.recommend.codex } },
  notice: config.notice,
  support: config.support.map((link) => ({ ...link })),
  minClientVersion: config.minClientVersion,
  tickets: { enabled: config.tickets.enabled, categories: [...config.tickets.categories] },
})

const load = async () => {
  loading.value = true
  loadError.value = ''
  try {
    const config = await getClientConfig()
    loaded.value = config
    form.value = fromConfig(config)
  } catch (error) {
    loadError.value = error instanceof Error ? error.message : 'admin.client.errors.request'
  } finally {
    loading.value = false
  }
}

const loadGroups = async () => {
  groupsLoading.value = true
  groupsError.value = ''
  try {
    groups.value = await getClientGroups()
  } catch (error) {
    groupsError.value = error instanceof Error ? error.message : 'admin.client.errors.request'
  } finally {
    groupsLoading.value = false
  }
}

onMounted(() => {
  void load()
  void loadGroups()
})

const hubUrlMissing = computed(() => !form.value?.hubUrl.trim())

const configUrl = computed(() => {
  if (!form.value || !loaded.value?.configPath || hubUrlMissing.value) return ''
  return `${form.value.hubUrl.trim().replace(/\/+$/, '')}${loaded.value.configPath}`
})

const link = computed(() => (configUrl.value ? `transitlink://connect?config=${encodeURIComponent(configUrl.value)}` : ''))

// 问题分类在文本框里每行一个：编辑时保留原文（不然回车换行会被马上去掉），保存时再拆分。
const ticketCategoriesText = ref('')
watch(form, (value) => { ticketCategoriesText.value = value?.tickets.categories.join('\n') ?? '' })

const save = async () => {
  if (!form.value) return
  form.value.tickets.categories = ticketCategoriesText.value.split('\n').map((line) => line.trim()).filter(Boolean)
  saving.value = true
  saved.value = false
  saveError.value = ''
  try {
    const config = await saveClientConfig(form.value)
    loaded.value = config
    form.value = fromConfig(config)
    saved.value = true
    window.setTimeout(() => { saved.value = false }, 2500)
  } catch (error) {
    saveError.value = error instanceof Error ? error.message : 'admin.client.errors.request'
  } finally {
    saving.value = false
  }
}

const resetCode = async () => {
  resetting.value = true
  saveError.value = ''
  try {
    const config = await resetClientConfigCode()
    loaded.value = config
    confirmingReset.value = false
  } catch (error) {
    saveError.value = error instanceof Error ? error.message : 'admin.client.errors.request'
  } finally {
    resetting.value = false
  }
}

const copyLink = async () => {
  if (!link.value) return
  await navigator.clipboard.writeText(link.value)
  copied.value = true
  window.setTimeout(() => { copied.value = false }, 1500)
}

// —— Logo ——
const logoInput = ref<HTMLInputElement | null>(null)
const logoError = ref('')
const uploadedLogo = computed(() => form.value?.brand.logo.startsWith('data:') ?? false)

const onLogoFile = (event: Event) => {
  const file = (event.target as HTMLInputElement).files?.[0]
  ;(event.target as HTMLInputElement).value = ''
  if (!file || !form.value) return
  logoError.value = ''
  if (!['image/png', 'image/jpeg', 'image/webp', 'image/gif'].includes(file.type)) {
    logoError.value = 'admin.client.errors.logoType'
    return
  }
  if (file.size > maxLogoBytes) {
    logoError.value = 'admin.client.errors.logoTooLarge'
    return
  }
  const reader = new FileReader()
  reader.onload = () => {
    if (form.value && typeof reader.result === 'string') form.value.brand.logo = reader.result
  }
  reader.readAsDataURL(file)
}

const logoUrlInput = computed({
  get: () => (uploadedLogo.value ? '' : form.value?.brand.logo ?? ''),
  set: (value: string) => { if (form.value) form.value.brand.logo = value },
})

// —— 品牌预览 ——
// 没填名称时 TransitLink 显示站点自己的名称，预览里用「站点名称」代替。
const previewName = computed(() => form.value?.brand.name || t('admin.client.brand.previewName'))
const previewColor = computed(() => (/^#[0-9a-fA-F]{6}$/.test(form.value?.brand.color ?? '') ? form.value!.brand.color : defaultColor))
const colorPicker = computed({
  get: () => previewColor.value,
  set: (value: string) => { if (form.value) form.value.brand.color = value },
})

// —— 推荐分组 ——
const groupLabel = (group: ClientGroup) => {
  const ratio = group.multiplier != null ? ` ×${group.multiplier}` : ''
  const platform = group.platform ? ` · ${group.platform}` : ''
  return `${group.name}${ratio}${platform}`
}
const missingGroup = (value: string) => value !== '' && !groupsLoading.value && !groupsError.value && !groups.value.some((group) => group.id === value)

// —— 客服 ——
const addSupport = () => {
  if (!form.value || form.value.support.length >= maxSupportLinks) return
  form.value.support.push({ type: 'telegram', label: '', value: '' } satisfies ClientSupportLink)
}
const removeSupport = (index: number) => form.value?.support.splice(index, 1)

watch(() => form.value?.brand.logo, () => { logoError.value = '' })
</script>

<template>
  <div class="mx-auto flex w-full max-w-5xl flex-col gap-6">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold tracking-tight text-foreground">{{ t('admin.client.title') }}</h1>
        <p class="mt-1 max-w-2xl text-sm text-muted-foreground">{{ t('admin.client.subtitle') }}</p>
      </div>
      <Button v-if="form" :disabled="saving" class="min-w-[120px]" @click="save">
        <Loader2 v-if="saving" class="mr-2 h-4 w-4 animate-spin" />
        <CheckCircle2 v-else-if="saved" class="mr-2 h-4 w-4 text-green-400" />
        <Save v-else class="mr-2 h-4 w-4" />
        {{ saved ? t('admin.client.saved') : (saving ? t('admin.client.saving') : t('admin.client.save')) }}
      </Button>
    </header>

    <p v-if="loading" class="text-sm text-muted-foreground">{{ t('admin.client.loading') }}</p>
    <p v-if="loadError" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{{ t(loadError) }}</p>
    <p v-if="saveError" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive" role="alert">{{ t(saveError) }}</p>

    <template v-if="form && loaded">
      <!-- 专属链接 -->
      <section class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
        <div class="flex items-start justify-between gap-4 border-b border-border/50 bg-surface/30 p-6">
          <div class="flex items-start gap-3">
            <div class="rounded-xl bg-blue-500/10 p-2 text-blue-500"><Link2 class="h-5 w-5" /></div>
            <div>
              <h2 class="text-lg font-semibold text-foreground">{{ t('admin.client.link.title') }}</h2>
              <p class="text-sm text-muted-foreground">{{ t('admin.client.link.description') }}</p>
            </div>
          </div>
          <label class="relative mt-1 inline-flex shrink-0 cursor-pointer items-center gap-3">
            <span class="text-sm font-medium text-foreground">{{ t('admin.client.link.enabled') }}</span>
            <input v-model="form.enabled" type="checkbox" class="peer sr-only" :aria-label="t('admin.client.link.enabled')">
            <div class="relative h-6 w-11 shrink-0 rounded-full bg-surface-elevated peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-border after:bg-white after:transition-transform after:content-[''] peer-checked:after:translate-x-full peer-checked:after:border-white"></div>
          </label>
        </div>
        <div class="space-y-5 p-6">
          <p class="text-xs text-muted-foreground">{{ t('admin.client.link.enabledHelp') }}</p>
          <div class="grid gap-4 md:grid-cols-2">
            <div class="grid gap-2">
              <label for="client-hub-url" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.link.hubUrl') }}</label>
              <Input id="client-hub-url" v-model="form.hubUrl" placeholder="https://hub.example.com" class="h-9 text-sm" />
              <p class="text-xs text-muted-foreground">{{ t('admin.client.link.hubUrlHelp') }}</p>
            </div>
            <div class="grid gap-2">
              <label for="client-site-url" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.link.siteUrl') }}</label>
              <Input id="client-site-url" v-model="form.siteUrl" placeholder="https://api.example.com" class="h-9 text-sm" />
              <p class="text-xs text-muted-foreground">{{ t('admin.client.link.siteUrlHelp') }}</p>
            </div>
            <div class="grid gap-2">
              <label for="client-min-version" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.link.minVersion') }}</label>
              <Input id="client-min-version" v-model="form.minClientVersion" :placeholder="t('admin.client.link.minVersionPlaceholder')" maxlength="20" class="h-9 text-sm" />
              <p class="text-xs text-muted-foreground">
                {{ t('admin.client.link.minVersionHelp') }}
                <a href="https://github.com/deviseo/transitlink/releases" target="_blank" rel="noopener noreferrer" class="text-primary hover:underline">{{ t('admin.client.link.versions') }}</a>
              </p>
            </div>
          </div>

          <div class="grid gap-2">
            <span class="text-xs font-medium text-muted-foreground">{{ t('admin.client.link.link') }}</span>
            <p v-if="!link" class="rounded-lg border border-dashed border-border px-3 py-2 text-sm text-muted-foreground">{{ t(loaded.configPath && hubUrlMissing ? 'admin.client.link.needHubUrl' : 'admin.client.link.unsaved') }}</p>
            <template v-else>
              <div class="flex flex-wrap items-center gap-2">
                <code class="min-w-0 flex-1 truncate rounded-lg border border-border/60 bg-surface/40 px-3 py-2 font-mono text-xs text-foreground">{{ link }}</code>
                <Button variant="secondary" size="sm" @click="copyLink">
                  <CheckCircle2 v-if="copied" class="mr-1.5 h-4 w-4 text-green-500" />
                  <Copy v-else class="mr-1.5 h-4 w-4" />
                  {{ copied ? t('admin.client.link.copied') : t('admin.client.link.copy') }}
                </Button>
                <Button v-if="!confirmingReset" variant="ghost" size="sm" @click="confirmingReset = true">
                  <RotateCcw class="mr-1.5 h-4 w-4" />
                  {{ t('admin.client.link.reset') }}
                </Button>
              </div>
              <div v-if="confirmingReset" class="flex flex-wrap items-center gap-2 rounded-lg border border-destructive/30 bg-destructive/5 px-3 py-2 text-sm">
                <span class="flex-1 text-foreground">{{ t('admin.client.link.resetConfirm') }}</span>
                <Button variant="ghost" size="sm" @click="confirmingReset = false">{{ t('admin.client.cancel') }}</Button>
                <Button variant="destructive" size="sm" :disabled="resetting" @click="resetCode">
                  <Loader2 v-if="resetting" class="mr-1.5 h-4 w-4 animate-spin" />
                  {{ t('admin.client.link.reset') }}
                </Button>
              </div>
              <p v-if="!loaded.enabled" class="text-xs text-amber-600 dark:text-amber-400">{{ t('admin.client.link.disabledNote') }}</p>
              <p class="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
                {{ t('admin.client.link.configUrl') }}：<span class="break-all font-mono">{{ configUrl }}</span>
                <a v-if="loaded.enabled" :href="configUrl" target="_blank" rel="noopener" class="inline-flex items-center gap-1 text-primary hover:underline">
                  <ExternalLink class="h-3.5 w-3.5" />{{ t('admin.client.link.openConfig') }}
                </a>
              </p>
            </template>
          </div>
        </div>
      </section>

      <!-- 品牌 -->
      <section class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
        <div class="flex items-start gap-3 border-b border-border/50 bg-surface/30 p-6">
          <div class="rounded-xl bg-purple-500/10 p-2 text-purple-500"><Palette class="h-5 w-5" /></div>
          <div>
            <h2 class="text-lg font-semibold text-foreground">{{ t('admin.client.brand.title') }}</h2>
            <p class="text-sm text-muted-foreground">{{ t('admin.client.brand.description') }}</p>
          </div>
        </div>
        <div class="grid gap-6 p-6 lg:grid-cols-[1fr_280px]">
          <div class="space-y-5">
            <div class="grid gap-4 md:grid-cols-2">
              <div class="grid gap-2">
                <label for="client-brand-name" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.brand.name') }}</label>
                <Input id="client-brand-name" v-model="form.brand.name" :placeholder="t('admin.client.brand.namePlaceholder')" maxlength="40" class="h-9 text-sm" />
              </div>
              <div class="grid gap-2">
                <label for="client-brand-color" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.brand.color') }}</label>
                <div class="flex items-center gap-2">
                  <input v-model="colorPicker" type="color" class="h-9 w-12 cursor-pointer rounded-lg border border-input bg-background p-1" :aria-label="t('admin.client.brand.color')">
                  <Input id="client-brand-color" v-model="form.brand.color" :placeholder="defaultColor" maxlength="7" class="h-9 font-mono text-sm" />
                </div>
              </div>
            </div>
            <div class="grid gap-2">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.client.brand.logo') }}</span>
              <div class="flex flex-wrap items-center gap-2">
                <input ref="logoInput" type="file" accept="image/png,image/jpeg,image/webp,image/gif" class="hidden" @change="onLogoFile">
                <Button variant="secondary" size="sm" @click="logoInput?.click()">
                  <Upload class="mr-1.5 h-4 w-4" />{{ t('admin.client.brand.upload') }}
                </Button>
                <Input v-model="logoUrlInput" :placeholder="t('admin.client.brand.logoUrl')" :disabled="uploadedLogo" class="h-9 min-w-0 flex-1 text-sm" />
                <Button v-if="form.brand.logo" variant="ghost" size="sm" @click="form.brand.logo = ''">
                  <Trash2 class="mr-1.5 h-4 w-4" />{{ t('admin.client.brand.remove') }}
                </Button>
              </div>
              <p class="text-xs text-muted-foreground">{{ uploadedLogo ? t('admin.client.brand.uploaded') : t('admin.client.brand.logoHelp') }}</p>
              <p v-if="logoError" class="text-xs text-destructive">{{ t(logoError) }}</p>
            </div>
            <div class="grid gap-2">
              <label for="client-welcome" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.brand.welcome') }}</label>
              <Input id="client-welcome" v-model="form.brand.welcome" :placeholder="t('admin.client.brand.welcomePlaceholder')" maxlength="200" class="h-9 text-sm" />
            </div>
          </div>

          <!-- 预览：TransitLink 侧边栏的样子 -->
          <div class="grid content-start gap-2">
            <span class="text-xs font-medium text-muted-foreground">{{ t('admin.client.brand.preview') }}</span>
            <div class="overflow-hidden rounded-xl border border-border/60 bg-background">
              <div class="flex items-center gap-2.5 border-b border-border/60 px-4 py-3">
                <img v-if="form.brand.logo" :src="form.brand.logo" alt="" class="h-8 w-8 rounded-lg object-cover">
                <div v-else class="flex h-8 w-8 items-center justify-center rounded-lg text-sm font-semibold text-white" :style="{ backgroundColor: previewColor }">{{ previewName.slice(0, 1) }}</div>
                <span class="truncate text-sm font-semibold text-foreground">{{ previewName }}</span>
              </div>
              <div class="space-y-2 p-3">
                <div class="rounded-lg px-3 py-1.5 text-xs font-medium text-white" :style="{ backgroundColor: previewColor }">{{ t('admin.client.brand.previewHome') }}</div>
                <div class="px-3 text-xs text-muted-foreground">{{ t('admin.client.brand.previewTools') }}</div>
                <p v-if="form.brand.welcome" class="rounded-lg bg-surface/50 px-3 py-2 text-xs text-foreground">{{ form.brand.welcome }}</p>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- 推荐配置 -->
      <section class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
        <div class="flex items-start gap-3 border-b border-border/50 bg-surface/30 p-6">
          <div class="rounded-xl bg-emerald-500/10 p-2 text-emerald-500"><Sparkles class="h-5 w-5" /></div>
          <div>
            <h2 class="text-lg font-semibold text-foreground">{{ t('admin.client.recommend.title') }}</h2>
            <p class="text-sm text-muted-foreground">{{ t('admin.client.recommend.description') }}</p>
          </div>
        </div>
        <div class="space-y-4 p-6">
          <p v-if="groupsLoading" class="text-xs text-muted-foreground">{{ t('admin.client.recommend.groupsLoading') }}</p>
          <p v-if="groupsError" class="rounded-lg border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
            {{ t('admin.client.recommend.groupsFailed', { error: t(groupsError) }) }}
          </p>
          <div v-for="tool in (['claudeCode', 'codex'] as const)" :key="tool" class="grid gap-3 rounded-xl border border-border/50 p-4 md:grid-cols-[140px_1fr_1fr] md:items-center">
            <span class="text-sm font-semibold text-foreground">{{ t(`admin.client.recommend.${tool}`) }}</span>
            <div class="grid gap-1.5">
              <label :for="`client-${tool}-group`" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.recommend.group') }}</label>
              <select
                v-if="!groupsError"
                :id="`client-${tool}-group`"
                v-model="form.recommend[tool].group"
                class="flex h-9 w-full rounded-lg border border-input bg-background px-3 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              >
                <option value="">{{ t('admin.client.recommend.groupAuto') }}</option>
                <option v-if="missingGroup(form.recommend[tool].group)" :value="form.recommend[tool].group">{{ t('admin.client.recommend.missingGroup', { group: form.recommend[tool].group }) }}</option>
                <option v-for="group in groups" :key="group.id" :value="group.id">{{ groupLabel(group) }}</option>
              </select>
              <Input v-else :id="`client-${tool}-group`" v-model="form.recommend[tool].group" :placeholder="t('admin.client.recommend.groupManual')" class="h-9 text-sm" />
            </div>
            <div class="grid gap-1.5">
              <label :for="`client-${tool}-model`" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.recommend.model') }}</label>
              <Input
                :id="`client-${tool}-model`"
                v-model="form.recommend[tool].model"
                :placeholder="t(tool === 'claudeCode' ? 'admin.client.recommend.claudeModelPlaceholder' : 'admin.client.recommend.codexModelPlaceholder')"
                maxlength="100"
                class="h-9 font-mono text-sm"
              />
            </div>
          </div>
        </div>
      </section>

      <!-- 公告与客服 -->
      <section class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
        <div class="flex items-start gap-3 border-b border-border/50 bg-surface/30 p-6">
          <div class="rounded-xl bg-amber-500/10 p-2 text-amber-500"><Megaphone class="h-5 w-5" /></div>
          <div>
            <h2 class="text-lg font-semibold text-foreground">{{ t('admin.client.notice.title') }}</h2>
            <p class="text-sm text-muted-foreground">{{ t('admin.client.notice.description') }}</p>
          </div>
        </div>
        <div class="space-y-5 p-6">
          <div class="grid gap-2">
            <label for="client-notice" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.notice.notice') }}</label>
            <textarea
              id="client-notice"
              v-model="form.notice"
              rows="3"
              maxlength="2000"
              :placeholder="t('admin.client.notice.noticePlaceholder')"
              class="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            ></textarea>
          </div>
          <div class="grid gap-2">
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.client.notice.support') }}</span>
              <Button variant="secondary" size="sm" :disabled="form.support.length >= maxSupportLinks" @click="addSupport">
                <Plus class="mr-1.5 h-4 w-4" />{{ t('admin.client.notice.add') }}
              </Button>
            </div>
            <p v-if="form.support.length === 0" class="text-xs text-muted-foreground">{{ t('admin.client.notice.supportEmpty') }}</p>
            <div v-for="(item, index) in form.support" :key="index" class="grid gap-2 md:grid-cols-[130px_160px_1fr_auto] md:items-center">
              <select v-model="item.type" :aria-label="t('admin.client.notice.type')" class="flex h-9 w-full rounded-lg border border-input bg-background px-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
                <option v-for="kind in supportTypes" :key="kind" :value="kind">{{ t(`admin.client.notice.types.${kind}`) }}</option>
              </select>
              <Input v-model="item.label" :placeholder="t('admin.client.notice.label')" maxlength="30" class="h-9 text-sm" />
              <Input v-model="item.value" :placeholder="t('admin.client.notice.value')" maxlength="300" class="h-9 text-sm" />
              <Button variant="ghost" size="sm" :aria-label="t('admin.client.notice.remove')" @click="removeSupport(index)">
                <Trash2 class="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>
      </section>

      <section class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
        <div class="flex items-start justify-between gap-4 border-b border-border/50 bg-surface/30 p-6">
          <div class="flex items-start gap-3">
            <div class="rounded-xl bg-emerald-500/10 p-2 text-emerald-500"><LifeBuoy class="h-5 w-5" /></div>
            <div>
              <h2 class="text-lg font-semibold text-foreground">{{ t('admin.client.tickets.title') }}</h2>
              <p class="text-sm text-muted-foreground">{{ t('admin.client.tickets.description') }}</p>
            </div>
          </div>
          <label class="relative mt-1 inline-flex shrink-0 cursor-pointer items-center gap-3">
            <span class="text-sm font-medium text-foreground">{{ t('admin.client.tickets.enabled') }}</span>
            <input v-model="form.tickets.enabled" type="checkbox" class="peer sr-only" :aria-label="t('admin.client.tickets.enabled')">
            <div class="relative h-6 w-11 shrink-0 rounded-full bg-surface-elevated peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-border after:bg-white after:transition-transform after:content-[''] peer-checked:after:translate-x-full peer-checked:after:border-white"></div>
          </label>
        </div>
        <div class="space-y-5 p-6">
          <div class="grid gap-2">
            <label for="client-ticket-categories" class="text-xs font-medium text-muted-foreground">{{ t('admin.client.tickets.categories') }}</label>
            <textarea
              id="client-ticket-categories"
              v-model="ticketCategoriesText"
              rows="5"
              class="w-full rounded-lg border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
            ></textarea>
            <p class="text-xs text-muted-foreground">{{ t('admin.client.tickets.categoriesHelp') }}</p>
          </div>
          <p class="text-xs text-muted-foreground">
            {{ t('admin.client.tickets.notifyHelp') }}
            <RouterLink to="/admin/tickets" class="text-primary hover:underline">{{ t('admin.client.tickets.openTickets') }}</RouterLink>
          </p>
        </div>
      </section>
    </template>
  </div>
</template>
