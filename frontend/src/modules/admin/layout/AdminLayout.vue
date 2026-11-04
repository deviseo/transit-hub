<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, watch, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LayoutDashboard, Network, Settings, LogOut, Globe, Moon, Sun, Percent, ChevronDown, ArrowRightLeft, Github, Mail, Menu, X, UsersRound, BadgeDollarSign, TrendingUp, Loader2, MonitorSmartphone } from 'lucide-vue-next'
import { useDark, useToggle } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAdminAccounts } from '../composables/useAdminAccounts'
import { COLLECTION_PAGES, useCollectionWatcher } from '../composables/useCollectionWatcher'
import NoticeStack from '../components/collections/NoticeStack.vue'
import { clearAccessToken } from '@/modules/auth/api/auth'
import { checkSystemUpdate, getSystemVersion } from '../api/system'
import type { SystemUpdateCheck, SystemVersionResponse } from '../api/system'
import UpdateDialog from '../components/system/UpdateDialog.vue'
import logoUrl from '@/assets/logo.png'

const route = useRoute()
const router = useRouter()

const isDark = useDark({
  selector: 'html',
  attribute: 'class',
  valueDark: 'dark',
  valueLight: '',
})
const toggleDark = useToggle(isDark)

const { t, locale } = useI18n()
const toggleLocale = () => {
  locale.value = locale.value === 'zh-CN' ? 'en-US' : 'zh-CN'
}

const { currentAccount, noticeKey, loadCurrentAccount } = useAdminAccounts()

// 后台采集提醒：顶部显示「后台采集中」，采集完成或失败时在右下角提醒（见 useCollectionWatcher）。
const { running: runningCollections } = useCollectionWatcher(computed(() => currentAccount.value?.id ?? null))
const runningCollectionsLabel = computed(() => runningCollections.value
  .map((kind) => t(kind === 'profit' ? 'admin.collections.runningProfit' : 'admin.collections.runningUsers'))
  .join(t('admin.collections.separator')))

// 版本信息：开源版仅用于纯展示，不依赖授权/更新服务
const versionInfo = ref<SystemVersionResponse | null>(null)

const loadVersionInfo = async () => {
  try {
    versionInfo.value = await getSystemVersion()
  } catch {
    // 版本信息加载失败不阻塞页面
  }
  // 后台检查一次有没有新版本（服务端缓存 10 分钟），有的话在版本号旁边提示；失败不打扰。
  try {
    updateInfo.value = await checkSystemUpdate()
  } catch {
    // 检查更新失败不影响使用
  }
}

// 点版本号打开「检查更新」。
const updateInfo = ref<SystemUpdateCheck | null>(null)
const showUpdateDialog = ref(false)
const hasUpdate = computed(() => updateInfo.value?.hasUpdate ?? false)

// GitHub 仓库地址：右上角的图标入口。版本号点开是「检查更新」，里面有对应版本的发布说明链接。
const githubRepoUrl = 'https://github.com/deviseo/transit-hub'

const versionLabel = computed(() => {
  const version = versionInfo.value?.version.trim()
  if (!version) return ''
  const bareVersion = version.replace(/^v+/i, '')
  return bareVersion ? `v${bareVersion}` : ''
})

// 工作区选择页不显示侧边栏和业务菜单
const isWorkspaceSelectionPage = computed(() => route.name === 'AdminAccounts')

const showUserMenu = ref(false)
const userMenuRef = ref<HTMLElement | null>(null)
const isMobileSidebarOpen = ref(false)

const openMobileSidebar = () => {
  isMobileSidebarOpen.value = true
}

const closeMobileSidebar = () => {
  isMobileSidebarOpen.value = false
}

const toggleUserMenu = () => {
  showUserMenu.value = !showUserMenu.value
}

const handleClickOutside = (e: MouseEvent) => {
  if (userMenuRef.value && !userMenuRef.value.contains(e.target as Node)) {
    showUserMenu.value = false
  }
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key !== 'Escape') return
  closeMobileSidebar()
  showUserMenu.value = false
}

onMounted(() => {
  void loadCurrentAccount()
  void loadVersionInfo()
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleKeydown)
})

const goToAccounts = () => {
  showUserMenu.value = false
  closeMobileSidebar()
  router.push('/admin/accounts')
}

// 二级菜单项：带独立小图标，方便在展开态下快速区分。
interface MenuChild {
  name: string
  path: string
  icon: Component
}

