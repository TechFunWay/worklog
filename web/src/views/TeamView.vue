<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="我的班组" description="老板建班组，组长和员工凭邀请码加入" />

    <!-- 无班组 -->
    <template v-if="!teamData">
      <div class="grid md:grid-cols-2 gap-4">
        <div class="surface rounded-2xl p-4 sm:p-6">
          <div class="w-12 h-12 rounded-2xl bg-brand-500/15 text-brand-500 flex items-center justify-center text-2xl mb-4">👑</div>
          <h3 class="font-bold text-foreground text-lg">创建班组</h3>
          <p class="text-sm text-muted-foreground mt-1 mb-4">我是老板，创建班组后邀请工人和组长加入，帮他们记工。</p>
          <input v-model="newTeamName" class="input-field mb-2" placeholder="班组名称，如：张记工程队" />
          <input v-model="newTeamNote" class="input-field mb-4" placeholder="备注（选填）" />
          <button class="btn-brand w-full" :disabled="saving" @click="create">创建班组</button>
        </div>
        <div class="surface rounded-2xl p-4 sm:p-6">
          <div class="w-12 h-12 rounded-2xl bg-teal-400/15 text-teal-500 flex items-center justify-center text-2xl mb-4">👷</div>
          <h3 class="font-bold text-foreground text-lg">加入班组</h3>
          <p class="text-sm text-muted-foreground mt-1 mb-4">我是工人/组长，向老板要邀请码加入。加入后可以给自己记工、查工资。</p>
          <input v-model="joinCode" class="input-field mb-2" placeholder="邀请码" />
          <input v-model="joinName" class="input-field mb-4" placeholder="你的姓名（用于记工）" />
          <button class="btn-brand w-full" :disabled="saving" @click="join">加入班组</button>
        </div>
      </div>
    </template>

    <!-- 有班组 -->
    <template v-else>
      <div class="surface rounded-2xl p-5 sm:p-6">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="flex items-center gap-2">
              <h3 class="text-lg font-bold text-foreground">{{ teamData.team.name }}</h3>
              <span class="badge bg-brand-500/15 text-brand-600 dark:text-brand-400">{{ roleLabel(myRole) }}</span>
            </div>
            <p v-if="teamData.team.note" class="text-sm text-muted-foreground mt-1">{{ teamData.team.note }}</p>
          </div>
          <div v-if="teamData.is_owner" class="text-right">
            <div class="text-xs text-muted-foreground mb-1">邀请码（发给工人/组长）</div>
            <div class="flex items-center gap-2">
              <code class="text-xl font-bold tracking-widest text-brand-600 dark:text-brand-400 font-display">{{ teamData.team.invite_code }}</code>
              <button class="btn-ghost !py-1.5 !px-3 text-xs" @click="copyCode">复制</button>
            </div>
          </div>
        </div>
      </div>

      <div class="surface rounded-2xl overflow-hidden">
        <div class="px-5 py-3 border-b border-border text-sm font-bold text-foreground">成员（{{ teamData.members.length }}）</div>
        <div class="divide-y divide-border">
          <div v-for="m in teamData.members" :key="m.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
            <div class="w-10 h-10 rounded-full bg-brand-gradient flex items-center justify-center text-white font-bold shrink-0">
              {{ (m.worker_name || m.username).charAt(0) }}
            </div>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-semibold text-foreground">{{ m.worker_name || m.username }}</span>
                <span class="badge" :class="roleBadgeClass(m.role)">{{ roleLabel(m.role) }}</span>
                <span class="text-xs text-muted-foreground">@{{ m.username }}</span>
              </div>
            </div>
            <div v-if="teamData.is_owner && m.role !== 'boss'" class="flex gap-1.5 shrink-0">
              <button class="btn-ghost !px-3 !py-2 text-xs" @click="toggleRole(m)">
                {{ m.role === 'leader' ? '降为员工' : '设为组长' }}
              </button>
              <button class="btn-ghost !px-3 !py-2 text-xs !text-red-500" @click="remove(m)">移除</button>
            </div>
            <div v-else-if="m.user_id === me?.uid" class="shrink-0">
              <button v-if="m.role !== 'boss'" class="btn-ghost !px-3 !py-2 text-xs !text-red-500" @click="leave">退出</button>
            </div>
          </div>
        </div>
      </div>

      <div v-if="teamData.is_owner" class="surface rounded-2xl p-3.5 sm:p-5">
        <button class="btn-ghost w-full !text-red-500" @click="disband">解散班组（数据保留在自己名下，成员解绑）</button>
      </div>
    </template>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import Toast from '../components/Toast.vue'
import {
  getTeam, createTeam, joinTeam, updateMemberRole, removeMember, leaveTeam, disbandTeam,
  type TeamMemberInfo,
} from '../api/worklog'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const me = ref(store.me)
const myRole = computed(() => teamData.value?.my_role ?? 'boss')
const teamData = ref<{ team: any; members: TeamMemberInfo[]; my_role: string; is_owner: boolean } | null>(null)
const newTeamName = ref('')
const newTeamNote = ref('')
const joinCode = ref('')
const joinName = ref('')
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

function roleLabel(r: string) {
  return r === 'boss' ? '老板' : r === 'leader' ? '组长' : '员工'
}
function roleBadgeClass(r: string) {
  return r === 'boss' ? 'bg-brand-500/15 text-brand-600 dark:text-brand-400' : r === 'leader' ? 'bg-teal-400/15 text-teal-600 dark:text-teal-400' : 'bg-muted text-muted-foreground'
}

async function load() {
  await store.load(true)
  me.value = store.me
  try {
    const res = await getTeam()
    if (res.data?.code === 0 && res.data.data) teamData.value = res.data.data
    else teamData.value = null
  } catch {}
}

async function create() {
  if (newTeamName.value.trim().length < 2) return toast('请填写班组名称', 'error')
  saving.value = true
  try {
    const res = await createTeam({ name: newTeamName.value.trim(), note: newTeamNote.value.trim() || undefined })
    if (res.data?.code === 0) {
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
      await load()
    } else toast(res.data?.message || '加入失败', 'error')
  } catch (e: any) {
    toast(e.response?.data?.message || '加入失败', 'error')
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

async function remove(m: TeamMemberInfo) {
  if (!confirm(`确定移除成员「${m.worker_name || m.username}」？其工人档案将归档。`)) return
  try {
    await removeMember(m.id)
    toast('已移除')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

async function leave() {
  if (!confirm('确定退出班组？退出后你的记工数据保留在老板处，你将无法查看。')) return
  try {
    await leaveTeam()
    toast('已退出')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

async function disband() {
  if (!confirm('确定解散班组？所有成员将解绑，记工数据保留在你名下。')) return
  try {
    await disbandTeam()
    toast('班组已解散')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

function copyCode() {
  if (!teamData.value) return
  navigator.clipboard.writeText(teamData.value.team.invite_code).then(
    () => toast('邀请码已复制'),
    () => toast('复制失败', 'error'),
  )
}

onMounted(load)
</script>
