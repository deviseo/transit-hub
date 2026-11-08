import { createRouter, createWebHistory } from 'vue-router'
import { getAccessToken } from './modules/auth/api/auth'
import { getCurrentAdminAccount } from './modules/admin/api/adminAccounts'
import {
  resetWorkspaceCheck,
  markWorkspaceActive,
  isWorkspaceChecked,
  isWorkspaceActive,
  setWorkspaceChecked,
} from './lib/workspaceGuard'

const routes = [
  {
    path: '/',
    redirect: '/login'
  },
  {
    path: '/login',
    name: 'Login',
    component: () => import('./modules/auth/LoginPage.vue')
  },
  {
    path: '/register',
    redirect: '/login'
  },
  {
    path: '/admin',
    component: () => import('./modules/admin/layout/AdminLayout.vue'),
    meta: { requiresAuth: true },
    children: [
      {
        path: '',
        name: 'AdminDashboard',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/DashboardView.vue')
      },
      {
        path: 'upstream',
        name: 'AdminUpstream',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/UpstreamView.vue')
      },
      {
        path: 'group-rates',
        name: 'AdminGroupRates',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/GroupRatesView.vue')
      },
      {
        path: 'settings',
        name: 'AdminSettings',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/SettingsView.vue')
      },
      {
        path: 'user-activity',
        name: 'AdminUserActivity',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/UserActivityView.vue')
      },
      {
        path: 'sales',
        name: 'AdminSales',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/SalesView.vue')
      },
      {
        path: 'vip',
        name: 'AdminVip',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/VipView.vue')
      },
      {
        path: 'profit',
        name: 'AdminProfit',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/ProfitView.vue')
      },
      {
        path: 'client',
        name: 'AdminClient',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/ClientView.vue')
      },
      {
        path: 'tickets',
        name: 'AdminTickets',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/TicketsView.vue')
      },
      {
        path: 'mass-email',
        name: 'AdminMassEmail',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/MassEmailView.vue')
      },
      {
        path: 'leads',
        name: 'AdminLeads',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/LeadsView.vue')
      },
      {
        path: 'accounts',
        name: 'AdminAccounts',
        component: () => import('./modules/admin/views/AdminAccountsView.vue')
      }
    ]
  }
]

export const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach(async (to) => {
  if (to.matched.some((route) => route.meta.requiresAuth) && !getAccessToken()) {
    return { path: '/login' }
  }

  if (to.matched.some((route) => route.meta.requiresWorkspace)) {
    if (!isWorkspaceChecked()) {
      try {
        await getCurrentAdminAccount()
        setWorkspaceChecked(true, true)
      } catch {
        setWorkspaceChecked(true, false)
      }
    }
    if (!isWorkspaceActive()) {
      return { name: 'AdminAccounts' }
    }
  }

  return true
})

export { resetWorkspaceCheck, markWorkspaceActive }
