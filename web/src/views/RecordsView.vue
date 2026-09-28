<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="记工" :description="isWorker ? '给自己记工：点工 / 点时 / 计件 / 休息' : '点工 / 点时 / 计件 / 休息，选人填数，工资自动算'">
      <template #actions>
        <span v-if="isWorker && store.me?.self_worker_name" class="badge bg-brand-500/15 text-brand-600 dark:text-brand-400 !py-2 !px-3 text-xs">
          👷 我是工人 · {{ store.me.self_worker_name }}
        </span>
      </template>
    </PageHeader>

    <!-- 日期横滑条 -->
    <div class="surface rounded-2xl p-2 sm:p-3 max-w-full">
      <div class="flex items-center gap-1.5 sm:gap-2 min-w-0">
        <button class="btn-ghost !px-2.5 !py-1.5 sm:!px-3 sm:!py-2 shrink-0" @click="shiftDays(-7)">‹</button>
        <div class="flex-1 min-w-0 flex gap-1.5 sm:gap-2 overflow-x-auto no-scrollbar" ref="dateStrip">
          <button
            v-for="d in dateOptions"
            :key="d"
            class="date-chip shrink-0"
            :class="d === selectedDate ? 'date-chip-active' : ''"
            @click="selectDate(d)"
          >
            <span class="text-[11px] opacity-80 leading-tight">{{ dayLabel(d) }}</span>
            <span class="text-base font-bold leading-tight">{{ d.slice(8) }}</span>
          </button>
        </div>
        <button class="btn-ghost !px-2.5 !py-1.5 sm:!px-3 sm:!py-2 shrink-0" @click="shiftDays(7)">›</button>
        <input type="date" class="input-field !w-36 !py-1.5 !px-2 text-sm shrink-0 hidden sm:block" v-model="selectedDate" @change="loadDay" />
      </div>
      <div class="sm:hidden mt-2">
        <input type="date" class="input-field !py-1.5 text-sm" v-model="selectedDate" @change="loadDay" />
      </div>
    </div>

    <!-- 当日汇总 -->
    <div v-if="dayTotalCents > 0" class="surface rounded-2xl px-4 py-2.5 flex items-center justify-between">
      <span class="text-xs sm:text-sm text-muted-foreground">当日 {{ selectedDate }} 共 {{ dayRecordCount }} 笔</span>
      <span class="text-base sm:text-lg font-extrabold text-brand-600 dark:text-brand-400 tabular-nums font-display">{{ fmtMoney(dayTotalCents) }}</span>
    </div>

    <!-- 工人出勤列表 -->
    <div class="surface rounded-2xl divide-y divide-border overflow-hidden">
      <div v-if="rows.length === 0" class="empty-state">
        <p>还没有工人，先去 <RouterLink to="/admin/workers" class="text-brand-600 font-semibold">添加工人</RouterLink> 吧</p>
      </div>
      <div v-for="row in rows" :key="row.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
        <div class="w-10 h-10 sm:w-11 sm:h-11 rounded-full flex items-center justify-center text-white font-bold shrink-0"
             :class="row.record_count > 0 ? 'bg-brand-gradient' : 'bg-muted-foreground/40'">
          {{ row.name.charAt(0) }}
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class="font-semibold text-foreground truncate">{{ row.name }}</span>
            <span v-if="row.status === 'archived'" class="badge bg-muted text-muted-foreground">已归档</span>
          </div>
          <div v-if="row.record_count > 0" class="text-xs text-muted-foreground mt-0.5">
            <span v-if="row.days !== '0'">点工 {{ row.days }} 天</span>
            <span v-if="row.hours !== '0'"> · 点时 {{ row.hours }} 时</span>
            <span v-if="row.pieces !== '0'"> · 计件 {{ row.pieces }}</span>
            <span v-if="hasRest(row)"> · 休</span>
            <span v-if="row.days === '0' && row.hours === '0' && row.pieces === '0' && hasRest(row)">休息</span>
          </div>
          <div v-else class="text-xs text-muted-foreground mt-0.5">
            日薪 {{ fmtMoney(row.daily_wage_cents) }} · 时薪 {{ fmtMoney(row.hourly_wage_cents) }}
          </div>
        </div>
        <div class="text-right shrink-0 mr-1" v-if="row.record_count > 0">
          <div class="font-bold text-foreground tabular-nums text-sm">{{ fmtMoney(row.amount_cents) }}</div>
        </div>
        <div class="flex gap-1.5 shrink-0">
          <button v-if="row.record_count > 0" class="btn-ghost !px-2 !py-1.5 sm:!px-3 sm:!py-2 text-xs" @click="openEditor(row)">详情</button>
          <button class="btn-brand !px-2 !py-1.5 sm:!px-3 sm:!py-2 text-xs whitespace-nowrap" @click="openEditor(row)">+ 记工</button>
        </div>
      </div>
    </div>

    <!-- 录入抽屉 -->
    <Modal v-model="editorOpen" :title="editingWorker ? `给 ${editingWorker.name} 记工 · ${selectedDate}` : ''">
      <div class="space-y-3.5 sm:space-y-4">
        <!-- 类型切换 -->
        <div class="grid grid-cols-4 gap-1.5 sm:gap-2 sm:grid-cols-3">
          <button v-for="t in types" :key="t.value" class="py-2 sm:py-2.5 rounded-xl text-xs sm:text-sm font-semibold border transition-all"
                  :class="form.type === t.value ? 'bg-brand-gradient text-white border-transparent shadow-glow' : 'border-border text-muted-foreground hover:bg-muted'"
                  @click="form.type = t.value as any; onTypeChange()">
            <div class="text-lg sm:text-xl leading-tight">{{ t.icon }}</div>{{ t.label }}
          </button>
        </div>

        <!-- 数量 -->
        <div v-if="form.type !== 'rest'">
          <label class="text-sm font-medium text-foreground block mb-1.5">{{ quantityLabel }}</label>
          <div class="flex items-center gap-2">
            <button class="btn-ghost !px-3.5 !py-2.5 sm:!px-4 sm:!py-3 text-lg" @click="stepQuantity(-1)">−</button>
            <input v-model="form.quantity" type="text" inputmode="decimal"
                   class="input-field text-center text-lg font-bold tabular-nums" placeholder="0" />
            <button class="btn-ghost !px-3.5 !py-2.5 sm:!px-4 sm:!py-3 text-lg" @click="stepQuantity(1)">＋</button>
          </div>
          <div v-if="form.type === 'day'" class="flex gap-2 mt-2">
            <button class="btn-ghost flex-1 !py-1.5 text-xs" @click="form.quantity = '1'">整天</button>
            <button class="btn-ghost flex-1 !py-1.5 text-xs" @click="form.quantity = '0.5'">半天</button>
            <button class="btn-ghost flex-1 !py-1.5 text-xs" @click="form.quantity = '1.5'">一天半</button>
          </div>
        </div>

        <!-- 计件项目 -->
        <div v-if="form.type === 'piece'">
          <label class="text-sm font-medium text-foreground block mb-1.5">计件项目</label>
          <SelectMenu v-model="form.piece_item_id" :options="pieceItemOptions" class="w-full" @change="applyDefaultPrice" />
        </div>

        <!-- 工地 -->
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">工地（选填）</label>
          <SelectMenu v-model="form.project_id" :options="projectOptions" class="w-full" />
        </div>

        <!-- 单价 -->
        <div v-if="form.type !== 'rest'">
          <label class="text-sm font-medium text-foreground block mb-1.5">单价（元）</label>
          <input v-model="priceInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" />
        </div>

        <!-- 备注 -->
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注（选填）</label>
          <input v-model="form.note" type="text" class="input-field" placeholder="如：加班、雨天停工半天" />
        </div>

        <!-- 多天连记 -->
        <div v-if="!editingId">
          <button class="text-sm text-brand-600 dark:text-brand-400 font-semibold" @click="multiDay = !multiDay">
            {{ multiDay ? '▾' : '▸' }} 多天连记（同样的工连记多天）
          </button>
          <div v-if="multiDay" class="grid grid-cols-4 sm:grid-cols-7 gap-1.5 mt-2">
            <button v-for="d in multiDayOptions" :key="d" class="py-1.5 rounded-lg text-xs font-semibold border transition-all"
                    :class="multiDays.includes(d) ? 'bg-brand-gradient text-white border-transparent' : 'border-border text-muted-foreground'"
                    @click="toggleMultiDay(d)">
              {{ d.slice(5) }}
            </button>
          </div>
        </div>

        <!-- 金额预览 -->
        <div class="rounded-xl sm:rounded-2xl bg-muted p-3 sm:p-4 flex items-center justify-between">
          <span class="text-sm text-muted-foreground">{{ form.type === 'rest' ? '休息不计工资' : multiDays.length > 1 ? `共 ${multiDays.length} 天合计` : '本次工资' }}</span>
          <span class="text-xl sm:text-2xl font-extrabold text-brand-600 dark:text-brand-400 font-display tabular-nums">{{ previewAmount }}</span>
        </div>

        <div class="flex gap-2 pt-0.5 sm:pt-1">
          <button v-if="editingId" class="btn-ghost flex-1 !text-red-500" @click="removeRecord">删除</button>
          <button v-if="!editingId" class="btn-ghost flex-1" :disabled="saving" @click="saveRecord(true)">确认并再记一笔</button>
          <button class="btn-brand flex-1" :disabled="saving" @click="saveRecord(false)">{{ editingId ? '保存修改' : '确认记工' }}</button>
        </div>

        <!-- 当日已有记录 -->
        <div v-if="editingWorker && editingWorker.records.length > 0" class="pt-2 border-t border-border">
          <div class="text-xs font-bold text-muted-foreground uppercase tracking-widest mb-2">当日已记 {{ editingWorker.record_count }} 笔 · 合计 {{ fmtMoney(editingWorker.amount_cents) }}</div>
          <div class="divide-y divide-border">
            <button v-for="r in editingWorker.records" :key="r.id" class="w-full flex items-center justify-between py-2 text-left" @click="editRecord(r)">
              <span class="text-sm text-foreground">
                <span class="badge bg-muted text-muted-foreground mr-1.5">{{ TYPE_LABELS[r.type] }}</span>
                {{ r.quantity }}{{ r.type === 'day' ? ' 天' : r.type === 'hour' ? ' 时' : ' 件' }}
                <span v-if="r.piece_name" class="text-muted-foreground">· {{ r.piece_name }}</span>
              </span>
              <span class="text-sm font-semibold tabular-nums" :class="r.id === editingId ? 'text-brand-600' : ''">{{ fmtMoney(r.amount_cents) }}</span>
            </button>
          </div>
        </div>
      </div>
    </Modal>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Modal from '../components/Modal.vue'
