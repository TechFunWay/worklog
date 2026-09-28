<template>
  <AuthShell :title="pageTitle" :subtitle="pageSubtitle">
    <!-- register disabled -->
    <div v-if="disabled" class="flex flex-col items-center text-center gap-3 rounded-2xl border border-white/10 bg-white/[0.03] px-6 py-10">
      <span class="w-12 h-12 rounded-full bg-amber-400/15 text-amber-300 flex items-center justify-center">
        <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 15v2m-6 4h12a2 2 0 002-2v-6a2 2 0 00-2-2H6a2 2 0 00-2 2v6a2 2 0 002 2zm10-10V7a4 4 0 00-8 0v4h8z"/></svg>
      </span>
      <p class="text-sm text-white/70">注册功能已关闭，请联系管理员</p>
    </div>

    <form v-else @submit.prevent="handleRegister" class="space-y-5">
      <div v-if="isFnOSBinding && !authStore.setupRequired" class="rounded-2xl border border-brand-400/30 bg-brand-400/10 px-5 py-4 text-sm text-white/85">
        当前飞牛 NAS 用户 <span class="font-semibold text-brand-200">{{ fnosUsername || '已登录用户' }}</span> 尚未绑定应用账号。
        <template v-if="fnosMode === 'bind'">请输入当前使用的应用账号密码；绑定后会继续使用该账号中的数据与设置。</template>
        <template v-else>创建后将成为一个数据独立的新账号；如果已有应用数据，请改为绑定已有账号。</template>
      </div>
      <div v-if="authStore.setupRequired" class="flex items-start gap-2 rounded-xl border border-brand-400/25 bg-brand-400/10 px-3.5 py-2.5 text-xs leading-5 text-white/70">
        <svg class="w-4 h-4 shrink-0 mt-0.5 text-brand-300" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
        <span v-if="isFnOSBinding">创建的账号将成为管理员，并绑定当前飞牛 NAS 用户，之后可一键登录</span>
        <span v-else-if="fnosEnabled">创建的账号将成为管理员；如需绑定飞牛 NAS 一键登录，可点击下方「使用飞牛 NAS 登录」</span>
        <span v-else>创建的账号将成为管理员</span>
      </div>

      <AuthField v-model="username" label="用户名" autocomplete="username" required placeholder="请输入用户名" autofocus>
        <template #icon>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M16 7a4 4 0 11-8 0 4 4 0 018 0zM12 14a7 7 0 00-7 7h14a7 7 0 00-7-7z"/></svg>
        </template>
      </AuthField>

      <AuthField v-model="password" label="密码" type="password" autocomplete="new-password" required placeholder="请输入密码">
        <template #icon>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><rect x="5" y="11" width="14" height="9" rx="2" stroke-width="2"/><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M8 11V8a4 4 0 118 0v3"/></svg>
        </template>
      </AuthField>

      <AuthField v-if="!isFnOSBinding || fnosMode === 'register'" v-model="confirmPassword" label="确认密码" type="password" autocomplete="new-password" required placeholder="请再次输入密码">
        <template #icon>
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
        </template>
      </AuthField>

      <transition enter-active-class="transition duration-200" enter-from-class="opacity-0 -translate-y-1" leave-active-class="transition duration-150" leave-to-class="opacity-0">
        <div v-if="errorMsg" class="flex items-center gap-2 text-sm text-red-300 bg-red-500/10 border border-red-500/25 rounded-xl px-3.5 py-2.5">
          <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 9v2m0 4h.01M5.07 19h13.86a2 2 0 001.74-3L13.74 4a2 2 0 00-3.48 0L3.34 16a2 2 0 001.73 3z"/></svg>
          {{ errorMsg }}
        </div>
      </transition>
      <transition enter-active-class="transition duration-200" enter-from-class="opacity-0 -translate-y-1" leave-active-class="transition duration-150" leave-to-class="opacity-0">
        <div v-if="successMsg" class="flex items-center gap-2 text-sm text-emerald-300 bg-emerald-500/10 border border-emerald-500/25 rounded-xl px-3.5 py-2.5">
          <svg class="w-4 h-4 shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z"/></svg>
          {{ successMsg }}
        </div>
      </transition>

      <button type="submit" :disabled="loading" class="btn-premium">
        <svg v-if="loading" class="w-5 h-5 animate-spin" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/></svg>
        {{ loading ? '处理中...' : submitLabel }}
      </button>

      <button v-if="isFnOSBinding && authStore.setupRequired" type="button" :disabled="loading" @click="exitFnOSBinding" class="w-full text-sm font-medium text-white/55 hover:text-white/80 transition-colors">
        不使用飞牛 NAS，改用用户名密码创建
      </button>

      <button v-if="isFnOSBinding && !authStore.setupRequired" type="button" :disabled="loading" @click="fnosMode = fnosMode === 'register' ? 'bind' : 'register'" class="w-full text-sm font-medium text-brand-300 hover:text-brand-200 transition-colors">
        {{ fnosMode === 'register' ? '已有应用账号？验证并绑定' : '没有应用账号？创建并绑定' }}
      </button>

      <button v-if="fnosEnabled && !isFnOSBinding" type="button" :disabled="loading" @click="handleFnOSAuthorize" class="w-full min-h-10 inline-flex items-center justify-center rounded-xl border border-white/15 bg-transparent px-4 py-3 text-sm font-semibold text-white transition-colors hover:bg-white/[0.08] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-300/70 disabled:cursor-not-allowed disabled:opacity-60">
        {{ loading ? '正在获取飞牛账号…' : '使用飞牛 NAS 登录' }}
      </button>
    </form>

    <Teleport to="body">
      <transition enter-active-class="transition duration-200" enter-from-class="opacity-0" leave-active-class="transition duration-150" leave-to-class="opacity-0">
        <div v-if="showFnOSConfirm" class="fixed inset-0 z-[100] flex items-center justify-center bg-black/65 px-4 backdrop-blur-sm" @click.self="closeFnOSConfirm">
          <section role="dialog" aria-modal="true" aria-labelledby="fnos-confirm-title" class="w-full max-w-md rounded-3xl border border-white/15 bg-[#171827] p-6 text-white shadow-2xl sm:p-8">
            <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-brand-500 to-violet-500 text-2xl font-bold shadow-lg shadow-brand-500/25">
              {{ fnosConfirmUsername.slice(0, 1) || '飞' }}
            </div>
            <h2 id="fnos-confirm-title" class="mt-5 text-center text-2xl font-bold">{{ authStore.setupRequired ? '确认使用飞牛 NAS 创建管理员' : '确认使用飞牛 NAS 登录' }}</h2>
            <p class="mt-2 text-center text-sm leading-6 text-white/65">“工记”正在请求使用下面的飞牛 NAS 账号</p>
            <div class="mt-6 flex items-center justify-center gap-3 rounded-2xl border border-white/10 bg-white/[0.06] px-4 py-4">
              <span class="flex h-10 w-10 items-center justify-center rounded-full bg-brand-400/25 font-semibold text-brand-100">{{ fnosConfirmUsername.slice(0, 1) || '飞' }}</span>
              <span class="font-semibold">{{ fnosConfirmUsername || '当前飞牛 NAS 用户' }}</span>
            </div>
            <button type="button" :disabled="loading" @click="confirmFnOSAuthorize" class="btn-premium mt-6">
              {{ loading ? '处理中…' : '确认' }}
            </button>
            <button v-if="!fnosMobile" type="button" :disabled="loading" @click="switchFnOSAccount" class="mt-3 w-full rounded-xl px-4 py-3 text-sm font-semibold text-brand-200 transition-colors hover:bg-white/[0.06] hover:text-brand-100 disabled:opacity-60">
              使用其他飞牛账号
            </button>
            <p v-else class="mt-3 text-xs leading-5 text-white/45">如需更换飞牛账号，请在飞牛 App 中切换后重新打开本应用。</p>
            <button type="button" :disabled="loading" @click="closeFnOSConfirm" class="mt-1 w-full rounded-xl px-4 py-2 text-sm text-white/55 transition-colors hover:text-white/80 disabled:opacity-60">取消</button>
          </section>
        </div>
      </transition>
    </Teleport>

    <template #footer v-if="!authStore.setupRequired">
      已有账号？
      <router-link to="/login" class="font-semibold text-brand-300 hover:text-brand-200 transition-colors">立即登录</router-link>
    </template>
  </AuthShell>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { register, bindFnOSAccount, getFnOSIdentity, fnosLogin } from '../api/auth'
