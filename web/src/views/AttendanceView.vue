<template>
  <div class="page-container animate-fade-in">
    <PageHeader
      title="考勤表"
      :description="isWorker ? '我的出勤日历与对工单' : '按工人与工地查考勤，支持自定义时间段与一键对工'"
    >
      <template #actions>
        <button type="button" class="btn-ghost !py-2 text-xs whitespace-nowrap" @click="exportOpen = true">导出考勤表</button>
      </template>
    </PageHeader>

    <!-- 查询控制面板：模式 / 工人 / 工地 / 时间段 / 一键对工 -->
    <div class="surface rounded-2xl p-3 space-y-2.5">
      <div class="grid grid-cols-2 gap-1.5">
        <button type="button" class="py-2 sm:py-2.5 rounded-xl text-xs sm:text-sm font-semibold transition-all"
                :class="mode === 'month' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground hover:bg-muted'"
                @click="setMode('month')">月度日历</button>
        <button type="button" class="py-2 sm:py-2.5 rounded-xl text-xs sm:text-sm font-semibold transition-all"
                :class="mode === 'range' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground hover:bg-muted'"
                @click="setMode('range')">自定义时间段</button>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <!-- 工人筛选：月度模式选单个工人（默认第一个），时间段模式可看全部工人 -->
        <SelectMenu v-if="!isWorker && workers.length > 0 && mode === 'month'" v-model="monthWorkerId"
                    :options="monthWorkerOptions" class="!py-2 text-sm max-w-[45vw] sm:max-w-none" @change="loadMonth" />
        <SelectMenu v-if="!isWorker && workers.length > 0 && mode === 'range'" v-model="rangeWorkerId"
                    :options="rangeWorkerOptions" class="!py-2 text-sm max-w-[45vw] sm:max-w-none" @change="loadRange" />
        <!-- 工地筛选：员工也能用（只影响自己的考勤） -->
        <SelectMenu v-model="projectFilter" :options="projectOptions" class="!py-2 text-sm max-w-[45vw] sm:max-w-none" @change="onProjectChange" />
      </div>

      <div v-if="mode === 'range'" class="space-y-2">
        <div class="grid grid-cols-[1fr_auto_1fr] items-center gap-1.5">
          <input v-model="rangeStart" type="date" class="input-field !px-2 !py-1.5 text-sm min-w-0" @change="loadRange" />
          <span class="text-xs text-muted-foreground">至</span>
          <input v-model="rangeEnd" type="date" class="input-field !px-2 !py-1.5 text-sm min-w-0" @change="loadRange" />
        </div>
        <div class="grid grid-cols-4 gap-1.5">
          <button v-for="q in quickRanges" :key="q.key" type="button" class="btn-ghost !px-1 !py-1.5 text-xs whitespace-nowrap"
                  @click="applyQuickRange(q.key)">{{ q.label }}</button>
        </div>
      </div>

      <button type="button" class="btn-brand w-full !py-2.5 text-sm" @click="openReconcile()">一键对工</button>
    </div>

    <!-- 月度日历 -->
    <template v-if="mode === 'month'">
      <!-- 月度汇总 -->
      <div class="relative overflow-hidden rounded-2xl bg-brand-gradient p-3.5 sm:p-6 shadow-glow">
        <div class="absolute -top-12 -right-12 w-48 h-48 rounded-full bg-white/10 blur-2xl"></div>
        <div class="relative flex flex-wrap items-center justify-between gap-2.5 sm:gap-4">
          <div class="flex items-center gap-2">
            <button class="w-9 h-9 rounded-xl bg-white/15 text-white flex items-center justify-center" @click="shiftMonth(-1)">‹</button>
            <span class="text-white font-bold text-lg font-display min-w-24 text-center">{{ year }}年{{ month }}月</span>
            <button class="w-9 h-9 rounded-xl bg-white/15 text-white flex items-center justify-center" @click="shiftMonth(1)">›</button>
          </div>
          <div class="flex flex-wrap gap-x-4 sm:gap-x-6 gap-y-1 text-white">
            <div class="text-center">
              <div class="text-xs text-white/75">上班</div>
              <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ fmtQty(monthData?.days ?? 0) }} <span class="text-xs font-normal">个工</span></div>
            </div>
            <div class="text-center">
              <div class="text-xs text-white/75">点时</div>
              <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ fmtQty(monthData?.hours ?? 0) }} <span class="text-xs font-normal">小时</span></div>
            </div>
            <div class="text-center">
              <div class="text-xs text-white/75">计件</div>
              <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ fmtQty(monthData?.pieces ?? 0) }}</div>
            </div>
            <div class="text-center">
              <div class="text-xs text-white/75">休息</div>
              <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ monthData?.rest_count ?? 0 }} <span class="text-xs font-normal">天</span></div>
            </div>
            <div class="text-center">
              <div class="text-xs text-white/75">本月工钱</div>
              <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ fmtYuan(monthData?.amount_cents ?? 0) }} <span class="text-xs font-normal">元</span></div>
            </div>
          </div>
        </div>
      </div>

      <!-- 日历 -->
      <div class="surface rounded-2xl p-3 sm:p-5">
        <div class="grid grid-cols-7 gap-1.5 sm:gap-2 mb-2">
          <div v-for="w in ['日','一','二','三','四','五','六']" :key="w" class="text-center text-xs text-muted-foreground font-semibold py-1">{{ w }}</div>
        </div>
        <div class="grid grid-cols-7 gap-1.5 sm:gap-2">
          <div v-for="(cell, i) in grid" :key="i" class="min-h-16 sm:min-h-20 rounded-xl border text-center p-1 flex flex-col"
               :class="cellClass(cell)" @click="onCellClick(cell)">
            <template v-if="cell.date">
              <div class="flex items-center justify-center gap-0.5 text-sm font-bold" :class="cell.date === today ? 'text-brand-600 dark:text-brand-400' : 'text-foreground'">
                {{ Number(cell.date.slice(8)) }}
                <svg v-if="cell.count > 0 && !cell.rest" class="w-3.5 h-3.5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" /></svg>
              </div>
              <div v-if="cell.rest" class="text-xs text-teal-500 font-bold mt-auto mb-1">休</div>
              <div v-else-if="cell.count > 0" class="mt-auto mb-0.5 text-[10px] leading-tight text-muted-foreground">
                <div v-if="cell.days > 0" class="text-foreground font-semibold">{{ cell.days }} 个工</div>
                <div v-if="cell.hours > 0">{{ cell.hours }} 小时</div>
                <div v-if="cell.pieces > 0">{{ cell.pieces }} 件</div>
              </div>
            </template>
          </div>
        </div>
        <div class="mt-3 text-xs text-muted-foreground text-center">点任意日期可去记工（当天有记录打 ✓）</div>
      </div>
    </template>

    <!-- 自定义时间段 -->
    <template v-else>
      <!-- 时间段汇总卡：手机端一行四列紧凑 -->
      <div class="grid grid-cols-4 gap-1.5 sm:gap-2 lg:gap-4">
        <div class="surface rounded-2xl p-2 sm:p-3 lg:p-5 min-w-0">
          <div class="truncate text-[10px] sm:text-xs text-muted-foreground">考勤天数</div>
          <div class="mt-0.5 text-sm sm:text-lg lg:text-2xl font-extrabold tabular-nums font-display text-foreground">{{ rangeTotal?.attend_days ?? 0 }}</div>
          <div class="mt-0.5 text-[10px] sm:text-[11px] text-muted-foreground leading-tight">点工 {{ fmtQty(rangeTotal?.days ?? 0) }} 个工</div>
        </div>
        <div class="surface rounded-2xl p-2 sm:p-3 lg:p-5 min-w-0">
          <div class="truncate text-[10px] sm:text-xs text-muted-foreground">计时</div>
          <div class="mt-0.5 text-sm sm:text-lg lg:text-2xl font-extrabold tabular-nums font-display text-foreground">{{ fmtQty(rangeTotal?.hours ?? 0) }}</div>
        </div>
        <div class="surface rounded-2xl p-2 sm:p-3 lg:p-5 min-w-0">
          <div class="truncate text-[10px] sm:text-xs text-muted-foreground">计件</div>
          <div class="mt-0.5 text-sm sm:text-lg lg:text-2xl font-extrabold tabular-nums font-display text-foreground">{{ fmtQty(rangeTotal?.pieces ?? 0) }}</div>
        </div>
        <div class="surface rounded-2xl p-2 sm:p-3 lg:p-5 min-w-0">
          <div class="truncate text-[10px] sm:text-xs text-muted-foreground">工钱（元）</div>
          <div class="mt-0.5 truncate text-sm sm:text-lg lg:text-2xl font-extrabold tabular-nums font-display text-brand-600 dark:text-brand-400">{{ fmtYuanShort(rangeTotal?.amount_cents ?? 0) }}</div>
        </div>
      </div>

      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="rangeLoading" class="empty-state"><p>查询中…</p></div>
        <div v-else-if="rangeRows.length === 0" class="empty-state"><p>该时间段暂无考勤数据</p></div>

        <template v-else>
          <!-- 桌面表格 -->
          <div class="hidden md:block overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="bg-muted text-muted-foreground text-xs">
                  <th class="text-left font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap">工人</th>
                  <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">考勤天数</th>
                  <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">点工</th>
                  <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">计时</th>
                  <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">计件</th>
                  <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">休息</th>
                  <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">工钱</th>
                  <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap">对工</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border">
                <tr v-for="r in rangeRows" :key="r.worker_id" class="hover:bg-muted/50">
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 font-semibold text-foreground whitespace-nowrap">{{ r.worker_name }}</td>
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 text-right tabular-nums">{{ r.attend_days }}</td>
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 text-right tabular-nums">{{ fmtQty(r.days) }}</td>
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 text-right tabular-nums">{{ fmtQty(r.hours) }}</td>
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 text-right tabular-nums">{{ fmtQty(r.pieces) }}</td>
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 text-right tabular-nums">{{ r.rest_count }}</td>
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 text-right tabular-nums font-bold">{{ fmtMoney(r.amount_cents) }}</td>
                  <td class="px-2.5 sm:px-4 py-2 sm:py-2.5 text-right">
                    <button type="button" class="btn-ghost !px-2.5 !py-1 text-xs whitespace-nowrap" @click="openReconcile(r.worker_id)">对工</button>
                  </td>
                </tr>
              </tbody>
              <tfoot v-if="rangeRows.length > 1">
                <tr class="bg-muted/50 font-bold">
                  <td class="px-2.5 sm:px-4 py-2">合计</td>
                  <td class="px-2.5 sm:px-4 py-2 text-right tabular-nums">{{ rangeTotal?.attend_days ?? 0 }}</td>
                  <td class="px-2.5 sm:px-4 py-2 text-right tabular-nums">{{ fmtQty(rangeTotal?.days ?? 0) }}</td>
                  <td class="px-2.5 sm:px-4 py-2 text-right tabular-nums">{{ fmtQty(rangeTotal?.hours ?? 0) }}</td>
                  <td class="px-2.5 sm:px-4 py-2 text-right tabular-nums">{{ fmtQty(rangeTotal?.pieces ?? 0) }}</td>
                  <td class="px-2.5 sm:px-4 py-2 text-right tabular-nums">{{ rangeTotal?.rest_count ?? 0 }}</td>
                  <td class="px-2.5 sm:px-4 py-2 text-right tabular-nums">{{ fmtMoney(rangeTotal?.amount_cents ?? 0) }}</td>
                  <td></td>
                </tr>
              </tfoot>
            </table>
          </div>

          <!-- 手机卡片 -->
          <div class="md:hidden p-2 space-y-1.5">
            <div v-for="r in rangeRows" :key="r.worker_id" class="rounded-xl border border-border bg-surface/80 px-3 py-2.5">
              <div class="flex items-center justify-between gap-2">
                <span class="min-w-0 truncate font-semibold text-foreground">{{ r.worker_name }}</span>
                <span class="shrink-0 text-sm font-bold tabular-nums text-brand-600 dark:text-brand-400">{{ fmtMoney(r.amount_cents) }}</span>
              </div>
              <div class="mt-1 flex flex-wrap gap-x-2.5 gap-y-0.5 text-[11px] text-muted-foreground">
                <span>考勤 {{ r.attend_days }} 天</span>
                <span>点工 {{ fmtQty(r.days) }} 个工</span>
                <span>计时 {{ fmtQty(r.hours) }} 小时</span>
                <span>计件 {{ fmtQty(r.pieces) }}</span>
                <span>休息 {{ r.rest_count }} 天</span>
              </div>
              <div class="mt-1.5 flex justify-end">
                <button type="button" class="btn-ghost !px-3 !py-1.5 text-xs" @click="openReconcile(r.worker_id)">对工</button>
              </div>
            </div>
          </div>
        </template>
      </div>
    </template>

    <ReconcileModal v-model="reconcileOpen" :params="reconcileParams" />
    <ExportAttendanceModal
      v-model="exportOpen"
      :mode="mode"
      :year="year"
      :month="month"
      :start="rangeStart"
      :end="rangeEnd"
      :project-id="projectFilter"
      :project-name="projectName"
      :month-worker-id="monthWorkerId"
      :range-worker-id="rangeWorkerId"
      :is-worker="isWorker"
      :can-manage="canManage"
    />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Toast from '../components/Toast.vue'