import Toast from '../components/Toast.vue'
import {
  recordsByDate, createRecord, updateRecord, deleteRecord,
  listWorkers, listProjects, listPieceItems,
  type DayWorkerRow, type WorkRecord, type Project, type PieceItem,
} from '../api/worklog'
import { fmtMoney, TYPE_LABELS, todayStr, addDays } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const isWorker = computed(() => store.isWorker)

const rows = ref<DayWorkerRow[]>([])
const projects = ref<Project[]>([])
// 下拉选项：计件项目带单价提示，工地含「不选」空值项
const pieceItemOptions = computed(() => [
  { value: '', label: '请选择' },
  ...pieceItems.value.map(p => ({ value: p.id as number, label: `${p.name}（${p.unit} · ${fmtMoney(p.unit_price_cents)}/${p.unit}）` })),
])
const projectOptions = computed(() => [
  { value: '', label: '不选' },
  ...projects.value.map(p => ({ value: p.id as number, label: p.name })),
])
const pieceItems = ref<PieceItem[]>([])
const selectedDate = ref(todayStr())
const editorOpen = ref(false)
const editingWorker = ref<DayWorkerRow | null>(null)
const editingId = ref<number | null>(null)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const dateStrip = ref<HTMLElement | null>(null)

const types = [
  { value: 'day', label: '点工（天）', icon: '📅' },
  { value: 'hour', label: '点时（小时）', icon: '⏱️' },
  { value: 'piece', label: '计件', icon: '📦' },
  { value: 'rest', label: '休息', icon: '🛌' },
]