// 菜单项分两种形态：叶子（单一路由入口）和分组（固定顺序的二级菜单集合）。
type MenuEntry =
  | { type: 'leaf'; name: string; path: string; icon: Component }
  | { type: 'group'; id: string; name: string; icon: Component; children: MenuChild[] }

const menuItems = computed<MenuEntry[]>(() => [
  { type: 'leaf', name: t('admin.menu.dashboard'), path: '/admin', icon: LayoutDashboard },
  { type: 'leaf', name: t('admin.menu.upstream'), path: '/admin/upstream', icon: Network },
  { type: 'leaf', name: t('admin.menu.groupRates'), path: '/admin/group-rates', icon: Percent },
  { type: 'leaf', name: t('admin.menu.profit'), path: '/admin/profit', icon: TrendingUp },
  { type: 'leaf', name: t('admin.menu.userActivity'), path: '/admin/user-activity', icon: UsersRound },
  { type: 'leaf', name: t('admin.menu.sales'), path: '/admin/sales', icon: BadgeDollarSign },
  { type: 'leaf', name: t('admin.menu.massEmail'), path: '/admin/mass-email', icon: Mail },
  { type: 'leaf', name: t('admin.menu.client'), path: '/admin/client', icon: MonitorSmartphone },
  { type: 'leaf', name: t('admin.menu.settings'), path: '/admin/settings', icon: Settings },
])

// 分组展开状态：未手动切换过时，按当前路由是否命中该分组的子项自动展开。
const expandedGroups = ref<Record<string, boolean>>({})

const isGroupActive = (group: Extract<MenuEntry, { type: 'group' }>) => group.children.some((child) => child.path === route.path)

const isGroupExpanded = (group: Extract<MenuEntry, { type: 'group' }>) => {
  const manual = expandedGroups.value[group.id]
  return manual === undefined ? isGroupActive(group) : manual
}

const toggleGroup = (group: Extract<MenuEntry, { type: 'group' }>) => {
  expandedGroups.value[group.id] = !isGroupExpanded(group)
}

const handleMenuRouteClick = () => {
  closeMobileSidebar()
}

// 摊平查找当前路由对应的菜单文案，供顶部标题使用（叶子和分组子项都要能查到）。
const findMenuLabel = (path: string): string | undefined => {
  for (const item of menuItems.value) {
    if (item.type === 'leaf' && item.path === path) return item.name
    if (item.type === 'group') {
      const child = item.children.find((c) => c.path === path)
      if (child) return child.name
    }
  }
  return undefined
}

const pageTitle = computed(() => (route.path === '/admin' ? t('admin.menu.dashboard') : findMenuLabel(route.path) ?? ''))

const handleLogout = () => {
  showUserMenu.value = false
  closeMobileSidebar()
  clearAccessToken()
  router.push('/login')
}

watch(
  () => route.fullPath,
  () => {
    closeMobileSidebar()
  },
)
</script>

