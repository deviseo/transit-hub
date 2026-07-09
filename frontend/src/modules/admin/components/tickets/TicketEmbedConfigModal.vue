<script setup lang="ts">
import { onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Check, Copy, ExternalLink, Loader2, MessageSquare, Plus, RefreshCw, RotateCcw, Trash2, X } from 'lucide-vue-next'
import { getEmbedConfig, rotateEmbedToken, updateEmbedConfig } from '../../api/tickets'
import type { TicketEmbedConfig, TicketEmbedTemplate } from '../../types/tickets'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()
const prefix = 'admin.tickets.embedConfig'

const config = ref<TicketEmbedConfig | null>(null)
const isLoading = ref(false)
const errorKey = ref<string | null>(null)

const templateDraft = ref<TicketEmbedTemplate>('default')
const maxImagesDraft = ref(0)
const isSaving = ref(false)
const saveErrorKey = ref<string | null>(null)
const saveSuccessKey = ref<string | null>(null)
let saveSuccessTimer: ReturnType<typeof setTimeout> | null = null

// saveRequestSeq/modalSessionSeq 是防止"保存请求过期后仍然污染界面"的版本号：
// modalSessionSeq 在弹窗每次打开/关闭时递增，代表一轮独立的弹窗会话；saveRequestSeq 在每次
// 点击保存时递增，代表一次独立的保存请求。updateEmbedConfig 的 then/catch/finally 回调触发时，
// 只有两个版本号都还等于"当前"时才允许写入 applyConfig/saveSuccessKey/saveErrorKey/isSaving，
// 否则说明用户已经关闭重开或又发起了一次新的保存，这次回调的结果必须静默丢弃。
let saveRequestSeq = 0
let modalSessionSeq = 0

const clearSaveSuccessTimer = () => {
  if (saveSuccessTimer) {
    clearTimeout(saveSuccessTimer)
    saveSuccessTimer = null
  }
}

const clearSaveFeedback = () => {
  saveErrorKey.value = null
  saveSuccessKey.value = null
  clearSaveSuccessTimer()
}

const isCurrentSave = (sessionSeq: number, requestSeq: number): boolean => (
  props.open && sessionSeq === modalSessionSeq && requestSeq === saveRequestSeq
)

// 组件本身随后台 TicketsView 一直挂载，v-if 只控制内部面板显隐，理论上不会真的被卸载；
// 仍然显式清理 timer，覆盖父组件被整体卸载（例如路由切换离开后台）的情况。
onUnmounted(clearSaveSuccessTimer)

const MIN_MAX_IMAGES = 0
const MAX_MAX_IMAGES = 9

const isRotating = ref(false)
const rotateErrorKey = ref<string | null>(null)

const isCopied = ref(false)
const copyErrorKey = ref<string | null>(null)

const templateOptions: TicketEmbedTemplate[] = ['default', 'minimal', 'support']

// 设置分区导航：基础设置 / 分类 / 优先级。分类和优先级是用户要求的两个独立左侧入口，
// 点击后在右侧编辑对应的 option list。
type SettingsSection = 'basic' | 'category' | 'priority'
const sections: SettingsSection[] = ['basic', 'category', 'priority']
const activeSection = ref<SettingsSection>('basic')

// 与后端 DefaultCategoryOptions/DefaultPriorityOptions 保持一致，仅用于"恢复默认值"按钮——
// 不作为渲染选项列表的最终数据来源（数据来源始终是后端返回的 config.categoryOptions/priorityOptions）。
const DEFAULT_CATEGORY_OPTIONS = ['通用问题', '余额/计费', '接口调用', '生图问题', '账号/登录']
const DEFAULT_PRIORITY_OPTIONS = ['低', '普通', '高', '紧急']

const categoryDraft = ref<string[]>([])
const priorityDraft = ref<string[]>([])
const newCategoryOption = ref('')
const newPriorityOption = ref('')

// 复制/打开按钮必须基于当前浏览器前端 origin 拼接地址，不能使用后端按请求 Host 拼出的
// config.embedUrl——本地开发时前端(5444)通过 Vite 代理访问后端(5555) API，两者 origin
// 不一致，直接用后端返回的地址会复制/打开出 5555 的页面地址导致 404。
const buildFrontendEmbedUrl = (embedToken: string): string => {
  const url = new URL('/embed/tickets', window.location.origin)
  url.searchParams.set('embed_token', embedToken)
  return url.toString()
}

