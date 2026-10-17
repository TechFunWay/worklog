<template>
  <div class="page-container animate-fade-in">
    <!-- Stat cards（手机端一行四列紧凑） -->
    <div class="grid grid-cols-4 gap-2 lg:gap-4">
      <div class="surface rounded-xl lg:rounded-2xl p-3 lg:p-5">
        <div class="flex items-center justify-between">
          <div class="min-w-0">
            <div class="text-xs lg:text-sm text-muted-foreground truncate">用户总数</div>
            <div class="text-lg lg:text-2xl font-extrabold text-foreground mt-0.5 lg:mt-1">{{ total }}</div>
          </div>
          <div class="hidden lg:flex w-11 h-11 rounded-xl bg-brand-50 dark:bg-brand-500/15 text-brand-600 dark:text-brand-300 items-center justify-center">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M17 20h5v-2a4 4 0 00-3-3.87M9 20H4v-2a4 4 0 013-3.87m6-3.13a4 4 0 10-4-4 4 4 0 004 4z"/></svg>
          </div>
        </div>
      </div>
      <div class="surface rounded-xl lg:rounded-2xl p-3 lg:p-5">
        <div class="flex items-center justify-between">
          <div class="min-w-0">
            <div class="text-xs lg:text-sm text-muted-foreground truncate">管理员</div>
            <div class="text-lg lg:text-2xl font-extrabold text-foreground mt-0.5 lg:mt-1">{{ adminCount }}</div>
          </div>
          <div class="hidden lg:flex w-11 h-11 rounded-xl bg-violet-50 dark:bg-violet-500/15 text-violet-600 dark:text-violet-300 items-center justify-center">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"/></svg>
          </div>
        </div>
      </div>
      <div class="surface rounded-xl lg:rounded-2xl p-3 lg:p-5">
        <div class="flex items-center justify-between">
          <div class="min-w-0">
            <div class="text-xs lg:text-sm text-muted-foreground truncate">已启用</div>
            <div class="text-lg lg:text-2xl font-extrabold text-foreground mt-0.5 lg:mt-1">{{ activeCount }}</div>
          </div>
          <div class="hidden lg:flex w-11 h-11 rounded-xl bg-emerald-50 dark:bg-emerald-500/15 text-emerald-600 dark:text-emerald-300 items-center justify-center">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
          </div>
        </div>
      </div>
      <div class="surface rounded-xl lg:rounded-2xl p-3 lg:p-5">
        <div class="flex items-center justify-between">
          <div class="min-w-0">
            <div class="text-xs lg:text-sm text-muted-foreground truncate">已禁用</div>
            <div class="text-lg lg:text-2xl font-extrabold text-foreground mt-0.5 lg:mt-1">{{ Math.max(total - activeCount, 0) }}</div>
          </div>
          <div class="hidden lg:flex w-11 h-11 rounded-xl bg-rose-50 dark:bg-rose-500/15 text-rose-600 dark:text-rose-300 items-center justify-center">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728A9 9 0 015.636 5.636m12.728 12.728L5.636 5.636"/></svg>
          </div>
        </div>
      </div>
    </div>

    <!-- Main panel -->
    <div class="surface rounded-2xl overflow-hidden">
      <div class="flex flex-wrap items-center justify-between gap-3 p-4 sm:p-6 border-b border-border">
        <div>
          <h2 class="text-lg font-bold text-foreground">用户列表</h2>
          <p class="text-sm text-muted-foreground">管理系统中的所有账户</p>
        </div>
        <div class="flex items-center gap-2 w-full sm:w-auto">
          <div class="relative flex-1 sm:flex-none">
            <svg class="absolute left-3.5 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground pointer-events-none" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/></svg>
            <input
              v-model="search"
              type="text"
              placeholder="搜索用户名..."
              class="input-field !pl-10 sm:w-64"
              @keyup.enter="reload"
            />
          </div>
          <button @click="reload" class="btn-brand whitespace-nowrap min-h-11">搜索</button>
        </div>
      </div>

      <!-- 桌面表格 -->
      <div class="hidden md:block overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="text-left text-xs uppercase tracking-wider text-muted-foreground bg-muted/60">
              <th class="py-3 px-3 sm:px-6 sm:py-3.5 font-semibold whitespace-nowrap">用户</th>
              <th class="py-3 px-3 sm:px-4 sm:py-3.5 font-semibold whitespace-nowrap">角色</th>
              <th class="py-3 px-3 sm:px-4 sm:py-3.5 font-semibold whitespace-nowrap">状态</th>
              <th class="py-3 px-3 sm:px-4 sm:py-3.5 font-semibold whitespace-nowrap">创建时间</th>
              <th class="py-3 px-3 sm:px-6 sm:py-3.5 font-semibold text-right whitespace-nowrap">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border">
            <tr v-for="user in users" :key="user.id" class="hover:bg-muted/50 transition-colors">
              <td class="py-3 px-3 sm:px-6 sm:py-3.5">
                <div class="flex items-center gap-3">
                  <div class="w-9 h-9 rounded-full flex items-center justify-center text-white font-bold text-xs shrink-0" :class="avatarClass(user.username)">
                    {{ user.username.charAt(0).toUpperCase() }}
                  </div>
                  <div>
                    <div class="font-semibold text-foreground">{{ user.username }}</div>
                    <div class="text-xs text-muted-foreground">ID #{{ user.id }}</div>
                  </div>
                </div>
              </td>
              <td class="py-3 px-3 sm:px-4 sm:py-3.5">
                <span v-if="user.role === 'admin'" class="badge bg-violet-100 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300">
                  <svg class="w-3 h-3" fill="currentColor" viewBox="0 0 20 20"><path d="M10 1l2.39 4.84L18 6.66l-4 3.9.94 5.5L10 13.5l-4.94 2.56.94-5.5-4-3.9 5.61-.82L10 1z"/></svg>
                  管理员
                </span>
                <span v-else class="badge bg-muted text-muted-foreground">普通用户</span>
              </td>
              <td class="py-3 px-3 sm:px-4 sm:py-3.5">
                <span v-if="user.status === 1" class="badge bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
                  <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>启用
                </span>
                <span v-else class="badge bg-rose-100 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300">
                  <span class="w-1.5 h-1.5 rounded-full bg-rose-500"></span>禁用
                </span>
              </td>
              <td class="py-3 px-3 sm:px-4 sm:py-3.5 text-muted-foreground whitespace-nowrap">{{ user.created_at }}</td>
              <td class="py-3 px-3 sm:px-6 sm:py-3.5">
                <div class="flex items-center justify-end gap-1">
                  <button @click="openJoin(user)" title="加入班组" class="icon-btn hover:text-teal-600 hover:bg-teal-50 dark:hover:bg-teal-500/10">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18 9v3m0 0v3m0-3h3m-3 0h-3m-2-5a4 4 0 11-8 0 4 4 0 018 0zM3 20a6 6 0 0112 0v1H3v-1z"/></svg>
                  </button>
                  <button @click="handleToggleStatus(user)" :title="user.status === 1 ? '禁用' : '启用'" class="icon-btn hover:text-amber-600 hover:bg-amber-50 dark:hover:bg-amber-500/10">
                    <svg v-if="user.status === 1" class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M18.364 18.364A9 9 0 005.636 5.636m12.728 12.728L5.636 5.636"/></svg>
                    <svg v-else class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7"/></svg>
                  </button>
                  <button @click="handleResetPassword(user)" title="重置密码" class="icon-btn hover:text-brand-600 hover:bg-brand-50 dark:hover:bg-brand-500/10">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/></svg>
                  </button>
                  <button @click="handleDelete(user)" title="删除" class="icon-btn hover:text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-500/10">
                    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg>
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="users.length === 0">
              <td colspan="5" class="py-16 text-center">
                <div class="flex flex-col items-center gap-3 text-muted-foreground">
                  <svg class="w-12 h-12 opacity-40" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M17 20h5v-2a4 4 0 00-3-3.87M9 20H4v-2a4 4 0 013-3.87m6-3.13a4 4 0 10-4-4 4 4 0 004 4z"/></svg>
                  <span class="text-sm">暂无用户数据</span>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 手机端卡片列表：每条独立卡片，操作按钮内联可见 -->
      <div class="md:hidden p-2 space-y-1.5">
        <div v-for="user in users" :key="user.id" class="rounded-xl border border-border bg-surface/80 px-3 py-2.5 space-y-1.5">
          <div class="flex items-center gap-2.5">
            <div class="w-9 h-9 rounded-full flex items-center justify-center text-white font-bold text-xs shrink-0" :class="avatarClass(user.username)">
              {{ user.username.charAt(0).toUpperCase() }}
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-semibold text-foreground truncate">{{ user.username }}</span>
                <span v-if="user.status === 1" class="badge shrink-0 bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300">
                  <span class="w-1.5 h-1.5 rounded-full bg-emerald-500"></span>启用
                </span>
                <span v-else class="badge shrink-0 bg-rose-100 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300">
                  <span class="w-1.5 h-1.5 rounded-full bg-rose-500"></span>禁用
                </span>
              </div>
              <div class="flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground mt-0.5">
                <span>ID #{{ user.id }}</span>
                <span v-if="user.role === 'admin'" class="text-violet-600 dark:text-violet-400">管理员</span>
                <span>{{ user.created_at }}</span>
              </div>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-1.5">
            <button class="btn-ghost !px-3 min-h-11 text-xs" @click="openJoin(user)">加入班组</button>
            <button class="btn-ghost !px-3 min-h-11 text-xs" @click="handleToggleStatus(user)">{{ user.status === 1 ? '禁用' : '启用' }}</button>
            <button class="btn-ghost !px-3 min-h-11 text-xs" @click="handleResetPassword(user)">重置密码</button>
            <button class="btn-ghost !px-3 min-h-11 text-xs !text-red-500" @click="handleDelete(user)">删除</button>
          </div>
        </div>
        <div v-if="users.length === 0" class="py-10 text-center text-sm text-muted-foreground">暂无用户数据</div>
      </div>

      <div class="flex flex-wrap items-center justify-between gap-3 p-4 sm:px-6 border-t border-border">
        <div class="text-sm text-muted-foreground">
          共 <span class="font-semibold text-foreground">{{ total }}</span> 条 · 第 {{ page }} / {{ totalPages }} 页
        </div>
        <div class="flex items-center gap-2">
          <button @click="prevPage" :disabled="page <= 1" class="btn-ghost !px-3 min-h-11">上一页</button>
          <button @click="nextPage" :disabled="page >= totalPages" class="btn-ghost !px-3 min-h-11">下一页</button>
        </div>
      </div>
    </div>

    <!-- Reset password modal -->
    <transition enter-active-class="transition-opacity duration-200" enter-from-class="opacity-0" leave-active-class="transition-opacity duration-150" leave-to-class="opacity-0">
      <div v-if="resetModalVisible" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-foreground/50 sm:backdrop-blur-sm" @click.self="resetModalVisible = false">
        <div class="surface rounded-2xl shadow-2xl p-6 w-full max-w-md animate-scale-in">
          <div class="flex items-center gap-3 mb-5">
            <div class="w-11 h-11 rounded-xl bg-brand-50 dark:bg-brand-500/15 text-brand-600 dark:text-brand-300 flex items-center justify-center">
              <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 7a2 2 0 012 2m4 0a6 6 0 01-7.743 5.743L11 17H9v2H7v2H4a1 1 0 01-1-1v-2.586a1 1 0 01.293-.707l5.964-5.964A6 6 0 1121 9z"/></svg>
            </div>
            <div>
              <h3 class="text-lg font-bold text-foreground">重置密码</h3>
              <p class="text-sm text-muted-foreground">用户：{{ resetTarget?.username }}</p>
            </div>
          </div>
          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium text-foreground mb-1.5">新密码</label>
              <input v-model="resetNewPassword" type="password" placeholder="请输入新密码" class="input-field" @keyup.enter="confirmResetPassword" />
            </div>
            <div v-if="resetMsg" class="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-lg">{{ resetMsg }}</div>
            <div class="flex gap-3 pt-1">
              <button @click="resetModalVisible = false" class="btn-ghost flex-1 min-h-11">取消</button>
              <button @click="confirmResetPassword" :disabled="resetLoading" class="btn-brand flex-1 min-h-11">{{ resetLoading ? '重置中...' : '确认重置' }}</button>
            </div>
          </div>
        </div>
      </div>
    </transition>

    <!-- 加入我的班组弹窗 -->
    <Modal v-model="joinModalVisible" title="加入我的班组">
      <div class="space-y-3">
        <div v-if="myTeams.length === 0" class="rounded-xl border border-border bg-muted/40 px-3 py-3">
          <p class="text-sm font-medium text-foreground mb-1">你还没有自己的班组</p>
          <p class="text-xs text-muted-foreground mb-3">请先在「我的班组」创建班组，再回到这里把用户加进来。</p>
          <RouterLink
            to="/admin/team"
            class="btn-brand inline-flex items-center justify-center min-h-11 px-4"
            @click="joinModalVisible = false"
          >
            去创建班组
          </RouterLink>
        </div>
        <template v-else>
          <div>
            <label class="block text-sm font-medium text-foreground mb-1.5">目标用户</label>
            <div class="input-field bg-muted/50 truncate">{{ joinTarget?.username }}（ID #{{ joinTarget?.id }}）</div>
          </div>
          <div>
            <label class="block text-sm font-medium text-foreground mb-1.5">班组</label>
            <SelectMenu
              v-model="joinForm.team_id"
              :options="myTeams.map(t => ({ value: t.id, label: `${t.name}（${t.member_count} 人）` }))"
              class="w-full"
            />
          </div>
          <div>
            <label class="block text-sm font-medium text-foreground mb-1.5">角色</label>
            <div class="grid grid-cols-2 gap-2">
              <button
                type="button"
                class="min-h-11 rounded-xl border px-3 text-sm font-medium transition-colors"
                :class="joinForm.role === 'worker' ? 'border-brand-500 bg-brand-500/10 text-brand-600 dark:text-brand-400' : 'border-border text-muted-foreground'"
                @click="joinForm.role = 'worker'"
              >
                员工
              </button>
              <button
                type="button"
                class="min-h-11 rounded-xl border px-3 text-sm font-medium transition-colors"
                :class="joinForm.role === 'leader' ? 'border-brand-500 bg-brand-500/10 text-brand-600 dark:text-brand-400' : 'border-border text-muted-foreground'"
                @click="joinForm.role = 'leader'"
              >
                组长
              </button>
            </div>
          </div>
          <div>
            <label class="block text-sm font-medium text-foreground mb-1.5">姓名</label>
            <input v-model="joinForm.worker_name" class="input-field" placeholder="用于记工的姓名，默认取用户名" />
          </div>
          <div class="grid grid-cols-2 gap-2">
            <div>
              <label class="block text-sm font-medium text-foreground mb-1.5">日薪（元）</label>
              <input v-model="joinForm.daily" type="number" inputmode="decimal" min="0" class="input-field" placeholder="0" />
            </div>
            <div>
              <label class="block text-sm font-medium text-foreground mb-1.5">时薪（元）</label>
              <input v-model="joinForm.hourly" type="number" inputmode="decimal" min="0" class="input-field" placeholder="0" />
            </div>
          </div>
          <div v-if="joinMsg" class="text-sm text-destructive bg-destructive/10 px-3 py-2 rounded-lg">{{ joinMsg }}</div>
          <div class="flex gap-2 pt-1">
            <button class="btn-ghost flex-1 min-h-11" @click="joinModalVisible = false">取消</button>
            <button class="btn-brand flex-1 min-h-11" :disabled="joinLoading" @click="submitJoin">{{ joinLoading ? '添加中...' : '确认加入' }}</button>
          </div>
        </template>
      </div>
    </Modal>

    <ConfirmDialog v-model="confirmOpen" :title="confirmTitle" :message="confirmMessage" :confirm-text="confirmText" confirm-type="danger" @confirm="confirmAction()" />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import Modal from '../components/Modal.vue'