import ReconcileModal from '../components/ReconcileModal.vue'
import ExportAttendanceModal from '../components/ExportAttendanceModal.vue'
import {
  getAttendance, getAttendanceRange, listProjects, listWorkers,
  type AttendanceData, type AttendanceRangeData, type Project, type ReconcileParams, type Worker,
} from '../api/worklog'
import { addDays, fmtMoney, fmtQty, fmtYuan, fmtYuanShort, todayStr } from '../utils/format'
import {
  isProjectFilterAvailable, readLastProjectFilter, saveLastProjectFilter,
  type ProjectFilter,
} from '../utils/project-filter'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const router = useRouter()
const isWorker = computed(() => store.isWorker)
const canManage = computed(() => store.canManage)

const now = new Date()
const year = ref(now.getFullYear())
const month = ref(now.getMonth() + 1)
const mode = ref<'month' | 'range'>('month')
const workers = ref<Worker[]>([])
const projects = ref<Project[]>([])
// 月度模式选单个工人；时间段模式默认「全部工人」
const monthWorkerId = ref<number | ''>('')
const rangeWorkerId = ref<number | ''>('')
const projectFilter = ref<ProjectFilter>('')
const monthData = ref<AttendanceData | null>(null)
const rangeData = ref<AttendanceRangeData | null>(null)
const rangeLoading = ref(false)
// 导出考勤表弹窗（Excel/图片存 NAS、本机下载、最近导出）
const exportOpen = ref(false)
const today = todayStr()
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

