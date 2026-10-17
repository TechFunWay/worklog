<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="我的班组" description="老板建班组，组长和员工凭邀请码加入" />

    <div v-if="loading" class="surface rounded-2xl p-8 text-center text-sm text-muted-foreground">加载中…</div>

    <!-- 无班组：创建或加入（保持原有个人/加入逻辑） -->
    <template v-else-if="teams.length === 0 && !joined">
      <div class="grid md:grid-cols-2 gap-4">
        <div class="surface rounded-2xl p-4 sm:p-6">
          <div class="w-12 h-12 rounded-2xl bg-brand-500/15 text-brand-500 flex items-center justify-center text-2xl mb-4">👑</div>
          <h3 class="font-bold text-foreground text-lg">创建班组</h3>
          <p class="text-sm text-muted-foreground mt-1 mb-4">我是老板，创建班组后邀请工人和组长加入，帮他们记工。可创建多个班组。</p>
          <input v-model="newTeamName" class="input-field mb-2" placeholder="班组名称，如：张记工程队" />
          <input v-model="newTeamNote" class="input-field mb-4" placeholder="备注（选填）" />
          <button class="btn-brand w-full min-h-11" :disabled="saving" @click="create">创建班组</button>
        </div>
        <div class="surface rounded-2xl p-4 sm:p-6">
          <div class="w-12 h-12 rounded-2xl bg-teal-400/15 text-teal-500 flex items-center justify-center text-2xl mb-4">👷</div>
          <h3 class="font-bold text-foreground text-lg">加入班组</h3>
          <p class="text-sm text-muted-foreground mt-1 mb-4">我是工人/组长，向老板要邀请码加入。加入后可以给自己记工、查工资。</p>
          <input v-model="joinCode" class="input-field mb-2" placeholder="邀请码" />
          <input v-model="joinName" class="input-field mb-4" placeholder="你的姓名（用于记工）" />
          <button class="btn-brand w-full min-h-11" :disabled="saving" @click="join">加入班组</button>
        </div>
      </div>
    </template>

    <!-- 有班组 -->
    <template v-else-if="detail">
      <!-- 班组切换条：老板可建多个班组 -->
      <div v-if="detail.is_owner" class="surface rounded-2xl p-3 sm:p-4">
        <div class="flex flex-wrap items-center justify-between gap-2 mb-2">
          <div class="text-sm font-bold text-foreground">我的班组（{{ teams.length }}/20）</div>
          <button
            class="btn-ghost !px-3 min-h-11 text-xs whitespace-nowrap"
            :disabled="teams.length >= 20"
            @click="openCreateTeam"
          >
            ＋ 新建班组
          </button>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="t in teams"
            :key="t.id"
            class="min-h-11 px-3 py-2 rounded-xl border text-sm font-medium transition-colors"
            :class="
              t.id === detail.team.id
                ? 'border-brand-500 bg-brand-500/10 text-brand-600 dark:text-brand-400'
                : 'border-border text-muted-foreground hover:bg-muted/60'
            "
            @click="selectTeam(t.id)"
          >
            {{ t.name }}<span class="ml-1 text-xs opacity-70">{{ t.member_count }}人</span>
          </button>
        </div>
      </div>

      <!-- 成员：显示所在班组 + 退出 -->
      <div v-else class="surface rounded-2xl p-4 sm:p-5 flex flex-wrap items-center justify-between gap-3">
        <div class="min-w-0">
          <div class="text-sm text-muted-foreground">我所在的班组</div>
          <div class="font-bold text-foreground truncate">{{ detail.team.name }}</div>
        </div>
        <button v-if="detail.my_role !== 'boss'" class="btn-ghost !px-3 min-h-11 text-xs !text-red-500" @click="leave">
          退出班组
        </button>
      </div>

      <!-- 班组信息 -->
      <div class="surface rounded-2xl p-4 sm:p-6">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="text-lg font-bold text-foreground truncate">{{ detail.team.name }}</h3>
              <span class="badge" :class="roleBadgeClass(detail.my_role)">{{ roleLabel(detail.my_role) }}</span>
            </div>
            <p v-if="detail.team.note" class="text-sm text-muted-foreground mt-1 break-words">{{ detail.team.note }}</p>
          </div>
          <div v-if="detail.is_owner" class="w-full sm:w-auto">
            <div class="text-xs text-muted-foreground mb-1">邀请码（发给工人/组长）</div>
            <div class="flex flex-wrap items-center gap-2">
              <code class="text-lg sm:text-xl font-bold tracking-wider text-brand-600 dark:text-brand-400 font-display break-all">{{ detail.team.invite_code }}</code>
              <button class="btn-ghost !py-2 !px-3 text-xs min-h-11" @click="copyCode">复制</button>
            </div>
          </div>
          <div v-else class="text-xs text-muted-foreground">邀请码由老板保管</div>
        </div>
        <div v-if="detail.is_owner" class="flex flex-wrap gap-2 mt-4">
          <button class="btn-ghost !px-3 min-h-11 text-xs" @click="openEditTeam">改名 / 备注</button>
        </div>
      </div>

      <!-- 成员列表：手机与桌面都用卡片行，操作按钮第一屏内联可见 -->
      <div class="surface rounded-2xl overflow-hidden">
        <div class="px-4 sm:px-5 py-3 border-b border-border text-sm font-bold text-foreground">成员（{{ detail.members.length }}）</div>
        <div class="p-2 space-y-1.5">
          <div
            v-for="m in detail.members"
            :key="m.id"
            class="rounded-xl border border-border bg-surface/80 px-3 py-2.5"
          >
            <div class="space-y-1.5">
              <div class="flex items-center gap-2.5">
                <div class="w-9 h-9 rounded-full bg-brand-gradient flex items-center justify-center text-white font-bold shrink-0">
                  {{ (m.worker_name || m.username).charAt(0) }}
                </div>
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
                    <span class="font-semibold text-foreground truncate">{{ m.worker_name || m.username }}</span>
                    <span class="badge" :class="roleBadgeClass(m.role)">{{ roleLabel(m.role) }}</span>
                    <span class="text-xs text-muted-foreground truncate">@{{ m.username }}</span>
                  </div>
                  <div class="text-xs text-muted-foreground mt-0.5">
                    日薪 {{ wageText(m.daily_wage_cents) }} · 时薪 {{ wageText(m.hourly_wage_cents) }}
                  </div>
                </div>
              </div>
              <div v-if="detail.is_owner && m.role !== 'boss'" class="flex flex-wrap items-center gap-1.5">
                <button class="btn-ghost !px-3 min-h-11 text-xs" @click="openWage(m)">设单价</button>
                <button class="btn-ghost !px-3 min-h-11 text-xs" @click="toggleRole(m)">
                  {{ m.role === 'leader' ? '降为员工' : '设为组长' }}
                </button>
                <button class="btn-ghost !px-3 min-h-11 text-xs !text-red-500" @click="remove(m)">移除</button>
              </div>
              <div v-else-if="m.user_id === me?.uid && m.role !== 'boss'" class="flex justify-end">
                <button class="btn-ghost !px-3 min-h-11 text-xs !text-red-500" @click="leave">退出</button>
              </div>
            </div>
          </div>
          <div v-if="detail.members.length === 0" class="py-10 text-center text-sm text-muted-foreground">
            暂无成员，把邀请码发给工人/组长吧
          </div>
        </div>
      </div>

      <div v-if="detail.is_owner" class="surface rounded-2xl p-3.5 sm:p-5">
        <button class="btn-ghost w-full min-h-11 !text-red-500" @click="disband">解散当前班组（数据保留在自己名下，成员解绑）</button>
      </div>
    </template>

    <!-- 新建班组弹窗（老板已有班组时） -->
    <Modal v-model="createOpen" title="新建班组">
      <div class="space-y-3">
        <input v-model="newTeamName" class="input-field" placeholder="班组名称，如：张记工程队" />
        <input v-model="newTeamNote" class="input-field" placeholder="备注（选填）" />
        <button class="btn-brand w-full min-h-11" :disabled="saving" @click="create">创建班组</button>
      </div>
    </Modal>

    <!-- 修改班组信息弹窗 -->
    <Modal v-model="editOpen" title="修改班组信息">
      <div class="space-y-3">
        <input v-model="editName" class="input-field" placeholder="班组名称" />
        <input v-model="editNote" class="input-field" placeholder="备注（选填）" />
        <button class="btn-brand w-full min-h-11" :disabled="saving" @click="saveEditTeam">保存</button>
      </div>
    </Modal>

    <!-- 设单价弹窗 -->
    <Modal v-model="wageOpen" :title="`设置单价 · ${wageMember?.worker_name || wageMember?.username || ''}`">
      <div class="space-y-3">
        <div>
          <label class="block text-sm font-medium text-foreground mb-1.5">日薪（元）</label>
          <input v-model="wageDaily" type="number" inputmode="decimal" min="0" class="input-field" placeholder="0" />
        </div>
        <div>
          <label class="block text-sm font-medium text-foreground mb-1.5">时薪（元）</label>
          <input v-model="wageHourly" type="number" inputmode="decimal" min="0" class="input-field" placeholder="0" />
        </div>
        <p class="text-xs text-muted-foreground">单价会更新到该成员绑定的工人档案，记工时自动带出。不填按 0 处理。</p>
        <button class="btn-brand w-full min-h-11" :disabled="saving" @click="saveWage">保存单价</button>
      </div>
    </Modal>

    <ConfirmDialog v-model="confirmOpen" :title="confirmTitle" :message="confirmMessage" :confirm-text="confirmText" confirm-type="danger" @confirm="confirmAction()" />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Toast from '../components/Toast.vue'
