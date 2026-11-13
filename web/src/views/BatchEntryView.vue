<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="批量报工" description="选产品给一班工人集中填数量，提交后直接生效">
      <template #actions>
        <RouterLink v-if="store.canManage" to="/admin/review" class="btn-ghost min-h-11 whitespace-nowrap">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M10 19l-7-7m0 0l7-7m-7 7h18"/></svg>
          返回审核
        </RouterLink>
      </template>
    </PageHeader>

    <!-- 角色上下文未就绪 -->
    <div v-if="!ready" class="surface rounded-2xl p-6 text-center text-sm text-muted-foreground">加载中…</div>

    <!-- 员工无权批量报工：不请求工人/产品列表 -->
    <div v-else-if="!store.canManage" class="surface rounded-2xl">
      <div class="empty-state space-y-2">
        <p class="text-base font-semibold text-foreground">仅老板/班组长可批量报工</p>
        <p>你的账号是员工，请使用扫码报工或让班组长代录。</p>
        <RouterLink to="/admin/records" class="inline-flex min-h-11 items-center justify-center px-4 text-sm font-semibold text-brand-600 hover:underline dark:text-brand-400">去我的记工 →</RouterLink>
      </div>
    </div>

    <template v-else>
      <!-- 表单卡 -->
      <div class="surface rounded-2xl p-3 sm:p-4 space-y-3">
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div class="min-w-0">
            <label for="batch-date" class="mb-1.5 block text-sm font-medium text-foreground">日期 <span class="text-destructive">*</span></label>
            <input id="batch-date" v-model="form.date" type="date" class="input-field min-h-11" />
          </div>
          <div class="min-w-0">
            <label class="mb-1.5 block text-sm font-medium text-foreground">计件项目 <span class="text-destructive">*</span></label>
            <SelectMenu v-model="form.piece_item_id" :options="pieceOptions" placeholder="请选择产品" class="w-full" />
            <p v-if="selectedPiece" class="mt-1 break-words text-xs text-muted-foreground">
              单价 {{ fmtMoney(selectedPiece.unit_price_cents) }} / {{ selectedPiece.unit || '件' }}
            </p>
            <p v-else-if="pieces.length === 0" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
              还没有计件项目，先去
              <RouterLink to="/admin/piece-items" class="font-semibold text-brand-600 hover:underline dark:text-brand-400">计件项目</RouterLink>
              添加
            </p>
          </div>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div class="min-w-0">
            <label class="mb-1.5 block text-sm font-medium text-foreground">工地（选填）</label>
            <SelectMenu v-model="form.project_id" :options="projectOptions" class="w-full" />
          </div>
          <div class="min-w-0">
            <label for="batch-note" class="mb-1.5 block text-sm font-medium text-foreground">备注（选填）</label>
            <input id="batch-note" v-model="form.note" type="text" maxlength="120" class="input-field min-h-11" placeholder="如：3 号楼二层" />
          </div>
        </div>
      </div>

      <!-- 工人数量录入 -->
      <div class="surface rounded-2xl overflow-hidden">
        <div class="flex flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-2.5">
          <div class="min-w-0">
            <h2 class="text-sm font-bold text-foreground">数量录入</h2>
            <p class="text-[11px] text-muted-foreground">只提交填写了数量的工人，未填的不记工</p>
          </div>
          <span class="badge shrink-0 bg-brand-500/10 text-brand-600 dark:text-brand-400">{{ activeWorkers.length }} 名在职工人</span>
        </div>

        <div v-if="activeWorkers.length > 0" class="px-3 pt-2">
          <input
            v-model="workerFilter"
            type="text"
            class="input-field !py-2 text-sm"
            placeholder="按姓名筛选（人多时更快找到）"
            aria-label="按姓名筛选工人"
          />
        </div>

        <div v-if="activeWorkers.length === 0" class="empty-state">
          <p>还没有在职工人，先去 <RouterLink to="/admin/workers" class="font-semibold text-brand-600 dark:text-brand-400">添加工人</RouterLink> 吧</p>
        </div>

        <template v-else>
          <!-- 桌面表格 -->
          <div class="hidden overflow-x-auto md:block">
            <table class="w-full min-w-[480px] text-sm">
              <thead>
                <tr class="border-b border-border bg-muted/60 text-left text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                  <th class="whitespace-nowrap px-3 py-2.5">工人</th>
                  <th class="whitespace-nowrap px-3 py-2.5">数量</th>
                  <th class="whitespace-nowrap px-3 py-2.5">快捷</th>
                  <th class="whitespace-nowrap px-3 py-2.5 text-right">小计</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border">
                <tr v-for="w in filteredWorkers" :key="w.id" class="transition-colors" :class="qtyNumber(w.id) > 0 ? 'bg-brand-500/5' : 'hover:bg-muted/40'">
                  <td class="whitespace-nowrap px-3 py-2 font-medium text-foreground">{{ workerLabel(w) }}</td>
                  <td class="px-3 py-2">
                    <input
                      :value="qty[w.id] || ''"
                      type="text"
                      inputmode="numeric"
                      pattern="[0-9]*"
                      placeholder="0"
                      class="input-field !w-28 !py-2 text-center text-base font-bold tabular-nums md:text-sm"
                      :aria-label="`${w.name} 的数量`"
                      @input="onQtyInput(w.id, $event)"
                    />
                  </td>
                  <td class="px-3 py-2">
                    <div class="flex gap-1.5">
                      <button
                        v-for="n in QUICK_STEPS"
                        :key="n"
                        type="button"
                        class="btn-ghost min-h-11 !px-2.5 !py-2 text-xs tabular-nums"
                        @click="bumpQty(w.id, n)"
                      >+{{ n }}</button>
                    </div>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 text-right">
                    <span class="font-semibold tabular-nums" :class="qtyNumber(w.id) > 0 ? 'text-brand-600 dark:text-brand-400' : 'text-muted-foreground'">
                      {{ fmtMoney(subtotalCents(w.id)) }}
                    </span>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- 手机卡片 -->
          <div class="space-y-1.5 p-2 md:hidden">
            <div
              v-for="w in filteredWorkers"
              :key="w.id"
              class="space-y-1.5 rounded-xl border px-3 py-2.5"
              :class="qtyNumber(w.id) > 0 ? 'border-brand-500/40 bg-brand-500/5' : 'border-border bg-surface/80'"
            >
              <div class="flex min-w-0 items-center justify-between gap-2">
                <span class="min-w-0 truncate text-sm font-semibold text-foreground">{{ workerLabel(w) }}</span>
                <span class="shrink-0 text-[11px] tabular-nums text-muted-foreground">小计 {{ fmtMoney(subtotalCents(w.id)) }}</span>
              </div>
              <div class="flex items-center gap-1.5">
                <input
                  :value="qty[w.id] || ''"
                  type="text"
                  inputmode="numeric"
                  pattern="[0-9]*"
                  placeholder="0"
                  class="input-field min-w-0 flex-1 !py-2 text-center text-base font-bold tabular-nums"
                  :aria-label="`${w.name} 的数量`"
                  @input="onQtyInput(w.id, $event)"
                />
                <button
                  v-for="n in QUICK_STEPS"
                  :key="n"
                  type="button"
                  class="btn-ghost min-h-11 shrink-0 !px-2 !py-2 text-xs tabular-nums"
                  @click="bumpQty(w.id, n)"
                >+{{ n }}</button>
              </div>
            </div>
          </div>
          <div v-if="filteredWorkers.length === 0" class="empty-state">
            <p>没有匹配「{{ workerFilter }}」的工人</p>
          </div>
        </template>
      </div>

      <!-- 底部汇总条 -->
      <div class="surface rounded-2xl p-3 sm:p-4 space-y-2.5">
        <div class="flex items-start justify-between gap-3">
          <div class="min-w-0 space-y-0.5">
            <div class="text-sm text-muted-foreground">
              已填 <span class="text-base font-bold tabular-nums text-foreground">{{ filledCount }}</span> 人
              · 合计 <span class="text-base font-bold tabular-nums text-foreground">{{ totalQuantity }}</span> 件
            </div>
            <div class="break-words text-[11px] text-muted-foreground">
              {{ selectedPiece ? `按 ${selectedPiece.name} ${fmtMoney(selectedPiece.unit_price_cents)}/${selectedPiece.unit || '件'} 估算` : '选择产品后显示预计金额' }}
            </div>
          </div>
          <div class="min-w-0 max-w-[55%] text-right">
            <div class="text-[11px] text-muted-foreground">预计金额</div>
            <div class="break-all text-lg font-extrabold tabular-nums text-brand-600 dark:text-brand-400 font-display sm:text-xl">{{ fmtMoney(estimatedCents) }}</div>
          </div>
        </div>
        <div class="flex gap-2">
          <button type="button" class="btn-ghost min-h-11 flex-1" :disabled="filledCount === 0 || submitting" @click="clearQuantities">清空</button>
          <button type="button" class="btn-brand min-h-11 flex-1" :disabled="submitting" @click="submit">
            {{ submitting ? '提交中…' : '提交批量报工' }}
          </button>
        </div>
      </div>
    </template>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Toast from '../components/Toast.vue'