// 对工弹窗（新组件 ReconcileModal）
const reconcileOpen = ref(false)
const reconcileParams = ref<ReconcileParams>({ start: '', end: '' })

// 时间段默认本月 1 日 ~ 今天
const rangeStart = ref(today.slice(0, 8) + '01')
const rangeEnd = ref(today)

const quickRanges = [
  { key: 'thisMonth', label: '本月' },
  { key: 'lastMonth', label: '上月' },
  { key: 'last7', label: '近7天' },
  { key: 'last30', label: '近30天' },
] as const

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

// ---- 选项 ----
const monthWorkerOptions = computed(() => workers.value.map(w => ({ value: w.id as number, label: w.name })))
const rangeWorkerOptions = computed(() => [
  { value: '', label: '全部工人' },
  ...workers.value.map(w => ({ value: w.id as number, label: w.name })),
])
const projectOptions = computed(() => [
  { value: '', label: '全部工地' },
  { value: 'none', label: '未分工地' },
  ...projects.value.map(p => ({ value: p.id as number, label: p.name })),
])

function projectParam(): number | 'none' | undefined {
  return projectFilter.value === '' ? undefined : projectFilter.value
}

// 导出弹窗里展示的工地名（与筛选值保持一致）
const projectName = computed(() => {
  if (projectFilter.value === '') return '全部工地'
  if (projectFilter.value === 'none') return '未分工地'
  return projects.value.find(p => p.id === projectFilter.value)?.name || '指定工地'
})

