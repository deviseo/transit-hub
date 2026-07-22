import { ref } from 'vue'
import { authUnauthorizedErrorKey } from '@/modules/auth/api/auth'
import {
  getDashboardAdminStatus,
  loginDashboardAdmin,
  refreshDashboardAdminSession,
} from '../api/dashboardAdmin'
import type { DashboardAdminLoginForm, DashboardAdminStatus } from '../types/dashboardAdmin'

/**
 * 管理仪表盘 admin 账户的登录门禁状态：
 * 进入仪表盘时检查是否已登录 admin；未登录则打开登录弹窗；并提供登录/退出能力。
 */
export function useDashboardAdmin() {
  const status = ref<DashboardAdminStatus>({ authenticated: false })
  const isChecking = ref(false)
  const isSubmitting = ref(false)
  const isRefreshingCredentials = ref(false)
  const isModalOpen = ref(false)
  const errorKey = ref<string | null>(null)

  // TransitHub 登录态失效时请求层已统一跳转登录页（见 auth.ts 的 handleAuthExpired），
  // 这里只需要识别该错误并提前退出，不再重复实现跳转。
  const handleUnauthorized = (message: string): boolean => message === authUnauthorizedErrorKey

  // 进入仪表盘后检查 admin 登录状态：已登录则不弹窗，未登录则弹窗让登录。
  const checkStatus = async (options: { preserveAuthenticatedOnError?: boolean } = {}) => {
    if (isChecking.value) return
    isChecking.value = true
    errorKey.value = null
    try {
      const next = await getDashboardAdminStatus()
      status.value = next
      isModalOpen.value = !next.authenticated
    } catch (error) {
      const message = error instanceof Error ? error.message : 'admin.dashboard.adminAuth.errors.unknown'
      if (handleUnauthorized(message)) return
      // 仪表盘已有内存快照时，临时网络故障不应立刻清空页面并误导用户重新登录。
      // 后续实时指标请求仍会执行完整 admin 校验；若后端明确返回未登录，上面的正常
      // 响应分支仍会把 authenticated 更新为 false，不会绕过权限门禁。
      if (options.preserveAuthenticatedOnError && status.value.authenticated) {
        errorKey.value = message
        return
      }
      // 状态接口失败时也打开弹窗，让用户可以尝试登录。
      status.value = { authenticated: false }
      isModalOpen.value = true
      errorKey.value = message
    } finally {
      isChecking.value = false
    }
  }

  const submitLogin = async (form: DashboardAdminLoginForm): Promise<boolean> => {
    if (isSubmitting.value) return false
    isSubmitting.value = true
    errorKey.value = null
    try {
      const next = await loginDashboardAdmin(form)
      status.value = next
      isModalOpen.value = false
      return true
    } catch (error) {
      const message = error instanceof Error ? error.message : 'admin.dashboard.adminAuth.errors.unknown'
      if (handleUnauthorized(message)) return false
      errorKey.value = message
      return false
    } finally {
      isSubmitting.value = false
    }
  }

  // 主动更新管理员凭证：刷新当前 admin session 并重新校验 admin 身份。
  // 校验失败时保留已有的非敏感状态字段（platform/baseUrl/authMethod/identity），
  // 仅将 authenticated 置为 false 并弹出登录弹窗，供 AdminLoginModal 预填这些字段。
  const updateAdminCredentials = async (): Promise<boolean> => {
    if (isRefreshingCredentials.value) return false
    isRefreshingCredentials.value = true
    errorKey.value = null
    try {
      const next = await refreshDashboardAdminSession()
      status.value = next
      return true
    } catch (error) {
      const message = error instanceof Error ? error.message : 'admin.dashboard.adminAuth.errors.unknown'
      if (handleUnauthorized(message)) return false
      status.value = { ...status.value, authenticated: false }
      isModalOpen.value = true
      errorKey.value = 'admin.dashboard.adminAuth.errors.reloginRequired'
      return false
    } finally {
      isRefreshingCredentials.value = false
    }
  }

  const openModal = () => {
    isModalOpen.value = true
  }

  const closeModal = () => {
    isModalOpen.value = false
  }

  return {
    status,
    isChecking,
    isSubmitting,
    isRefreshingCredentials,
    isModalOpen,
    errorKey,
    checkStatus,
    submitLogin,
    updateAdminCredentials,
    openModal,
    closeModal,
  }
}