import {
  batchCreateRecords,
  listPieceItems,
  listProjects,
  listWorkers,
  type PieceItem,
  type Project,
  type Worker,
} from '../api/worklog'
import { fmtMoney, todayStr } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

const QUICK_STEPS = [10, 50, 100]
const MAX_QUANTITY = 1000000

const store = useWorklogStore()
const ready = ref(false)
const workers = ref<Worker[]>([])
const pieces = ref<PieceItem[]>([])
const projects = ref<Project[]>([])
const qty = ref<Record<number, string>>({})
const workerFilter = ref('')
const submitting = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

const form = ref({
  date: todayStr(),
  piece_item_id: '' as number | '',
  project_id: '' as number | '',
  note: '',
})

const activeWorkers = computed(
  () => workers.value.filter((w): w is Worker & { id: number } => typeof w.id === 'number'),
)
const filteredWorkers = computed(() => {
  const q = workerFilter.value.trim()
  return q ? activeWorkers.value.filter((w) => w.name.includes(q)) : activeWorkers.value
})

// 同名工人加序号，避免批量报工填错人。
const workerLabelMap = computed(() => {
  const counts = new Map<string, number>()
  for (const w of activeWorkers.value) counts.set(w.name, (counts.get(w.name) || 0) + 1)
  const seen = new Map<string, number>()
  const labels = new Map<number, string>()
  for (const w of activeWorkers.value) {
    if ((counts.get(w.name) || 0) > 1) {
      const n = (seen.get(w.name) || 0) + 1
      seen.set(w.name, n)
      labels.set(w.id, `${w.name}（${n}）`)
    } else {
      labels.set(w.id, w.name)
    }
  }
  return labels
})