import {
  createTeam, joinTeam, updateMemberRole, removeMember, leaveTeam, deleteTeam,
  listTeams, getTeamInfo, updateTeam, updateMemberWage,
  type TeamCard, type TeamDetail, type TeamMemberInfo,
} from '../api/worklog'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const me = computed(() => store.me)

const loading = ref(true)
const teams = ref<TeamCard[]>([])
const joined = ref<TeamCard | null>(null)
const selectedTeamID = ref<number | null>(null)
const detail = ref<TeamDetail | null>(null)

const newTeamName = ref('')
const newTeamNote = ref('')
const joinCode = ref('')
const joinName = ref('')
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

// 弹窗状态
const createOpen = ref(false)
const editOpen = ref(false)
const editName = ref('')
const editNote = ref('')
const wageOpen = ref(false)
const wageMember = ref<TeamMemberInfo | null>(null)
const wageDaily = ref('')
const wageHourly = ref('')

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

function roleLabel(r: string) {
  return r === 'boss' ? '老板' : r === 'leader' ? '组长' : '员工'
}
function roleBadgeClass(r: string) {
  return r === 'boss'
    ? 'bg-brand-500/15 text-brand-600 dark:text-brand-400'
    : r === 'leader'
      ? 'bg-teal-400/15 text-teal-600 dark:text-teal-400'
      : 'bg-muted text-muted-foreground'
}
function money(cents?: number | null) {
  return '¥' + (Number(cents || 0) / 100).toLocaleString('zh-CN', { maximumFractionDigits: 2 })
}
function wageText(cents?: number | null) {
  return cents && cents > 0 ? money(cents) : '未设置'
}

