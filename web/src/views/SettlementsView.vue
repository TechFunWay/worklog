<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="结算" description="未结算汇总 → 确认结算 → 生成工资条">
      <template #actions>
        <button v-if="isBoss" class="btn-brand" @click="openBatch">一键月结</button>
      </template>
    </PageHeader>

    <!-- Tab -->
    <div v-if="canManage" class="surface rounded-2xl p-1.5 grid grid-cols-3 gap-1.5">
      <button v-if="canManage" class="py-2.5 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'unsettled' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'unsettled'">
        按人结算
      </button>
      <button v-if="canManage" class="py-2.5 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'projects' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'projects'">
        按项目
      </button>
      <button class="py-2.5 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'settled' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'settled'">
        已结账
      </button>
    </div>

    <!-- 按项目 -->
    <template v-if="tab === 'projects'">
      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="projRows.length === 0" class="empty-state"><p>记工时选择工地后，可在这里按工地结算</p></div>
        <div v-else class="divide-y divide-border">
          <div v-for="p in projRows" :key="p.project_id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
            <span class="w-10 h-10 rounded-xl bg-teal-400/15 text-teal-500 flex items-center justify-center text-lg shrink-0">🏗️</span>
            <div class="min-w-0 flex-1">
              <div class="font-semibold text-foreground">{{ p.project_name || '未命名工地' }}</div>
              <div class="text-xs text-muted-foreground mt-0.5">{{ p.record_count }} 笔 · {{ p.worker_count }} 名工人</div>
            </div>
            <div class="text-right shrink-0 mr-1">
              <div class="font-bold text-foreground tabular-nums">{{ fmtMoney(p.amount_cents) }}</div>
              <div class="text-[10px] text-muted-foreground">未结算（借支不抵扣）</div>
            </div>
            <button v-if="isBoss" class="btn-brand !px-4 !py-2 text-xs shrink-0" @click="settleProject(p)">结算</button>
          </div>
        </div>
      </div>
    </template>

    <!-- 未结算（按人） -->
    <template v-if="tab === 'unsettled'">
      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="unsettled.length === 0" class="empty-state"><p>没有待结算的记工 🎉</p></div>
        <div v-else class="divide-y divide-border">
          <div v-for="row in unsettled" :key="row.worker_id" class="p-4">
            <div class="flex items-center gap-3">
              <div class="w-10 h-10 sm:w-11 sm:h-11 rounded-full bg-brand-gradient flex items-center justify-center text-white font-bold shrink-0">
                {{ row.worker_name.charAt(0) }}
              </div>
              <div class="min-w-0 flex-1">
                <div class="font-semibold text-foreground">{{ row.worker_name }}</div>
                <div class="text-xs text-muted-foreground mt-0.5">
                  {{ row.record_count }} 笔记工（{{ row.earliest_date }} ~ {{ row.latest_date }}）
                  <template v-if="row.advance_count > 0">· 借支 {{ row.advance_count }} 笔</template>
                </div>
              </div>
              <div class="text-right shrink-0 mr-1">
                <div class="text-xs text-muted-foreground">应发 {{ fmtMoney(row.work_amount_cents) }}</div>
                <div v-if="row.advance_amount_cents > 0" class="text-xs text-red-500">借支 -{{ fmtMoney(row.advance_amount_cents) }}</div>
                <div class="font-extrabold tabular-nums" :class="row.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">
                  实发 {{ fmtMoney(row.payable_cents) }}
                </div>
              </div>
              <button v-if="isBoss" class="btn-brand !px-4 !py-2 text-xs shrink-0" @click="openConfirm(row)">结算</button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <!-- 已结账 -->
    <template v-else>
      <div class="surface rounded-2xl p-3 flex flex-wrap gap-2">
        <SelectMenu v-model="sFilterWorker" :options="workerFilterOptions" class="!py-2 text-sm" @change="sPage = 1; loadSettled()" />
        <input type="date" v-model="sFilterStart" class="input-field !w-auto !py-2 text-sm" @change="sPage = 1; loadSettled()" />
        <span class="self-center text-muted-foreground text-sm">至</span>
        <input type="date" v-model="sFilterEnd" class="input-field !w-auto !py-2 text-sm" @change="sPage = 1; loadSettled()" />
        <div class="flex-1"></div>
        <a :href="exportUrl('settlements.csv')" class="btn-ghost !py-2 text-xs">导出 CSV</a>
      </div>

      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="settled.length === 0" class="empty-state"><p>还没有结算记录</p></div>
        <div v-else class="divide-y divide-border">
          <RouterLink v-for="s in settled" :key="s.id" :to="`/admin/settlements/${s.id}`" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3 hover:bg-muted/50">
            <span class="w-10 h-10 rounded-xl bg-emerald-400/15 text-emerald-500 flex items-center justify-center text-lg shrink-0">✅</span>
            <div class="min-w-0 flex-1">
              <div class="font-semibold text-foreground">{{ s.worker_name }}</div>
              <div class="text-xs text-muted-foreground mt-0.5">
                单号 JS{{ String(s.id).padStart(6, '0') }} · {{ s.period_start }} ~ {{ s.period_end }} · {{ s.record_count }} 笔
              </div>
            </div>
            <div class="text-right shrink-0">
              <div class="font-bold text-foreground tabular-nums">{{ fmtMoney(s.payable_cents) }}</div>
              <div class="text-xs text-muted-foreground">{{ (s.settled_at || '').slice(0, 10) }}</div>
            </div>
            <svg class="w-4 h-4 text-muted-foreground shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>
          </RouterLink>
        </div>
        <div v-if="sTotal > sPageSize" class="p-3 flex justify-center gap-2 border-t border-border">
          <button class="btn-ghost !py-1.5 text-xs" :disabled="sPage <= 1" @click="sPage--; loadSettled()">上一页</button>
          <span class="text-xs text-muted-foreground self-center">{{ sPage }} / {{ Math.ceil(sTotal / sPageSize) }}</span>
          <button class="btn-ghost !py-1.5 text-xs" :disabled="sPage >= Math.ceil(sTotal / sPageSize)" @click="sPage++; loadSettled()">下一页</button>
        </div>
      </div>
    </template>

    <!-- 单人结算确认 -->
    <Modal v-model="confirmOpen" title="确认结算">
      <div v-if="confirmSummary" class="space-y-4">
        <div class="rounded-2xl bg-muted p-4 space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-muted-foreground">工人</span><span class="font-semibold">{{ confirmSummary.worker_name }}</span></div>
          <div class="flex justify-between"><span class="text-muted-foreground">结算期间</span><span>{{ confirmSummary.earliest_date }} ~ {{ confirmSummary.latest_date }}</span></div>
          <div class="flex justify-between"><span class="text-muted-foreground">应发工资</span><span class="tabular-nums">{{ fmtMoney(confirmSummary.work_amount_cents) }}</span></div>
          <div class="flex justify-between"><span class="text-muted-foreground">借支抵扣</span><span class="text-red-500 tabular-nums">-{{ fmtMoney(confirmSummary.advance_amount_cents) }}</span></div>
          <div class="flex justify-between border-t border-border pt-2 text-base">
            <span class="font-semibold">实发</span>
            <span class="font-extrabold tabular-nums" :class="confirmSummary.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">
              {{ fmtMoney(confirmSummary.payable_cents) }}
            </span>
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注（选填）</label>
          <input v-model="confirmNote" class="input-field" placeholder="如：9月工资已结清" />
        </div>
        <p class="text-xs text-muted-foreground">结算后相关记工与借支将锁定，可在结算单详情中撤销重结。</p>
        <button class="btn-brand w-full" :disabled="saving" @click="doSettle">确认结算</button>
      </div>
    </Modal>

    <!-- 一键月结 -->
    <Modal v-model="batchOpen" title="一键月结">
      <div class="space-y-4">
        <p class="text-sm text-muted-foreground">把选定时间段内所有工人的未结算记录，按工人生成结算单。</p>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">开始日期</label>
            <input type="date" v-model="batchStart" class="input-field" />
          </div>
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">结束日期</label>
            <input type="date" v-model="batchEnd" class="input-field" />
          </div>
        </div>
        <button class="btn-brand w-full" :disabled="saving" @click="doBatch">开始结算</button>
      </div>
    </Modal>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Modal from '../components/Modal.vue'