const monthStart = computed(() => `${year.value}-${String(month.value).padStart(2, '0')}-01`)
const monthEnd = computed(() => {
  const last = new Date(year.value, month.value, 0).getDate()
  return `${year.value}-${String(month.value).padStart(2, '0')}-${String(last).padStart(2, '0')}`
})

const rangeRows = computed(() => rangeData.value?.rows ?? [])
const rangeTotal = computed(() => rangeData.value?.total)

// ---- 加载 ----
// 请求序号：快速切月份/日期时丢弃过期响应，避免旧数据盖掉新查询
let monthSeq = 0
let rangeSeq = 0

async function loadMonth() {
  const seq = ++monthSeq
  const params: Record<string, unknown> = { year: year.value, month: month.value }
  if (!isWorker.value && monthWorkerId.value) params.worker_id = monthWorkerId.value
  const pid = projectParam()
  if (pid !== undefined) params.project_id = pid
  try {
    const res = await getAttendance(params)
    if (seq !== monthSeq) return
    if (res.data?.code === 0) {
      const data = res.data.data as AttendanceData
      monthData.value = data
      // 后端默认取第一个在职工人；首次加载后把工人选择器同步到该工人
      if (!isWorker.value && monthWorkerId.value === '' && data.worker?.id != null) {
        monthWorkerId.value = data.worker.id
      }
    }
  } catch {}
}