import SelectMenu from '../components/SelectMenu.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Toast from '../components/Toast.vue'
import { getUsers, toggleUserStatus, deleteUser, resetUserPassword } from '../api/user'
import { listTeams, addTeamMember, type TeamCard } from '../api/worklog'
import { passwordValidationError } from '../utils/password'

const users = ref<any[]>([])
const search = ref('')
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)
const totalPages = computed(() => Math.ceil(total.value / pageSize.value) || 1)

// 我的班组（老板身份下的班组列表），用于「加入班组」
const myTeams = ref<TeamCard[]>([])
const joinModalVisible = ref(false)
const joinTarget = ref<any>(null)
const joinForm = ref<{ team_id: number; role: 'worker' | 'leader'; worker_name: string; daily: string; hourly: string }>({
  team_id: 0,
  role: 'worker',
  worker_name: '',
  daily: '',
  hourly: '',
})
const joinLoading = ref(false)
const joinMsg = ref('')

const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

// 通用确认弹窗（替代原生 confirm）
const confirmOpen = ref(false)
const confirmTitle = ref('')
const confirmMessage = ref('')
const confirmText = ref('确认')
const confirmAction = ref<() => void | Promise<void>>(() => {})
function askConfirm(title: string, message: string, action: () => void | Promise<void>, text = '确认') {
  confirmTitle.value = title
  confirmMessage.value = message
  confirmText.value = text
  confirmAction.value = action
  confirmOpen.value = true
}