import { getPublicConfigs } from '../api/config'
import { useAuthStore } from '../stores/auth'
import AuthShell from '../components/auth/AuthShell.vue'
import AuthField from '../components/auth/AuthField.vue'
import { passwordValidationError } from '../utils/password'
import { fnOSMobileClient, startFnOSAccountSwitch } from '../utils/fnos-auth'

const router = useRouter()
const route = useRoute()
const authStore = useAuthStore()

const fnosEnabled = import.meta.env.VITE_FNOS_APP === 'true'
const username = ref(typeof route.query.fnos_username === 'string' ? route.query.fnos_username : '')
const password = ref('')
const confirmPassword = ref('')
const loading = ref(false)
const errorMsg = ref('')
const successMsg = ref('')
const disabled = ref(false)
const fnosMode = ref<'register' | 'bind'>(route.query.fnos_mode === 'bind' ? 'bind' : 'register')
// 绑定态用本地状态承接：既从 URL 查询参数初始化（登录页一键流程带过来的），
// 也允许用户在本页手动进入/退出飞牛绑定模式，而不必反复改写 URL。
const isFnOSBinding = ref(fnosEnabled && route.query.fnos === 'bind')
const fnosUsername = ref(typeof route.query.fnos_username === 'string' ? route.query.fnos_username : '')
const fnosConfirmUsername = ref('')
const showFnOSConfirm = ref(false)
const fnosMobile = fnOSMobileClient()
const pageTitle = computed(() => authStore.setupRequired
  ? (isFnOSBinding.value ? '使用飞牛 NAS 创建管理员' : '创建管理员账号')
  : (isFnOSBinding.value ? '绑定飞牛 NAS 账号' : '创建账号'))