async function loadRange() {
  if (!rangeStart.value || !rangeEnd.value) return
  if (rangeStart.value > rangeEnd.value) {
    toast('开始日期不能晚于结束日期', 'error')
    return
  }
  const seq = ++rangeSeq
  rangeLoading.value = true
  const params: Record<string, unknown> = { start: rangeStart.value, end: rangeEnd.value }
  if (!isWorker.value && rangeWorkerId.value) params.worker_id = rangeWorkerId.value
  const pid = projectParam()
  if (pid !== undefined) params.project_id = pid
  try {
    const res = await getAttendanceRange(params)
    if (seq !== rangeSeq) return
    if (res.data?.code === 0) rangeData.value = res.data.data
  } catch (e: any) {
    if (seq !== rangeSeq) return
    toast(e.response?.data?.message || '查询失败，请稍后重试', 'error')
  } finally {
    if (seq === rangeSeq) rangeLoading.value = false
  }
}

function setMode(next: 'month' | 'range') {
  if (mode.value === next) return
  mode.value = next
  if (next === 'month') loadMonth()
  else loadRange()
}

// 工地切换：写回上次工地并重载当前模式的查询
function onProjectChange() {
  saveLastProjectFilter(projectFilter.value)
  if (mode.value === 'month') loadMonth()
  else loadRange()
}

function shiftMonth(delta: number) {
  let m = month.value + delta
  let y = year.value
  if (m < 1) { m = 12; y-- }
  if (m > 12) { m = 1; y++ }
  year.value = y
  month.value = m
  loadMonth()
}