const form = ref({
  type: 'day' as 'day' | 'hour' | 'piece' | 'rest',
  quantity: '1',
  project_id: '' as number | '',
  piece_item_id: '' as number | '',
  note: '',
})
const priceInput = ref('')
const multiDay = ref(false)
const multiDays = ref<string[]>([])
const multiDayOptions = computed(() => Array.from({ length: 14 }, (_, i) => addDays(selectedDate.value, -i)))

function toggleMultiDay(d: string) {
  const i = multiDays.value.indexOf(d)
  if (i >= 0) multiDays.value.splice(i, 1)
  else multiDays.value.push(d)
}

const dateOptions = ref<string[]>([])
function rebuildDateOptions() {
  const base = selectedDate.value
  dateOptions.value = Array.from({ length: 15 }, (_, i) => addDays(base, i - 7))
}

function dayLabel(d: string) {
  const today = todayStr()
  if (d === today) return '今天'
  if (d === addDays(today, 1)) return '明天'
  if (d === addDays(today, -1)) return '昨天'
  const date = new Date(d + 'T00:00:00')
  return ['周日', '周一', '周二', '周三', '周四', '周五', '周六'][date.getDay()]
}

const quantityLabel = computed(() => form.value.type === 'day' ? '天数（0.5 步进）' : form.value.type === 'hour' ? '小时数（0.25 步进）' : '件数')

