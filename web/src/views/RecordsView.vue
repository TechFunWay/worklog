<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="记工" :description="isWorker ? '给自己记工：点工 / 点时 / 计件 / 休息' : '点工 / 点时 / 计件 / 休息，选人填数，工资自动算'">
      <template #actions>
        <span v-if="isWorker && store.me?.self_worker_name" class="badge bg-brand-500/15 text-brand-600 dark:text-brand-400 !py-2 !px-3 text-xs">
          👷 我是工人 · {{ store.me.self_worker_name }}
        </span>
      </template>
    </PageHeader>

    <!-- 工地上下文：切换后当日列表/合计只显示该工地的记录，新记工默认落在这里 -->
    <div class="surface rounded-2xl p-2 sm:p-3">
      <div class="flex items-center gap-2 min-w-0">
        <span class="shrink-0 text-xs sm:text-sm text-muted-foreground">工地</span>
        <SelectMenu v-model="projectFilter" :options="projectFilterOptions" class="!py-1.5 sm:!py-2 text-sm max-w-[60vw] sm:max-w-none" />
        <span class="ml-auto min-w-0 truncate text-[11px] sm:text-xs text-muted-foreground">{{ projectHint }}</span>
      </div>
    </div>

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
        <input type="date" class="input-field !w-36 !py-1.5 !px-2 text-sm shrink-0 hidden sm:block" v-model="selectedDate" />
      </div>
      <div class="sm:hidden mt-2">
        <input type="date" class="input-field !py-1.5 text-sm" v-model="selectedDate" />
      </div>
    </div>

    <!-- 当日汇总 -->
    <div v-if="dayTotalCents > 0" class="surface rounded-2xl px-4 py-2.5 flex items-center justify-between">
      <span class="text-xs sm:text-sm text-muted-foreground">当日{{ projectLabel ? ` · ${projectLabel}` : '' }} 共 {{ dayRecordCount }} 笔</span>
      <span class="text-base sm:text-lg font-extrabold text-brand-600 dark:text-brand-400 tabular-nums font-display">{{ fmtMoney(dayTotalCents) }}</span>
    </div>

    <!-- 工人出勤列表：桌面行 / 手机独立卡片两套并存 -->
    <div class="surface rounded-2xl overflow-hidden">
      <div v-if="rows.length === 0" class="empty-state">
        <p>还没有工人，先去 <RouterLink to="/admin/workers" class="text-brand-600 font-semibold">添加工人</RouterLink> 吧</p>
      </div>
      <template v-else>
        <!-- 桌面：行内操作 -->
        <div class="hidden md:block divide-y divide-border">
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
              <button class="btn-ghost !px-2 !py-1.5 sm:!px-3 sm:!py-2 text-xs whitespace-nowrap"
                      :disabled="isQuickPending(row.id)" @click="quickRecord(row)">
                {{ isQuickPending(row.id) ? '记工中…' : '一键记工' }}
              </button>
              <button class="btn-brand !px-2 !py-1.5 sm:!px-3 sm:!py-2 text-xs whitespace-nowrap" @click="openEditor(row)">+ 记工</button>
            </div>
          </div>
        </div>

        <!-- 手机：独立卡片，操作按钮内联在卡片内一行可见 -->
        <div class="md:hidden p-2 space-y-1.5">
          <div v-for="row in rows" :key="row.id" class="rounded-xl border border-border bg-surface/80 px-3 py-2.5 space-y-1.5">
            <div class="flex items-center gap-2">
              <div class="w-9 h-9 rounded-full flex items-center justify-center text-white text-sm font-bold shrink-0"
                   :class="row.record_count > 0 ? 'bg-brand-gradient' : 'bg-muted-foreground/40'">
                {{ row.name.charAt(0) }}
              </div>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5">
                  <span class="font-semibold text-foreground truncate">{{ row.name }}</span>
                  <span v-if="row.status === 'archived'" class="badge bg-muted text-muted-foreground !px-1.5 !py-0 text-[10px]">已归档</span>
                </div>
                <div v-if="row.record_count > 0" class="mt-0.5 truncate text-[11px] text-muted-foreground">
                  <span v-if="row.days !== '0'">点工 {{ row.days }} 天</span>
                  <span v-if="row.hours !== '0'"> · 点时 {{ row.hours }} 时</span>
                  <span v-if="row.pieces !== '0'"> · 计件 {{ row.pieces }}</span>
                  <span v-if="hasRest(row)"> · 休</span>
                </div>
                <div v-else class="mt-0.5 truncate text-[11px] text-muted-foreground">
                  日薪 {{ fmtMoney(row.daily_wage_cents) }} · 时薪 {{ fmtMoney(row.hourly_wage_cents) }}
                </div>
              </div>
              <span v-if="row.record_count > 0" class="shrink-0 text-sm font-bold tabular-nums text-foreground">{{ fmtMoney(row.amount_cents) }}</span>
            </div>
            <div class="flex gap-1.5">
              <button v-if="row.record_count > 0" class="btn-ghost flex-1 !px-2 !py-1.5 text-xs" @click="openEditor(row)">详情</button>
              <button class="btn-ghost flex-1 !px-2 !py-1.5 text-xs whitespace-nowrap"
                      :disabled="isQuickPending(row.id)" @click="quickRecord(row)">
                {{ isQuickPending(row.id) ? '记工中…' : '一键记工' }}
              </button>
              <button class="btn-brand flex-1 !px-2 !py-1.5 text-xs whitespace-nowrap" @click="openEditor(row)">+ 记工</button>
            </div>
          </div>
        </div>
      </template>
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

        <!-- 月薪工提示：自动折算日薪 + 缺勤扣款口径（只有单价确实来自月薪折算时才提示） -->
        <div v-if="isMonthlyWorker && form.type === 'day' && priceSource === 'monthly'" class="rounded-xl bg-brand-500/10 border border-brand-500/20 px-3 py-2 text-xs text-brand-700 dark:text-brand-300">
          {{ editingWorker?.name }} 是月薪工（月薪 {{ fmtMoney(editingWorker?.monthly_wage_cents ?? 0) }}），单价已按当月天数自动折算日薪；缺勤一天不记工即扣一天折算日薪。
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
          <SelectMenu v-model="form.project_id" :options="projectOptions" class="w-full" @change="onProjectChange" />
        </div>

        <!-- 单价：按工地单价 / 工人单价自动带出，来源小字说明，可手动改 -->
        <div v-if="form.type !== 'rest'">
          <label class="text-sm font-medium text-foreground block mb-1.5">单价（元）</label>
          <input v-model="priceInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" @input="priceTouched = true" />
          <p v-if="priceSourceText" class="mt-1 text-xs text-muted-foreground">{{ priceSourceText }}</p>
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
          <span class="text-sm text-muted-foreground">{{ form.type === 'rest' ? '休息不计工资' : multiDays.length > 1 ? `共 ${multiDays.length} 天合计` : isMonthlyWorker && form.type === 'day' && priceSource === 'monthly' ? '本次工资（月薪折算）' : '本次工资' }}</span>
          <span class="text-xl sm:text-2xl font-extrabold text-brand-600 dark:text-brand-400 font-display tabular-nums">{{ previewAmount }}</span>
        </div>

        <div class="flex gap-2 pt-0.5 sm:pt-1">
          <button v-if="editingId" class="btn-ghost flex-1 !text-red-500" @click="askDeleteRecord(editingId)">删除</button>
          <button class="btn-ghost flex-1" @click="editorOpen = false">取消</button>
          <button v-if="!editingId" class="btn-ghost flex-1" :disabled="saving" @click="saveRecord(true)">确认并再记一笔</button>
          <button class="btn-brand flex-1" :disabled="saving" @click="saveRecord(false)">{{ editingId ? '保存修改' : '确认记工' }}</button>
        </div>

        <!-- 当日已有记录 -->
        <div v-if="editingWorker && editingWorker.records.length > 0" class="pt-2 border-t border-border">
          <div class="text-xs font-bold text-muted-foreground uppercase tracking-widest mb-2">当日已记 {{ editingWorker.record_count }} 笔 · 合计 {{ fmtMoney(editingWorker.amount_cents) }} · 点记录可改，点右侧🗑可删</div>
          <div class="divide-y divide-border">
            <div v-for="r in editingWorker.records" :key="r.id" class="w-full flex items-center justify-between py-1.5 text-left cursor-pointer" @click="editRecord(r)">
              <span class="text-sm text-foreground">
                <span class="badge bg-muted text-muted-foreground mr-1.5">{{ TYPE_LABELS[r.type] }}</span>
                {{ r.quantity }}{{ r.type === 'day' ? ' 天' : r.type === 'hour' ? ' 时' : ' 件' }}
                <span v-if="r.piece_name" class="text-muted-foreground">· {{ r.piece_name }}</span>
              </span>
              <span class="flex items-center gap-1.5 shrink-0">
                <span class="text-sm font-semibold tabular-nums" :class="r.id === editingId ? 'text-brand-600' : ''">{{ fmtMoney(r.amount_cents) }}</span>
                <button type="button" aria-label="删除这条记录" class="p-2 -mr-1.5 text-red-400 hover:text-red-500 transition-colors" @click.stop="askDeleteRecord(r.id!)">
                  <svg class="w-4.5 h-4.5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
                </button>
              </span>
            </div>
          </div>
        </div>
      </div>
    </Modal>

    <ConfirmDialog v-model="deleteConfirmOpen" title="删除记录" message="删除后当日合计、月度统计与工资都会同步更新。确定删除这条记录？" confirm-text="删除" confirm-type="danger" @confirm="removeRecord" />

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch, nextTick, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Toast from '../components/Toast.vue'
import {
  recordsByDate, createRecord, updateRecord, deleteRecord, getEffectivePrice,
  listWorkers, listProjects, listPieceItems,
  type DayWorkerRow, type WorkRecord, type Project, type PieceItem,
  type EffectivePrice, type PriceSource,
} from '../api/worklog'
import { fmtMoney, TYPE_LABELS, todayStr, addDays } from '../utils/format'
import {
  isProjectFilterAvailable, parseProjectFilter, readLastProjectFilter, saveLastProjectFilter,
  type ProjectFilter,
} from '../utils/project-filter'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const route = useRoute()
const isWorker = computed(() => store.isWorker)