import Toast from '../components/Toast.vue'
import {
  unsettledList, unsettledProjects, createSettlement, batchSettlement, listSettlements, listWorkers,
  exportUrl, type workerUnsettledRow, type Settlement, type Worker,
} from '../api/worklog'
import { fmtMoney } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const isBoss = computed(() => store.isBoss)
const canManage = computed(() => store.canManage)

const tab = ref<'unsettled' | 'projects' | 'settled'>('unsettled')
const unsettled = ref<workerUnsettledRow[]>([])
const settled = ref<Settlement[]>([])
const workers = ref<Worker[]>([])
// 下拉选项：已结账筛选含「全部工人」空值项
const workerFilterOptions = computed(() => [
  { value: '', label: '全部工人' },
  ...workers.value.map(w => ({ value: w.id as number, label: w.name })),
])
const sTotal = ref(0)
const sPage = ref(1)
const sPageSize = 20
const sFilterWorker = ref('')
const sFilterStart = ref('')
const sFilterEnd = ref('')

const projRows = ref<{ project_id: number; project_name: string; worker_count: number; record_count: number; amount_cents: number }[]>([])

async function loadProjRows() {
  try {
    const res = await unsettledProjects()
    if (res.data?.code === 0) projRows.value = res.data.data
  } catch {}
}

