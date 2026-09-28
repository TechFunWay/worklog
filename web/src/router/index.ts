import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/login',
      name: 'Login',
      component: () => import('../views/LoginView.vue'),
      meta: { requiresAuth: false }
    },
    {
      path: '/register',
      name: 'Register',
      component: () => import('../views/RegisterView.vue'),
      meta: { requiresAuth: false }
    },
    {
      path: '/forgot-password',
      name: 'ForgotPassword',
      component: () => import('../views/ForgotPasswordView.vue'),
      meta: { requiresAuth: false }
    },
    {
      // 后台路由统一前缀为 /admin，需要登录。
      path: '/admin',
      component: () => import('../layouts/MainLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        { path: '', name: 'Home', component: () => import('../views/HomeView.vue') },
        { path: 'profile', name: 'Profile', component: () => import('../views/ProfileView.vue') },
        { path: 'settings', name: 'Settings', component: () => import('../views/SettingsView.vue') },
        { path: 'records', name: 'Records', component: () => import('../views/RecordsView.vue') },
        { path: 'workers', name: 'Workers', component: () => import('../views/WorkersView.vue') },
        { path: 'projects', name: 'Projects', component: () => import('../views/ProjectsView.vue') },
        { path: 'piece-items', name: 'PieceItems', component: () => import('../views/PieceItemsView.vue') },
        { path: 'advances', name: 'Advances', component: () => import('../views/AdvancesView.vue') },
        { path: 'settlements', name: 'Settlements', component: () => import('../views/SettlementsView.vue') },
        { path: 'settlements/:id', name: 'SettlementDetail', component: () => import('../views/SettlementDetailView.vue') },
        { path: 'stats', name: 'Stats', component: () => import('../views/StatsView.vue') },
        { path: 'team', name: 'Team', component: () => import('../views/TeamView.vue') },
        { path: 'attendance', name: 'Attendance', component: () => import('../views/AttendanceView.vue') },
        { path: 'data', name: 'DataBackup', component: () => import('../views/DataBackupView.vue') },
        { path: 'users', name: 'AdminUsers', component: () => import('../views/AdminUsersView.vue'), meta: { requiresAdmin: true } },
        { path: 'configs', name: 'AdminConfigs', component: () => import('../views/AdminConfigView.vue'), meta: { requiresAdmin: true } },
        { path: 'audit', name: 'AdminAudit', component: () => import('../views/AdminAuditView.vue'), meta: { requiresAdmin: true } },
      ]
    },
    {
      // 后期 / 用于免登录的门户或前端页面，当前先重定向到后台首页。
      path: '/',
      redirect: '/admin',
      meta: { requiresAuth: false }
    }
  ]
})

router.beforeEach(async (to, from, next) => {
  const authStore = useAuthStore()
  await authStore.init()

  if (authStore.setupRequired && to.name !== 'Register') {
    // 首次安装默认走普通的用户名密码创建管理员流程，不做任何默认的飞牛
    // 授权；只有用户主动点击「使用飞牛 NAS 登录」并确认后，才进入飞牛绑定
    // 创建模式（注册页内会带上 fnos=bind 查询参数）。
    next({ name: 'Register' })
    return
  }

  if (to.meta.requiresAuth !== false && !authStore.isAuthenticated && authStore.requireLogin) {
    next({ name: 'Login' })
  } else if (to.meta.requiresAdmin && !authStore.isAdmin) {
    next({ name: 'Home' })
  } else {
    next()
  }
})

export default router
