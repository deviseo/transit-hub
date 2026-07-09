<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, type Component } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { LayoutDashboard, Network, Settings, LogOut, Globe, Moon, Sun, Percent, Megaphone, ChevronDown, ArrowRightLeft, FolderTree, Link2, Activity, MessageSquare, Github } from 'lucide-vue-next'
import { useDark, useToggle } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useAdminAccounts } from '../composables/useAdminAccounts'
import { clearAccessToken } from '@/modules/auth/api/auth'
import { getSystemVersion } from '../api/system'
import type { SystemVersionResponse } from '../api/system'
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

const { currentAccount, loadCurrentAccount } = useAdminAccounts()

// 版本信息：开源版仅用于纯展示，不依赖授权/更新服务
const versionInfo = ref<SystemVersionResponse | null>(null)

const loadVersionInfo = async () => {
  try {
    versionInfo.value = await getSystemVersion()
  } catch {
    // 版本信息加载失败不阻塞页面
  }
}

// GitHub 仓库地址是本项目唯一来源，版本号链接和图标入口都从这里派生，避免散落硬编码。
const githubRepoUrl = 'https://github.com/deviseo/transit-hub'
const githubReleasesUrl = `${githubRepoUrl}/releases`

// 非正式发布的占位版本号（本地预览/开发/未设置 APP_VERSION 时的默认值）不对应真实 tag，
// 点击后退回 release 列表页，而不是跳到一个不存在的 tag 地址。
const nonReleaseVersionPlaceholders = ['latest', 'local-preview', 'dev', '0.0.0']

const releaseUrl = computed(() => {
  const version = versionInfo.value?.version.trim()
  if (!version) return githubReleasesUrl
  if (nonReleaseVersionPlaceholders.includes(version)) return githubReleasesUrl
  // 版本号可能已经带 v 前缀（如 v0.0.4）也可能没有（如 0.0.4），统一补齐成 v 前缀，
  // 避免拼出 vv0.0.4。
  const tag = version.startsWith('v') ? version : `v${version}`
  return `${githubReleasesUrl}/tag/${tag}`
})

// 工作区选择页不显示侧边栏和业务菜单
const isWorkspaceSelectionPage = computed(() => route.name === 'AdminAccounts')

const showUserMenu = ref(false)
const userMenuRef = ref<HTMLElement | null>(null)

const toggleUserMenu = () => {
  showUserMenu.value = !showUserMenu.value
}

const handleClickOutside = (e: MouseEvent) => {
  if (userMenuRef.value && !userMenuRef.value.contains(e.target as Node)) {
    showUserMenu.value = false
  }
}

onMounted(() => {
  void loadCurrentAccount()
  void loadVersionInfo()
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})

const goToAccounts = () => {
  showUserMenu.value = false
  router.push('/admin/accounts')
}

// 二级菜单项：带独立小图标，方便在展开态下快速区分。
interface MenuChild {
  name: string
  path: string
  icon: Component
}

// 菜单项分两种形态：叶子（单一路由入口）和分组（固定顺序的二级菜单集合）。
// “分组管理”下的三个二级菜单顺序固定：分组倍率 -> 分组关联 -> 分组健康，不随业务改动调整。
type MenuEntry =
  | { type: 'leaf'; name: string; path: string; icon: Component }
  | { type: 'group'; name: string; icon: Component; children: MenuChild[] }

const menuItems = computed<MenuEntry[]>(() => [
  { type: 'leaf', name: t('admin.menu.dashboard'), path: '/admin', icon: LayoutDashboard },
  { type: 'leaf', name: t('admin.menu.upstream'), path: '/admin/upstream', icon: Network },
  {
    type: 'group',
    name: t('admin.menu.groupManagement'),
    icon: FolderTree,
    children: [
      { name: t('admin.menu.groupRates'), path: '/admin/group-rates', icon: Percent },
      { name: t('admin.menu.groupAssociations'), path: '/admin/group-associations', icon: Link2 },
      { name: t('admin.menu.connectionHealth'), path: '/admin/connection-health', icon: Activity },
    ],
  },
  { type: 'leaf', name: t('admin.menu.groupRateCampaigns'), path: '/admin/group-rate-campaigns', icon: Megaphone },
  { type: 'leaf', name: t('admin.menu.tickets'), path: '/admin/tickets', icon: MessageSquare },
  { type: 'leaf', name: t('admin.menu.settings'), path: '/admin/settings', icon: Settings },
])

// 分组展开状态：未手动切换过时，按当前路由是否命中该分组的子项自动展开。
const expandedGroups = ref<Record<string, boolean>>({})

const isGroupActive = (group: Extract<MenuEntry, { type: 'group' }>) => group.children.some((child) => child.path === route.path)

const isGroupExpanded = (group: Extract<MenuEntry, { type: 'group' }>) => {
  const manual = expandedGroups.value[group.name]
  return manual === undefined ? isGroupActive(group) : manual
}

const toggleGroup = (group: Extract<MenuEntry, { type: 'group' }>) => {
  expandedGroups.value[group.name] = !isGroupExpanded(group)
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
  clearAccessToken()
  router.push('/login')
}
</script>

