<template>
  <div class="min-h-dvh flex">
    <!-- Ambient theme-aware background -->
    <div class="app-bg" aria-hidden="true">
      <div class="glow glow-1"></div>
      <div class="glow glow-2"></div>
      <div class="glow glow-3"></div>
      <div class="app-grid"></div>
    </div>

    <!-- Sidebar -->
    <aside
      :class="[
        'fixed inset-y-0 left-0 z-40 w-[min(19rem,85vw)] lg:w-72 transform transition-transform duration-300 ease-out lg:translate-x-0',
        sidebarOpen ? 'translate-x-0' : '-translate-x-full'
      ]"
    >
      <div class="h-full m-3 mr-0 lg:mr-3 rounded-2xl surface flex flex-col overflow-hidden">
        <!-- Brand -->
        <div class="flex items-center gap-3 h-20 px-6 border-b border-border">
          <div class="w-11 h-11 rounded-xl bg-brand-gradient flex items-center justify-center shadow-glow shrink-0">
            <span class="text-white font-display font-extrabold text-lg">{{ siteInitial }}</span>
          </div>
          <div class="min-w-0">
            <h1 class="text-base font-display font-bold text-foreground truncate">{{ authStore.siteTitle }}</h1>
            <p class="text-xs text-muted-foreground">管理控制台</p>
          </div>
        </div>

        <!-- Nav -->
        <nav class="flex-1 overflow-y-auto px-4 py-5 space-y-1">
          <template v-for="group in navGroups" :key="group.label">
            <div class="px-3 pt-5 pb-2 first:pt-0">
              <span class="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">{{ group.label }}</span>
            </div>
            <RouterLink
              v-for="item in group.items"
              :key="item.to"
              :to="item.to"
              class="nav-link group"
              :class="isActive(item.to) ? 'nav-link-active' : 'nav-link-idle'"
              @click="sidebarOpen = false"
            >
              <span
                class="nav-icon"
                :class="isActive(item.to) ? 'bg-white/20 text-white' : 'bg-muted text-muted-foreground group-hover:text-brand-600 dark:group-hover:text-brand-300'"
              >
                <span v-html="item.icon" class="w-5 h-5 block"></span>
              </span>
              {{ item.label }}
            </RouterLink>
          </template>

          <template v-if="authStore.isAdmin">
            <div class="px-3 pt-6 pb-2">
              <span class="text-[11px] font-bold text-muted-foreground uppercase tracking-widest">系统管理</span>
            </div>
            <RouterLink
              v-for="item in adminNav"
              :key="item.to"
              :to="item.to"
              class="nav-link group"
              :class="isActive(item.to) ? 'nav-link-active' : 'nav-link-idle'"
              @click="sidebarOpen = false"
            >
              <span
                class="nav-icon"
                :class="isActive(item.to) ? 'bg-white/20 text-white' : 'bg-muted text-muted-foreground group-hover:text-brand-600 dark:group-hover:text-brand-300'"
              >
                <span v-html="item.icon" class="w-5 h-5 block"></span>
              </span>
              {{ item.label }}
            </RouterLink>
          </template>
        </nav>

        <!-- User card -->
        <div class="p-4 border-t border-border">
          <div class="flex items-center gap-3 p-2.5 rounded-xl bg-muted">
            <div class="w-10 h-10 rounded-full bg-brand-gradient flex items-center justify-center text-white font-bold text-sm shrink-0">
              {{ userInitial }}
            </div>
            <div class="min-w-0 flex-1">
              <div class="text-sm font-semibold text-foreground truncate">{{ authStore.user?.username }}</div>
              <div class="text-xs text-muted-foreground">{{ authStore.isAdmin ? '管理员' : '普通用户' }}</div>
            </div>
            <button
              @click="handleLogout"
              title="退出登录"
              class="p-2 rounded-lg text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
            >
              <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1" /></svg>
            </button>
          </div>
        </div>

        <!-- Version -->
        <div class="px-4 pb-3 flex justify-center">
          <span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-[11px] font-medium bg-muted text-muted-foreground border border-border/50">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
            {{ versionInfo }}
          </span>
        </div>
      </div>
    </aside>

    <!-- Mobile overlay -->
    <transition enter-active-class="transition-opacity duration-200" enter-from-class="opacity-0" leave-active-class="transition-opacity duration-200" leave-to-class="opacity-0">
      <div v-if="sidebarOpen" class="fixed inset-0 z-30 bg-foreground/40 backdrop-blur-sm lg:hidden" @click="sidebarOpen = false"></div>
    </transition>

    <!-- Main column -->
    <div class="flex-1 flex flex-col min-h-dvh min-w-0 lg:pl-72">
      <header class="sticky top-0 z-20 px-3 sm:px-4 lg:px-8 pt-2 sm:pt-3">
        <div class="h-12 sm:h-16 flex items-center justify-between px-3 sm:px-6 rounded-2xl surface">
          <div class="flex items-center gap-2 sm:gap-3 min-w-0">
            <button class="lg:hidden p-2 -ml-1 rounded-lg text-foreground hover:bg-muted" @click="sidebarOpen = !sidebarOpen">
              <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" /></svg>
            </button>
            <div class="min-w-0 flex-1">
              <!-- Mobile title: the page name replaces the empty breadcrumb area -->
              <div class="sm:hidden text-base font-bold text-foreground truncate">{{ currentTitle }}</div>
              <!-- breadcrumb -->
              <div class="hidden sm:flex items-center gap-2 text-sm">
                <RouterLink to="/admin" class="text-muted-foreground hover:text-foreground transition-colors truncate">{{ authStore.siteTitle }}</RouterLink>
                <svg class="w-4 h-4 text-muted-foreground shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>
                <span class="text-foreground font-semibold truncate">{{ currentTitle }}</span>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-2 sm:gap-3">
            <UiThemeToggle />

            <!-- user dropdown -->
            <div class="relative" ref="menuRef">
              <button
                @click="menuOpen = !menuOpen"
                class="flex items-center gap-2.5 pl-1 pr-2 py-1 rounded-xl hover:bg-muted transition-colors"
              >
                <div class="w-9 h-9 rounded-full bg-brand-gradient flex items-center justify-center text-white font-bold text-sm">{{ userInitial }}</div>
                <span class="hidden sm:block text-sm font-medium text-foreground">{{ authStore.user?.username }}</span>
                <svg class="hidden sm:block w-4 h-4 text-muted-foreground transition-transform" :class="menuOpen ? 'rotate-180' : ''" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7"/></svg>
              </button>
              <transition enter-active-class="transition duration-150 ease-out" enter-from-class="opacity-0 scale-95 -translate-y-1" leave-active-class="transition duration-100" leave-to-class="opacity-0 scale-95 -translate-y-1">
                <div v-if="menuOpen" class="absolute right-0 mt-2 w-56 rounded-2xl surface shadow-card p-2 origin-top-right">
                  <div class="px-3 py-2.5 mb-1 border-b border-border">
                    <div class="text-sm font-semibold text-foreground truncate">{{ authStore.user?.username }}</div>
                    <div class="text-xs text-muted-foreground">{{ authStore.isAdmin ? '管理员' : '普通用户' }}</div>
                  </div>
                  <RouterLink to="/admin/profile" @click="menuOpen = false" class="menu-item">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
                    个人资料
                  </RouterLink>
                  <button @click="handleLogout" class="menu-item w-full text-destructive hover:bg-destructive/10">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 16l4-4m0 0l-4-4m4 4H7m6 4v1a3 3 0 01-3 3H6a3 3 0 01-3-3V7a3 3 0 013-3h4a3 3 0 013 3v1"/></svg>
                    退出登录
                  </button>
                </div>
              </transition>
            </div>
          </div>
        </div>
      </header>

      <main class="flex-1 min-w-0 p-3 sm:p-6 xl:p-8 overflow-auto pb-[calc(72px+env(safe-area-inset-bottom))] lg:pb-8">
        <!-- 赞赏横幅：班组长（老板）与系统管理员可见，工人登录不显示；
             关闭不持久化，刷新后再次出现；支持过当前版本后隐藏，
             应用升级到新版本后再次出现 -->
        <div v-if="supportStore.bannerVisible" class="mb-3 sm:mb-4 flex items-center justify-between gap-2 rounded-xl border border-amber-400/40 bg-amber-400/10 px-3 py-2 text-xs sm:rounded-2xl sm:px-4 sm:py-3 sm:text-sm">
          <span class="min-w-0 truncate text-foreground">
            ❤ <button class="font-semibold text-brand-600 hover:underline dark:text-brand-300" @click="supportStore.open()">请作者喝杯咖啡</button><span class="hidden sm:inline"> 如果这个应用对你有帮助，欢迎（不赞赏不影响任何功能）。</span>
          </span>
          <button class="shrink-0 px-1 text-lg leading-none text-muted-foreground hover:text-foreground" title="关闭" aria-label="关闭赞赏横幅" @click="supportStore.dismissBanner()">×</button>
        </div>

        <RouterView v-slot="{ Component }">
          <transition mode="out-in" enter-active-class="transition-all duration-300 ease-out" enter-from-class="opacity-0 translate-y-2" leave-active-class="transition-all duration-150" leave-to-class="opacity-0">
            <component :is="Component" />
          </transition>
        </RouterView>
      </main>
    </div>


    <!-- Mobile bottom nav -->
    <nav
      class="mobile-bottom-nav fixed inset-x-0 bottom-0 z-40 lg:hidden grid grid-cols-5 border-t border-border"
      style="padding-bottom: env(safe-area-inset-bottom)"
    >
      <RouterLink
        v-for="item in mobileNav"
        :key="item.to"
        :to="item.to"
        class="flex flex-col items-center justify-center gap-0.5 py-2 text-[11px] font-semibold min-h-14"
        :class="isActive(item.to) ? 'text-brand-600 dark:text-brand-400' : 'text-muted-foreground'"
      >
        <span class="w-6 h-6 block" v-html="item.icon"></span>
        {{ item.label }}
      </RouterLink>
      <button
        class="flex flex-col items-center justify-center gap-0.5 py-2 text-[11px] font-semibold text-muted-foreground min-h-14"
        @click="sidebarOpen = true"
      >
        <span class="w-6 h-6 block"><svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16"/></svg></span>
        更多
      </button>
    </nav>

    <!-- Security question prompt -->
    <Modal v-model="showSecurityModal" title="设置安全问题" :closable="false">
      <div class="flex flex-col items-center text-center gap-4 py-2">
        <div class="w-14 h-14 rounded-2xl bg-amber-400/15 text-amber-400 flex items-center justify-center">
          <svg class="w-7 h-7" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z"/></svg>
        </div>
        <p class="text-sm text-muted-foreground leading-relaxed">
          你尚未设置安全问题。设置后可在忘记密码时通过安全问题找回账号。
        </p>
        <div class="flex items-center gap-3 w-full pt-2">
          <button @click="dismissSecurityPrompt" class="flex-1 px-4 py-2.5 rounded-xl text-sm font-semibold text-muted-foreground hover:bg-muted transition-colors">
            稍后再说
          </button>
          <button @click="goToSecurityQuestions" class="flex-1 px-4 py-2.5 rounded-xl text-sm font-semibold bg-brand-500 text-white hover:bg-brand-600 transition-colors">
            去设置
          </button>
        </div>
      </div>
    </Modal>

    <!-- 赞赏支持弹窗 -->
    <SupportModal />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute, RouterLink, RouterView } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'