<template>
  <div class="flex h-dvh overflow-hidden overflow-x-hidden bg-background text-foreground">
    <a
      href="#admin-main-content"
      class="sr-only fixed left-3 top-3 z-[70] rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground focus:not-sr-only focus:outline-none focus:ring-2 focus:ring-primary focus:ring-offset-2 focus:ring-offset-background"
    >
      {{ t('admin.layout.skipToContent') }}
    </a>
    <button
      v-if="!isWorkspaceSelectionPage && isMobileSidebarOpen"
      type="button"
      class="fixed inset-0 z-40 bg-background/70 backdrop-blur-sm lg:hidden"
      :aria-label="t('admin.layout.closeNavigation')"
      @click="closeMobileSidebar"
    />

    <!-- Sidebar: 工作区选择页不显示 -->
    <aside
      v-if="!isWorkspaceSelectionPage"
      id="admin-mobile-sidebar"
      class="fixed inset-y-0 left-0 z-50 flex w-64 shrink-0 flex-col border-r border-border/40 bg-surface-elevated transition-transform duration-200 lg:static lg:z-auto lg:translate-x-0 lg:transition-none"
      :class="isMobileSidebarOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <div class="h-16 flex items-center justify-between gap-3 px-4 lg:px-6 border-b border-border/40">
        <div class="flex items-center gap-2">
          <img :src="logoUrl" :alt="t('brand.logoAlt')" width="32" height="32" class="h-8 w-8 shrink-0 object-contain" />
          <span class="text-xl font-bold tracking-tight text-foreground">{{ t('brand.name') }}</span>
        </div>
        <button
          type="button"
          class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary lg:hidden"
          :aria-label="t('admin.layout.closeNavigation')"
          @click="closeMobileSidebar"
        >
          <X class="h-4 w-4" />
        </button>
      </div>

      <nav class="flex-1 py-6 px-4 space-y-2 overflow-y-auto">
        <template v-for="item in menuItems" :key="item.type === 'leaf' ? item.path : item.id">
          <router-link
            v-if="item.type === 'leaf'"
            :to="item.path"
            class="flex items-center gap-3 rounded-lg px-3 py-2.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :class="[
              route.path === item.path
                ? 'bg-primary text-primary-foreground font-medium shadow-md shadow-primary/20'
                : 'text-muted-foreground hover:bg-surface-line hover:text-foreground'
            ]"
            @click="handleMenuRouteClick"
          >
            <component :is="item.icon" class="w-5 h-5" aria-hidden="true" />
            {{ item.name }}
          </router-link>

          <div v-else>
            <button
              type="button"
              class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :aria-expanded="isGroupExpanded(item)"
              :aria-controls="`admin-menu-${item.id}`"
              :class="[
                isGroupActive(item) && !isGroupExpanded(item)
                  ? 'bg-primary/10 text-primary font-medium'
                  : 'text-muted-foreground hover:bg-surface-line hover:text-foreground'
              ]"
              @click="toggleGroup(item)"
            >
              <component :is="item.icon" class="w-5 h-5" aria-hidden="true" />
              <span class="flex-1 text-left">{{ item.name }}</span>
              <ChevronDown class="w-4 h-4 transition-transform" :class="{ 'rotate-180': isGroupExpanded(item) }" aria-hidden="true" />
            </button>

            <div v-if="isGroupExpanded(item)" :id="`admin-menu-${item.id}`" class="mt-1 ml-4 space-y-1 border-l border-border/40 pl-3">
              <router-link
                v-for="child in item.children"
                :key="child.path"
                :to="child.path"
                class="flex items-center gap-2.5 rounded-lg px-3 py-2 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                :class="[
                  route.path === child.path
                    ? 'bg-primary text-primary-foreground font-medium shadow-md shadow-primary/20'
                    : 'text-muted-foreground hover:bg-surface-line hover:text-foreground'
                ]"
                @click="handleMenuRouteClick"
              >
                <component :is="child.icon" class="w-4 h-4" aria-hidden="true" />
                {{ child.name }}
              </router-link>
            </div>
          </div>
        </template>
      </nav>

      <div class="p-4 border-t border-border/40">
        <button
          @click="handleLogout"
          class="flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-muted-foreground transition-colors hover:bg-surface-line hover:text-red-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          <LogOut class="w-5 h-5" />
          {{ t('admin.menu.signOut') }}
        </button>
      </div>
    </aside>

    <!-- Main Content -->
    <div class="flex-1 flex flex-col min-w-0 w-full">
      <!-- Header: 工作区选择页不显示业务导航头 -->
      <header v-if="!isWorkspaceSelectionPage" class="h-16 shrink-0 border-b border-border/40 bg-surface/50 backdrop-blur-md flex items-center justify-between gap-2 px-3 sm:px-6 sticky top-0 z-30">
        <div class="flex min-w-0 items-center gap-2">
          <button
            type="button"
            class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary lg:hidden"
            :aria-label="t('admin.layout.openNavigation')"
            :aria-expanded="isMobileSidebarOpen"
            aria-controls="admin-mobile-sidebar"
            @click="openMobileSidebar"
          >
            <Menu class="h-4 w-4" />
          </button>
          <h1 class="min-w-0 truncate text-base font-semibold sm:text-lg">{{ pageTitle }}</h1>
        </div>

        <div class="flex min-w-0 shrink-0 items-center gap-1 sm:gap-4">
          <div class="flex items-center gap-1 sm:gap-2">
            <router-link
              v-if="runningCollections.length > 0"
              :to="COLLECTION_PAGES[runningCollections[0]]"
              class="flex h-7 items-center gap-1.5 rounded-md bg-primary/10 px-2 text-xs font-medium text-primary transition-colors hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :title="runningCollectionsLabel"
              :aria-label="runningCollectionsLabel"
            >
              <Loader2 class="h-3.5 w-3.5 animate-spin" aria-hidden="true" />
              <span class="hidden sm:inline">{{ t('admin.collections.running') }}</span>
            </router-link>
            <!-- 版本号：点击检查更新；有新版本时显示提示点。 -->
            <button
              v-if="versionInfo"
              type="button"
              class="hidden sm:flex items-center gap-1.5 h-7 px-2 rounded-md text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="hasUpdate ? 'bg-primary/10 text-primary hover:bg-primary/15' : 'text-muted-foreground hover:bg-surface-elevated hover:text-foreground'"
              :title="hasUpdate ? t('admin.update.newVersion', { version: updateInfo?.latest }) : t('admin.update.title')"
              :aria-label="hasUpdate ? t('admin.update.newVersion', { version: updateInfo?.latest }) : t('admin.update.title')"
              @click="showUpdateDialog = true"
            >
              <span v-if="hasUpdate" class="h-1.5 w-1.5 rounded-full bg-primary" aria-hidden="true"></span>
              {{ versionLabel }}
              <span v-if="hasUpdate" class="hidden lg:inline">· {{ t('admin.update.badge') }}</span>
            </button>

            <a
              :href="githubRepoUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors"
              :title="t('admin.system.openGithubRepository')"
              :aria-label="t('admin.system.openGithubRepository')"
            >
              <Github class="h-4 w-4" />
            </a>

            <button @click="toggleLocale" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" :title="t('admin.layout.toggleLanguage')" :aria-label="t('admin.layout.toggleLanguage')">
              <Globe class="h-4 w-4" />
            </button>
            <button @click="toggleDark()" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" :title="t('admin.layout.toggleTheme')" :aria-label="t('admin.layout.toggleTheme')">
              <Moon v-if="!isDark" class="h-4 w-4" />
              <Sun v-else class="h-4 w-4" />
            </button>
          </div>

          <div ref="userMenuRef" class="relative">
            <button
              @click="toggleUserMenu"
              class="flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-surface-elevated transition-colors"
              :aria-label="t('admin.layout.userProfile')"
              :aria-expanded="showUserMenu"
            >
              <div class="w-8 h-8 rounded-full bg-gradient-to-tr from-primary to-accent shrink-0"></div>
              <span v-if="currentAccount" class="text-sm font-medium text-foreground max-w-[120px] truncate hidden sm:inline">{{ currentAccount.displayName }}</span>
              <ChevronDown class="h-3.5 w-3.5 text-muted-foreground" />
            </button>

            <transition name="dropdown">
              <div
                v-if="showUserMenu"
                class="absolute right-0 top-full mt-2 w-56 rounded-xl border border-border/60 bg-surface-elevated shadow-lg py-1 z-[60]"
              >
                <div v-if="currentAccount" class="px-3 py-2.5 border-b border-border/40">
                  <div class="text-sm font-medium text-foreground truncate">{{ currentAccount.displayName }}</div>
                  <div class="text-xs text-muted-foreground truncate">{{ currentAccount.platform }} · {{ currentAccount.identity }}</div>
                </div>
                <button
                  @click="goToAccounts"
                  class="flex w-full items-center gap-2.5 px-3 py-2.5 text-sm text-muted-foreground hover:bg-surface-line hover:text-foreground transition-colors"
                >
                  <ArrowRightLeft class="h-4 w-4" />
                  {{ t('admin.layout.switchWorkspace') }}
                </button>
                <button
                  @click="handleLogout"
                  class="flex w-full items-center gap-2.5 px-3 py-2.5 text-sm text-muted-foreground hover:bg-surface-line hover:text-red-400 transition-colors"
                >
                  <LogOut class="h-4 w-4" />
                  {{ t('admin.menu.signOut') }}
                </button>
              </div>
            </transition>
          </div>
        </div>
      </header>

      <!-- Content Area -->
      <main id="admin-main-content" class="flex-1 overflow-auto" :class="isWorkspaceSelectionPage ? '' : 'p-3 sm:p-6'" tabindex="-1">
        <div v-if="!isWorkspaceSelectionPage && noticeKey" class="mb-4 rounded-lg border border-warning/40 bg-warning/10 p-4 text-sm text-warning">
          {{ t(noticeKey) }}
        </div>
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
    </div>
    <NoticeStack />
    <UpdateDialog v-if="showUpdateDialog" @close="showUpdateDialog = false" @checked="(result) => (updateInfo = result)" />
  </div>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(10px);
}

.dropdown-enter-active,
.dropdown-leave-active {
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.dropdown-enter-from,
.dropdown-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}
</style>
