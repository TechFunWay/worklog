<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="考勤表" :description="isWorker ? '我的出勤日历' : '按工人查看当月出勤日历'">
      <template #actions>
        <SelectMenu v-if="!isWorker && workers.length > 0" v-model="workerId" :options="workers.map(w => ({ value: w.id as number, label: w.name }))" class="!py-2 text-sm" @change="load" />
      </template>
    </PageHeader>

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
            <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ data?.days ?? 0 }} <span class="text-xs font-normal">个工</span></div>
          </div>
          <div class="text-center">
            <div class="text-xs text-white/75">点时</div>
            <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ data?.hours ?? 0 }} <span class="text-xs font-normal">小时</span></div>
          </div>
          <div class="text-center">
            <div class="text-xs text-white/75">计件</div>
            <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ data?.pieces ?? 0 }}</div>
          </div>
          <div class="text-center">
            <div class="text-xs text-white/75">休息</div>
            <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ data?.rest_count ?? 0 }} <span class="text-xs font-normal">天</span></div>
          </div>
          <div class="text-center">
            <div class="text-xs text-white/75">本月工钱</div>
            <div class="text-lg sm:text-xl font-extrabold tabular-nums font-display">{{ fmtYuan(data?.amount_cents ?? 0) }} <span class="text-xs font-normal">元</span></div>
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
              <svg v-if="cell.count > 0 && !cell.rest" class="w-3.5 h-3.5 text-emerald-500" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7"/></svg>
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
      <div class="mt-3 text-xs text-muted-foreground text-center">点日期可去记工（当天有记录打 ✓）</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import { getAttendance, listWorkers, getMe, type AttendanceData, type Worker } from '../api/worklog'
import { fmtYuan, todayStr } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const router = useRouter()
const isWorker = computed(() => store.isWorker)

const now = new Date()
const year = ref(now.getFullYear())
const month = ref(now.getMonth() + 1)
const workerId = ref<number | ''>('')
const workers = ref<Worker[]>([])
const data = ref<AttendanceData | null>(null)
const today = todayStr()

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
  for (const c of data.value?.cells ?? []) byDate.set(c.date, c)
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
  if (cell.rest) return 'border-teal-400/40 bg-teal-400/5'
  if (cell.count > 0) return 'border-brand-400/40 bg-brand-500/5 cursor-pointer hover:bg-brand-500/10'
  return 'border-border'
}

function shiftMonth(delta: number) {
  let m = month.value + delta
  let y = year.value
  if (m < 1) { m = 12; y-- }
  if (m > 12) { m = 1; y++ }
  year.value = y
  month.value = m
  load()
}

function onCellClick(cell: GridCell) {
  if (cell.date && cell.count > 0) router.push(`/admin/records?date=${cell.date}`)
}

async function load() {
  try {
    const params: Record<string, unknown> = { year: year.value, month: month.value }
    if (workerId.value) params.worker_id = workerId.value
    const res = await getAttendance(params)
    if (res.data?.code === 0) data.value = res.data.data
  } catch {}
}

onMounted(async () => {
  await store.load()
  if (!isWorker.value) {
    try {
      const res = await listWorkers({ status: 'active' })
      if (res.data?.code === 0) {
        workers.value = res.data.data
        // 默认选中地址栏/考勤数据返回的工人
        await load()
        if (data.value?.worker?.id != null) workerId.value = data.value.worker.id
      }
    } catch {}
  } else {
    await load()
  }
})
</script>
