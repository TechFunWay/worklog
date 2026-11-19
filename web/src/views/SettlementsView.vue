<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="结算" description="未结算汇总 → 确认结算 → 生成工资条">
      <template #actions>
        <button v-if="isBoss" class="btn-brand" @click="openBatch">一键月结</button>
      </template>
    </PageHeader>

    <!-- Tab：待付款只给老板看（欠薪留在结算单上，不退回未结算池） -->
    <div v-if="canManage" class="surface rounded-2xl p-1.5 grid grid-cols-4 gap-1.5">
      <button class="py-2.5 px-1 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'unsettled' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'unsettled'">
        按人结算
      </button>
      <button class="py-2.5 px-1 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'projects' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'projects'">
        按项目
      </button>
      <button class="py-2.5 px-1 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'settled' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'settled'">
        已结账
      </button>
      <button class="relative py-2.5 px-1 rounded-xl text-sm font-semibold transition-all"
              :class="tab === 'due' ? 'bg-brand-gradient text-white shadow-glow' : 'text-muted-foreground'"
              @click="tab = 'due'; loadDue()">
        待付款
        <span v-if="dueRows.length > 0" class="ml-1 inline-block min-w-[1.25rem] rounded-full bg-red-500/85 px-1 text-[10px] font-bold leading-4 text-white">{{ dueRows.length }}</span>
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
              <div class="text-xs text-muted-foreground mt-0.5">
                {{ p.record_count }} 笔 · {{ p.worker_count }} 名工人
                <span v-if="(p.advance_cents ?? 0) > 0" class="text-red-500">· 抵本项目借支 {{ fmtYuan(p.advance_cents) }}</span>
                <span v-if="(p.loose_advance_count ?? 0) > 0" class="block text-[10px] leading-snug">另有 {{ p.loose_advance_count }} 笔未指定工地的借支，按人结算时抵扣</span>
              </div>
            </div>
            <div class="text-right shrink-0 mr-1">
              <div class="font-bold text-foreground tabular-nums">{{ fmtMoney(p.amount_cents) }}</div>
              <div class="text-[10px] text-muted-foreground">未结算（只抵扣该工地借支）</div>
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

    <!-- 待付款（欠薪明细） -->
    <template v-else-if="tab === 'due'">
      <div class="surface rounded-2xl p-3 flex items-center gap-2">
        <div class="min-w-0 flex-1 text-xs sm:text-sm text-muted-foreground">
          已结算但没付清的单子；欠薪留在结算单上，只登记付款，不改结算归属。
        </div>
        <div class="shrink-0 text-right">
          <div class="text-[10px] text-muted-foreground">尚欠合计</div>
          <div class="text-base sm:text-lg font-extrabold text-red-500 tabular-nums font-display">{{ fmtMoney(dueTotalCents) }}</div>
        </div>
      </div>
      <div v-if="dueError" class="rounded-2xl border border-amber-400/40 bg-amber-400/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
        {{ dueError }}
        <button type="button" class="ml-2 font-semibold underline" @click="loadDue">重试</button>
      </div>
      <div class="surface rounded-2xl overflow-hidden">
        <div v-if="dueRows.length === 0 && !dueError" class="empty-state"><p>没有欠薪，全部结清 🎉</p></div>
        <div v-else class="divide-y divide-border">
          <RouterLink v-for="d in dueRows" :key="d.settlement_id" :to="`/admin/settlements/${d.settlement_id}`" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3 hover:bg-muted/50">
            <span class="w-10 h-10 rounded-xl bg-red-400/15 text-red-500 flex items-center justify-center text-lg shrink-0">💰</span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-1.5 min-w-0">
                <span class="font-semibold text-foreground truncate">{{ d.worker_name }}</span>
                <span v-if="d.project_name" class="badge bg-muted text-muted-foreground !px-1.5 !py-0 text-[10px]">🏗 {{ d.project_name }}</span>
              </div>
              <div class="text-[11px] text-muted-foreground mt-0.5">
                单号 {{ d.settlement_no || `JS${String(d.settlement_id).padStart(6, '0')}` }} · {{ d.period_start }} ~ {{ d.period_end }}
              </div>
            </div>
            <div class="text-right shrink-0">
              <div class="text-sm font-bold text-red-500 tabular-nums">欠 {{ fmtMoney(d.due_cents) }}</div>
              <div class="text-[10px] text-muted-foreground tabular-nums">应付 {{ fmtYuan(d.payable_cents) }} · 已付 {{ fmtYuan(d.paid_cents) }}</div>
            </div>
            <svg class="w-4 h-4 text-muted-foreground shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 5l7 7-7 7"/></svg>
          </RouterLink>
        </div>
      </div>
    </template>

    <!-- 已结账 -->
    <template v-else-if="tab === 'settled'">
      <div class="surface rounded-2xl p-3 flex flex-wrap items-center gap-2">
        <SelectMenu v-model="sFilterWorker" :options="workerFilterOptions" class="!py-2 text-sm" @change="sPage = 1; loadSettled()" />
        <SelectMenu v-model="sFilterPay" :options="payFilterOptions" class="!py-2 text-sm" @change="sPage = 1; loadSettled()" />
        <input type="date" v-model="sFilterStart" class="input-field !w-auto !py-2 text-sm" @change="sPage = 1; loadSettled()" />
        <span class="self-center text-muted-foreground text-sm">至</span>
        <input type="date" v-model="sFilterEnd" class="input-field !w-auto !py-2 text-sm" @change="sPage = 1; loadSettled()" />
        <div class="flex-1"></div>
        <button type="button" class="btn-ghost !py-2 text-xs" :disabled="exporting" @click="exportSettlements">导出 CSV</button>
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
              <!-- 只有没付清时才占一行，付清的单子版面与今天一致 -->
              <div v-if="settlementDue(s) > 0" class="text-[10px] font-semibold text-red-500 tabular-nums">
                已付 {{ fmtYuan(s.paid_cents) }} / 尚欠 {{ fmtYuan(settlementDue(s)) }}
              </div>
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
          <div class="flex justify-between border-t border-border pt-2">
            <span class="font-semibold text-muted-foreground">小计（未计调整）</span>
            <span class="font-semibold tabular-nums" :class="confirmSummary.payable_cents < 0 ? 'text-red-500' : 'text-foreground'">
              {{ fmtMoney(confirmSummary.payable_cents) }}
            </span>
          </div>
        </div>
        <!-- 调整项（v0.6.0）：抹零/迟到扣款填负数，额外奖励填正数。
             只允许这一处补差，别去改实发——明细加起来 ≠ 实发，工资条就对不上账了。 -->
        <div class="grid grid-cols-2 gap-3">
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">调整项（元，可为负）</label>
            <input v-model="confirmAdjust" type="text" inputmode="text" class="input-field tabular-nums" placeholder="如 -200 抹零 / +100 奖励" />
          </div>
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">调整原因（选填）</label>
            <input v-model="confirmAdjustNote" type="text" class="input-field" placeholder="如：抹零 200" maxlength="40" />
          </div>
        </div>
        <div class="rounded-2xl border border-border px-3 py-2.5 space-y-2">
          <div class="flex items-center justify-between text-base">
            <span class="font-semibold">应付（系统算）</span>
            <span class="font-extrabold tabular-nums" :class="confirmPayableCents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">{{ fmtMoney(confirmPayableCents) }}</span>
          </div>
          <div class="grid grid-cols-2 items-end gap-3">
            <div class="min-w-0">
              <label class="text-sm font-medium text-foreground block mb-1.5">本次实付（元）</label>
              <input v-model="confirmPaid" type="text" inputmode="decimal" class="input-field tabular-nums"
                     placeholder="留空 = 全额" :disabled="!canPartialPay" @input="confirmPaidTouched = true" />
            </div>
            <p class="pb-2.5 text-[11px] leading-snug text-muted-foreground">
              {{ canPartialPay ? '分几次给就填几次，剩下的记入待付款。' : '应付不大于 0（借支倒挂），只能全额结清，差额请用调整项。' }}
            </p>
          </div>
          <div v-if="confirmDueCents > 0" class="flex items-center justify-between rounded-xl bg-amber-400/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
            <span>尚欠 {{ fmtMoney(confirmDueCents) }}，将记入待付款</span>
            <span class="shrink-0 font-semibold">可在工资条继续付</span>
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注（选填）</label>
          <input v-model="confirmNote" class="input-field" placeholder="如：9月工资已结清" />
        </div>
        <p class="text-xs text-muted-foreground">结算后相关记工与借支将锁定，可在结算单详情中撤销重结；欠薪不会退回未结算池（否则同一笔工会被结两次）。</p>
        <div class="flex gap-2">
          <button class="btn-ghost flex-1 min-h-11" @click="confirmOpen = false">取消</button>
          <button class="btn-brand flex-1 min-h-11" :disabled="saving" @click="doSettle">确认结算</button>
        </div>
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
        <!-- 月结一次给每个工人各生成一张单，同一个金额对多人没有意义，
             所以这里只让选「全额付清 / 全额挂欠」；要分期请进各自的结算确认或工资条。 -->
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">本次实付</label>
          <div class="grid grid-cols-2 gap-2">
            <button v-for="o in batchPaidOptions" :key="String(o.value)" type="button"
                    class="min-h-11 rounded-xl text-sm font-semibold border transition-all"
                    :class="batchPaidMode === o.value ? 'bg-brand-gradient text-white border-transparent' : 'border-border text-muted-foreground'"
                    @click="batchPaidMode = o.value">
              {{ o.label }}
            </button>
          </div>
          <p class="mt-1 text-xs text-muted-foreground">默认全额结清；选「全额挂欠」则每张单都先记入待付款。要给某个人只付一部分，请在「按人结算」的确认弹窗里填本次实付。</p>
        </div>
        <div class="flex gap-2">
          <button class="btn-ghost flex-1 min-h-11" @click="batchOpen = false">取消</button>
          <button class="btn-brand flex-1 min-h-11" :disabled="saving" @click="doBatch">开始结算</button>
        </div>
      </div>
    </Modal>

    <ConfirmDialog v-model="confirmAskOpen" :title="confirmAskTitle" :message="confirmAskMessage" :confirm-text="confirmAskText" :confirm-type="confirmAskType" @confirm="confirmAskAction()" />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Toast from '../components/Toast.vue'