// 从考勤表点日期进来时带 ?date=YYYY-MM-DD，记工页要直接落在那一天；
// 之前忽略了这个参数，所以粉丝反馈“点日期记工没反应、还停在今天”。
const DATE_RE = /^\d{4}-\d{2}-\d{2}$/
function queryDate(): string {
  const raw = typeof route.query.date === 'string' ? route.query.date : ''
  return DATE_RE.test(raw) ? raw : ''
}

const rows = ref<DayWorkerRow[]>([])
const projects = ref<Project[]>([])
// 工地上下文：'' 全部工地 / 'none' 未分工地 / 数字 指定工地
const projectFilter = ref<ProjectFilter>('')
// 一键记工进行中的工人（同一工人请求中禁用按钮，防连点重复记工）
const quickPending = ref<Record<number, boolean>>({})
// 下拉选项：计件项目带单价提示，工地含「不选」空值项
const pieceItemOptions = computed(() => [
  { value: '', label: '请选择' },
  ...pieceItems.value.map(p => ({ value: p.id as number, label: `${p.name}（${p.unit} · ${fmtMoney(p.unit_price_cents)}/${p.unit}）` })),
])
const projectOptions = computed(() => [
  { value: '', label: '不选' },
  ...projects.value.map(p => ({ value: p.id as number, label: p.name })),
])
// 工地上下文切换器的选项
const projectFilterOptions = computed(() => [
  { value: '', label: '全部工地' },
  { value: 'none', label: '未分工地' },
  ...projects.value.map(p => ({ value: p.id as number, label: p.name })),
])
// 当前工地上下文的名字（全部工地时为空，汇总文案里不显示）
const projectLabel = computed(() => {
  if (projectFilter.value === '') return ''
  if (projectFilter.value === 'none') return '未分工地'
  return projects.value.find(p => p.id === projectFilter.value)?.name || `工地 #${projectFilter.value}`
})
// 上下文提示：全部工地时一键记工会落到「上次工地」，措辞不能写成「该工地」。
const projectHint = computed(() => {
  if (projectFilter.value === '') return '一键记工默认上次工地'
  if (projectFilter.value === 'none') return '记工默认不分工地'
  return '记工默认记到该工地'
})
const pieceItems = ref<PieceItem[]>([])
const selectedDate = ref(queryDate() || todayStr())
const editorOpen = ref(false)
const editingWorker = ref<DayWorkerRow | null>(null)
const editingId = ref<number | null>(null)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const dateStrip = ref<HTMLElement | null>(null)
// 删除确认（替代原生 confirm）：deleteTargetId 为待删记录，确认后由 removeRecord 执行
const deleteConfirmOpen = ref(false)
const deleteTargetId = ref<number | null>(null)

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
// 单价来源：project_worker 工地单独价 / project 工地默认价 / monthly 月薪折算 /
// worker 工人日薪时薪 / none 未设置；record = 编辑已有记录时保留的原记录单价。
const priceSource = ref<PriceSource | 'record'>('none')
// 自动带价请求序号：连续切工地/类型时只认最后一次响应，避免旧价覆盖新价。
let priceSeq = 0
// 用户是否手动改过单价：异步取价返回得晚时，不覆盖用户已经输入的价（手动填价最高优先）。
const priceTouched = ref(false)
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
  // 切点工/点时后按当前工地重新带价（工地单独价/默认价优先），失败回退本地默认算法
  applyEffectivePrice(true)
  if (form.value.type === 'rest') form.value.quantity = '0'
  else if (form.value.type === 'piece' && form.value.quantity === '1') form.value.quantity = '100'
}