async function settleProject(p: { project_id: number; project_name: string; record_count: number }) {
  const now = new Date()
  const start = new Date(now.getFullYear(), 0, 1).toISOString().slice(0, 10)
  const end = now.toISOString().slice(0, 10)
  if (!confirm(`按项目结算「${p.project_name}」？\n只结算该工地的记工（借支不抵扣），共 ${p.record_count} 笔。`)) return
  saving.value = true
  try {
    const res = await batchSettlement({ start, end, project_id: p.project_id, note: `${p.project_name} 结算` })
    if (res.data?.code === 0) {
      toast('项目结算完成')
      await Promise.all([loadUnsettled(), loadSettled(), loadProjRows()])
    } else toast(res.data?.message || '结算失败', 'error')
  } catch (e: any) {
    toast(e.response?.data?.message || '结算失败', 'error')
  } finally {
    saving.value = false
  }
}

const confirmOpen = ref(false)
const batchOpen = ref(false)
const confirmSummary = ref<workerUnsettledRow | null>(null)
const confirmNote = ref('')
const batchStart = ref('')
const batchEnd = ref('')
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

async function loadUnsettled() {
  try {
    const res = await unsettledList()
    if (res.data?.code === 0) unsettled.value = res.data.data
  } catch {}
}

async function loadSettled() {
  try {
    const res = await listSettlements({
      page: sPage.value, page_size: sPageSize,
      worker_id: sFilterWorker.value || undefined,
      start: sFilterStart.value || undefined,
      end: sFilterEnd.value || undefined,
    })
    if (res.data?.code === 0) {
      settled.value = res.data.data.items
      sTotal.value = res.data.data.total
    }
  } catch {}
}

function openConfirm(row: workerUnsettledRow) {
  confirmSummary.value = row
  confirmNote.value = ''
  confirmOpen.value = true
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

async function doSettle() {
  if (!confirmSummary.value) return
  saving.value = true
  try {
    await createSettlement({
      worker_id: confirmSummary.value.worker_id,
      start: confirmSummary.value.earliest_date || undefined,
      end: confirmSummary.value.latest_date || undefined,
      note: confirmNote.value || undefined,
    })
    toast('结算成功，已生成工资条')
    confirmOpen.value = false
    tab.value = 'settled'
    await Promise.all([loadUnsettled(), loadSettled()])
  } catch (e: any) {
    toast(e.response?.data?.message || '结算失败', 'error')
  } finally {
    saving.value = false
  }
}

function openBatch() {
  const now = new Date()
  const first = new Date(now.getFullYear(), now.getMonth(), 1)
  batchStart.value = first.toISOString().slice(0, 10)
  batchEnd.value = now.toISOString().slice(0, 10)
  batchOpen.value = true
}

async function doBatch() {
  saving.value = true
  try {
    const res = await batchSettlement({ start: batchStart.value, end: batchEnd.value })
    if (res.data?.code === 0) {
      const n = (res.data.data.settled || []).length
      toast(`已生成 ${n} 张结算单`)
      batchOpen.value = false
      tab.value = 'settled'
      await Promise.all([loadUnsettled(), loadSettled()])
    } else {
      toast(res.data?.message || '结算失败', 'error')
    }
  } catch (e: any) {
    toast(e.response?.data?.message || '结算失败', 'error')
  } finally {
    saving.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadUnsettled(), loadSettled(), loadProjRows()])
  try {
    const res = await listWorkers()
    if (res.data?.code === 0) workers.value = res.data.data
  } catch {}
})
</script>
