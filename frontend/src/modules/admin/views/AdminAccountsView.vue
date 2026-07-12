<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useDark, useToggle } from '@vueuse/core'
import { Globe, User, Clock, Check, Loader2, Plus, Moon, Sun, LogOut, Trash2, AlertTriangle } from 'lucide-vue-next'
import { useAdminAccounts } from '../composables/useAdminAccounts'
import { clearAccessToken } from '@/modules/auth/api/auth'
import { loginDashboardAdmin } from '../api/dashboardAdmin'
import { markWorkspaceActive } from '@/lib/workspaceGuard'
import AdminLoginModal from '../components/dashboard/AdminLoginModal.vue'
import type { DashboardAdminLoginForm } from '../types/dashboardAdmin'
import { WORKSPACE_DELETE_CONFIRMATION, type AdminAccount } from '../types/adminAccounts'
import logoUrl from '@/assets/logo.png'

const { t, locale } = useI18n()
const router = useRouter()

const isDark = useDark({
  selector: 'html',
  attribute: 'class',
  valueDark: 'dark',
  valueLight: '',
})
const toggleDark = useToggle(isDark)
const toggleLocale = () => {
  locale.value = locale.value === 'zh-CN' ? 'en-US' : 'zh-CN'
}

const {
  accounts,
  isLoading,
  isSwitching,
  isDeleting,
  errorKey,
  noticeKey,
  loadAccounts,
  switchAccount,
  deleteAccount,
} = useAdminAccounts()

const showAddModal = ref(false)
const addSubmitting = ref(false)
const addErrorKey = ref<string | null>(null)
const deleteTarget = ref<AdminAccount | null>(null)
const deleteConfirmation = ref('')
const deleteTypedManually = ref(false)
const deleteErrorKey = ref<string | null>(null)

const deleteTitleId = 'workspace-delete-title'
const deleteDescriptionId = 'workspace-delete-description'

const canDeleteWorkspace = computed(() => (
  !isDeleting.value
  && deleteTypedManually.value
  && deleteConfirmation.value === WORKSPACE_DELETE_CONFIRMATION
))

onMounted(() => {
  void loadAccounts()
})

const formatTime = (value: string | null): string => {
  if (!value) return '-'
  return new Date(value).toLocaleString()
}

const handleLogout = () => {
  clearAccessToken()
  router.push('/login')
}

const openAddModal = () => {
  addErrorKey.value = null
  showAddModal.value = true
}

const closeAddModal = () => {
  showAddModal.value = false
  addErrorKey.value = null
}

const resetDeleteConfirmation = () => {
  deleteConfirmation.value = ''
  deleteTypedManually.value = false
}

const openDeleteModal = (account: AdminAccount) => {
  if (isSwitching.value || isDeleting.value) return
  deleteTarget.value = account
  deleteErrorKey.value = null
  resetDeleteConfirmation()
}

const closeDeleteModal = () => {
  if (isDeleting.value) return
  deleteTarget.value = null
  deleteErrorKey.value = null
  resetDeleteConfirmation()
}

const resetBlockedConfirmation = (event: Event) => {
  event.preventDefault()
  resetDeleteConfirmation()
}

const syncInputSelection = async (input: HTMLInputElement, cursorPosition: number) => {
  await nextTick()
  input.setSelectionRange(cursorPosition, cursorPosition)
}

const updateManualConfirmation = (input: HTMLInputElement, nextValue: string, cursorPosition: number) => {
  deleteConfirmation.value = nextValue
  deleteTypedManually.value = nextValue.length > 0
  void syncInputSelection(input, cursorPosition)
}

const replaceSelection = (input: HTMLInputElement, replacement: string) => {
  const start = input.selectionStart ?? input.value.length
  const end = input.selectionEnd ?? input.value.length
  const nextValue = `${deleteConfirmation.value.slice(0, start)}${replacement}${deleteConfirmation.value.slice(end)}`
  updateManualConfirmation(input, nextValue, start + replacement.length)
}