const pageSubtitle = computed(() => authStore.setupRequired
  ? (isFnOSBinding.value ? '创建首个管理员账号，并绑定当前飞牛 NAS 用户' : '首次使用，请先完成管理员初始化')
  : (isFnOSBinding.value ? '创建或绑定应用账号，之后即可使用飞牛 NAS 一键登录' : '注册一个新账号以访问控制台'))
const submitLabel = computed(() => {
  if (authStore.setupRequired && isFnOSBinding.value) return '创建管理员并绑定飞牛 NAS'
  if (isFnOSBinding.value) return fnosMode.value === 'register' ? '创建并绑定' : '验证并绑定'
  return '注册'
})

onMounted(async () => {
  try {
    const res = await getPublicConfigs()
    if (res.data?.code === 0) {
      disabled.value = res.data.data?.allow_register === 'false' && !authStore.setupRequired && !isFnOSBinding.value
    }
  } catch {}
})

async function handleRegister() {
  if (!isFnOSBinding.value || fnosMode.value === 'register') {
    const validationError = passwordValidationError(password.value)
    if (validationError) {
      errorMsg.value = validationError
      return
    }
  }
  if ((!isFnOSBinding.value || fnosMode.value === 'register') && password.value !== confirmPassword.value) {
    errorMsg.value = '两次密码不一致'
    return
  }
  loading.value = true
  errorMsg.value = ''
  successMsg.value = ''
  try {
    if (isFnOSBinding.value) {
      const res = await bindFnOSAccount(fnosMode.value, username.value, password.value)
      if (res.data?.code === 0) {
        authStore.setToken(res.data.data.token)
        authStore.setUser(res.data.data.user)
        authStore.resetInit()
        router.push('/admin')
      } else {
        errorMsg.value = res.data?.message || '绑定失败'
      }
      return
    }
    const res = await register(username.value, password.value)
    if (res.data?.code === 0) {
      if (authStore.setupRequired) {
        successMsg.value = '注册成功，正在进入控制台'
        const token = res.data.data?.token
        const user = res.data.data?.user
        if (token && user) {
          authStore.setToken(token)
          authStore.setUser(user)
          authStore.resetInit()
          setTimeout(() => router.push('/admin'), 800)
        } else {
          successMsg.value = ''
          errorMsg.value = '注册成功但无法自动登录，请手动登录'
        }
      } else {
        successMsg.value = '注册成功，即将跳转登录页'
        setTimeout(() => router.push('/login'), 1500)
      }
    } else {
      errorMsg.value = res.data?.message || '注册失败'
    }
  } catch (err: any) {
    errorMsg.value = err.response?.data?.message || '网络错误'
  } finally {
    loading.value = false
  }
}