import {
  unsettledList, unsettledProjects, createSettlement, batchSettlement, listSettlements, listWorkers,
  settlementPreview, listAdvances, listSettlementDue, looseAdvanceCount, downloadExport,
  type workerUnsettledRow, type Settlement, type SettlementDueRow, type SettlementPreviewData,
  type UnsettledProjectRow, type Advance, type Worker,
} from '../api/worklog'
import { fmtMoney, fmtYuan, yuanToCents, signedYuanToCents } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()
const route = useRoute()
const isBoss = computed(() => store.isBoss)
const canManage = computed(() => store.canManage)

// 通用确认弹窗（替代原生 confirm）
const confirmAskOpen = ref(false)
const confirmAskTitle = ref('')
const confirmAskMessage = ref('')
const confirmAskText = ref('确认')
const confirmAskType = ref<'primary' | 'danger'>('primary')
const confirmAskAction = ref<() => void | Promise<void>>(() => {})
function askConfirm(title: string, message: string, action: () => void | Promise<void>, opts: { text?: string; type?: 'primary' | 'danger' } = {}) {
  confirmAskTitle.value = title
  confirmAskMessage.value = message
  confirmAskText.value = opts.text ?? '确认'
  confirmAskType.value = opts.type ?? 'primary'
  confirmAskAction.value = action
  confirmAskOpen.value = true
}