function onProjectChange() {
  applyEffectivePrice(true)
}

// 本地默认价（接口不可用时的回退）：月薪折算日薪 / 工人日薪 / 时薪 / 计件项目单价。
function applyDefaultPrice() {
  if (!editingWorker.value) return
  const w = editingWorker.value
  if (form.value.type === 'day') {
    // 月薪工：按记工日期所在月的天数折算日薪（缺勤不记工即自然扣款）
    if (w.monthly_wage_cents && w.monthly_wage_cents > 0) {
      priceInput.value = (monthlyDailyCents(w.monthly_wage_cents, selectedDate.value) / 100).toFixed(2)
      priceSource.value = 'monthly'
    } else if (w.daily_wage_cents && w.daily_wage_cents > 0) {
      priceInput.value = (w.daily_wage_cents / 100).toFixed(2)
      priceSource.value = 'worker'
    } else {
      priceInput.value = ''
      priceSource.value = 'none'
    }
  } else if (form.value.type === 'hour') {
    if (w.hourly_wage_cents && w.hourly_wage_cents > 0) {
      priceInput.value = (w.hourly_wage_cents / 100).toFixed(2)
      priceSource.value = 'worker'
    } else {
      priceInput.value = ''
      priceSource.value = 'none'
    }
  } else if (form.value.type === 'piece') {
    const p = pieceItems.value.find((x) => x.id === Number(form.value.piece_item_id))
    priceInput.value = p?.unit_price_cents ? (p.unit_price_cents / 100).toFixed(2) : ''
    priceSource.value = 'none'
  }
}