// ---- 快捷时间段 ----
function toDateStr(d: Date): string {
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

function applyQuickRange(key: string) {
  const d = new Date(today + 'T00:00:00')
  if (key === 'thisMonth') {
    rangeStart.value = today.slice(0, 8) + '01'
    rangeEnd.value = today
  } else if (key === 'lastMonth') {
    rangeStart.value = toDateStr(new Date(d.getFullYear(), d.getMonth() - 1, 1))
    rangeEnd.value = toDateStr(new Date(d.getFullYear(), d.getMonth(), 0))
  } else if (key === 'last7') {
    rangeStart.value = addDays(today, -6)
    rangeEnd.value = today
  } else {
    rangeStart.value = addDays(today, -29)
    rangeEnd.value = today
  }
  loadRange()
}

// ---- 月度日历 ----
interface GridCell {
  date: string
  days: number
  hours: number
  pieces: number
  rest: boolean
  count: number
  inMonth: boolean
}

const grid = computed<GridCell[]>(() => {
  const first = new Date(year.value, month.value - 1, 1)
  const daysInMonth = new Date(year.value, month.value, 0).getDate()
  const lead = first.getDay()
  const cells: GridCell[] = []
  for (let i = 0; i < lead; i++) {
    cells.push({ date: '', days: 0, hours: 0, pieces: 0, rest: false, count: 0, inMonth: false })
  }
  const byDate = new Map<string, AttendanceData['cells'][number]>()
  for (const c of monthData.value?.cells ?? []) byDate.set(c.date, c)
  for (let d = 1; d <= daysInMonth; d++) {
    const date = `${year.value}-${String(month.value).padStart(2, '0')}-${String(d).padStart(2, '0')}`
    const c = byDate.get(date)
    cells.push({
      date,
      days: c?.days ?? 0,
      hours: c?.hours ?? 0,
      pieces: c?.pieces ?? 0,
      rest: c?.rest ?? false,
      count: c?.record_count ?? 0,
      inMonth: true,
    })
  }
  return cells
})

function cellClass(cell: GridCell) {
  if (!cell.inMonth) return 'opacity-0 pointer-events-none border-transparent'
  if (cell.rest) return 'border-teal-400/40 bg-teal-400/5 cursor-pointer hover:bg-teal-400/10'
  if (cell.count > 0) return 'border-brand-400/40 bg-brand-500/5 cursor-pointer hover:bg-brand-500/10'
  return 'border-border cursor-pointer hover:bg-muted'
}

// 点日期跳记工：带上当前工地筛选，记工页自动落到该工地的上下文
function onCellClick(cell: GridCell) {
  if (!cell.date) return
  const query: Record<string, string> = { date: cell.date }
  if (projectFilter.value !== '') query.project = String(projectFilter.value)
  router.push({ name: 'Records', query })
}

// ---- 一键对工 ----
function openReconcile(rowWorkerId?: number) {
  const start = mode.value === 'month' ? monthStart.value : rangeStart.value
  const end = mode.value === 'month' ? monthEnd.value : rangeEnd.value
  if (!start || !end) return toast('请先选择时间段', 'error')
  if (start > end) return toast('开始日期不能晚于结束日期', 'error')

  // 员工角色由后端强制只看自己，不传 worker_id
  const wid: number | '' = isWorker.value
    ? ''
    : rowWorkerId ?? (mode.value === 'month' ? monthWorkerId.value : rangeWorkerId.value)
  if (!isWorker.value && !wid) {
    if (mode.value === 'month') return toast('请先选择工人', 'error')
    return toast('请先选择工人，或点某位工人的「对工」按钮', 'error')
  }

  reconcileParams.value = { worker_id: wid, start, end, project_id: projectFilter.value }
  reconcileOpen.value = true
}

// ---- 导出 ---- 已移至 ExportAttendanceModal（Excel/图片存 NAS、本机下载、最近导出）

onMounted(async () => {
  await store.load()

  // 工地列表用于筛选与默认值校验；工人列表只有老板/组长需要
  const tasks: Promise<unknown>[] = [
    listProjects({ status: 'active' })
      .then((res) => { if (res.data?.code === 0) projects.value = res.data.data })
      .catch(() => {}),
  ]
  if (!isWorker.value) {
    tasks.push(
      listWorkers({ status: 'active' })
        .then((res) => { if (res.data?.code === 0) workers.value = res.data.data })
        .catch(() => {}),
    )
  }
  await Promise.all(tasks)

  // 月度模式默认第一个工人
  if (!isWorker.value && monthWorkerId.value === '' && workers.value.length > 0) {
    monthWorkerId.value = workers.value[0].id as number
  }
  // 工地筛选默认上次选择的工地（须仍存在）
  const stored = readLastProjectFilter()
  const projectIds = projects.value.map(p => p.id).filter((id): id is number => typeof id === 'number')
  projectFilter.value = isProjectFilterAvailable(stored, projectIds) ? stored : ''

  await loadMonth()
})
</script>