type SettlementTab = 'unsettled' | 'projects' | 'settled' | 'due'
const TAB_VALUES: SettlementTab[] = ['unsettled', 'projects', 'settled', 'due']
const tab = ref<SettlementTab>('unsettled')
// 首页「待付款」卡是 /admin/settlements?tab=due 进来的：这里只解析入口参数，
// Tab 的切换逻辑照旧走点击，不重写。（员工/组长没有待付款 Tab，忽略该参数）
function applyTabQuery() {
  const q = typeof route.query.tab === 'string' ? route.query.tab : ''
  if (!q || !TAB_VALUES.includes(q as SettlementTab)) return
  if (q === 'due' && !canManage.value) return
  tab.value = q as SettlementTab
  if (q === 'due') void loadDue()
}
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
// 付款状态筛选（v0.6.0）：全部 / 未付 / 部分 / 已付，值与服务端 pay_status 一致
const payFilterOptions = [
  { value: '', label: '全部付款状态' },
  { value: 'unpaid', label: '未付款' },
  { value: 'partial', label: '部分付款' },
  { value: 'paid', label: '已付清' },
]
const sFilterWorker = ref('')
const sFilterPay = ref('')
const sFilterStart = ref('')
const sFilterEnd = ref('')

// 结算单的尚欠：due_cents 是服务端派生值，没返回时按 应付 − 已付 兜一下
function settlementDue(s: Settlement): number {
  const due = s.due_cents ?? (s.payable_cents ?? 0) - (s.paid_cents ?? 0)
  return due > 0 ? due : 0
}