const adminCount = computed(() => users.value.filter((u) => u.role === 'admin').length)
const activeCount = computed(() => users.value.filter((u) => u.status === 1).length)

const resetModalVisible = ref(false)
const resetTarget = ref<any>(null)
const resetNewPassword = ref('')
const resetLoading = ref(false)
const resetMsg = ref('')

const avatarPalette = [
  'bg-gradient-to-br from-indigo-500 to-purple-500',
  'bg-gradient-to-br from-sky-500 to-blue-600',
  'bg-gradient-to-br from-emerald-500 to-teal-600',
  'bg-gradient-to-br from-amber-500 to-orange-600',
  'bg-gradient-to-br from-pink-500 to-rose-600',
  'bg-gradient-to-br from-violet-500 to-fuchsia-600',
]
function avatarClass(name: string) {
  let h = 0
  for (let i = 0; i < name.length; i++) h = name.charCodeAt(i) + ((h << 5) - h)
  return avatarPalette[Math.abs(h) % avatarPalette.length]
}

onMounted(() => {
  loadUsers()
  loadMyTeams()
})

async function loadUsers() {
  try {
    const res = await getUsers(page.value, pageSize.value, search.value || undefined)
    if (res.data?.code === 0) {
      users.value = res.data.data?.list || res.data.data?.items || []
      total.value = res.data.data?.total || 0
    }
  } catch {}
}