// 按「工地单独价 > 工地默认价 > 工人月薪折算/日薪/时薪」取该工人在此工地、此日期的生效价，
// 填入单价框。接口失败（旧后端/网络问题）时回退本地默认算法，绝不阻塞记工。
// force=false（打开弹窗时）不覆盖用户已手动输入的价；切工地/类型时 force=true 重新带价。
async function applyEffectivePrice(force = false) {
  const w = editingWorker.value
  if (!w?.id) return
  if (form.value.type === 'piece') {
    // 计件单价来自计件项目，与工地无关
    applyDefaultPrice()
    return
  }
  if (form.value.type === 'rest') return
  const seq = ++priceSeq
  try {
    const res = await getEffectivePrice({
      worker_id: w.id,
      project_id: form.value.project_id === '' ? undefined : Number(form.value.project_id),
      date: selectedDate.value,
    })
    if (seq !== priceSeq) return
    if (!force && priceTouched.value) return
    if (res.data?.code !== 0 || !res.data.data) throw new Error(res.data?.message || '取价失败')
    const p = res.data.data as EffectivePrice
    if (form.value.type === 'day') {
      if (p.daily_wage_cents > 0) {
        priceInput.value = (p.daily_wage_cents / 100).toFixed(2)
        priceSource.value = p.daily_source
      } else {
        priceInput.value = ''
        priceSource.value = 'none'
      }
    } else {
      if (p.hourly_wage_cents > 0) {
        priceInput.value = (p.hourly_wage_cents / 100).toFixed(2)
        priceSource.value = p.hourly_source
      } else {
        priceInput.value = ''
        priceSource.value = 'none'
      }
    }
  } catch {
    if (seq !== priceSeq) return
    if (!force && priceTouched.value) return
    applyDefaultPrice()
  }
}

