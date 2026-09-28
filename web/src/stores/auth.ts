import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { checkAuth, checkSetupRequired as apiCheckSetupRequired, logout as apiLogout } from '../api/auth'
import { getPublicConfigs, getUserConfigMeta } from '../api/config'
import { getSecurityQuestions } from '../api/security'
import { useThemeStore } from './theme'
import { SK } from '../utils/storage-keys'
import { onFnOSGatewayOrigin } from '../utils/gateway'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem(SK.token) || '')
  const user = ref<any>(null)
  const requireLogin = ref(true)
  const allowRegister = ref(true)
  const setupRequired = ref(false)
  const siteTitle = ref('工记')
  // 当前登录态从哪来，由服务端在 checkAuth 里判定（只有服务端知道这次请求是
  // 不是走网关 socket、以及有没有应用自己的凭证）：
  //   gateway —— 仅凭网关注入的 NAS 身份成立，会话实际归 NAS 所有，NAS 那侧
  //              退出后应用必须跟着退出（服务端会拒绝，此时自动登出并回登录页）；
  //   app     —— 应用自己签发的会话（显式登录或直连端口），可以独立退出，
  //              也完全不需要跟随 NAS。
  const sessionSource = ref<'app' | 'gateway'>('app')
  watch(siteTitle, (t) => { document.title = t }, { immediate: true })
  const hasSecurityQuestions = ref(true)
  const DISMISS_KEY = SK.securityPromptDismissed
  const DISMISS_TTL = 60 * 60 * 1000 // 1 hour
  const securityPromptDismissed = ref((() => {
    const ts = localStorage.getItem(DISMISS_KEY)
    if (!ts) return false
    return Date.now() - Number(ts) < DISMISS_TTL
  })())
  let initialized = false
  let initPromise: Promise<void> | null = null

  // 网关域下登录态由服务端用网关注入身份回填 user（本地无 token），故以 user 为准；
  // 直连端口 user 也只在 checkAuth 通过后才设置，等价于原来的 !!token && !!user。
  const isAuthenticated = computed(() => !!user.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  function clearSession() {
    token.value = ''
    user.value = null
    localStorage.removeItem(SK.token)
  }

  async function init() {
    if (initialized) return
    if (initPromise) return initPromise

    initPromise = (async () => {
      try {
        const configsRes = await getPublicConfigs()
        if (configsRes.data?.code === 0) {
          requireLogin.value = configsRes.data.data?.require_login !== 'false'
          allowRegister.value = configsRes.data.data?.allow_register !== 'false'
          if (configsRes.data.data?.site_title) {
            siteTitle.value = configsRes.data.data.site_title
          }
        }
      } catch {}

      await checkSetupRequired()

      if (setupRequired.value && token.value) {
        clearSession()
      }

      // 网关域没有本地令牌也要继续：checkAuth 会带回「网关注入身份对应的应用
      // 账号」，刷新后正是靠它保住登录态（见 utils/gateway.ts）。
      if (token.value || onFnOSGatewayOrigin()) {
        try {
          const res = await checkAuth()
          if (res.data?.code === 0 && res.data.data?.authenticated) {
            user.value = res.data.data.user
            // 服务端只在本应用确实靠网关注入身份登录时才回 gateway；其余情况
            // （直连端口、应用自己的令牌）都算应用会话。
            sessionSource.value = res.data.data?.session_source === 'gateway' ? 'gateway' : 'app'
            try {
              const secRes = await getSecurityQuestions()
              if (secRes.data?.code === 0) {
                hasSecurityQuestions.value = !!secRes.data.data?.has_questions
              }
            } catch {}
            try {
              const metaRes = await getUserConfigMeta()
              if (metaRes.data?.code === 0 && Array.isArray(metaRes.data.data)) {
                const themeItem = metaRes.data.data.find((it: any) => it.key === 'theme_mode')
                if (themeItem && ['system', 'light', 'dark'].includes(themeItem.value)) {
                  const themeStore = useThemeStore()
                  if (themeStore.mode !== themeItem.value) {
                    themeStore.setMode(themeItem.value)
                  }
                }
              }
            } catch {}
          } else {
            clearSession()
          }
        } catch {
          clearSession()
        }
      }
      initialized = true
    })()

    return initPromise
  }

  function resetInit() {
    initialized = false
    initPromise = null
  }

  function setToken(newToken: string) {
    token.value = newToken
    localStorage.setItem(SK.token, newToken)
  }

  function setUser(newUser: any) {
    user.value = newUser
  }

  // 退出登录（用户点击）。必须先让服务端记下「这位用户主动登出了」，再清本地：
  // 网关域上服务端每个请求都能从 X-Trim-Userid 认出应用账号，只清前端的话
  // 下一个请求就把登录态认回来了，用户看到的正是「退不出去」。请求失败也要
  // 继续本地登出——宁可本地先退出、下次刷新由服务端再纠正，也不能把用户卡在
  // 已登录界面里。
  async function endSession() {
    try {
      await apiLogout()
    } catch {}
    logout()
  }

  // 只清本地登录态（401 拦截器等同步路径用）。用户主动点退出请用 endSession()：
  // 它会先落服务端抑制标记再走到这里。
  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem(SK.token)
    localStorage.removeItem(DISMISS_KEY)
    securityPromptDismissed.value = false
    sessionSource.value = 'app'
  }

  async function checkSetupRequired() {
    try {
      const setupRes = await apiCheckSetupRequired()
      if (setupRes.data?.code === 0) {
        setupRequired.value = !!setupRes.data.data?.setup_required
      }
    } catch {}
    return setupRequired.value
  }

  function dismissSecurityPrompt() {
    securityPromptDismissed.value = true
    localStorage.setItem(DISMISS_KEY, String(Date.now()))
  }

  function refreshSecurityQuestions() {
    if (!token.value) return
    getSecurityQuestions()
      .then(res => {
        if (res.data?.code === 0) {
          hasSecurityQuestions.value = !!res.data.data?.has_questions
          if (hasSecurityQuestions.value) {
            localStorage.removeItem(DISMISS_KEY)
          }
        }
      })
      .catch(() => {})
  }

  return { token, user, requireLogin, allowRegister, setupRequired, siteTitle, hasSecurityQuestions, securityPromptDismissed, sessionSource, isAuthenticated, isAdmin, init, resetInit, setToken, setUser, endSession, logout, checkSetupRequired, dismissSecurityPrompt, refreshSecurityQuestions }
})