async function loadMyTeams() {
  try {
    const res = await listTeams()
    if (res.data?.code === 0 && res.data.data) myTeams.value = res.data.data.teams || []
  } catch {}
}

function reload() {
  page.value = 1
  loadUsers()
}

function prevPage() {
  if (page.value > 1) {
    page.value--
    loadUsers()
  }
}

function nextPage() {
  if (page.value < totalPages.value) {
    page.value++
    loadUsers()
  }
}

async function handleToggleStatus(user: any) {
  try {
    const res = await toggleUserStatus(user.id)
    if (res.data?.code === 0) await loadUsers()
  } catch {}
}

function handleDelete(user: any) {
  askConfirm('删除用户', `确定删除用户 ${user.username}？`, async () => {
    try {
      const res = await deleteUser(user.id)
      if (res.data?.code === 0) await loadUsers()
    } catch {}
  }, '删除')
}

function handleResetPassword(user: any) {
  resetTarget.value = user
  resetNewPassword.value = ''
  resetMsg.value = ''
  resetModalVisible.value = true
}

async function confirmResetPassword() {
  const validationError = passwordValidationError(resetNewPassword.value)
  if (validationError) {
    resetMsg.value = validationError
    return
  }
  resetLoading.value = true
  resetMsg.value = ''
  try {
    const res = await resetUserPassword(resetTarget.value.id, resetNewPassword.value)
    if (res.data?.code === 0) {
      resetModalVisible.value = false
    } else {
      resetMsg.value = res.data?.message || '重置失败'
    }
  } catch (err: any) {
    resetMsg.value = err.response?.data?.message || '网络错误'
  } finally {
    resetLoading.value = false
  }
}