const handleDeleteConfirmationKeydown = (event: KeyboardEvent) => {
  if (isDeleting.value || !event.isTrusted) {
    event.preventDefault()
    return
  }

  const input = event.currentTarget as HTMLInputElement

  if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
    event.preventDefault()
    replaceSelection(input, event.key)
    return
  }

  if (event.key === 'Backspace') {
    event.preventDefault()
    const start = input.selectionStart ?? deleteConfirmation.value.length
    const end = input.selectionEnd ?? deleteConfirmation.value.length
    if (start !== end) {
      replaceSelection(input, '')
      return
    }
    if (start > 0) {
      const nextValue = `${deleteConfirmation.value.slice(0, start - 1)}${deleteConfirmation.value.slice(end)}`
      updateManualConfirmation(input, nextValue, start - 1)
    }
    return
  }

  if (event.key === 'Delete') {
    event.preventDefault()
    const start = input.selectionStart ?? deleteConfirmation.value.length
    const end = input.selectionEnd ?? deleteConfirmation.value.length
    if (start !== end) {
      replaceSelection(input, '')
      return
    }
    if (start < deleteConfirmation.value.length) {
      const nextValue = `${deleteConfirmation.value.slice(0, start)}${deleteConfirmation.value.slice(start + 1)}`
      updateManualConfirmation(input, nextValue, start)
    }
  }
}

const handleDeleteConfirmationInput = (event: Event) => {
  const input = event.currentTarget as HTMLInputElement
  if (input.value !== deleteConfirmation.value) {
    resetDeleteConfirmation()
    input.value = ''
  }
}

const handleDeleteConfirmationBeforeInput = (event: InputEvent) => {
  if (event.inputType === 'insertFromPaste' || event.inputType === 'insertFromDrop') {
    resetBlockedConfirmation(event)
  }
}

const handleDeleteDialogEscape = () => {
  closeDeleteModal()
}

const submitDeleteWorkspace = async () => {
  if (!deleteTarget.value || !canDeleteWorkspace.value) return
  if (deleteConfirmation.value !== WORKSPACE_DELETE_CONFIRMATION) return

  const confirmation = deleteConfirmation.value
  deleteErrorKey.value = null
  try {
    await deleteAccount(deleteTarget.value.id, confirmation)
    closeDeleteModal()
  } catch (err) {
    deleteErrorKey.value = err instanceof Error ? err.message : 'admin.adminAccounts.errors.deleteFailed'
    resetDeleteConfirmation()
  }
}

const handleAddSubmit = async (form: DashboardAdminLoginForm) => {
  if (addSubmitting.value) return
  addSubmitting.value = true
  addErrorKey.value = null
  try {
    await loginDashboardAdmin(form)
    showAddModal.value = false
    markWorkspaceActive()
    await router.push('/admin')
  } catch (err) {
    addErrorKey.value = err instanceof Error ? err.message : 'admin.dashboard.adminAuth.errors.unknown'
  } finally {
    addSubmitting.value = false
  }
}
</script>