const applyConfig = (next: TicketEmbedConfig) => {
  config.value = next
  templateDraft.value = next.template
  maxImagesDraft.value = next.maxImagesPerTicket
  // 旧后端响应可能缺失 categoryOptions/priorityOptions（还未升级），用空数组兜底避免白屏；
  // 保存时仍然以后端校验为准，空数组会被后端拒绝，用户需要先补充选项。
  categoryDraft.value = [...(next.categoryOptions ?? [])]
  priorityDraft.value = [...(next.priorityOptions ?? [])]
}

const decrementMaxImages = () => {
  maxImagesDraft.value = Math.max(MIN_MAX_IMAGES, maxImagesDraft.value - 1)
}

const incrementMaxImages = () => {
  maxImagesDraft.value = Math.min(MAX_MAX_IMAGES, maxImagesDraft.value + 1)
}

const load = async () => {
  isLoading.value = true
  errorKey.value = null
  try {
    applyConfig(await getEmbedConfig())
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
  } finally {
    isLoading.value = false
  }
}

watch(() => props.open, (isOpen) => {
  // 打开和关闭都各自开启新一轮弹窗会话：递增 modalSessionSeq 让任何仍在途中的旧 save() 请求
  // 在 resolve/reject 时被 isCurrentSave 判定为过期而静默丢弃，不会用旧请求的结果（无论成功还是
  // 失败）污染新一轮会话——包括"保存后立刻关闭再重开"这种旧请求仍未返回的场景。
  // isSaving 在这里同步复位是安全的：后续旧请求的 finally 分支会因为 isCurrentSave 为 false
  // 而不再回写 isSaving，不会出现"复位后又被旧请求重新置为 true"的 stale update。
  modalSessionSeq += 1
  isSaving.value = false
  clearSaveFeedback()
  if (isOpen) {
    activeSection.value = 'basic'
    void load()
  }
})

const addOption = (list: typeof categoryDraft, input: typeof newCategoryOption) => {
  const value = input.value.trim()
  if (!value) return
  list.value = [...list.value, value]
  input.value = ''
}

const removeOption = (list: typeof categoryDraft, index: number) => {
  if (list.value.length <= 1) return
  list.value = list.value.filter((_, i) => i !== index)
}

const addCategoryOption = () => addOption(categoryDraft, newCategoryOption)
const removeCategoryOption = (index: number) => removeOption(categoryDraft, index)
const restoreCategoryDefaults = () => { categoryDraft.value = [...DEFAULT_CATEGORY_OPTIONS] }

const addPriorityOption = () => addOption(priorityDraft, newPriorityOption)
const removePriorityOption = (index: number) => removeOption(priorityDraft, index)
const restorePriorityDefaults = () => { priorityDraft.value = [...DEFAULT_PRIORITY_OPTIONS] }

const save = async () => {
  const sessionSeq = modalSessionSeq
  const requestSeq = ++saveRequestSeq
  isSaving.value = true
  clearSaveFeedback()
  try {
    const nextConfig = await updateEmbedConfig({
      template: templateDraft.value,
      maxImagesPerTicket: maxImagesDraft.value,
      categoryOptions: categoryDraft.value.map((option) => option.trim()),
      priorityOptions: priorityDraft.value.map((option) => option.trim()),
    })
    // 只有这轮弹窗会话和这次保存请求仍然是"当前"时才应用结果——用户点击保存后立刻关闭/重开，
    // 或紧接着又发起了一次新的保存，都会让这里判定为过期请求并静默丢弃，不会用旧结果覆盖
    // 新一轮弹窗的 draft，也不会误报"已保存"。
    if (!isCurrentSave(sessionSeq, requestSeq)) return
    applyConfig(nextConfig)
    saveSuccessKey.value = `${prefix}.saveSuccess`
    saveSuccessTimer = setTimeout(() => {
      saveSuccessKey.value = null
      saveSuccessTimer = null
    }, 2000)
  } catch (error) {
    if (!isCurrentSave(sessionSeq, requestSeq)) return
    saveErrorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
  } finally {
    if (isCurrentSave(sessionSeq, requestSeq)) {
      isSaving.value = false
    }
  }
}