// ---- 加入班组 ----

function openJoin(user: any) {
  joinTarget.value = user
  joinMsg.value = ''
  joinForm.value = {
    team_id: myTeams.value[0]?.id ?? 0,
    role: 'worker',
    worker_name: user?.username || '',
    daily: '',
    hourly: '',
  }
  if (myTeams.value.length === 0) loadMyTeams()
  joinModalVisible.value = true
}

async function submitJoin() {
  if (!joinTarget.value) return
  if (!joinForm.value.team_id) {
    joinMsg.value = '请选择班组'
    return
  }
  const d = joinForm.value.daily === '' ? 0 : Number(joinForm.value.daily)
  const h = joinForm.value.hourly === '' ? 0 : Number(joinForm.value.hourly)
  if (!Number.isFinite(d) || !Number.isFinite(h) || d < 0 || h < 0) {
    joinMsg.value = '单价格式不正确'
    return
  }
  joinLoading.value = true
  joinMsg.value = ''
  try {
    const res = await addTeamMember({
      team_id: joinForm.value.team_id,
      user_id: joinTarget.value.id,
      role: joinForm.value.role,
      worker_name: joinForm.value.worker_name.trim() || undefined,
      daily_wage_cents: Math.round(d * 100),
      hourly_wage_cents: Math.round(h * 100),
    })
    if (res.data?.code === 0) {
      joinModalVisible.value = false
      toast(`已把 ${joinTarget.value.username} 加入班组`)
      loadMyTeams()
    } else {
      joinMsg.value = res.data?.message || '添加失败'
    }
  } catch (err: any) {
    joinMsg.value = err.response?.data?.message || '网络错误'
  } finally {
    joinLoading.value = false
  }
}
</script>

<style scoped>
.icon-btn {
  @apply p-2 rounded-lg text-muted-foreground transition-colors;
}
</style>
