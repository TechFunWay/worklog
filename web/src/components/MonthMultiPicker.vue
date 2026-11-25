<template>
  <div class="rounded-xl border border-border bg-surface/60 p-2.5">
    <!-- 月份切换 -->
    <div class="flex items-center justify-between mb-2">
      <button type="button" class="btn-ghost !px-2.5 !py-1 text-sm shrink-0" @click="shiftMonth(-1)">‹ 上月</button>
      <span class="text-sm font-bold text-foreground tabular-nums">{{ ymLabel }}</span>
      <button type="button" class="btn-ghost !px-2.5 !py-1 text-sm shrink-0" @click="shiftMonth(1)">下月 ›</button>
    </div>
    <!-- 星期 -->
    <div class="grid grid-cols-7 gap-1 mb-1">
      <div v-for="w in weekdays" :key="w" class="text-center text-[11px] text-muted-foreground py-1">{{ w }}</div>
    </div>
    <!-- 日期格 -->
    <div class="grid grid-cols-7 gap-1">
      <div v-for="n in leadingBlanks" :key="'b' + n" class="h-11"></div>
      <button
        v-for="d in days"
        :key="d.date"
        type="button"
        class="relative h-11 rounded-lg border text-sm font-semibold flex flex-col items-center justify-center transition-all"
        :class="cellClass(d)"
        :aria-label="d.date + (d.recorded ? '（已有记工）' : '')"
        @click="toggle(d.date)"
      >
        <span class="leading-none">{{ d.day }}</span>
        <span v-if="d.recorded" class="absolute top-0.5 right-1 text-[10px] leading-none opacity-80">✓</span>
      </button>
    </div>
    <!-- 图例与操作 -->
    <div class="mt-2 flex items-center justify-between gap-2 text-[11px] text-muted-foreground">
      <span><span class="opacity-80">✓</span> 当天已有记工 · 蓝底为已选</span>
      <div class="flex items-center gap-2 shrink-0">
        <span v-if="modelValue.length" class="text-brand-600 dark:text-brand-400 font-semibold">已选 {{ modelValue.length }} 天</span>
        <button v-if="modelValue.length" type="button" class="text-muted-foreground hover:text-foreground" @click="clearAll">清空</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, onMounted } from 'vue'
import { getAttendance } from '../api/worklog'
import { todayStr } from '../utils/format'

// 月历多选：给「多天连记」用。按月翻页、标出该工人当月已记工的日期，避免重复记账。
const props = defineProps<{ modelValue: string[]; workerId?: number | null }>()
const emit = defineEmits<{ (e: 'update:modelValue', v: string[]): void }>()

const weekdays = ['日', '一', '二', '三', '四', '五', '六']

function parseYm(s: string | undefined): { y: number; m: number } {
  if (s && /^\d{4}-\d{2}-\d{2}$/.test(s)) {
    const [y, m] = s.split('-').map(Number)
    return { y, m }
  }
  const t = todayStr()
  return { y: Number(t.slice(0, 4)), m: Number(t.slice(5, 7)) }
}

const view = ref(parseYm(props.modelValue[0] || todayStr()))
const recorded = ref<Set<string>>(new Set())
const loading = ref(false)

const ymLabel = computed(() => `${view.value.y}年${view.value.m}月`)
const daysInMonth = computed(() => new Date(view.value.y, view.value.m, 0).getDate())
const firstWeekday = computed(() => new Date(view.value.y, view.value.m - 1, 1).getDay())
const leadingBlanks = computed(() => firstWeekday.value)
const today = todayStr()

const days = computed(() => {
  const out: { date: string; day: number; recorded: boolean }[] = []
  for (let i = 1; i <= daysInMonth.value; i++) {
    const date = `${view.value.y}-${String(view.value.m).padStart(2, '0')}-${String(i).padStart(2, '0')}`
    out.push({ date, day: i, recorded: recorded.value.has(date) })
  }
  return out
})

// 已记工的日期：复用考勤接口的逐日出勤数据（cells 带 record_count），无后端改动。
async function loadRecorded() {
  const wid = props.workerId
  if (!wid) { recorded.value = new Set(); return }
  loading.value = true
  try {
    const res = await getAttendance({ worker_id: wid, year: view.value.y, month: view.value.m })
    const cells = res.data?.code === 0 ? (res.data.data?.cells || []) : []
    const s = new Set<string>()
    for (const c of cells) if (c.record_count > 0) s.add(c.date)
    recorded.value = s
  } catch {
    // 拉不到标记不挡住记工，只是不显示 ✓
    recorded.value = new Set()
  } finally {
    loading.value = false
  }
}

watch(() => [view.value.y, view.value.m, props.workerId], loadRecorded)
onMounted(loadRecorded)
// 选中集合里已有别的月份时，切到那个月更顺手
watch(() => props.modelValue, (v) => {
  if (v && v.length === 1) {
    const { y, m } = parseYm(v[0])
    if (y !== view.value.y || m !== view.value.m) view.value = { y, m }
  }
})

function shiftMonth(delta: number) {
  const d = new Date(view.value.y, view.value.m - 1 + delta, 1)
  view.value = { y: d.getFullYear(), m: d.getMonth() + 1 }
}

function cellClass(d: { date: string; recorded: boolean }) {
  if (props.modelValue.includes(d.date)) return 'bg-brand-gradient text-white border-transparent shadow-glow'
  if (d.date === today) return 'border-brand-500 text-foreground bg-brand-500/5'
  if (d.recorded) return 'border-border bg-muted text-muted-foreground'
  return 'border-border text-foreground hover:bg-muted'
}

function toggle(date: string) {
  const cur = props.modelValue.slice()
  const i = cur.indexOf(date)
  if (i >= 0) cur.splice(i, 1)
  else cur.push(date)
  cur.sort()
  emit('update:modelValue', cur)
}

function clearAll() {
  emit('update:modelValue', [])
}
</script>