function workerLabel(w: Worker): string {
  return typeof w.id === 'number' ? workerLabelMap.value.get(w.id) || w.name : w.name
}

const pieceOptions = computed(() =>
  pieces.value
    .filter((p) => typeof p.id === 'number')
    .map((p) => ({
      value: p.id as number,
      label: `${p.name}（${p.unit || '件'} · ${fmtMoney(p.unit_price_cents)}/${p.unit || '件'}）`,
    })),
)

const projectOptions = computed(() => [
  { value: '', label: '不选' },
  ...projects.value
    .filter((p) => typeof p.id === 'number')
    .map((p) => ({ value: p.id as number, label: p.name })),
])

const selectedPiece = computed(
  () => pieces.value.find((p) => p.id === form.value.piece_item_id) || null,
)

// 输入保留原始内容（含小数/非数字），提交时统一校验并 Toast 提示，避免静默改成别的数量
function rawQty(id: number): string {
  return (qty.value[id] || '').trim()
}

function qtyNumber(id: number): number {
  const raw = rawQty(id)
  if (!/^\d+$/.test(raw)) return 0
  const n = parseInt(raw, 10)
  return Number.isFinite(n) ? n : 0
}

const filledRows = computed(() =>
  activeWorkers.value
    .map((w) => ({ worker: w, quantity: qtyNumber(w.id) }))
    .filter((row) => row.quantity > 0),
)
const filledCount = computed(() => filledRows.value.length)
const totalQuantity = computed(() => filledRows.value.reduce((sum, row) => sum + row.quantity, 0))
const estimatedCents = computed(
  () => totalQuantity.value * (selectedPiece.value?.unit_price_cents || 0),
)