<template>
  <div class="min-h-screen flex flex-col">
    <!-- 轻量头部：logo + 工具按钮 -->
    <header class="h-16 shrink-0 flex items-center justify-between px-6 border-b border-border/40">
      <div class="flex items-center gap-2">
        <img :src="logoUrl" :alt="t('brand.logoAlt')" class="h-8 w-8 shrink-0 object-contain" />
        <span class="text-xl font-bold tracking-tight text-foreground">{{ t('brand.name') }}</span>
      </div>

      <div class="flex items-center gap-2">
        <button @click="toggleLocale" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" :title="t('admin.layout.toggleLanguage')">
          <Globe class="h-4 w-4" />
        </button>
        <button @click="toggleDark()" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" :title="t('admin.layout.toggleTheme')">
          <Moon v-if="!isDark" class="h-4 w-4" />
          <Sun v-else class="h-4 w-4" />
        </button>
        <button @click="handleLogout" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-red-400 transition-colors" :title="t('admin.menu.signOut')">
          <LogOut class="h-4 w-4" />
        </button>
      </div>
    </header>

    <!-- 内容区域 -->
    <div class="flex-1 flex items-start justify-center pt-12 pb-12 px-6">
      <div class="w-full max-w-3xl">
        <div class="mb-10 text-center">
          <h2 class="text-2xl font-bold text-foreground">{{ t('admin.adminAccounts.title') }}</h2>
          <p class="mt-2 text-sm text-muted-foreground">{{ t('admin.adminAccounts.subtitle') }}</p>
        </div>

        <div v-if="errorKey" class="mb-6 rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-sm text-destructive">
          {{ t(errorKey) }}
        </div>

        <div v-if="noticeKey" class="mb-6 rounded-lg border border-warning/40 bg-warning/10 p-4 text-sm text-warning">
          {{ t(noticeKey) }}
        </div>

        <div v-if="isLoading" class="flex items-center justify-center py-20">
          <Loader2 class="h-8 w-8 animate-spin text-muted-foreground" />
        </div>

        <div v-else class="grid gap-4 sm:grid-cols-2">
          <!-- 工作区卡片 -->
          <div
            v-for="account in accounts"
            :key="account.id"
            class="group relative overflow-hidden rounded-xl border transition-all hover:shadow-lg"
            :class="[
              account.current
                ? 'border-primary bg-primary/5 shadow-md shadow-primary/10'
                : 'border-border/60 bg-surface-elevated hover:border-primary/40'
            ]"
          >
            <div v-if="account.current" class="absolute top-3 right-14 z-10 flex h-6 w-6 items-center justify-center rounded-full bg-primary text-primary-foreground">
              <Check class="h-3.5 w-3.5" />
            </div>

            <button
              type="button"
              class="absolute right-3 top-3 z-10 flex h-8 w-8 items-center justify-center rounded-full border border-destructive/30 bg-destructive/10 text-destructive transition-colors hover:bg-destructive hover:text-destructive-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-destructive disabled:pointer-events-none disabled:opacity-50"
              :aria-label="t('admin.adminAccounts.delete.actionLabel', { name: account.displayName })"
              :title="t('admin.adminAccounts.delete.actionLabel', { name: account.displayName })"
              :disabled="isSwitching || isDeleting"
              @click.stop="openDeleteModal(account)"
            >
              <Trash2 class="h-4 w-4" />
            </button>

            <button
              type="button"
              :disabled="isSwitching || isDeleting"
              @click="switchAccount(account.id)"
              class="flex min-h-[160px] w-full flex-col gap-3 p-5 pr-24 text-left transition-all disabled:opacity-50"
            >
              <div class="flex items-center gap-3">
                <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 border border-primary/20">
                  <Globe class="h-5 w-5 text-primary" />
                </div>
                <div class="min-w-0">
                  <div class="font-semibold text-foreground truncate">{{ account.displayName }}</div>
                  <div class="text-xs text-muted-foreground truncate">{{ account.platform }}</div>
                </div>
              </div>

              <div class="space-y-1.5 text-xs text-muted-foreground">
                <div class="flex items-center gap-1.5 truncate">
                  <Globe class="h-3.5 w-3.5 shrink-0" />
                  <span class="truncate">{{ account.baseUrl }}</span>
                </div>
                <div class="flex items-center gap-1.5 truncate">
                  <User class="h-3.5 w-3.5 shrink-0" />
                  <span class="truncate">{{ account.identity }}</span>
                </div>
                <div class="flex items-center gap-1.5">
                  <Clock class="h-3.5 w-3.5 shrink-0" />
                  <span>{{ formatTime(account.lastUsedAt) }}</span>
                </div>
              </div>

              <div
                v-if="account.current"
                class="mt-1 text-xs font-medium text-primary"
              >
                {{ t('admin.adminAccounts.currentLabel') }}
              </div>
            </button>
          </div>

          <!-- 添加工作区卡片 -->
          <button
            @click="openAddModal"
            class="flex flex-col items-center justify-center gap-3 rounded-xl border-2 border-dashed border-border/60 p-5 text-center transition-all hover:border-primary/40 hover:bg-primary/5 hover:shadow-lg min-h-[160px]"
          >
            <div class="flex h-10 w-10 items-center justify-center rounded-full bg-muted/50 text-muted-foreground">
              <Plus class="h-5 w-5" />
            </div>
            <div>
              <div class="text-sm font-medium text-foreground">{{ t('admin.adminAccounts.addWorkspace') }}</div>
              <div class="mt-1 text-xs text-muted-foreground">{{ t('admin.adminAccounts.addWorkspaceHint') }}</div>
            </div>
          </button>
        </div>

        <!-- 空状态提示 -->
        <div v-if="!isLoading && accounts.length === 0" class="text-center mt-4 text-sm text-muted-foreground">
          {{ t('admin.adminAccounts.empty') }}
        </div>
      </div>
    </div>

    <!-- 复用 AdminLoginModal 创建新工作区 -->
    <AdminLoginModal
      :open="showAddModal"
      :submitting="addSubmitting"
      :error-key="addErrorKey"
      @submit="handleAddSubmit"
      @close="closeAddModal"
    />

    <Teleport defer to="body">
      <div
        v-if="deleteTarget"
        class="fixed inset-0 z-[100] flex items-center justify-center p-4"
        @keydown.esc.prevent.stop="handleDeleteDialogEscape"
      >
        <div class="absolute inset-0 bg-background/80 backdrop-blur-sm" @click="closeDeleteModal" />

        <form
          role="alertdialog"
          aria-modal="true"
          :aria-labelledby="deleteTitleId"
          :aria-describedby="deleteDescriptionId"
          class="relative w-full max-w-lg overflow-hidden rounded-2xl border border-border/70 bg-card text-card-foreground shadow-2xl animate-in fade-in zoom-in-95 duration-200"
          @submit.prevent="submitDeleteWorkspace"
        >
          <div class="absolute inset-x-0 top-0 h-1 bg-gradient-to-r from-destructive via-warning to-destructive" />

          <div class="p-6">
            <div class="flex items-start gap-4">
              <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-destructive/30 bg-destructive/10 text-destructive">
                <Trash2 class="h-5 w-5" />
              </div>
              <div class="min-w-0 flex-1">
                <h3 :id="deleteTitleId" class="text-lg font-semibold text-foreground">
                  {{ t('admin.adminAccounts.delete.title', { name: deleteTarget.displayName }) }}
                </h3>
                <div :id="deleteDescriptionId" class="mt-2 space-y-2 text-sm leading-6 text-muted-foreground">
                  <p>{{ t('admin.adminAccounts.delete.localDataWarning') }}</p>
                  <p>{{ t('admin.adminAccounts.delete.remoteResourcesRetained') }}</p>
                  <p>{{ t('admin.adminAccounts.delete.phraseInstruction', { phrase: WORKSPACE_DELETE_CONFIRMATION }) }}</p>
                </div>
              </div>
            </div>

            <div class="mt-5 rounded-xl border border-border/70 bg-muted/40 px-4 py-3 font-mono text-sm font-semibold tracking-wide text-foreground select-none">
              {{ WORKSPACE_DELETE_CONFIRMATION }}
            </div>

            <label class="mt-5 block text-sm font-medium text-foreground" for="workspace-delete-confirmation">
              {{ t('admin.adminAccounts.delete.inputLabel') }}
            </label>
            <input
              id="workspace-delete-confirmation"
              :value="deleteConfirmation"
              type="text"
              class="mt-2 h-11 w-full rounded-xl border border-border/70 bg-surface px-4 text-sm text-foreground outline-none transition placeholder:text-muted-foreground focus:border-destructive focus:ring-2 focus:ring-destructive/20 disabled:opacity-50"
              :placeholder="t('admin.adminAccounts.delete.inputPlaceholder')"
              autocomplete="off"
              autocorrect="off"
              autocapitalize="off"
              :spellcheck="false"
              :disabled="isDeleting"
              @keydown="handleDeleteConfirmationKeydown"
              @beforeinput="handleDeleteConfirmationBeforeInput"
              @input="handleDeleteConfirmationInput"
              @paste="resetBlockedConfirmation"
              @drop="resetBlockedConfirmation"
              @dragover.prevent
              @contextmenu.prevent
            />

            <div v-if="deleteErrorKey" class="mt-5 flex items-start gap-2 rounded-xl border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" />
              <span>{{ t(deleteErrorKey) }}</span>
            </div>

            <div class="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
              <button
                type="button"
                class="inline-flex h-10 items-center justify-center rounded-xl bg-secondary px-4 py-2 text-sm font-semibold text-secondary-foreground transition-colors hover:bg-surface-line focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:pointer-events-none disabled:opacity-50"
                :disabled="isDeleting"
                @click="closeDeleteModal"
              >
                {{ t('admin.adminAccounts.delete.cancel') }}
              </button>
              <button
                type="submit"
                class="inline-flex h-10 items-center justify-center gap-2 rounded-xl bg-destructive px-4 py-2 text-sm font-semibold text-destructive-foreground transition-colors hover:bg-destructive/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-destructive disabled:pointer-events-none disabled:opacity-50"
                :disabled="!canDeleteWorkspace"
              >
                <Loader2 v-if="isDeleting" class="h-4 w-4 animate-spin" />
                {{ t(isDeleting ? 'admin.adminAccounts.delete.deleting' : 'admin.adminAccounts.delete.confirm') }}
              </button>
            </div>
          </div>
        </form>
      </div>
    </Teleport>
  </div>
</template>