import { useWorklogStore } from '../stores/worklog'
import { useSupportStore } from '../stores/support'
import { getVersion } from '../api/config'
import UiThemeToggle from '../components/ui/ThemeToggle.vue'
import Modal from '../components/Modal.vue'
import SupportModal from '../components/SupportModal.vue'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()
const themeStore = useThemeStore()
const worklogStore = useWorklogStore()
const supportStore = useSupportStore()

const isWorker = computed(() => worklogStore.isWorker)
const canManage = computed(() => worklogStore.canManage)
const isBoss = computed(() => worklogStore.isBoss)
const sidebarOpen = ref(false)
const menuOpen = ref(false)
const menuRef = ref<HTMLElement | null>(null)
const versionInfo = ref('')

onMounted(async () => {
  await worklogStore.load()
  try {
    const res = await getVersion()
    if (res.data?.code === 0) {
      const d = res.data.data
      versionInfo.value = `${d.appName} ${d.version}`
      // 赞赏提示只给班组长（老板）与系统管理员：工人登录时不提示，
      // 免得工人看到向自己募捐的横幅，老板和包工头没面子。
      if (!worklogStore.isWorker && (worklogStore.isBoss || authStore.isAdmin)) await supportStore.init(true, d)
    }
  } catch {}
})

const iconBoard = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6"/></svg>'
const iconRecord = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-6 9l2 2 4-4"/></svg>'
const iconSettle = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 14l6-6m-5.5.5h.01m5 5h.01M19 21V5a2 2 0 00-2-2H7a2 2 0 00-2 2v16l3-2 2 2 2-2 2 2 2-2 2 2z"/></svg>'
const iconStats = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z"/></svg>'
const iconWorkers = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"/></svg>'
const iconAdvance = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>'
const iconProject = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M3 21h18M5 21V7l8-4v18m9 0V11l-7-4m-3 4h.01M9 13h.01M9 17h.01M15 13h.01M15 17h.01"/></svg>'
const iconPiece = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 7l-8-4-8 4m16 0l-8 4m8-4v10l-8 4m0-10L4 7m8 4v10M4 7v10l8 4"/></svg>'
const iconData = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4"/></svg>'
const iconGear = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.5 6h9.75M10.5 6a1.5 1.5 0 11-3 0m3 0a1.5 1.5 0 10-3 0M3.75 6H7.5m3 12h9.75m-9.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-3.75 0H7.5m9-6h3.75m-3.75 0a1.5 1.5 0 01-3 0m3 0a1.5 1.5 0 00-3 0m-9.75 0h9.75"/></svg>'
const iconUser = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>'