// 价格输入框下方小字：说明这个单价从哪一层来的，用户知道不填/改动之后是什么口径。
const priceSourceText = computed(() => {
  if (form.value.type === 'rest') return ''
  if (priceSource.value === 'record') return '当前记录单价，可手动修改'
  if (form.value.type === 'piece') return '计件项目单价'
  switch (priceSource.value) {
    case 'project_worker': return '工地单价（该工人）'
    case 'project': return '工地默认单价'
    case 'monthly': return '工人月薪折算'
    case 'worker': return form.value.type === 'day' ? '工人日薪' : '工人时薪'
    default: return ''
  }
})

// 月薪折算日薪（分）：月薪 ÷ 当月实际天数，四舍五入（与后端 dailyFromMonthly 同口径）。
function monthlyDailyCents(monthlyCents: number, date: string): number {
  return Math.round(monthlyCents / daysInMonthOf(date))
}

// 某日期所在月份的实际天数（与后端 daysInMonthOf 同口径）
function daysInMonthOf(date: string): number {
  const d = new Date(date + 'T00:00:00')
  return new Date(d.getFullYear(), d.getMonth() + 1, 0).getDate()
}

const isMonthlyWorker = computed(() => !!editingWorker.value?.monthly_wage_cents)

const previewAmount = computed(() => {
  if (form.value.type === 'rest') return fmtMoney(0)
  const q = parseFloat(form.value.quantity) || 0
  const price = parseFloat(priceInput.value) || 0
  const one = Math.round(q * price * 100)
  const days = Math.max(1, multiDays.value.length)
  // 月薪工：工资 = 折算日薪 × 天数 = 月薪 × 天数 ÷ 当月天数（小数先加、最后只取整一次，
  // 避免每天四舍五入累积误差，保证「记满整月 = 月薪」）。
  const w = editingWorker.value
  if (isMonthlyWorker.value && form.value.type === 'day' && w?.monthly_wage_cents && priceSource.value === 'monthly') {
    const monthDays = daysInMonthOf(selectedDate.value)
    return fmtMoney(Math.round(w.monthly_wage_cents * q * days / monthDays))
  }
  return fmtMoney(one * days)
})

const dayTotalCents = computed(() => rows.value.reduce((s, r) => s + r.amount_cents, 0))
const dayRecordCount = computed(() => rows.value.reduce((s, r) => s + r.record_count, 0))

// 记工该落到哪个工地：优先当前上下文；全部工地时用「上次工地」；
// 未分工地上下文或没有有效的上次工地时为 null（不分工地）。
function resolveRecordProjectId(): number | null {
  if (typeof projectFilter.value === 'number') return projectFilter.value
  if (projectFilter.value === 'none') return null
  const last = readLastProjectFilter()
  return typeof last === 'number' && projects.value.some(p => p.id === last) ? last : null
}

// 当日数据加载：带工地上下文；seq 丢弃过期响应，快速切日期时不会串数据。
let dayLoadSeq = 0
async function loadDay() {
  rebuildDateOptions()
  const seq = ++dayLoadSeq
  const pid = projectFilter.value === '' ? undefined : projectFilter.value
  try {
    const res = await recordsByDate(selectedDate.value, pid)
    if (seq !== dayLoadSeq) return
    if (res.data?.code === 0) rows.value = res.data.data.workers
  } catch {}
}

function selectDate(d: string) {
  selectedDate.value = d
}

// 日期或工地上下文变化都重新加载当日数据（同一 tick 内合并为一次请求）
watch([selectedDate, projectFilter], loadDay)
// 工地上下文写回 localStorage（选「全部工地」时保留上次的具体工地）
watch(projectFilter, (v) => saveLastProjectFilter(v))

// 已在记工页时再从考勤表点另一个日期（同路由仅 query 变化），组件不会重建，
// 需要监听 query 同步过去。
watch(() => route.query.date, (value) => {
  const raw = typeof value === 'string' ? value : ''
  if (DATE_RE.test(raw) && raw !== selectedDate.value) selectedDate.value = raw
})