// 轮换 embed token 会让旧的 iframe 嵌入地址立即失效，属于有一定破坏性的操作，必须二次确认。
const rotateToken = async () => {
  if (!window.confirm(t(`${prefix}.confirmRotate`))) return
  isRotating.value = true
  rotateErrorKey.value = null
  try {
    applyConfig(await rotateEmbedToken())
  } catch (error) {
    rotateErrorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
  } finally {
    isRotating.value = false
  }
}

const copyEmbedUrl = async () => {
  if (!config.value) return
  try {
    await navigator.clipboard.writeText(buildFrontendEmbedUrl(config.value.embedToken))
    isCopied.value = true
    copyErrorKey.value = null
    setTimeout(() => { isCopied.value = false }, 1500)
  } catch (error) {
    copyErrorKey.value = `${prefix}.copyFailed`
    setTimeout(() => { copyErrorKey.value = null }, 1500)
  }
}

// 预览按钮用于 TransitHub 后台快速打开 embed 页面自查；不是在真实 Sub2API iframe 中打开，
// 因此通常会缺少 user_id/token/src_host/src_url，页面会展示"缺少参数"提示，这是预期行为，
// 页面路径本身不应该 404。
const openPreview = () => {
  if (!config.value) return
  window.open(buildFrontendEmbedUrl(config.value.embedToken), '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-if="open" class="fixed inset-0 z-[150] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

        <Transition
          enter-active-class="transition duration-200 ease-out"
          enter-from-class="opacity-0 scale-95"
          enter-to-class="opacity-100 scale-100"
          leave-active-class="transition duration-150 ease-in"
          leave-from-class="opacity-100 scale-100"
          leave-to-class="opacity-0 scale-95"
        >
          <div
            v-if="open"
            class="relative flex h-[min(760px,85vh)] w-full max-w-3xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl"
          >
            <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <MessageSquare class="h-4 w-4" />
                </div>
                <h2 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h2>
              </div>
              <button
                type="button"
                class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                @click="emit('close')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>

            <!-- loading/error/config 三种状态共用同一个固定高度外壳（flex-1 撑满 header 之外的
                 剩余空间，父容器已是固定高度），切换状态或选项数量变化都不会让弹窗整体跳高跳低。 -->
            <div class="min-h-0 flex-1 overflow-hidden">
              <div v-if="isLoading" class="flex h-full items-center justify-center">
                <Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
              </div>

              <div v-else-if="errorKey && !config" class="h-full overflow-y-auto px-5 py-5">
                <div class="rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-sm text-red-600 dark:text-red-400">
                  {{ t(errorKey) }}
                </div>
              </div>

              <div v-else-if="config" class="flex h-full flex-col overflow-hidden sm:flex-row">
              <!-- 左侧分组导航：桌面端纵向排列固定宽度侧栏；移动端退化为顶部横向按钮行，避免溢出屏幕。 -->
              <nav class="flex shrink-0 gap-1 overflow-x-auto border-b border-border/40 px-3 py-2 sm:w-40 sm:flex-col sm:overflow-x-visible sm:border-b-0 sm:border-r sm:px-2 sm:py-3">
                <button
                  v-for="section in sections"
                  :key="section"
                  type="button"
                  class="shrink-0 rounded-lg px-3 py-2 text-left text-xs font-medium transition-colors sm:w-full"
                  :class="activeSection === section
                    ? 'bg-primary/10 text-primary'
                    : 'text-muted-foreground hover:bg-surface-elevated hover:text-foreground'"
                  @click="activeSection = section"
                >
                  {{ t(`${prefix}.sections.${section}`) }}
                </button>
              </nav>

              <div class="flex-1 space-y-5 overflow-y-auto px-5 py-5">
                <template v-if="activeSection === 'basic'">
                  <p class="rounded-lg border border-border/40 bg-surface/30 p-3 text-xs text-muted-foreground">
                    {{ t(`${prefix}.legacyNotice`) }}
                  </p>

                  <div>
                    <label class="mb-1 block text-xs font-medium text-muted-foreground">{{ t(`${prefix}.embedUrl`) }}</label>
                    <div class="flex items-center gap-2">
                      <input
                        :value="buildFrontendEmbedUrl(config.embedToken)"
                        type="text"
                        readonly
                        class="h-10 flex-1 truncate rounded-lg border border-border/50 bg-surface px-3 text-sm text-foreground outline-none"
                      />
                      <button
                        type="button"
                        class="inline-flex h-10 shrink-0 items-center gap-1.5 rounded-lg border border-border/50 px-3 text-sm text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                        @click="copyEmbedUrl"
                      >
                        <Check v-if="isCopied" class="h-3.5 w-3.5 text-emerald-500" />
                        <Copy v-else class="h-3.5 w-3.5" />
                        {{ isCopied ? t(`${prefix}.copied`) : t(`${prefix}.copy`) }}
                      </button>
                    </div>
                    <p v-if="copyErrorKey" class="mt-1 text-xs text-red-600 dark:text-red-400">{{ t(copyErrorKey) }}</p>
                    <p class="mt-1 text-xs text-muted-foreground">{{ t(`${prefix}.embedUrlHint`) }}</p>
                  </div>

                  <button
                    type="button"
                    class="inline-flex h-9 w-full items-center justify-center gap-1.5 rounded-lg border border-border/50 px-3 text-sm font-medium text-foreground transition-colors hover:bg-surface-elevated"
                    @click="openPreview"
                  >
                    <ExternalLink class="h-3.5 w-3.5" />
                    {{ t(`${prefix}.openPreview`) }}
                  </button>
                  <p class="-mt-3 text-xs text-muted-foreground">{{ t(`${prefix}.openPreviewHint`) }}</p>

                  <div>
                    <label class="mb-1 block text-xs font-medium text-muted-foreground">{{ t(`${prefix}.template`) }}</label>
                    <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
                      <button
                        v-for="option in templateOptions"
                        :key="option"
                        type="button"
                        class="rounded-lg border px-3 py-2.5 text-left text-xs transition-colors"
                        :class="templateDraft === option
                          ? 'border-primary bg-primary/10 text-foreground'
                          : 'border-border/50 text-muted-foreground hover:bg-surface-elevated hover:text-foreground'"
                        @click="templateDraft = option"
                      >
                        <p class="font-medium">{{ t(`${prefix}.templates.${option}.name`) }}</p>
                        <p class="mt-0.5 text-muted-foreground">{{ t(`${prefix}.templates.${option}.description`) }}</p>
                      </button>
                    </div>
                  </div>

                  <div>
                    <label class="mb-1 block text-xs font-medium text-muted-foreground">{{ t(`${prefix}.maxImages`) }}</label>
                    <div class="flex items-center gap-2">
                      <button
                        type="button"
                        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border/50 text-foreground transition-colors hover:bg-surface-elevated disabled:opacity-50"
                        :disabled="maxImagesDraft <= MIN_MAX_IMAGES"
                        @click="decrementMaxImages"
                      >
                        −
                      </button>
                      <input
                        :value="maxImagesDraft"
                        type="number"
                        :min="MIN_MAX_IMAGES"
                        :max="MAX_MAX_IMAGES"
                        readonly
                        class="h-9 w-16 rounded-lg border border-border/50 bg-surface text-center text-sm text-foreground outline-none"
                      />
                      <button
                        type="button"
                        class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border/50 text-foreground transition-colors hover:bg-surface-elevated disabled:opacity-50"
                        :disabled="maxImagesDraft >= MAX_MAX_IMAGES"
                        @click="incrementMaxImages"
                      >
                        +
                      </button>
                    </div>
                    <p class="mt-1 text-xs text-muted-foreground">{{ t(`${prefix}.maxImagesHint`) }}</p>
                  </div>
                </template>

                <template v-else-if="activeSection === 'category'">
                  <div>
                    <div class="mb-2 flex items-center justify-between">
                      <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.categoryOptions`) }}</label>
                      <button
                        type="button"
                        class="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
                        @click="restoreCategoryDefaults"
                      >
                        <RotateCcw class="h-3 w-3" />
                        {{ t(`${prefix}.restoreDefaults`) }}
                      </button>
                    </div>
                    <div class="space-y-2">
                      <div v-for="(option, index) in categoryDraft" :key="index" class="flex items-center gap-2">
                        <input
                          v-model="categoryDraft[index]"
                          type="text"
                          class="h-9 flex-1 rounded-lg border border-border/50 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                        />
                        <button
                          type="button"
                          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border/50 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-40"
                          :disabled="categoryDraft.length <= 1"
                          :aria-label="t(`${prefix}.removeOption`)"
                          @click="removeCategoryOption(index)"
                        >
                          <Trash2 class="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                    <div class="mt-2 flex items-center gap-2">
                      <input
                        v-model="newCategoryOption"
                        type="text"
                        :placeholder="t(`${prefix}.addOptionPlaceholder`)"
                        class="h-9 flex-1 rounded-lg border border-dashed border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                        @keyup.enter="addCategoryOption"
                      />
                      <button
                        type="button"
                        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-border/50 px-3 text-xs text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                        @click="addCategoryOption"
                      >
                        <Plus class="h-3.5 w-3.5" />
                        {{ t(`${prefix}.addOption`) }}
                      </button>
                    </div>
                    <p class="mt-2 text-xs text-muted-foreground">{{ t(`${prefix}.optionsHint`) }}</p>
                  </div>
                </template>

                <template v-else-if="activeSection === 'priority'">
                  <div>
                    <div class="mb-2 flex items-center justify-between">
                      <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.priorityOptions`) }}</label>
                      <button
                        type="button"
                        class="inline-flex items-center gap-1 text-xs text-muted-foreground transition-colors hover:text-foreground"
                        @click="restorePriorityDefaults"
                      >
                        <RotateCcw class="h-3 w-3" />
                        {{ t(`${prefix}.restoreDefaults`) }}
                      </button>
                    </div>
                    <div class="space-y-2">
                      <div v-for="(option, index) in priorityDraft" :key="index" class="flex items-center gap-2">
                        <input
                          v-model="priorityDraft[index]"
                          type="text"
                          class="h-9 flex-1 rounded-lg border border-border/50 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                        />
                        <button
                          type="button"
                          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border border-border/50 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-red-500 disabled:cursor-not-allowed disabled:opacity-40"
                          :disabled="priorityDraft.length <= 1"
                          :aria-label="t(`${prefix}.removeOption`)"
                          @click="removePriorityOption(index)"
                        >
                          <Trash2 class="h-3.5 w-3.5" />
                        </button>
                      </div>
                    </div>
                    <div class="mt-2 flex items-center gap-2">
                      <input
                        v-model="newPriorityOption"
                        type="text"
                        :placeholder="t(`${prefix}.addOptionPlaceholder`)"
                        class="h-9 flex-1 rounded-lg border border-dashed border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                        @keyup.enter="addPriorityOption"
                      />
                      <button
                        type="button"
                        class="inline-flex h-9 shrink-0 items-center gap-1.5 rounded-lg border border-border/50 px-3 text-xs text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                        @click="addPriorityOption"
                      >
                        <Plus class="h-3.5 w-3.5" />
                        {{ t(`${prefix}.addOption`) }}
                      </button>
                    </div>
                    <p class="mt-2 text-xs text-muted-foreground">{{ t(`${prefix}.optionsHint`) }}</p>
                  </div>
                </template>

                <div
                  v-if="saveSuccessKey"
                  role="status"
                  aria-live="polite"
                  class="flex items-start gap-2 rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-3 text-xs text-emerald-600 dark:text-emerald-400"
                >
                  <Check class="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span>{{ t(saveSuccessKey) }}</span>
                </div>
                <div
                  v-if="saveErrorKey"
                  role="alert"
                  class="flex items-start gap-2 rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-xs text-red-600 dark:text-red-400"
                >
                  <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span>{{ t(saveErrorKey) }}</span>
                </div>
                <div
                  v-if="rotateErrorKey"
                  role="alert"
                  class="flex items-start gap-2 rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-xs text-red-600 dark:text-red-400"
                >
                  <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span>{{ t(rotateErrorKey) }}</span>
                </div>

                <div class="flex flex-wrap items-center justify-between gap-2 border-t border-border/40 pt-4">
                  <button
                    type="button"
                    class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-red-500/30 px-3 text-sm font-medium text-red-600 transition-colors hover:bg-red-500/10 disabled:opacity-50 dark:text-red-400"
                    :disabled="isRotating"
                    @click="rotateToken"
                  >
                    <Loader2 v-if="isRotating" class="h-3.5 w-3.5 animate-spin" />
                    <RefreshCw v-else class="h-3.5 w-3.5" />
                    {{ t(`${prefix}.rotateToken`) }}
                  </button>
                  <button
                    type="button"
                    class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
                    :disabled="isSaving"
                    @click="save"
                  >
                    <Loader2 v-if="isSaving" class="h-3.5 w-3.5 animate-spin" />
                    {{ isSaving ? t(`${prefix}.saving`) : t(`${prefix}.saveTemplate`) }}
                  </button>
                </div>
              </div>
            </div>
          </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