const projRows = ref<UnsettledProjectRow[]>([])

async function loadProjRows() {
  try {
    const res = await unsettledProjects()
    if (res.data?.code === 0) projRows.value = res.data.data
  } catch {}
}

// 「未指定工地」的借支笔数：优先读预览接口的 loose_advance_count（按项目结算时
// 服务端算得最准，只数参与本次结算的工人）；预览接口不支持整项目预览时，
// 退化成直接数未挂结算单的未指定工地借支。
async function looseAdvanceCountFor(p: { project_id: number }): Promise<number> {
  try {
    const res = await settlementPreview({ project_id: p.project_id })
    if (res.data?.code === 0) return looseAdvanceCount(res.data.data as SettlementPreviewData)
  } catch {}
  try {
    const res = await listAdvances({ project_id: 'none', page: 1, page_size: 200 })
    if (res.data?.code === 0) {
      const rows = (res.data.data.items || []) as Advance[]
      return rows.filter((a) => !a.settlement_id).length
    }
  } catch {}
  return 0
}

async function settleProject(p: UnsettledProjectRow) {
  const now = new Date()
  const start = new Date(now.getFullYear(), 0, 1).toISOString().slice(0, 10)
  const end = now.toISOString().slice(0, 10)
  // 未指定工地的借支笔数优先用项目行自带的（列表请求已经拿到，多一次请求不如少一次）；
  // 老后端没这个字段时才退回预览接口 / 借支列表去数。
  const loose = (p.loose_advance_count ?? 0) > 0 ? (p.loose_advance_count as number) : await looseAdvanceCountFor(p)
  const looseText = loose > 0 ? `\n另有 ${loose} 笔未指定工地的借支将在按人结算时抵扣。` : ''
  askConfirm('按项目结算', `按项目结算「${p.project_name}」？\n只结算该工地的记工，并只抵扣该工地的借支，共 ${p.record_count} 笔。${looseText}`, async () => {
    saving.value = true
    try {
      const res = await batchSettlement({ start, end, project_id: p.project_id, note: `${p.project_name} 结算` })
      if (res.data?.code === 0) {
        toast('项目结算完成')
        await Promise.all([loadUnsettled(), loadSettled(), loadProjRows(), loadDue()])
      } else toast(res.data?.message || '结算失败', 'error')
    } catch (e: any) {
      toast(e.response?.data?.message || '结算失败', 'error')
    } finally {
      saving.value = false
    }
  }, { text: '结算' })
}

