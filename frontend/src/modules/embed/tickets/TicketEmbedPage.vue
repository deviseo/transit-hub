<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2 } from 'lucide-vue-next'
import { createEmbedSession, listEmbedTickets, setEmbedSessionToken } from './api/tickets'
import EmbedTicketDetail from './components/EmbedTicketDetail.vue'
import EmbedTicketList from './components/EmbedTicketList.vue'
import type { EmbedTicketListItem, TicketEmbedTemplate } from './types'

const route = useRoute()
const { t, locale } = useI18n()

type PageState = 'loading' | 'error' | 'ready'
const pageState = ref<PageState>('loading')
const initErrorKey = ref<string | null>(null)

// 模板只影响视觉表现（背景/卡片密度/对话感），不改变任何接口调用、身份校验或数据隔离逻辑。
// 会话建立前统一使用 default，避免闪烁成其它模板再切回来。
const template = ref<TicketEmbedTemplate>('default')

// 每次工单最多上传图片数，由当前 workspace 的嵌入配置决定，会话建立后从响应中取得；
// 0 表示当前工作区未开启图片上传。
const maxImages = ref(0)

// 分类/优先级选项，由当前 workspace 的嵌入配置决定，会话建立后从响应中取得。创建工单弹窗的
// 选项必须始终来自这里（当前 workspace 的实时配置），不能由前端写死默认数组作为最终来源。
const categoryOptions = ref<string[]>([])
const priorityOptions = ref<string[]>([])

const view = ref<'list' | 'detail'>('list')
const selectedTicketId = ref<string | null>(null)

const tickets = ref<EmbedTicketListItem[]>([])
const isListLoading = ref(false)
const listErrorKey = ref<string | null>(null)

// vue-router 的 query 值类型是 string | null | (string | null)[]，这里统一收敛成 string，
// 缺失/非字符串一律当作空字符串处理。
const queryString = (value: unknown): string => {
  if (Array.isArray(value)) {
    const first = value[0]
    return typeof first === 'string' ? first : ''
  }
  return typeof value === 'string' ? value : ''
}

const applyTheme = (theme: string) => {
  if (theme === 'dark') {
    document.documentElement.classList.add('dark')
  } else if (theme === 'light') {
    document.documentElement.classList.remove('dark')
  }
}

const applyLocale = (lang: string) => {
  locale.value = lang === 'zh' ? 'zh-CN' : 'en-US'
}

// 会话建立后从 URL 中移除 Sub2API token，避免用户刷新页面或分享地址时把 token 泄露出去；
// 其余非敏感参数（embed_token/user_id/theme/lang/ui_mode/src_host/src_url）保留，便于排查问题。
const stripTokenFromUrl = () => {
  const params = new URLSearchParams(window.location.search)
  params.delete('token')
  const query = params.toString()
  const nextUrl = query ? `${window.location.pathname}?${query}` : window.location.pathname
  window.history.replaceState(window.history.state, '', nextUrl)
}

const loadList = async () => {
  isListLoading.value = true
  listErrorKey.value = null
  try {
    const response = await listEmbedTickets()
    tickets.value = response.items
  } catch (error) {
    listErrorKey.value = error instanceof Error ? error.message : 'embed.tickets.errors.unknown'
  } finally {
    isListLoading.value = false
  }
}

const openDetail = (id: string) => {
  selectedTicketId.value = id
  view.value = 'detail'
}

const backToList = () => {
  view.value = 'list'
  selectedTicketId.value = null
  void loadList()
}

const handleCreated = (id: string) => {
  openDetail(id)
  void loadList()
}

