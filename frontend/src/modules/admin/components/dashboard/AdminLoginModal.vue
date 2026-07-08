<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, ShieldCheck, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type {
  DashboardAdminLoginForm,
  DashboardAdminPlatform,
  Sub2apiAuthMethod,
} from '../../types/dashboardAdmin'

const props = defineProps<{
  open: boolean
  submitting: boolean
  errorKey: string | null
  /** 打开弹窗时用于预填的非敏感已知信息（不包含 password/accessToken/refreshToken/tokenType）。 */
  initialValue?: Partial<DashboardAdminLoginForm>
}>()

const emit = defineEmits<{
  (event: 'submit', form: DashboardAdminLoginForm): void
  (event: 'close'): void
}>()

const { t } = useI18n()

const platform = ref<DashboardAdminPlatform>('sub2api')
const authMethod = ref<Sub2apiAuthMethod>('password')
const authMethods: Sub2apiAuthMethod[] = ['password', 'token']

const siteUrl = ref('')
const email = ref('')
const password = ref('')
const accessToken = ref('')
const refreshToken = ref('')
const tokenType = ref('')

// 弹窗打开瞬间用 initialValue 预填非敏感字段，只填充一次，避免覆盖用户正在输入的内容；
// 同时必须清空敏感字段，因为组件常驻挂载，关闭弹窗不会销毁内部 ref，上一次输入的
// password/accessToken/refreshToken/tokenType 会残留在内存里。
const applyInitialValue = () => {
  platform.value = props.initialValue?.platform ?? 'sub2api'
  siteUrl.value = props.initialValue?.siteUrl ?? ''
  authMethod.value = props.initialValue?.authMethod ?? 'password'
  email.value = props.initialValue?.email ?? ''

  password.value = ''
  accessToken.value = ''
  refreshToken.value = ''
  tokenType.value = ''
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    applyInitialValue()
  },
)

// new-api 只支持密码登录，不显示 token 登录方式
const showAuthMethodToggle = computed(() => platform.value === 'sub2api')

const canSubmit = computed(() => {
  if (!siteUrl.value.trim()) return false
  if (platform.value === 'newapi') {
    return Boolean(email.value.trim() && password.value.trim())
  }
  switch (authMethod.value) {
    case 'password':
      return Boolean(email.value.trim() && password.value.trim())
    case 'token':
      return Boolean(refreshToken.value.trim())
    default:
      return false
  }
})

const selectPlatform = (next: DashboardAdminPlatform) => {
  platform.value = next
  if (next === 'newapi') {
    authMethod.value = 'password'
  }
}

const handleSubmit = () => {
  if (!canSubmit.value || props.submitting) return
  emit('submit', {
    platform: platform.value,
    siteUrl: siteUrl.value.trim(),
    authMethod: authMethod.value,
    email: email.value.trim(),
    password: password.value,
    accessToken: accessToken.value.trim(),
    refreshToken: refreshToken.value.trim(),
    tokenType: tokenType.value.trim(),
  })
}
</script>