function subtotalCents(id: number): number {
  return qtyNumber(id) * (selectedPiece.value?.unit_price_cents || 0)
}

function setQty(id: number, value: string) {
  qty.value[id] = value.slice(0, 12)
}

function onQtyInput(id: number, e: Event) {
  setQty(id, (e.target as HTMLInputElement).value)
}

function bumpQty(id: number, delta: number) {
  const next = Math.min(MAX_QUANTITY, qtyNumber(id) + delta)
  setQty(id, String(next))
}

function clearQuantities() {
  qty.value = {}
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  // 先清空再赋值：连续两条同样的提示也能重新弹出
  toastMsg.value = ''
  toastType.value = type
  setTimeout(() => {
    toastMsg.value = msg
  }, 20)
}

async function loadWorkers() {
  try {
    const res = await listWorkers({ status: 'active' })
    if (res.data?.code === 0) workers.value = res.data.data || []
  } catch {
    /* 保留已有列表，页面可重试 */
  }
}

async function loadPieces() {
  try {
    const res = await listPieceItems({ status: 'active' })
    if (res.data?.code === 0) pieces.value = res.data.data || []
  } catch {
    /* 保留已有列表，页面可重试 */
  }
}

async function loadProjects() {
  try {
    const res = await listProjects({ status: 'active' })
    if (res.data?.code === 0) projects.value = res.data.data || []
  } catch {
    /* 工地可选，失败不影响报工 */
  }
}

async function refreshLists() {
  await Promise.all([loadWorkers(), loadPieces(), loadProjects()])
}

async function submit() {
  if (submitting.value) return
  if (!form.value.date) {
    toast('请选择日期', 'error')
    return
  }
  const piece = selectedPiece.value
  if (!piece || typeof piece.id !== 'number') {
    toast('请选择计件项目', 'error')
    return
  }
  // 数量必须是非负整数；0 视为未填，不提交
  const invalid = Object.keys(qty.value).some((key) => {
    const raw = (qty.value[Number(key)] || '').trim()
    if (raw === '') return false
    if (!/^\d+$/.test(raw)) return true
    return Number(raw) > MAX_QUANTITY
  })
  if (invalid) {
    toast('数量需为不超过 1000000 的整数', 'error')
    return
  }
  const rows = activeWorkers.value
    .map((w) => ({ worker_id: w.id, quantity: qtyNumber(w.id) }))
    .filter((row) => row.quantity > 0)
  if (rows.length === 0) {
    toast('请至少给一名工人填写数量', 'error')
    return
  }
  if (rows.length > 200) {
    toast('一次最多提交 200 人，请分两批', 'error')
    return
  }
  const amount = rows.reduce((sum, row) => sum + row.quantity, 0)
  submitting.value = true
  try {
    const res = await batchCreateRecords({
      date: form.value.date,
      piece_item_id: piece.id,
      project_id: form.value.project_id === '' ? null : Number(form.value.project_id),
      note: form.value.note.trim(),
      rows: rows.map((row) => ({ worker_id: row.worker_id, quantity: String(row.quantity) })),
    })
    if (res.data?.code !== 0) {
      toast(res.data?.message || '提交失败，请稍后重试', 'error')
      return
    }
    const created = Number(res.data?.data?.created ?? rows.length)
    const doneQty = res.data?.data?.total_quantity ?? String(amount)
    toast(`已为 ${created} 人记工 ${doneQty} 件`)
    // 保留产品/日期/工地/备注，只清空数量并刷新基础数据
    clearQuantities()
    await refreshLists()
  } catch (e: any) {
    toast(e.response?.data?.message || '提交失败，请稍后重试', 'error')
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  await store.load()
  ready.value = true
  // 员工无批量报工权限：不请求工人/产品列表
  if (!store.canManage) return
  await refreshLists()
})
</script>

<style scoped>
/* 共享 SelectMenu 触发器默认约 42px；页面内把输入/选择控件统一补到 44px 触控高度 */
:deep(.input-field) {
  min-height: 2.75rem;
}
</style>
