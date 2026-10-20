<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="统计" description="按月汇总 / 按人员与时间段查询" />

    <!-- Tab -->
    <div class="surface rounded-2xl p-1.5 grid grid-cols-3 gap-1.5">
      <button class="py-2.5 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'monthly' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'monthly'">月度统计</button>
      <button class="py-2.5 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'project' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'project'">按工地</button>
      <button class="py-2.5 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'range' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'range'">按时间段查询</button>
    </div>

    <!-- 月度统计 -->
    <template v-if="tab === 'monthly'">
      <div class="surface rounded-2xl p-3 flex flex-wrap items-center gap-2">
        <SelectMenu v-model.number="year" :options="yearOptions.map(y => ({ value: y, label: `${y} 年` }))" class="!py-2 text-sm" @change="loadMonthly" />
        <SelectMenu v-model.number="month" :options="monthOptions" class="!py-2 text-sm" @change="loadMonthly" />
        <SelectMenu v-model="mProject" :options="projectFilterOptions" class="!py-2 text-sm" @change="loadMonthly" />
        <div class="flex-1"></div>
        <button type="button" class="btn-ghost !py-2 text-xs" :disabled="exporting" @click="exportCsv('monthly.csv', { year, month })">导出月报 CSV</button>
      </div>

      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="monthly.length === 0" class="empty-state"><p>该月暂无记工数据</p></div>
        <!-- 桌面表格 -->
        <div class="hidden lg:block overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="bg-muted text-muted-foreground text-xs">
                <th class="text-left font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap">工人</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">出勤（天）</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">工时（小时）</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">计件数量</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">应发工资</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">借支</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">净额</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              <tr v-for="r in monthly" :key="r.worker_id" class="hover:bg-muted/50">
                <td class="px-4 py-3 font-semibold text-foreground">{{ r.worker_name }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ r.days }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ r.hours }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ r.pieces }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ fmtMoney(r.work_amount_cents) }}</td>
                <td class="px-4 py-3 text-right tabular-nums text-red-500">{{ r.advance_cents > 0 ? '-' + fmtMoney(r.advance_cents) : '—' }}</td>
                <td class="px-4 py-3 text-right tabular-nums font-bold" :class="r.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">{{ fmtMoney(r.payable_cents) }}</td>
              </tr>
            </tbody>
            <tfoot v-if="monthly.length > 1">
              <tr class="bg-muted/50 font-bold">
                <td class="px-4 py-3">合计</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ totalRow.days }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ totalRow.hours }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ totalRow.pieces }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ fmtMoney(totalRow.work) }}</td>
                <td class="px-4 py-3 text-right tabular-nums text-red-500">-{{ fmtMoney(totalRow.advance) }}</td>
                <td class="px-4 py-3 text-right tabular-nums">{{ fmtMoney(totalRow.net) }}</td>
              </tr>
            </tfoot>
          </table>
        </div>
        <!-- 移动卡片 -->
        <div class="lg:hidden divide-y divide-border">
          <div v-for="r in monthly" :key="r.worker_id" class="p-4">
            <div class="flex items-center justify-between">
              <span class="font-semibold text-foreground">{{ r.worker_name }}</span>
              <span class="font-bold tabular-nums" :class="r.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">{{ fmtMoney(r.payable_cents) }}</span>
            </div>
            <div class="text-xs text-muted-foreground mt-1">
              出勤 {{ r.days }} 天 · 工时 {{ r.hours }} 时 · 计件 {{ r.pieces }} · 应发 {{ fmtMoney(r.work_amount_cents) }}
              <template v-if="r.advance_cents > 0"> · 借支 {{ fmtMoney(r.advance_cents) }}</template>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- 按工地 -->
    <template v-else-if="tab === 'project'">
      <div class="surface rounded-2xl p-3 flex flex-wrap items-center gap-2">
        <SelectMenu v-model="pYear" :options="yearOptions.map(y => ({ value: y, label: `${y} 年` }))" class="!py-2 text-sm" @change="loadProject" />
        <SelectMenu v-model="pMonth" :options="monthOptions" class="!py-2 text-sm" @change="loadProject" />
      </div>

      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="projectRows.length === 0" class="empty-state"><p>该月暂无数据</p></div>
        <!-- 桌面表格 -->
        <div class="hidden lg:block overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="bg-muted text-muted-foreground text-xs">
                <th class="text-left font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap">工地</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">记工笔数</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">应发工资</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">借支</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">净工资</th>
                <th class="text-right font-semibold px-2.5 sm:px-4 py-2 sm:py-3 whitespace-nowrap tabular-nums">已结算</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              <tr v-for="r in projectRows" :key="r.project_name" class="hover:bg-muted/50">
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 font-semibold text-foreground">{{ r.project_name }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums">{{ r.record_count }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums">{{ fmtMoney(r.work_amount_cents) }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums text-red-500">{{ r.advance_cents > 0 ? '-' + fmtMoney(r.advance_cents) : '—' }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums font-bold" :class="r.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">{{ fmtMoney(r.payable_cents) }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums">{{ fmtMoney(r.settled_cents) }}</td>
              </tr>
            </tbody>
            <tfoot v-if="projectRows.length > 0">
              <tr class="bg-muted/50 font-bold">
                <td class="px-2.5 sm:px-4 py-2 sm:py-3">合计</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums">{{ projectTotal.record_count }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums">{{ fmtMoney(projectTotal.work_amount_cents) }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums text-red-500">-{{ fmtMoney(projectTotal.advance_cents) }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums">{{ fmtMoney(projectTotal.payable_cents) }}</td>
                <td class="px-2.5 sm:px-4 py-2 sm:py-3 text-right tabular-nums">{{ fmtMoney(projectTotal.settled_cents) }}</td>
              </tr>
            </tfoot>
          </table>
        </div>
        <!-- 移动卡片 -->
        <div class="lg:hidden divide-y divide-border">
          <div v-for="r in projectRows" :key="r.project_name" class="p-4">
            <div class="flex items-center justify-between gap-2">
              <span class="font-semibold text-foreground">{{ r.project_name }}</span>
              <span class="font-bold tabular-nums" :class="r.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">{{ fmtMoney(r.payable_cents) }}</span>
            </div>
            <div class="text-xs text-muted-foreground mt-1">
              {{ r.record_count }} 笔 · 工资 {{ fmtMoney(r.work_amount_cents) }}<template v-if="r.advance_cents > 0"> · 借支 {{ fmtMoney(r.advance_cents) }}</template> · 已结 {{ fmtMoney(r.settled_cents) }}
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- 按时间段查询 -->
    <template v-else>
      <div class="surface rounded-2xl p-3 flex flex-wrap gap-2">
        <SelectMenu v-model="rWorker" :options="workerFilterOptions" class="!py-2 text-sm" @change="loadRange" />
        <SelectMenu v-model="rProject" :options="projectFilterOptions" class="!py-2 text-sm" @change="loadRange" />
        <input type="date" v-model="rStart" class="input-field !w-auto !py-2 text-sm" @change="loadRange" />
        <span class="self-center text-muted-foreground text-sm">至</span>
        <input type="date" v-model="rEnd" class="input-field !w-auto !py-2 text-sm" @change="loadRange" />
        <div class="flex-1"></div>
        <button type="button" class="btn-ghost !py-2 text-xs" :disabled="exporting" @click="exportCsv('records.csv', { worker_id: rWorker || undefined, start: rStart, end: rEnd })">导出明细 CSV</button>
      </div>

      <div v-if="rangeSummary" class="grid grid-cols-3 gap-3">
        <div class="surface rounded-2xl p-4">
          <div class="text-xs text-muted-foreground">应发工资</div>
          <div class="text-lg font-extrabold tabular-nums mt-1 font-display">{{ fmtMoney(rangeSummary.work_amount_cents) }}</div>
        </div>
        <div class="surface rounded-2xl p-4">
          <div class="text-xs text-muted-foreground">借支</div>
          <div class="text-lg font-extrabold text-red-500 tabular-nums mt-1 font-display">{{ fmtMoney(rangeSummary.advance_cents) }}</div>
        </div>
        <div class="surface rounded-2xl p-4">
          <div class="text-xs text-muted-foreground">记录笔数</div>
          <div class="text-lg font-extrabold tabular-nums mt-1 font-display">{{ rangeSummary.record_count }}</div>
        </div>
      </div>

      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="rangeRecords.length === 0" class="empty-state"><p>该时间段暂无记录</p></div>
        <div v-else class="divide-y divide-border">
          <div v-for="r in rangeRecords" :key="r.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
            <span class="w-10 h-10 rounded-xl bg-muted flex items-center justify-center text-base shrink-0">
              {{ r.type === 'day' ? '📅' : r.type === 'hour' ? '⏱️' : '📦' }}
            </span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-semibold text-foreground">{{ r.worker_name }}</span>
                <span class="badge" :class="r.settlement_id ? 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400' : 'bg-amber-400/15 text-amber-600 dark:text-amber-500'">
                  {{ r.settlement_id ? '已结算' : '未结算' }}
                </span>
              </div>
              <div class="text-xs text-muted-foreground mt-0.5">
                {{ r.date }} · {{ TYPE_LABELS[r.type] }} {{ r.quantity }}{{ r.type === 'day' ? '天' : r.type === 'hour' ? '时' : (r.piece_unit || '件') }}
                × {{ fmtMoney(r.unit_price_cents) }}
                <span v-if="r.project_name"> · {{ r.project_name }}</span>
                <span v-if="r.piece_name"> · {{ r.piece_name }}</span>
              </div>
            </div>
            <span class="font-bold tabular-nums shrink-0">{{ fmtMoney(r.amount_cents) }}</span>
          </div>
        </div>
      </div>
    </template>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Toast from '../components/Toast.vue'
import { statsMonthly, statsRange, statsByProject, listWorkers, listProjects, downloadExport, type MonthlyStatRow, type ProjectStatRow, type WorkRecord, type Worker, type Project } from '../api/worklog'
import { fmtMoney, TYPE_LABELS, todayStr } from '../utils/format'

// 导出：走带认证的 blob 下载（<a href> 在直连端口会 401）
const exporting = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}
async function exportCsv(path: string, params: Record<string, unknown>) {
  if (exporting.value) return
  exporting.value = true
  try {
    await downloadExport(path, params)
    toast('已开始下载')
  } catch (e: any) {
    toast(e.response?.data?.message || '导出失败，请稍后重试', 'error')
  } finally {
    exporting.value = false
  }
}

const tab = ref<'monthly' | 'project' | 'range'>('monthly')
const now = new Date()
const year = ref(now.getFullYear())
const month = ref(now.getMonth() + 1)
const yearOptions = Array.from({ length: 6 }, (_, i) => now.getFullYear() - 5 + i)
// 下拉选项：筛选含「全部工人 / 全部工地」空值项
const monthOptions = Array.from({ length: 12 }, (_, i) => ({ value: i + 1, label: `${i + 1} 月` }))
const workerFilterOptions = computed(() => [
  { value: '', label: '全部工人' },
  ...workers.value.map(w => ({ value: w.id as number, label: w.name })),
])
const projectFilterOptions = computed(() => [
  { value: '', label: '全部工地' },
  ...projects.value.map(p => ({ value: p.id as number, label: p.name })),
])
const mProject = ref('')

const rWorker = ref('')
const rProject = ref('')
const rStart = ref(todayStr().slice(0, 8) + '01')
const rEnd = ref(todayStr())

const monthly = ref<MonthlyStatRow[]>([])
const rangeRecords = ref<WorkRecord[]>([])
const rangeSummary = ref<{ work_amount_cents: number; advance_cents: number; record_count: number } | null>(null)
const workers = ref<Worker[]>([])
const projects = ref<Project[]>([])

// 按工地统计：年份/月份独立于月度页签，切换页签时才加载
const pYear = ref(now.getFullYear())
const pMonth = ref(now.getMonth() + 1)
// 只放真实工地行（含「未分工地」）；合计单独存，避免表格里出现两遍合计
const projectRows = ref<ProjectStatRow[]>([])
const emptyProjectTotal = (): ProjectStatRow => ({
  project_name: '合计', record_count: 0, work_amount_cents: 0, advance_cents: 0, payable_cents: 0, settled_cents: 0,
})
const projectTotal = ref<ProjectStatRow>(emptyProjectTotal())

async function loadProject() {
  try {
    const res = await statsByProject({ year: pYear.value, month: pMonth.value })
    if (res.data?.code === 0) {
      projectRows.value = res.data.data.rows ?? []
      projectTotal.value = res.data.data.total ?? emptyProjectTotal()
    }
  } catch {}
}

const totalRow = computed(() => {
  const t = { days: 0, hours: 0, pieces: 0, work: 0, advance: 0, net: 0 }
  for (const r of monthly.value) {
    t.days += r.days
    t.hours += r.hours
    t.pieces += r.pieces
    t.work += r.work_amount_cents
    t.advance += r.advance_cents
    t.net += r.payable_cents
  }
  t.days = +t.days.toFixed(2)
  t.hours = +t.hours.toFixed(2)
  t.pieces = +t.pieces.toFixed(2)
  return t
})

async function loadMonthly() {
  try {
    const res = await statsMonthly({ year: year.value, month: month.value, project_id: mProject.value || undefined })
    if (res.data?.code === 0) monthly.value = res.data.data.rows
  } catch {}
}

async function loadRange() {
  try {
    const res = await statsRange({ worker_id: rWorker.value || undefined, project_id: rProject.value || undefined, start: rStart.value, end: rEnd.value })
    if (res.data?.code === 0) {
      rangeRecords.value = res.data.data.records
      rangeSummary.value = res.data.data
    }
  } catch {}
}

watch(tab, (t) => {
  if (t === 'project' && projectRows.value.length === 0) loadProject()
})

onMounted(async () => {
  await loadMonthly()
  try {
    const [w, p] = await Promise.all([listWorkers(), listProjects()])
    if (w.data?.code === 0) workers.value = w.data.data
    if (p.data?.code === 0) projects.value = p.data.data
  } catch {}
})
</script>