<template>
  <Teleport defer to="body">
    <div v-if="open" class="fixed inset-0 z-[100] flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-background/80 backdrop-blur-sm" @click="emit('close')"></div>

      <div
        role="dialog"
        aria-modal="true"
        class="relative w-full max-w-lg overflow-hidden rounded-[2rem] border border-border/60 bg-card text-card-foreground shadow-2xl shadow-primary/10 animate-in fade-in zoom-in-95 duration-200"
      >
        <div class="absolute left-0 right-0 top-0 h-1 bg-gradient-to-r from-primary via-accent to-primary" />

        <!-- Header -->
        <div class="flex items-start justify-between gap-4 px-6 pt-6">
          <div class="flex items-center gap-3">
            <div class="flex h-11 w-11 items-center justify-center rounded-full bg-primary/10 text-primary">
              <ShieldCheck class="h-5 w-5" />
            </div>
            <div>
              <h2 class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.adminAuth.modal.title') }}</h2>
              <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.adminAuth.modal.subtitle') }}</p>
            </div>
          </div>
          <button
            type="button"
            class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
            :title="t('admin.dashboard.adminAuth.modal.close')"
            @click="emit('close')"
          >
            <X class="h-5 w-5" />
          </button>
        </div>

        <form class="space-y-5 px-6 py-6" @submit.prevent="handleSubmit">
          <!-- 平台选择 -->
          <div class="space-y-2">
            <label class="text-sm font-medium text-foreground">{{ t('admin.dashboard.adminAuth.modal.platformLabel') }}</label>
            <div class="grid grid-cols-2 gap-3">
              <button
                type="button"
                class="rounded-xl border px-4 py-3 text-left transition-colors"
                :class="platform === 'sub2api'
                  ? 'border-primary bg-primary/5 text-foreground'
                  : 'border-border/60 text-muted-foreground hover:border-border hover:text-foreground'"
                @click="selectPlatform('sub2api')"
              >
                <span class="block text-sm font-semibold">{{ t('admin.dashboard.adminAuth.modal.platform.sub2api') }}</span>
              </button>
              <button
                type="button"
                class="rounded-xl border px-4 py-3 text-left transition-colors"
                :class="platform === 'newapi'
                  ? 'border-primary bg-primary/5 text-foreground'
                  : 'border-border/60 text-muted-foreground hover:border-border hover:text-foreground'"
                @click="selectPlatform('newapi')"
              >
                <span class="block text-sm font-semibold">{{ t('admin.dashboard.adminAuth.modal.platform.newapi') }}</span>
              </button>
            </div>
          </div>

          <!-- 站点域名 -->
          <div class="space-y-2">
            <label class="text-sm font-medium text-foreground">{{ t('admin.dashboard.adminAuth.modal.siteUrlLabel') }}</label>
            <Input v-model="siteUrl" :placeholder="t('admin.dashboard.adminAuth.modal.siteUrlPlaceholder')" autocomplete="url" />
          </div>

          <!-- 登录方式（仅 sub2api 显示切换） -->
          <div v-if="showAuthMethodToggle" class="space-y-2">
            <label class="text-sm font-medium text-foreground">{{ t('admin.dashboard.adminAuth.modal.methodLabel') }}</label>
            <div class="inline-flex w-full items-center rounded-xl border border-border/60 bg-surface/40 p-1">
              <button
                v-for="method in authMethods"
                :key="method"
                type="button"
                class="flex-1 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors"
                :class="authMethod === method
                  ? 'bg-primary text-primary-foreground shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'"
                @click="authMethod = method"
              >
                {{ t(`admin.dashboard.adminAuth.modal.method.${method}`) }}
              </button>
            </div>
          </div>

          <!-- 邮箱/账号 + 密码 -->
          <div v-if="platform === 'newapi' || authMethod === 'password'" class="space-y-3">
            <Input
              v-model="email"
              :type="platform === 'newapi' ? 'text' : 'email'"
              :placeholder="platform === 'newapi' ? t('admin.dashboard.adminAuth.modal.fields.usernamePlaceholder') : t('admin.dashboard.adminAuth.modal.fields.emailPlaceholder')"
              autocomplete="username"
            />
            <Input v-model="password" type="password" :placeholder="t('admin.dashboard.adminAuth.modal.fields.passwordPlaceholder')" autocomplete="current-password" />
          </div>

          <!-- RT + AT（仅 sub2api token 模式） -->
          <div v-else class="space-y-3">
            <Input v-model="accessToken" :placeholder="t('admin.dashboard.adminAuth.modal.fields.accessTokenPlaceholder')" autocomplete="off" />
            <Input v-model="refreshToken" :placeholder="t('admin.dashboard.adminAuth.modal.fields.refreshTokenPlaceholder')" autocomplete="off" />
            <Input v-model="tokenType" :placeholder="t('admin.dashboard.adminAuth.modal.fields.tokenTypePlaceholder')" autocomplete="off" />
            <p class="text-xs text-muted-foreground">{{ t('admin.dashboard.adminAuth.modal.fields.tokenHelp') }}</p>
          </div>

          <!-- 错误提示 -->
          <div v-if="errorKey" class="flex items-start gap-2 rounded-xl border border-warning/20 bg-warning/10 p-3 text-sm text-warning">
            <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
            <span>{{ t(errorKey) }}</span>
          </div>

          <Button type="submit" class="w-full" :disabled="!canSubmit || submitting">
            <Loader2 v-if="submitting" class="h-4 w-4 animate-spin" />
            <ShieldCheck v-else class="h-4 w-4" />
            {{ submitting ? t('admin.dashboard.adminAuth.modal.submitting') : t('admin.dashboard.adminAuth.modal.submit') }}
          </Button>
        </form>
      </div>
    </div>
  </Teleport>
</template>