const stepSize = computed(() => (form.value.type === 'day' ? 0.5 : form.value.type === 'hour' ? 0.25 : 1))

function stepQuantity(dir: number) {
  const cur = parseFloat(form.value.quantity) || 0
  const next = Math.max(0, +(cur + dir * stepSize.value).toFixed(2))
  form.value.quantity = String(next)
}

function onTypeChange() {
  applyDefaultPrice()
  if (form.value.type === 'rest') form.value.quantity = '0'
  else if (form.value.type === 'piece' && form.value.quantity === '1') form.value.quantity = '100'
}

function applyDefaultPrice() {
  if (!editingWorker.value) return
  const w = editingWorker.value
  if (form.value.type === 'day') priceInput.value = w.daily_wage_cents ? (w.daily_wage_cents / 100).toFixed(2) : ''
  else if (form.value.type === 'hour') priceInput.value = w.hourly_wage_cents ? (w.hourly_wage_cents / 100).toFixed(2) : ''
  else if (form.value.type === 'piece') {
    const p = pieceItems.value.find((x) => x.id === Number(form.value.piece_item_id))
    priceInput.value = p?.unit_price_cents ? (p.unit_price_cents / 100).toFixed(2) : ''
  }
}

const previewAmount = computed(() => {
  if (form.value.type === 'rest') return fmtMoney(0)
  const q = parseFloat(form.value.quantity) || 0
  const price = parseFloat(priceInput.value) || 0
  const one = Math.round(q * price * 100)
  return fmtMoney(one * Math.max(1, multiDays.value.length))
})

const dayTotalCents = computed(() => rows.value.reduce((s, r) => s + r.amount_cents, 0))
const dayRecordCount = computed(() => rows.value.reduce((s, r) => s + r.record_count, 0))

async function loadDay() {
  rebuildDateOptions()
  try {
    const res = await recordsByDate(selectedDate.value)
    if (res.data?.code === 0) rows.value = res.data.data.workers
  } catch {}
}

function selectDate(d: string) {
  selectedDate.value = d
}