// 从考勤表点日期进来时会带 ?project=<id|none>：同步工地上下文。
// 没带或解析不了时保持当前上下文（不把用户手动选的工地冲掉）。
watch(() => route.query.project, (value) => {
  const p = parseProjectFilter(value)
  if (p === '') return
  if (p !== projectFilter.value) projectFilter.value = p
})

function shiftDays(delta: number) {
  selectedDate.value = addDays(selectedDate.value, delta)
}

function openEditor(row: DayWorkerRow) {
  editingWorker.value = row
  editingId.value = null
  // 工地默认当前上下文；全部工地时用上次记过的工地，没有则留空
  form.value = { type: 'day', quantity: '1', project_id: resolveRecordProjectId() ?? '', piece_item_id: '', note: '' }
  priceInput.value = ''
  priceSource.value = 'none'
  priceTouched.value = false
  multiDay.value = false
  multiDays.value = []
  // 按当前工地自动带价（工地单独价/默认价优先，失败回退工人日薪/月薪折算）
  applyEffectivePrice()
  editorOpen.value = true
}

function isQuickPending(id?: number): boolean {
  return id != null && !!quickPending.value[id]
}

// 一键记工：按当前工地 + 点工 1 天 + 工人默认单价直接记一笔，不弹窗。
// 不传 unit_price_cents，由服务端按日薪/月薪折算自动算钱。
async function quickRecord(row: DayWorkerRow) {
  const id = row.id
  if (id == null || quickPending.value[id]) return
  quickPending.value = { ...quickPending.value, [id]: true }
  try {
    const res = await createRecord({
      worker_id: id,
      date: selectedDate.value,
      type: 'day',
      quantity: '1',
      project_id: resolveRecordProjectId(),
      note: '',
    })
    const amount = res.data?.data?.amount_cents
    toast(`已一键记工：${row.name} · 点工 1 天 · ${fmtMoney(amount)}`)
    await loadDay()
  } catch (e: any) {
    const msg = e.response?.data?.message || '记工失败，请稍后重试'
    toast(`${msg}，可点『+ 记工』手动填写单价`, 'error')
  } finally {
    const next = { ...quickPending.value }
    delete next[id]
    quickPending.value = next
  }
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
  priceTouched.value = false
  // 编辑已有记录：保留原单价，不刷成工地价；用户切点工/点时类型时才重新带价
  priceSource.value = r.type === 'rest' ? 'none' : 'record'
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
    // 记住实际记到的工地：下次开记工弹窗 / 一键记工默认用它
    const savedProjectId = form.value.project_id === '' ? null : Number(form.value.project_id)
    if (savedProjectId) saveLastProjectFilter(savedProjectId)
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
  const id = deleteTargetId.value
  if (!id) return
  try {
    await deleteRecord(id)
    toast('已删除')
    editorOpen.value = false
    await loadDay()
  } catch (e: any) {
    toast(e.response?.data?.message || '删除失败', 'error')
  } finally {
    deleteTargetId.value = null
  }
}

function askDeleteRecord(id: number) {
  deleteTargetId.value = id
  deleteConfirmOpen.value = true
}

onMounted(async () => {
  // 先取工地与计件项目：工地列表用于校验「上次工地」并决定初始上下文
  try {
    const [p, pi] = await Promise.all([listProjects({ status: 'active' }), listPieceItems({ status: 'active' })])
    if (p.data?.code === 0) projects.value = p.data.data
    if (pi.data?.code === 0) pieceItems.value = pi.data.data
  } catch {}

  // 初始化工地上下文：route.query.project → 上次工地（须仍在工地列表里）→ 全部工地
  const fromQuery = parseProjectFilter(route.query.project)
  let initial: ProjectFilter = fromQuery
  if (initial === '') {
    const stored = readLastProjectFilter()
    const ids = projects.value.map(p => p.id).filter((id): id is number => typeof id === 'number')
    initial = isProjectFilterAvailable(stored, ids) ? stored : ''
  }
  if (initial === projectFilter.value) {
    await loadDay()
  } else {
    // 赋值会触发 watch → loadDay，并写回 localStorage
    projectFilter.value = initial
  }

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