const iconTeam = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a4 4 0 00-3-3.87M9 20H4v-2a4 4 0 013-3.87m6-1.13a4 4 0 10-4-4 4 4 0 004 4zm6-4a3 3 0 11-3-3"/></svg>'
const iconCalendar = '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 7V3m8 4V3m-9 8h10M5 21h14a2 2 0 002-2V7a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z"/></svg>'

const navGroups = computed(() => {
  const manage = [
    {
      label: '记工管理',
      items: [
        { to: '/admin', label: '工作台', icon: iconBoard },
        { to: '/admin/records', label: '记工', icon: iconRecord },
        { to: '/admin/attendance', label: '考勤表', icon: iconCalendar },
        { to: '/admin/settlements', label: '结算', icon: iconSettle },
        { to: '/admin/stats', label: '统计', icon: iconStats },
      ],
    },
    {
      label: '基础档案',
      items: [
        { to: '/admin/workers', label: '工人', icon: iconWorkers },
        { to: '/admin/advances', label: '借支', icon: iconAdvance },
        { to: '/admin/projects', label: '工地', icon: iconProject },
        { to: '/admin/piece-items', label: '计件项目', icon: iconPiece },
      ],
    },
    {
      label: '通用',
      items: [
        { to: '/admin/team', label: '我的班组', icon: iconTeam },
        ...(isBoss.value ? [{ to: '/admin/data', label: '数据备份', icon: iconData }] : []),
        { to: '/admin/settings', label: '偏好设置', icon: iconGear },
        { to: '/admin/profile', label: '个人资料', icon: iconUser },
      ],
    },
  ]
  if (canManage.value) return manage
  // 员工视图：自己记工 + 查自己的钱
  return [
    {
      label: '我的记工',
      items: [
        { to: '/admin', label: '我的工资', icon: iconBoard },
        { to: '/admin/records', label: '记工', icon: iconRecord },
        { to: '/admin/attendance', label: '考勤表', icon: iconCalendar },
        { to: '/admin/settlements', label: '未结/已结', icon: iconSettle },
      ],
    },
    {
      label: '通用',
      items: [
        { to: '/admin/team', label: '我的班组', icon: iconTeam },
        { to: '/admin/settings', label: '偏好设置', icon: iconGear },
        { to: '/admin/profile', label: '个人资料', icon: iconUser },
      ],
    },
  ]
})