onMounted(async () => {
  applyTheme(queryString(route.query.theme))
  applyLocale(queryString(route.query.lang))

  const embedToken = queryString(route.query.embed_token)
  const sub2apiToken = queryString(route.query.token)
  const urlUserId = queryString(route.query.user_id)
  const srcHost = queryString(route.query.src_host)
  const srcUrl = queryString(route.query.src_url)

  if (!embedToken || !sub2apiToken || !srcHost) {
    pageState.value = 'error'
    initErrorKey.value = 'embed.tickets.errors.missingParams'
    return
  }

  try {
    const session = await createEmbedSession({ embedToken, sub2apiToken, urlUserId, srcHost, srcUrl })
    setEmbedSessionToken(session.sessionToken)
    template.value = session.template
    maxImages.value = session.maxImagesPerTicket
    categoryOptions.value = session.categoryOptions ?? []
    priorityOptions.value = session.priorityOptions ?? []
    stripTokenFromUrl()
    pageState.value = 'ready'
    await loadList()
  } catch (error) {
    pageState.value = 'error'
    initErrorKey.value = error instanceof Error ? error.message : 'embed.tickets.errors.unknown'
  }
})

// default：保持紧凑、圆角卡片和标准背景；minimal：背景更平、去掉外层大容器的边框密度，
// 更适合嵌入已有后台风格；support：更像客服/帮助台面板，用略深的背景衬托对话卡片。
const rootBackgroundClass = computed(() => {
  switch (template.value) {
    case 'minimal':
      return 'bg-background'
    case 'support':
      return 'bg-surface-elevated/50'
    default:
      return 'bg-background'
  }
})

// 页面必须铺满 Sub2API iframe 的可用宽度，不能用 max-w-* + mx-auto 收缩成中间窄列——
// iframe 本身的宽度已经由 Sub2API 侧配置决定，这里再叠加一层最大宽度限制只会在宽 iframe
// 里留出大片空白。三种模板统一用 w-full，仅保留响应式 padding 控制左右留白。
const containerWidthClass = 'w-full'

const containerPaddingClass = computed(() => {
  if (template.value === 'minimal') return 'px-3 py-4 sm:px-4'
  if (template.value === 'default') return 'px-4 py-6 sm:px-6 md:px-8 md:py-10'
  return 'px-4 py-6 sm:px-6'
})

// default 用一整块卡片承载全部内容（列表/表单/详情共享同一个外层容器），减少多层小卡片
// 叠在一起的拥挤感；minimal/support 不套这层外壳，视觉保持原样。会话建立前 template 恒为
// 'default'，因此 loading/error 状态也会获得同样舒展的卡片感。
const pageShellClass = computed(() => (
  template.value === 'default'
    ? 'rounded-2xl border border-border/60 bg-card p-6 shadow-sm sm:p-8'
    : ''
))
</script>

<template>
  <div :class="['min-h-screen text-foreground', rootBackgroundClass]">
    <div :class="[containerWidthClass, containerPaddingClass]">
      <div :class="pageShellClass">
        <div v-if="pageState === 'loading'" class="flex flex-col items-center justify-center gap-3 py-16 text-muted-foreground">
          <Loader2 class="h-6 w-6 animate-spin" />
          <span class="text-sm">{{ t('embed.tickets.page.loading') }}</span>
        </div>

        <div v-else-if="pageState === 'error'" class="flex flex-col items-center justify-center gap-3 rounded-xl border border-warning/20 bg-warning/10 px-6 py-10 text-center">
          <AlertCircle class="h-6 w-6 text-warning" />
          <p class="text-sm text-warning">{{ initErrorKey ? t(initErrorKey) : t('embed.tickets.errors.unknown') }}</p>
        </div>

        <template v-else>
          <EmbedTicketList
            v-if="view === 'list'"
            :tickets="tickets"
            :is-loading="isListLoading"
            :error-key="listErrorKey"
            :template="template"
            :max-images="maxImages"
            :category-options="categoryOptions"
            :priority-options="priorityOptions"
            @select="openDetail"
            @refresh="loadList"
            @created="handleCreated"
          />
          <EmbedTicketDetail
            v-else-if="selectedTicketId"
            :ticket-id="selectedTicketId"
            :template="template"
            @back="backToList"
            @updated="loadList"
          />
        </template>
      </div>
    </div>
  </div>
</template>