async function load(resetSelection = false) {
  loading.value = true
  await store.load(true)
  try {
    const res = await listTeams()
    if (res.data?.code === 0 && res.data.data) {
      teams.value = res.data.data.teams || []
      joined.value = res.data.data.joined || null
    }
  } catch {}
  if (resetSelection) selectedTeamID.value = null
  if (teams.value.length > 0) {
    if (!selectedTeamID.value || !teams.value.some((t) => t.id === selectedTeamID.value)) {
      selectedTeamID.value = teams.value[0].id
    }
  } else if (joined.value) {
    selectedTeamID.value = joined.value.id
  } else {
    selectedTeamID.value = null
  }
  if (selectedTeamID.value) {
    await loadDetail()
  } else {
    detail.value = null
  }
  loading.value = false
}

async function loadDetail() {
  if (!selectedTeamID.value) {
    detail.value = null
    return
  }
  try {
    const res = await getTeamInfo(selectedTeamID.value)
    detail.value = res.data?.code === 0 && res.data.data ? res.data.data : null
  } catch {
    detail.value = null
  }
}

function selectTeam(id: number) {
  if (selectedTeamID.value === id) return
  selectedTeamID.value = id
  loadDetail()
}

function openCreateTeam() {
  newTeamName.value = ''
  newTeamNote.value = ''
  createOpen.value = true
}

function openEditTeam() {
  if (!detail.value) return
  editName.value = detail.value.team.name
  editNote.value = detail.value.team.note || ''
  editOpen.value = true
}