// 手动进入飞牛绑定：先取当前网关注入的 NAS 身份，弹确认框（与登录页同款），
// 用户点「确认」才真正调一键登录接口。
async function handleFnOSAuthorize() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await getFnOSIdentity()
    if (res.data?.code !== 0) {
      errorMsg.value = res.data?.message || '无法获取飞牛 NAS 账号'
      return
    }
    fnosConfirmUsername.value = res.data.data?.fnos_username || ''
    showFnOSConfirm.value = true
  } catch (err: any) {
    errorMsg.value = err.response?.data?.message || '无法获取飞牛 NAS 账号'
  } finally {
    loading.value = false
  }
}

function closeFnOSConfirm() {
  if (!loading.value) {
    showFnOSConfirm.value = false
  }
}

// 换飞牛账号：弹窗打开飞牛登录页，主页面与确认框保持原位不跳转；换完（或弹窗
// 被手动关闭）重取一次网关注入的 NAS 身份，把确认框里的账号刷新成当前账号。
function switchFnOSAccount() {
  const ok = startFnOSAccountSwitch(() => {
    void refreshFnOSIdentity()
  })
  if (!ok) {
    errorMsg.value = '当前环境无法弹出换号窗口，请在飞牛 App 中切换账号后重试'
  }
}

async function refreshFnOSIdentity() {
  try {
    const res = await getFnOSIdentity()
    if (res.data?.code === 0) {
      fnosConfirmUsername.value = res.data.data?.fnos_username || ''
    }
  } catch { /* 取不到就保留原显示 */ }
}

async function confirmFnOSAuthorize() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await fnosLogin()
    if (res.data?.code !== 0) {
      showFnOSConfirm.value = false
      errorMsg.value = res.data?.message || '飞牛一键登录失败'
      return
    }
    if (res.data.data?.binding_required) {
      // 该 NAS 用户还没绑定应用账号：本页切换到绑定模式，按后端建议选
      // 创建新账号或验证已有账号，并带出建议用户名。
      showFnOSConfirm.value = false
      isFnOSBinding.value = true
      fnosMode.value = res.data.data.suggested_mode === 'bind' ? 'bind' : 'register'
      username.value = res.data.data.suggested_username || ''
      fnosUsername.value = res.data.data.fnos_username || ''
      return
    }
    authStore.setToken(res.data.data.token)
    authStore.setUser(res.data.data.user)
    authStore.resetInit()
    showFnOSConfirm.value = false
    router.push('/admin')
  } catch (err: any) {
    showFnOSConfirm.value = false
    errorMsg.value = err.response?.data?.message || '飞牛一键登录失败'
  } finally {
    loading.value = false
  }
}

function exitFnOSBinding() {
  isFnOSBinding.value = false
  fnosUsername.value = ''
  fnosMode.value = 'register'
  username.value = ''
  router.replace({ name: 'Register' })
}
</script>