<template>
  <div class="h-screen flex overflow-hidden bg-background text-foreground">
    <!-- Sidebar: 工作区选择页不显示 -->
    <aside v-if="!isWorkspaceSelectionPage" class="w-64 border-r border-border/40 bg-surface-elevated flex flex-col">
      <div class="h-16 flex items-center px-6 border-b border-border/40">
        <div class="flex items-center gap-2">
          <img :src="logoUrl" :alt="t('brand.logoAlt')" class="h-8 w-8 shrink-0 object-contain" />
          <span class="text-xl font-bold tracking-tight text-foreground">{{ t('brand.name') }}</span>
        </div>
      </div>

      <nav class="flex-1 py-6 px-4 space-y-2 overflow-y-auto">
        <template v-for="item in menuItems" :key="item.type === 'leaf' ? item.path : item.name">
          <router-link
            v-if="item.type === 'leaf'"
            :to="item.path"
            class="flex items-center gap-3 px-3 py-2.5 rounded-xl transition-colors"
            :class="[
              route.path === item.path
                ? 'bg-primary text-primary-foreground font-medium shadow-md shadow-primary/20'
                : 'text-muted-foreground hover:bg-surface-line hover:text-foreground'
            ]"
          >
            <component :is="item.icon" class="w-5 h-5" />
            {{ item.name }}
          </router-link>

          <div v-else>
            <button
              type="button"
              class="flex w-full items-center gap-3 px-3 py-2.5 rounded-xl transition-colors"
              :class="[
                isGroupActive(item) && !isGroupExpanded(item)
                  ? 'bg-primary/10 text-primary font-medium'
                  : 'text-muted-foreground hover:bg-surface-line hover:text-foreground'
              ]"
              @click="toggleGroup(item)"
            >
              <component :is="item.icon" class="w-5 h-5" />
              <span class="flex-1 text-left">{{ item.name }}</span>
              <ChevronDown class="w-4 h-4 transition-transform" :class="{ 'rotate-180': isGroupExpanded(item) }" />
            </button>

            <div v-if="isGroupExpanded(item)" class="mt-1 ml-4 space-y-1 border-l border-border/40 pl-3">
              <router-link
                v-for="child in item.children"
                :key="child.path"
                :to="child.path"
                class="flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm transition-colors"
                :class="[
                  route.path === child.path
                    ? 'bg-primary text-primary-foreground font-medium shadow-md shadow-primary/20'
                    : 'text-muted-foreground hover:bg-surface-line hover:text-foreground'
                ]"
              >
                <component :is="child.icon" class="w-4 h-4" />
                {{ child.name }}
              </router-link>
            </div>
          </div>
        </template>
      </nav>

      <div class="p-4 border-t border-border/40">
        <button
          @click="handleLogout"
          class="flex items-center gap-3 px-3 py-2.5 w-full rounded-xl text-muted-foreground hover:bg-surface-line hover:text-red-400 transition-colors"
        >
          <LogOut class="w-5 h-5" />
          {{ t('admin.menu.signOut') }}
        </button>
      </div>
    </aside>

    <!-- Main Content -->
    <div class="flex-1 flex flex-col min-w-0">
      <!-- Header: 工作区选择页不显示业务导航头 -->
      <header v-if="!isWorkspaceSelectionPage" class="h-16 shrink-0 border-b border-border/40 bg-surface/50 backdrop-blur-md flex items-center justify-between px-6 sticky top-0 z-50">
        <h1 class="text-lg font-semibold">{{ pageTitle }}</h1>

        <div class="flex items-center gap-4">
          <div class="flex items-center gap-2">
            <!-- 版本号展示：点击跳转到对应 GitHub release（非正式发布占位版本号退回 releases 列表）。 -->
            <a
              v-if="versionInfo"
              :href="releaseUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="flex items-center gap-1 h-7 px-2 rounded-md text-xs font-medium text-muted-foreground hover:bg-surface-elevated hover:text-foreground transition-colors"
              :title="t('admin.system.openRelease')"
              :aria-label="t('admin.system.openRelease')"
            >
              v{{ versionInfo.version }}
            </a>

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

            <button @click="toggleLocale" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" :title="t('admin.layout.toggleLanguage')">
              <Globe class="h-4 w-4" />
            </button>
            <button @click="toggleDark()" class="flex h-9 w-9 items-center justify-center rounded-full hover:bg-surface-elevated text-muted-foreground hover:text-foreground transition-colors" :title="t('admin.layout.toggleTheme')">
              <Moon v-if="!isDark" class="h-4 w-4" />
              <Sun v-else class="h-4 w-4" />
            </button>
          </div>

          <div ref="userMenuRef" class="relative">
            <button
              @click="toggleUserMenu"
              class="flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-surface-elevated transition-colors"
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
      <main class="flex-1 overflow-auto" :class="isWorkspaceSelectionPage ? '' : 'p-6'">
        <router-view v-slot="{ Component }">
          <transition name="fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
    </div>
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
