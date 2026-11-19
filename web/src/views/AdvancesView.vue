<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="借支" description="工人预支工资，结算时自动抵扣">
      <template #actions>
        <button class="btn-brand" @click="openForm">+ 记借支</button>
      </template>
    </PageHeader>

    <!-- 筛选：一行两个下拉（工人 / 工地），日期与合计一行，手机端不挤成一堆 -->
    <div class="surface rounded-2xl p-2.5 sm:p-3 space-y-2">
      <div class="flex items-center gap-2">
        <SelectMenu v-model="filterWorker" :options="workerFilterOptions" class="min-w-0 flex-1 sm:flex-none sm:w-44 !py-2 text-sm" @change="reload" />
        <SelectMenu v-model="filterProject" :options="projectFilterOptions" class="min-w-0 flex-1 sm:flex-none sm:w-44 !py-2 text-sm" @change="reload" />
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <input type="date" v-model="filterStart" class="input-field !w-auto !py-2 text-sm min-w-0 flex-1" @change="reload" />
        <span class="shrink-0 text-muted-foreground text-sm">至</span>
        <input type="date" v-model="filterEnd" class="input-field !w-auto !py-2 text-sm min-w-0 flex-1" @change="reload" />
        <div class="self-center text-sm text-muted-foreground sm:ml-auto">
          筛选合计：<span class="font-bold text-foreground tabular-nums">{{ fmtMoney(filteredTotal) }}</span>
        </div>
      </div>
    </div>

    <div class="surface rounded-2xl overflow-hidden">
      <div v-if="items.length === 0" class="empty-state"><p>暂无借支记录</p></div>
      <div v-else class="divide-y divide-border">
        <div v-for="a in items" :key="a.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
          <span class="w-10 h-10 rounded-xl bg-red-400/15 text-red-500 flex items-center justify-center text-lg shrink-0">💸</span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-foreground">{{ a.worker_name }}</span>
              <span class="badge bg-muted text-muted-foreground">{{ METHOD_LABELS[a.method as keyof typeof METHOD_LABELS] || a.method }}</span>
              <span class="badge" :class="a.settlement_id ? 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400' : 'bg-amber-400/15 text-amber-600 dark:text-amber-500'">
                {{ a.settlement_id ? '已结算' : '未结算' }}
              </span>
            </div>
            <div class="mt-0.5 flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-muted-foreground">
              <span>{{ a.date }}</span>
              <span v-if="a.project_name" class="badge bg-teal-400/15 text-teal-600 dark:text-teal-400 !px-1.5 !py-0 text-[10px]">🏗 {{ a.project_name }}</span>
              <span v-if="a.note">{{ a.note }}</span>
            </div>
          </div>
          <div class="text-right shrink-0 flex items-center gap-2">
            <span class="font-bold text-red-500 tabular-nums">-{{ fmtMoney(a.amount_cents) }}</span>
            <button v-if="!a.settlement_id" class="btn-ghost !px-2.5 !py-1.5 text-xs !text-red-500" @click="remove(a)">删除</button>
          </div>
        </div>
      </div>
      <div v-if="total > pageSize" class="p-3 flex justify-center gap-2 border-t border-border">
        <button class="btn-ghost !py-1.5 text-xs" :disabled="page <= 1" @click="page--; load()">上一页</button>
        <span class="text-xs text-muted-foreground self-center">{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
        <button class="btn-ghost !py-1.5 text-xs" :disabled="page >= Math.ceil(total / pageSize)" @click="page++; load()">下一页</button>
      </div>
    </div>

    <Modal v-model="formOpen" title="记借支">
      <div class="space-y-4">
        <div class="grid grid-cols-2 gap-3">
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">工人 *</label>
            <SelectMenu v-model="form.worker_id" :options="workerFormOptions" class="w-full" />
          </div>
          <!-- 借支归属工地（v0.6.0）：按项目结算时只抵扣该工地的借支；
               选「未指定工地」的在按人结算时统一抵扣，不会被扣两次。 -->
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">工地（选填）</label>
            <SelectMenu v-model="projectPick" :options="projectFormOptions" class="w-full" />
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">日期 *</label>
            <input type="date" v-model="form.date" class="input-field" />
          </div>
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">金额（元）*</label>
            <input v-model="amountInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" />
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">方式</label>
          <div class="grid grid-cols-4 gap-2">
            <button v-for="(label, key) in METHOD_LABELS" :key="key" class="py-2 rounded-xl text-sm font-semibold border transition-all"
                    :class="form.method === key ? 'bg-brand-gradient text-white border-transparent' : 'border-border text-muted-foreground'"
                    @click="form.method = key as string">
              {{ label }}
            </button>
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注</label>
          <input v-model="form.note" class="input-field" placeholder="选填，如：家里急用" />
        </div>
        <div class="flex gap-3">
          <button type="button" class="btn-ghost min-h-11 flex-1" @click="closeForm">取消</button>
          <button type="button" class="btn-brand min-h-11 flex-1" :disabled="saving" @click="save">保存</button>
        </div>
      </div>
    </Modal>

    <ConfirmDialog v-model="confirmOpen" :title="confirmTitle" :message="confirmMessage" :confirm-text="confirmText" confirm-type="danger" @confirm="confirmAction()" />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Toast from '../components/Toast.vue'
import {
  listAdvances, createAdvance, deleteAdvance, listWorkers, listProjects,
  type Advance, type Worker, type Project,
} from '../api/worklog'
import { fmtMoney, METHOD_LABELS, yuanToCents, todayStr } from '../utils/format'
import type { ProjectFilter } from '../utils/project-filter'

const items = ref<Advance[]>([])
const workers = ref<Worker[]>([])
const projects = ref<Project[]>([])
// 下拉选项：筛选含「全部工人」空值项，表单含「请选择」空值项
const workerFilterOptions = computed(() => [
  { value: '', label: '全部工人' },
  ...workers.value.map(w => ({ value: w.id as number, label: w.name })),
])
const workerFormOptions = computed(() => [
  { value: '', label: '请选择' },
  ...workers.value.map(w => ({ value: w.id as number, label: w.name })),
])
// 工地选项与记工页同口径（'' 全部 / 'none' 未分工地 / 数字 指定工地）；
// 表单里的「未指定工地」就是 NULL，与 WorkRecord.ProjectID 的语义一致。
const projectFilterOptions = computed(() => [
  { value: '', label: '全部工地' },
  { value: 'none', label: '未分工地' },
  ...projects.value.map(p => ({ value: p.id as number, label: p.name })),
])
const projectFormOptions = computed(() => [
  { value: '', label: '未指定工地' },
  ...projects.value.map(p => ({ value: p.id as number, label: p.name })),
])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const filterWorker = ref('')
const filterProject = ref<ProjectFilter>('')
const filterStart = ref('')
const filterEnd = ref('')
const formOpen = ref(false)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const form = ref<Partial<Advance>>({})
const amountInput = ref('')
// 弹窗里的工地选择（'' = 未指定工地）；Advance.project_id 是 number|null，
// 直接绑 SelectMenu 会和它的 string|number 值类型打架，所以单独放一个 ref。
const projectPick = ref<string | number>('')

const filteredTotal = computed(() => items.value.reduce((s, a) => s + a.amount_cents, 0))

// 筛选条件变了要回到第一页，否则停在第 3 页换筛选会看到空列表
function reload() {
  page.value = 1
  load()
}

async function load() {
  try {
    const res = await listAdvances({
      page: page.value, page_size: pageSize,
      worker_id: filterWorker.value || undefined,
      project_id: filterProject.value === '' ? undefined : filterProject.value,
      start: filterStart.value || undefined,
      end: filterEnd.value || undefined,
    })
    if (res.data?.code === 0) {
      items.value = res.data.data.items
      total.value = res.data.data.total
    }
  } catch {}
}

function openForm() {
  form.value = { date: todayStr(), method: 'cash' }
  amountInput.value = ''
  // 默认跟随当前工地筛选：正盯着某个工地查时，记的借支大概率就属于它
  projectPick.value = typeof filterProject.value === 'number' ? filterProject.value : ''
  formOpen.value = true
}

// 取消：只关闭弹窗，不提交表单。
// 与右上角 X 保持一致——保存中 X 仍可关，这里也不该把用户堵在弹窗里；
// 保存请求会自行结束（save() 的 finally 会把 saving 复位），关掉弹窗不影响结果。
function closeForm() {
  formOpen.value = false
}

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

async function save() {
  const cents = amountInput.value.trim() ? yuanToCents(amountInput.value) : null
  if (!form.value.worker_id) return toast('请选择工人', 'error')
  if (cents === null || cents <= 0) return toast('请填写金额', 'error')
  saving.value = true
  try {
    await createAdvance({
      worker_id: Number(form.value.worker_id),
      project_id: projectPick.value === '' ? null : Number(projectPick.value),
      date: form.value.date || todayStr(),
      amount_cents: cents,
      method: form.value.method || 'cash',
      note: form.value.note,
    })
    toast('借支已记录')
    formOpen.value = false
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

function remove(a: Advance) {
  askConfirm('删除借支', `确定删除「${a.worker_name}」的这笔借支？`, async () => {
    try {
      await deleteAdvance(a.id!)
      toast('已删除')
      await load()
    } catch (e: any) {
      toast(e.response?.data?.message || '删除失败', 'error')
    }
  }, '删除')
}

onMounted(async () => {
  await load()
  try {
    const [w, p] = await Promise.all([listWorkers(), listProjects({ status: 'active' })])
    if (w.data?.code === 0) workers.value = w.data.data
    if (p.data?.code === 0) projects.value = p.data.data
  } catch {}
})
</script>