const confirmOpen = ref(false)
const batchOpen = ref(false)
const confirmSummary = ref<workerUnsettledRow | null>(null)
const confirmNote = ref('')
// 调整项与本次实付（v0.6.0）：输入都是「元」字符串，提交转「分」
const confirmAdjust = ref('')
const confirmAdjustNote = ref('')
const confirmPaid = ref('')
const confirmPaidTouched = ref(false)
const batchStart = ref('')
const batchEnd = ref('')
// 月结的「本次实付」只能整体选全额结清或全额挂欠：一次月结给每个工人各生成一张单，
// 同一个金额对多人没有意义（服务端 toSettle(batch) 也会拒绝非 0 的 paid_cents），
// 要按人分期请在各人的结算确认里填。
const batchPaidMode = ref<'full' | 'debt'>('full')
const batchPaidOptions = [
  { value: 'full' as const, label: '全额结清' },
  { value: 'debt' as const, label: '全额挂欠（进待付款）' },
]

// 调整项（分，有符号）：填错不阻塞看别的，只是不参与预览计算
const confirmAdjustCents = computed(() => signedYuanToCents(confirmAdjust.value || '0') ?? 0)
// 应付 = 应发 − 借支 + 调整（与后端 payable_cents 同口径）
const confirmPayableCents = computed(() => (confirmSummary.value?.payable_cents ?? 0) + confirmAdjustCents.value)
// 借支倒挂（应付 ≤ 0）时服务端不允许分期，输入框直接禁用，避免用户填了被拒
const canPartialPay = computed(() => confirmPayableCents.value > 0)
const confirmPaidCents = computed(() => {
  const raw = confirmPaid.value.trim()
  if (raw === '') return confirmPayableCents.value
  const c = yuanToCents(raw)
  if (c === null) return confirmPayableCents.value
  return Math.min(Math.max(0, c), Math.max(0, confirmPayableCents.value))
})
const confirmDueCents = computed(() => Math.max(0, confirmPayableCents.value - confirmPaidCents.value))

// 调整项改了应付金额时，未手动改过的「本次实付」跟着新应付走（默认=全额）
watch(confirmPayableCents, (v) => {
  if (!confirmPaidTouched.value) confirmPaid.value = v > 0 ? (v / 100).toFixed(2) : ''
})
const saving = ref(false)
const exporting = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

async function loadUnsettled() {
  try {
    const res = await unsettledList()
    if (res.data?.code === 0) unsettled.value = res.data.data
  } catch {}
}

// 待付款（欠薪）明细：合计用服务端给的 total_due_cents，前端只做一次兜底求和。
// 加载失败要说出来——空列表被当成「全部结清」会掩盖问题。
const dueRows = ref<SettlementDueRow[]>([])
const dueTotalCents = ref(0)
const dueError = ref('')
async function loadDue() {
  // 服务端口径是 canManage（老板与组长都能看欠薪看板）；登记付款仍然是仅老板
  if (!canManage.value) return
  dueError.value = ''
  try {
    const res = await listSettlementDue()
    if (res.data?.code === 0) {
      const d = res.data.data as { rows?: SettlementDueRow[]; total_due_cents?: number }
      dueRows.value = d.rows || []
      dueTotalCents.value = d.total_due_cents ?? dueRows.value.reduce((s, r) => s + (r.due_cents || 0), 0)
    } else {
      dueRows.value = []
      dueTotalCents.value = 0
      dueError.value = res.data?.message || '待付款加载失败'
    }
  } catch (e: any) {
    dueRows.value = []
    dueTotalCents.value = 0
    dueError.value = e.response?.data?.message || '待付款加载失败，请稍后重试'
  }
}