async function create() {
  if (newTeamName.value.trim().length < 2) return toast('请填写班组名称', 'error')
  saving.value = true
  try {
    const res = await createTeam({ name: newTeamName.value.trim(), note: newTeamNote.value.trim() || undefined })
    if (res.data?.code === 0) {
      const created = res.data.data as TeamCard | undefined
      newTeamName.value = ''
      newTeamNote.value = ''
      createOpen.value = false
      selectedTeamID.value = created?.id ?? null
      toast('班组已创建，把邀请码发给工人吧')
      await load()
    } else toast(res.data?.message || '创建失败', 'error')
  } catch (e: any) {
    toast(e.response?.data?.message || '创建失败', 'error')
  } finally {
    saving.value = false
  }
}

async function join() {
  if (!joinCode.value.trim()) return toast('请填写邀请码', 'error')
  if (!joinName.value.trim()) return toast('请填写姓名', 'error')
  saving.value = true
  try {
    const res = await joinTeam({ invite_code: joinCode.value.trim(), worker_name: joinName.value.trim() })
    if (res.data?.code === 0) {
      toast('已加入班组')
      joinCode.value = ''
      joinName.value = ''
      await load()
    } else toast(res.data?.message || '加入失败', 'error')
  } catch (e: any) {
    toast(e.response?.data?.message || '加入失败', 'error')
  } finally {
    saving.value = false
  }
}

async function saveEditTeam() {
  if (!detail.value) return
  if (editName.value.trim().length < 2) return toast('请填写班组名称（至少 2 个字）', 'error')
  saving.value = true
  try {
    const res = await updateTeam(detail.value.team.id, {
      name: editName.value.trim(),
      note: editNote.value.trim(),
    })
    if (res.data?.code === 0) {
      editOpen.value = false
      toast('已保存')
      await load()
    } else toast(res.data?.message || '保存失败', 'error')
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

function openWage(m: TeamMemberInfo) {
  wageMember.value = m
  wageDaily.value = m.daily_wage_cents ? String(m.daily_wage_cents / 100) : ''
  wageHourly.value = m.hourly_wage_cents ? String(m.hourly_wage_cents / 100) : ''
  wageOpen.value = true
}

async function saveWage() {
  if (!wageMember.value) return
  const d = wageDaily.value === '' ? 0 : Number(wageDaily.value)
  const h = wageHourly.value === '' ? 0 : Number(wageHourly.value)
  if (!Number.isFinite(d) || !Number.isFinite(h) || d < 0 || h < 0) {
    return toast('单价格式不正确', 'error')
  }
  saving.value = true
  try {
    const res = await updateMemberWage(wageMember.value.id, {
      daily_wage_cents: Math.round(d * 100),
      hourly_wage_cents: Math.round(h * 100),
    })
    if (res.data?.code === 0) {
      wageOpen.value = false
      toast('单价已保存')
      await loadDetail()
    } else toast(res.data?.message || '保存失败', 'error')
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

async function toggleRole(m: TeamMemberInfo) {
  try {
    await updateMemberRole(m.id, m.role === 'leader' ? 'worker' : 'leader')
    toast('已更新')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

function remove(m: TeamMemberInfo) {
  askConfirm('移除成员', `确定移除成员「${m.worker_name || m.username}」？其工人档案将归档。`, async () => {
    try {
      await removeMember(m.id)
      toast('已移除')
      await load()
    } catch (e: any) {
      toast(e.response?.data?.message || '操作失败', 'error')
    }
  }, '移除')
}

function leave() {
  askConfirm('退出班组', '确定退出班组？退出后你的记工数据保留在老板处，你将无法查看。', async () => {
    try {
      await leaveTeam()
      toast('已退出')
      await load(true)
    } catch (e: any) {
      toast(e.response?.data?.message || '操作失败', 'error')
    }
  }, '退出')
}

function disband() {
  if (!detail.value) return
  const teamName = detail.value.team.name
  askConfirm('解散班组', `确定解散班组「${teamName}」？所有成员将解绑，记工数据保留在你名下。`, async () => {
    try {
      await deleteTeam(detail.value!.team.id)
      toast('班组已解散')
      await load(true)
    } catch (e: any) {
      toast(e.response?.data?.message || '操作失败', 'error')
    }
  }, '解散')
}

function copyCode() {
  const code = detail.value?.team.invite_code
  if (!code) return
  navigator.clipboard.writeText(code).then(
    () => toast('邀请码已复制'),
    () => toast('复制失败', 'error'),
  )
}

onMounted(() => load())
</script>
