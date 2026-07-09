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
    path: '/embed/tickets',
    name: 'EmbedTickets',
    component: () => import('./modules/embed/tickets/TicketEmbedPage.vue')
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
        path: 'group-associations',
        name: 'AdminGroupAssociations',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/GroupAssociationsView.vue')
      },
      {
        path: 'connection-health',
        name: 'AdminConnectionHealth',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/ConnectionHealthView.vue')
      },
      {
        path: 'group-rate-campaigns',
        name: 'AdminGroupRateCampaigns',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/GroupRateCampaignsView.vue')
      },
      {
        path: 'settings',
        name: 'AdminSettings',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/SettingsView.vue')
      },
      {
        path: 'tickets',
        name: 'AdminTickets',
        meta: { requiresWorkspace: true },
        component: () => import('./modules/admin/views/TicketsView.vue')
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