async function loadSettled() {
  try {
    const res = await listSettlements({
      page: sPage.value, page_size: sPageSize,
      worker_id: sFilterWorker.value || undefined,
      pay_status: sFilterPay.value || undefined,
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
  confirmAdjust.value = ''
  confirmAdjustNote.value = ''
  confirmPaidTouched.value = false
  // 默认填实发（=全额）：老板要分期就把它改小，改小后立刻显示尚欠
  confirmPaid.value = row.payable_cents > 0 ? (row.payable_cents / 100).toFixed(2) : ''
  confirmOpen.value = true
}

async function exportSettlements() {
  if (exporting.value) return
  exporting.value = true
  try {
    await downloadExport('settlements.csv')
    toast('已开始下载')
  } catch (e: any) {
    toast(e.response?.data?.message || '导出失败，请稍后重试', 'error')
  } finally {
    exporting.value = false
  }
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

async function doSettle() {
  const row = confirmSummary.value
  if (!row) return
  if (confirmAdjust.value.trim() && signedYuanToCents(confirmAdjust.value) === null) {
    return toast('调整项请填数字，抹零/罚款用负号，如 -200', 'error')
  }
  const paidRaw = confirmPaid.value.trim()
  const paidParsed = paidRaw ? yuanToCents(paidRaw) : null
  if (paidRaw && paidParsed === null) return toast('本次实付请填非负数字，如 5000 或 5000.50', 'error')
  if (paidRaw && confirmPayableCents.value <= 0) return toast('应付不大于 0，不能分期支付，请用调整项把账做平', 'error')
  if (paidParsed !== null && paidParsed > confirmPayableCents.value) {
    return toast(`本次实付不能大于应付 ${fmtMoney(confirmPayableCents.value)}`, 'error')
  }
  if (!confirmAdjust.value.trim() && confirmAdjustNote.value.trim()) {
    return toast('填了调整原因也要填调整金额（抹零用负数，如 -200）', 'error')
  }
  const adjustCents = confirmAdjust.value.trim() ? signedYuanToCents(confirmAdjust.value) ?? 0 : undefined
  // 留空 = 全额（不传该字段，与 v0.5.x 行为一致）；填 0 = 全额挂欠薪
  const paidCents = paidRaw ? paidParsed! : undefined
  saving.value = true
  try {
    await createSettlement({
      worker_id: row.worker_id,
      start: row.earliest_date || undefined,
      end: row.latest_date || undefined,
      note: confirmNote.value || undefined,
      adjust_cents: adjustCents,
      adjust_note: confirmAdjustNote.value.trim() || undefined,
      // 留空 = 全额（服务端按不传处理，与老版本行为一致）
      paid_cents: paidCents,
    })
    toast(confirmDueCents.value > 0 ? `结算成功，已生成工资条（尚欠 ${fmtMoney(confirmDueCents.value)} 已记入待付款）` : '结算成功，已生成工资条')
    confirmOpen.value = false
    tab.value = 'settled'
    await Promise.all([loadUnsettled(), loadSettled(), loadDue()])
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
  batchPaidMode.value = 'full'
  batchOpen.value = true
}

async function doBatch() {
  saving.value = true
  try {
    const res = await batchSettlement({
      start: batchStart.value,
      end: batchEnd.value,
      // 全额结清 = 不传该字段（与 v0.5.x 行为一致）；全额挂欠 = 传 0
      paid_cents: batchPaidMode.value === 'debt' ? 0 : undefined,
    })
    if (res.data?.code === 0) {
      const n = (res.data.data.settled || []).length
      const failed = (res.data.data.failed || []) as string[]
      toast(failed.length > 0 ? `已生成 ${n} 张结算单，${failed.length} 个工人失败：${failed[0]}` : `已生成 ${n} 张结算单`)
      batchOpen.value = false
      tab.value = 'settled'
      await Promise.all([loadUnsettled(), loadSettled(), loadDue()])
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
  await Promise.all([loadUnsettled(), loadSettled(), loadProjRows(), loadDue()])
  applyTabQuery()
  try {
    const res = await listWorkers()
    if (res.data?.code === 0) workers.value = res.data.data
  } catch {}
})

// 已经在本页时再从首页点「待付款」卡（同路由只换 query），组件不会重建，得跟着切
watch(() => route.query.tab, applyTabQuery)
</script>