const mainNav = computed(() => navGroups.value.flatMap((g) => g.items))

const mobileNav = computed(() => navGroups.value[0].items.slice(0, 4))

const adminNav = [
  { to: '/admin/users', label: '用户管理', icon: '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4.354a4 4 0 110 5.292M15 21H3v-1a6 6 0 0112 0v1zm0 0h6v-1a6 6 0 00-9-5.197M13 7a4 4 0 11-8 0 4 4 0 018 0z"/></svg>' },
  { to: '/admin/configs', label: '系统配置', icon: '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"/></svg>' },
  { to: '/admin/audit', label: '操作日志', icon: '<svg fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5H7a2 2 0 00-2 2v12a2 2 0 002 2h10a2 2 0 002-2V7a2 2 0 00-2-2h-2M9 5a2 2 0 002 2h2a2 2 0 002-2M9 5a2 2 0 012-2h2a2 2 0 012 2m-3 7h3m-3 4h3m-6-4h.01M9 16h.01"/></svg>' },
]

const titleMap: Record<string, string> = {
  '/admin': '工作台',
  '/admin/records': '记工',
  '/admin/settlements': '结算',
  '/admin/settlements/': '工资条',
  '/admin/stats': '统计',
  '/admin/workers': '工人管理',
  '/admin/advances': '借支',
  '/admin/projects': '工地管理',
  '/admin/piece-items': '计件项目',
  '/admin/data': '数据备份',
  '/admin/team': '我的班组',
  '/admin/attendance': '考勤表',
  '/admin/profile': '个人资料',
  '/admin/settings': '偏好设置',
  '/admin/users': '用户管理',
  '/admin/configs': '系统配置',
  '/admin/audit': '操作日志',
}

