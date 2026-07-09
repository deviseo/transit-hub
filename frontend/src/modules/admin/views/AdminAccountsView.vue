<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useDark, useToggle } from '@vueuse/core'
import { Globe, User, Clock, Check, Loader2, Plus, Moon, Sun, LogOut } from 'lucide-vue-next'
import { useAdminAccounts } from '../composables/useAdminAccounts'
import { clearAccessToken } from '@/modules/auth/api/auth'
import { loginDashboardAdmin } from '../api/dashboardAdmin'
import { markWorkspaceActive } from '@/lib/workspaceGuard'
import AdminLoginModal from '../components/dashboard/AdminLoginModal.vue'
import type { DashboardAdminLoginForm } from '../types/dashboardAdmin'
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
  errorKey,
  loadAccounts,
  switchAccount,
} = useAdminAccounts()

const showAddModal = ref(false)
const addSubmitting = ref(false)
const addErrorKey = ref<string | null>(null)

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

        <div v-if="isLoading" class="flex items-center justify-center py-20">
          <Loader2 class="h-8 w-8 animate-spin text-muted-foreground" />
        </div>

        <div v-else class="grid gap-4 sm:grid-cols-2">
          <!-- 工作区卡片 -->
          <button
            v-for="account in accounts"
            :key="account.id"
            :disabled="isSwitching"
            @click="switchAccount(account.id)"
            class="group relative flex flex-col gap-3 rounded-xl border p-5 text-left transition-all hover:shadow-lg disabled:opacity-50"
            :class="[
              account.current
                ? 'border-primary bg-primary/5 shadow-md shadow-primary/10'
                : 'border-border/60 bg-surface-elevated hover:border-primary/40'
            ]"
          >
            <div v-if="account.current" class="absolute top-3 right-3 flex h-6 w-6 items-center justify-center rounded-full bg-primary text-primary-foreground">
              <Check class="h-3.5 w-3.5" />
            </div>

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
  </div>
</template>