watch(selectedDate, loadDay)

function shiftDays(delta: number) {
  selectedDate.value = addDays(selectedDate.value, delta)
}

function openEditor(row: DayWorkerRow) {
  editingWorker.value = row
  editingId.value = null
  form.value = { type: 'day', quantity: '1', project_id: '', piece_item_id: '', note: '' }
  priceInput.value = ''
  multiDay.value = false
  multiDays.value = []
  applyDefaultPrice()
  editorOpen.value = true
}

function hasRest(row: DayWorkerRow): boolean {
  return row.records.some((r) => r.type === 'rest')
}

function editRecord(r: WorkRecord) {
  editingId.value = r.id!
  form.value = {
    type: r.type as any,
    quantity: r.quantity,
    project_id: r.project_id ?? '',
    piece_item_id: r.piece_item_id ?? '',
    note: r.note || '',
  }
  priceInput.value = (r.unit_price_cents / 100).toFixed(2)
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

async function saveRecord(keepOpen: boolean) {
  if (!editingWorker.value) return
  const isRest = form.value.type === 'rest'
  const priceCents = isRest ? 0 : Math.round((parseFloat(priceInput.value) || 0) * 100)
  if (!isRest && (!form.value.quantity || parseFloat(form.value.quantity) <= 0)) return toast('请填写数量', 'error')
  if (!isRest && priceCents <= 0) return toast('请填写单价', 'error')
  if (form.value.type === 'piece' && !form.value.piece_item_id) return toast('请选择计件项目', 'error')
  saving.value = true
  const dates = !editingId.value && multiDay.value && multiDays.value.length > 0
    ? multiDays.value
    : [selectedDate.value]
  let ok = 0
  try {
    for (const d of dates) {
      const payload: WorkRecord = {
        worker_id: editingWorker.value.id!,
        date: d,
        type: form.value.type,
        quantity: isRest ? '0' : form.value.quantity,
        unit_price_cents: priceCents,
        project_id: form.value.project_id === '' ? null : Number(form.value.project_id),
        piece_item_id: form.value.piece_item_id === '' ? null : Number(form.value.piece_item_id),
        note: form.value.note,
      }
      if (editingId.value) {
        await updateRecord(editingId.value, payload)
        toast('修改成功')
      } else {
        await createRecord(payload)
        ok++
      }
    }
    if (!editingId.value) {
      if (keepOpen) {
        toast(`已记录，可继续记一笔`)
        form.value.quantity = form.value.type === 'rest' ? '0' : form.value.quantity
        multiDays.value = []
      } else {
        toast(ok > 1 ? `已连记 ${ok} 天` : '记工成功')
        editorOpen.value = false
      }
    } else {
      editorOpen.value = false
    }
    await loadDay()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

async function removeRecord() {
  if (!editingId.value) return
  if (!confirm('确定删除这条记录？')) return
  try {
    await deleteRecord(editingId.value)
    toast('已删除')
    editorOpen.value = false
    await loadDay()
  } catch (e: any) {
    toast(e.response?.data?.message || '删除失败', 'error')
  }
}

onMounted(async () => {
  await loadDay()
  try {
    const [p, pi] = await Promise.all([listProjects({ status: 'active' }), listPieceItems({ status: 'active' })])
    if (p.data?.code === 0) projects.value = p.data.data
    if (pi.data?.code === 0) pieceItems.value = pi.data.data
  } catch {}
  await nextTick()
  dateStrip.value?.querySelector('.date-chip-active')?.scrollIntoView({ inline: 'center', block: 'nearest' })
})
</script>

<style scoped>
.date-chip {
  @apply flex flex-col items-center justify-center w-[52px] sm:w-14 py-1.5 sm:py-2 rounded-xl border border-border text-foreground hover:bg-muted transition-all;
}
.date-chip-active {
  @apply bg-brand-gradient text-white border-transparent shadow-glow;
}
.no-scrollbar::-webkit-scrollbar {
  display: none;
}
</style>