const currentTitle = computed(() => {
  if (titleMap[route.path]) return titleMap[route.path]
  if (route.path.startsWith('/admin/settlements/')) return '工资条'
  return authStore.siteTitle
})
const siteInitial = computed(() => (authStore.siteTitle || 'S').charAt(0).toUpperCase())
const userInitial = computed(() => (authStore.user?.username || 'U').charAt(0).toUpperCase())

function isActive(to: string) {
  return to === '/admin' ? route.path === '/admin' : route.path.startsWith(to)
}

async function handleLogout() {
  menuOpen.value = false
  // 先让服务端落「主动登出」抑制标记，再清本地：网关域上只清前端的话，
  // 下一个请求会被网关注入身份重新认回登录态（退不出去）。
  await authStore.endSession()
  router.push('/login')
}

const showSecurityModal = ref(false)

watch(
  () => authStore.isAuthenticated && !authStore.hasSecurityQuestions && !authStore.securityPromptDismissed,
  (shouldShow) => {
    if (shouldShow) {
      showSecurityModal.value = true
    }
  },
  { immediate: true }
)

function goToSecurityQuestions() {
  showSecurityModal.value = false
  authStore.dismissSecurityPrompt()
  router.push('/admin/profile')
}

function dismissSecurityPrompt() {
  showSecurityModal.value = false
  authStore.dismissSecurityPrompt()
}

function onClickOutside(e: MouseEvent) {
  if (menuRef.value && !menuRef.value.contains(e.target as Node)) menuOpen.value = false
}
onMounted(() => document.addEventListener('click', onClickOutside))
onBeforeUnmount(() => document.removeEventListener('click', onClickOutside))
</script>

<style scoped>
.mobile-bottom-nav {
  background: rgb(var(--color-surface) / 0.88);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
}
.nav-link {
  @apply flex items-center gap-3 px-3 py-2.5 rounded-xl text-sm font-semibold transition-all duration-200;
}
.nav-link-active {
  @apply bg-brand-gradient text-white shadow-glow;
}
.nav-link-idle {
  @apply text-foreground hover:bg-muted;
}
.nav-icon {
  @apply w-9 h-9 rounded-lg flex items-center justify-center transition-colors shrink-0;
}
.menu-item {
  @apply flex items-center gap-2.5 px-3 py-2.5 rounded-xl text-sm font-medium text-foreground hover:bg-muted transition-colors;
}

/* Ambient background */
.app-bg {
  position: fixed;
  inset: 0;
  z-index: -10;
  overflow: hidden;
  pointer-events: none;
}
.glow {
  position: absolute;
  border-radius: 9999px;
  filter: blur(110px);
}
.glow-1 {
  width: 40rem;
  height: 40rem;
  top: -14rem;
  left: -10rem;
  background: rgba(234, 88, 12, 0.14);
}
.glow-2 {
  width: 36rem;
  height: 36rem;
  top: -8rem;
  right: -12rem;
  background: rgba(20, 184, 166, 0.12);
}
.glow-3 {
  width: 34rem;
  height: 34rem;
  bottom: -16rem;
  left: 30%;
  background: rgba(251, 191, 36, 0.12);
}
.app-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(rgba(234, 88, 12, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(234, 88, 12, 0.04) 1px, transparent 1px);
  background-size: 60px 60px;
  -webkit-mask: radial-gradient(circle at 50% 0%, #000 0%, transparent 70%);
  mask: radial-gradient(circle at 50% 0%, #000 0%, transparent 70%);
}
.dark .glow-1 { background: rgba(234, 88, 12, 0.24); }
.dark .glow-2 { background: rgba(20, 184, 166, 0.2); }
.dark .glow-3 { background: rgba(251, 191, 36, 0.18); }
</style>
