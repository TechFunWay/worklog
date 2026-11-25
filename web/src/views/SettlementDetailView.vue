<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="工资条" :description="`单号 JS${serial}`">
      <template #back>
        <button
          type="button"
          aria-label="返回结算列表"
          class="inline-flex shrink-0 items-center justify-center min-w-11 min-h-11 -ml-1 rounded-xl text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
          @click="goBack"
        >
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 19l-7-7 7-7" /></svg>
        </button>
      </template>
      <template #actions>
        <div class="flex gap-2">
          <button class="btn-ghost !px-3 !py-2 text-xs sm:!px-4 sm:!py-2.5 sm:text-sm" @click="copyText">复制文字</button>
          <button class="btn-brand !px-3 !py-2 text-xs sm:!px-4 sm:!py-2.5 sm:text-sm" @click="exportPNG" :disabled="exporting">{{ exporting ? '生成中…' : '导出图片' }}</button>
        </div>
      </template>
    </PageHeader>

    <div v-if="s" class="grid lg:grid-cols-[minmax(0,1fr)_320px] gap-6 items-start">
      <!-- 工资条卡片 -->
      <div ref="slipRef" class="payslip surface rounded-2xl p-4 sm:p-8" id="payslip-card">
        <div class="flex items-start justify-between border-b-2 border-brand-500/30 pb-4">
          <div class="flex items-center gap-3">
            <div class="w-12 h-12 rounded-xl bg-brand-gradient flex items-center justify-center text-white font-extrabold text-lg shadow-glow">工</div>
            <div>
              <div class="text-lg font-extrabold text-foreground font-display">工资条</div>
              <div class="text-xs text-muted-foreground">单号 JS{{ serial }} · {{ siteTitle }}</div>
            </div>
          </div>
          <div class="text-right text-xs text-muted-foreground">
            <div>结算时间：{{ (s.settled_at || '').replace('T', ' ').slice(0, 16) }}</div>
            <div class="mt-1 inline-flex items-center gap-1 px-2 py-0.5 rounded-full font-semibold" :class="payBadgeClass">{{ payStatusLabel }}</div>
            <!-- 分期时顶部就把尚欠说清楚，工人一眼看到还差多少（付清时不占行） -->
            <div v-if="showPaySplit" class="mt-1 text-[11px] font-semibold tabular-nums">
              已付 <span class="text-emerald-600 dark:text-emerald-400">{{ fmtYuan(paidCents) }}</span>
              · 尚欠 <span class="text-red-500">{{ fmtYuan(dueCents) }}</span>
            </div>
          </div>
        </div>

        <!-- 工人信息 -->
        <div class="grid grid-cols-3 gap-3 py-4">
          <div>
            <div class="text-xs text-muted-foreground">工人</div>
            <div class="flex items-center gap-1.5 mt-0.5">
              <span class="font-bold text-foreground text-base truncate">{{ s.worker_name }}</span>
              <span v-if="s.worker_no" class="badge bg-brand-500/10 text-brand-600 dark:text-brand-400 !px-1.5 !py-0 text-[10px]">{{ s.worker_no }}</span>
            </div>
          </div>
          <div>
            <div class="text-xs text-muted-foreground">出工期间</div>
            <div class="font-semibold text-foreground mt-0.5 tabular-nums text-sm">{{ s.period_start }} ~ {{ s.period_end }}</div>
          </div>
          <div>
            <div class="text-xs text-muted-foreground">记工笔数</div>
            <div class="font-semibold text-foreground mt-0.5">{{ s.record_count }} 笔</div>
            <div v-if="s.project_name" class="text-[11px] text-muted-foreground mt-0.5">项目：{{ s.project_name }}</div>
          </div>
        </div>

        <!-- 明细 -->
        <div class="rounded-xl border border-border overflow-hidden overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="bg-muted text-muted-foreground text-xs">
                <th class="text-left font-semibold px-2 py-1.5 sm:px-3 sm:py-2 whitespace-nowrap">日期</th>
                <th class="text-left font-semibold px-2 py-1.5 sm:px-3 sm:py-2 whitespace-nowrap">类型</th>
                <th class="text-right font-semibold px-2 py-1.5 sm:px-3 sm:py-2 whitespace-nowrap">数量</th>
                <th class="text-right font-semibold px-2 py-1.5 sm:px-3 sm:py-2 whitespace-nowrap">单价</th>
                <th class="text-right font-semibold px-2 py-1.5 sm:px-3 sm:py-2 whitespace-nowrap">金额</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              <tr v-for="r in detail.records" :key="'r' + r.record_id">
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 tabular-nums text-muted-foreground whitespace-nowrap">{{ r.date.slice(5) }}</td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2">
                  <span class="badge bg-muted text-muted-foreground">{{ r.type_label }}</span>
                  <span v-if="r.piece_name" class="text-xs text-muted-foreground ml-1">{{ r.piece_name }}</span>
                </td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 text-right tabular-nums whitespace-nowrap">{{ r.quantity }}{{ r.type === 'day' ? '天' : r.type === 'hour' ? '时' : (r.piece_unit || '件') }}</td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 text-right tabular-nums text-muted-foreground whitespace-nowrap">{{ fmtYuan(r.unit_price_cents) }}</td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 text-right tabular-nums font-semibold whitespace-nowrap">
                  {{ fmtYuan(r.amount_cents) }}
                  <!-- 只有真带补贴/扣款的那几行才多一行小字，普通记录版面与今天一致 -->
                  <span v-if="recordSplit(r)" class="block text-[10px] font-normal text-muted-foreground leading-tight">{{ recordSplit(r) }}</span>
                </td>
              </tr>
              <tr v-for="a in detail.advances" :key="'a' + a.advance_id" class="bg-red-400/5">
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 tabular-nums text-muted-foreground whitespace-nowrap">{{ a.date.slice(5) }}</td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2">
                  <span class="badge bg-red-400/15 text-red-500">借支 · {{ a.method_label }}</span>
                  <span v-if="a.project_name" class="block text-[10px] text-muted-foreground mt-0.5">🏗 {{ a.project_name }}</span>
                </td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2"></td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2"></td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 text-right tabular-nums text-red-500 font-semibold whitespace-nowrap">-{{ fmtYuan(a.amount_cents) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 汇总：应发 − 借支 + 调整 = 应付；已付 / 尚欠只在分期时出现 -->
        <div class="mt-4 space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-muted-foreground">应发合计</span><span class="tabular-nums font-semibold">{{ fmtMoney(s.work_amount_cents) }}</span></div>
          <div v-if="s.advance_amount_cents > 0" class="flex justify-between"><span class="text-muted-foreground">借支抵扣</span><span class="tabular-nums font-semibold text-red-500">-{{ fmtMoney(s.advance_amount_cents) }}</span></div>
          <div v-if="adjustCents !== 0" class="flex justify-between">
            <span class="text-muted-foreground">调整项<span v-if="s.adjust_note" class="text-[11px]"> · {{ s.adjust_note }}</span></span>
            <span class="tabular-nums font-semibold" :class="adjustCents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">{{ adjustCents > 0 ? '+' : '-' }}{{ fmtMoney(Math.abs(adjustCents)) }}</span>
          </div>
          <div class="flex justify-between items-center border-t-2 border-dashed border-border pt-3">
            <span class="font-bold text-foreground">{{ showPaySplit ? '应付工资' : '实发工资' }}</span>
            <span class="text-2xl font-extrabold tabular-nums font-display" :class="s.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">
              {{ fmtMoney(s.payable_cents) }}
            </span>
          </div>
          <template v-if="showPaySplit">
            <div class="flex justify-between"><span class="text-muted-foreground">已付</span><span class="tabular-nums font-semibold text-emerald-600 dark:text-emerald-400">{{ fmtMoney(paidCents) }}</span></div>
            <div class="flex justify-between"><span class="text-muted-foreground">尚欠</span><span class="tabular-nums font-semibold text-red-500">{{ fmtMoney(dueCents) }}</span></div>
          </template>
        </div>
        <div v-if="s.note" class="mt-2 text-xs text-muted-foreground">备注：{{ s.note }}</div>

        <!-- 签字栏 -->
        <div class="mt-6 pt-4 border-t border-border grid grid-cols-2 gap-4 text-sm">
          <div class="text-muted-foreground">出行人签字：<span class="inline-block w-28 border-b border-border"></span></div>
          <div class="text-muted-foreground">领款人签字：<span class="inline-block w-28 border-b border-border"></span></div>
        </div>
      </div>

      <!-- 操作侧栏 -->
      <div class="space-y-4">
        <!-- 付款记录：一张工资条可分几笔给（仅老板可登记/删除），工人可核对哪天到账多少 -->
        <div class="surface rounded-2xl p-5">
          <div class="mb-2 flex flex-wrap items-center justify-between gap-x-2 gap-y-1.5">
            <h3 class="shrink-0 font-bold text-foreground">付款记录</h3>
            <div v-if="store.isBoss" class="flex min-w-0 items-center gap-1.5">
              <!-- 调整项入口：付清的单子也能改（抹零抹错了是真实场景），
                   按钮上直接写现值，老板不用点开就知道这单有没有调整 -->
              <button type="button" class="btn-ghost min-w-0 !px-2.5 !py-1.5 text-xs" @click="openAdjust">
                {{ adjustEntryLabel }}
              </button>
              <button v-if="canAddPayment" type="button" class="btn-brand shrink-0 !px-3 !py-1.5 text-xs" @click="openPayment">登记付款</button>
            </div>
          </div>
          <div v-if="showPaySplit" class="mb-2 rounded-xl bg-muted px-3 py-2 flex items-center justify-between text-xs">
            <span class="text-muted-foreground">已付 <span class="font-semibold text-emerald-600 dark:text-emerald-400 tabular-nums">{{ fmtMoney(paidCents) }}</span></span>
            <span class="text-muted-foreground">尚欠 <span class="font-semibold text-red-500 tabular-nums">{{ fmtMoney(dueCents) }}</span></span>
          </div>
          <div v-if="payments.length === 0" class="text-xs text-muted-foreground">
            {{ dueCents > 0 ? '还没有登记付款，点「登记付款」记一笔。' : '这笔工资已一次结清，没有单独流水。' }}
          </div>
          <div v-else class="divide-y divide-border -my-1">
            <div v-for="p in payments" :key="p.id" class="py-2 flex items-center gap-2">
              <span class="w-8 h-8 rounded-lg bg-emerald-400/15 text-emerald-500 flex items-center justify-center text-sm shrink-0">✓</span>
              <div class="min-w-0 flex-1">
                <div class="text-sm font-semibold text-foreground tabular-nums">{{ fmtMoney(p.amount_cents) }}</div>
                <div class="text-[11px] text-muted-foreground truncate">
                  {{ p.paid_on }} · {{ METHOD_LABELS[p.method as keyof typeof METHOD_LABELS] || p.method || '未记方式' }}<span v-if="p.note"> · {{ p.note }}</span>
                </div>
              </div>
              <button v-if="store.isBoss" type="button" aria-label="删除这笔付款" class="p-2 -mr-1.5 shrink-0 text-red-400 hover:text-red-500 transition-colors" @click="askDeletePayment(p)">
                <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" /></svg>
              </button>
            </div>
          </div>
        </div>
        <div class="surface rounded-2xl p-5 space-y-3">
          <h3 class="font-bold text-foreground">操作</h3>
          <button class="btn-brand w-full" @click="exportPNG">📤 导出工资条图片</button>
          <button class="btn-ghost w-full" @click="copyText">📋 复制为文字</button>
          <button v-if="store.isBoss" class="btn-ghost w-full !text-red-500" @click="revert">↩️ 撤销结算（重新调整）</button>
        </div>
        <div class="surface rounded-2xl p-5">
          <h3 class="font-bold text-foreground mb-2">备注</h3>
          <textarea v-model="noteInput" rows="3" class="input-field resize-none"></textarea>
          <button class="btn-ghost w-full mt-2 !py-2 text-xs" @click="saveNote">保存备注</button>
        </div>
      </div>
    </div>

    <div v-else class="surface rounded-2xl p-10 text-center text-muted-foreground">加载中…</div>

    <!-- 登记付款（仅老板）：金额/方式/日期/备注，提交后服务端重算已付与付款状态 -->
    <Modal v-model="payOpen" title="登记付款">
      <div class="space-y-3.5">
        <div class="rounded-xl bg-muted px-3 py-2 text-xs text-muted-foreground flex items-center justify-between gap-2">
          <span>应付 <span class="font-semibold text-foreground tabular-nums">{{ fmtMoney(s?.payable_cents ?? 0) }}</span></span>
          <span>已付 <span class="font-semibold text-emerald-600 dark:text-emerald-400 tabular-nums">{{ fmtMoney(paidCents) }}</span></span>
          <span>尚欠 <span class="font-semibold text-red-500 tabular-nums">{{ fmtMoney(dueCents) }}</span></span>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">金额（元）*</label>
            <input v-model="payAmount" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" />
          </div>
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">付款日期 *</label>
            <input v-model="payDate" type="date" class="input-field" />
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">方式</label>
          <div class="grid grid-cols-4 gap-2">
            <button v-for="(label, key) in METHOD_LABELS" :key="key" type="button"
                    class="min-h-11 rounded-xl text-sm font-semibold border transition-all"
                    :class="payMethod === key ? 'bg-brand-gradient text-white border-transparent' : 'border-border text-muted-foreground'"
                    @click="payMethod = String(key)">
              {{ label }}
            </button>
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注（选填）</label>
          <input v-model="payNote" type="text" class="input-field" placeholder="如：微信转给本人" />
        </div>
        <p class="text-xs text-muted-foreground">登记后已付与付款状态由服务端按流水重算；欠薪只留在这张结算单上，不会退回未结算池。</p>
        <div class="flex gap-2">
          <button type="button" class="btn-ghost flex-1 min-h-11" @click="payOpen = false">取消</button>
          <button type="button" class="btn-brand flex-1 min-h-11" :disabled="paySaving" @click="savePayment">{{ paySaving ? '登记中…' : '确认登记' }}</button>
        </div>
      </div>
    </Modal>

    <!-- 改调整项（仅老板）：应付由服务端按「记工 − 借支 + 调整」重算，前端不自己拼数 -->
    <Modal v-model="adjustOpen" title="调整项（抹零 / 罚款 / 奖励）">
      <div class="space-y-3.5">
        <div class="rounded-xl bg-muted px-3 py-2 space-y-1 text-xs">
          <div class="flex justify-between"><span class="text-muted-foreground">记工合计</span><span class="tabular-nums font-semibold text-foreground">{{ fmtMoney(s?.work_amount_cents ?? 0) }}</span></div>
          <div class="flex justify-between"><span class="text-muted-foreground">借支抵扣</span><span class="tabular-nums font-semibold text-red-500">-{{ fmtMoney(s?.advance_amount_cents ?? 0) }}</span></div>
          <div class="flex justify-between"><span class="text-muted-foreground">已付</span><span class="tabular-nums font-semibold text-emerald-600 dark:text-emerald-400">{{ fmtMoney(paidCents) }}</span></div>
          <div class="flex justify-between border-t border-border pt-1"><span class="font-semibold text-foreground">调整后应付</span><span class="tabular-nums font-extrabold" :class="adjustPayableCents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">{{ fmtMoney(adjustPayableCents) }}</span></div>
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">调整金额（元，可为负）</label>
            <input v-model="adjustAmount" type="text" inputmode="text" class="input-field tabular-nums" placeholder="如 -200 抹零 / +100 奖励" />
          </div>
          <div class="min-w-0">
            <label class="text-sm font-medium text-foreground block mb-1.5">调整原因（选填）</label>
            <input v-model="adjustNote" type="text" maxlength="60" class="input-field" placeholder="如：抹零 3 元" />
          </div>
        </div>
        <p v-if="adjustBadInput" class="rounded-xl bg-amber-400/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">{{ adjustBadInput }}</p>
        <p v-else class="text-xs text-muted-foreground">工资条会把这一行连同原因一起打出来；留空或填 0 表示撤销调整。</p>
        <div class="flex gap-2">
          <button type="button" class="btn-ghost flex-1 min-h-11" @click="adjustOpen = false">取消</button>
          <button type="button" class="btn-brand flex-1 min-h-11" :disabled="adjustSaving" @click="saveAdjust">{{ adjustSaving ? '保存中…' : '保存' }}</button>
        </div>
      </div>
    </Modal>

    <ConfirmDialog v-model="confirmOpen" :title="confirmTitle" :message="confirmMessage" :confirm-text="confirmText" confirm-type="danger" @confirm="confirmAction()" />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Toast from '../components/Toast.vue'
import {
  getSettlement, revertSettlement, updateSettlementNote, createPayment, deletePayment,
  updateSettlementAdjust, PAY_STATUS_LABELS,
  type Settlement, type SettlementDetails, type SettlementDetailRecord, type SettlementPayment, type PayStatus,
} from '../api/worklog'
import { centsToYuanInput, fmtMoney, fmtYuan, METHOD_LABELS, signedYuanToCents, yuanToCents, todayStr } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()

const route = useRoute()
const router = useRouter()
const s = ref<Settlement | null>(null)
const detail = ref<SettlementDetails>({ records: [], advances: [] })
const noteInput = ref('')
const exporting = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const slipRef = ref<HTMLElement | null>(null)
const siteTitle = ref('工记')

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

const serial = computed(() => String(route.params.id).padStart(6, '0'))

// ---- 付款进度（v0.6.0）----
// pay_status 只由服务端写；老结算单没这个字段时按「已结清」处理（版面与今天一致）。
const payStatus = computed<PayStatus | ''>(() => s.value?.pay_status ?? '')
const paidCents = computed(() => s.value?.paid_cents ?? 0)
const dueCents = computed(() => {
  const sv = s.value
  if (!sv) return 0
  if (typeof sv.due_cents === 'number') return Math.max(0, sv.due_cents)
  return Math.max(0, (sv.payable_cents ?? 0) - (sv.paid_cents ?? 0))
})
const adjustCents = computed(() => s.value?.adjust_cents ?? 0)
const payments = computed<SettlementPayment[]>(() => s.value?.payments ?? [])
// 只有分期（有付款状态且没付清）时才把「实发」改口成「应付」并补两行
const showPaySplit = computed(() => payStatus.value === 'unpaid' || payStatus.value === 'partial')
const payStatusLabel = computed(() => (payStatus.value ? PAY_STATUS_LABELS[payStatus.value] : '已结清'))
const payBadgeClass = computed(() => {
  if (dueCents.value > 0) {
    return payStatus.value === 'unpaid' ? 'bg-red-500/10 text-red-500' : 'bg-amber-400/15 text-amber-600 dark:text-amber-300'
  }
  return 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400'
})
const canAddPayment = computed(() => dueCents.value > 0)

// 明细行的拆项小字：没有补贴/扣款时返回空串，那一行就不渲染
function recordSplit(r: SettlementDetailRecord): string {
  const sub = r.subsidy_cents ?? 0
  const ded = r.deduct_cents ?? 0
  if (sub <= 0 && ded <= 0) return ''
  const wage = typeof r.wage_cents === 'number' ? r.wage_cents : r.amount_cents + ded - sub
  const parts = [`工钱 ${fmtYuan(wage)}`]
  if (sub > 0) parts.push(`+补贴 ${fmtYuan(sub)}${r.subsidy_note ? `（${r.subsidy_note}）` : ''}`)
  if (ded > 0) parts.push(`-扣款 ${fmtYuan(ded)}`)
  return parts.join(' ')
}

// ---- 登记付款（仅老板）----
const payOpen = ref(false)
const paySaving = ref(false)
const payAmount = ref('')
const payMethod = ref('cash')
const payDate = ref(todayStr())
const payNote = ref('')

function openPayment() {
  if (!store.isBoss || !canAddPayment.value) return
  // 默认填尚欠：一次结清的人最多，剩下的自己改小
  payAmount.value = (dueCents.value / 100).toFixed(2)
  payMethod.value = 'cash'
  payDate.value = todayStr()
  payNote.value = ''
  payOpen.value = true
}

async function savePayment() {
  const sv = s.value
  if (!sv) return
  const cents = yuanToCents(payAmount.value)
  if (cents === null || cents <= 0) return toast('请填写付款金额', 'error')
  if (cents > dueCents.value) return toast(`本次付款不能超过尚欠 ${fmtMoney(dueCents.value)}`, 'error')
  if (!payDate.value) return toast('请选择付款日期', 'error')
  paySaving.value = true
  try {
    const res = await createPayment(sv.id, {
      amount_cents: cents,
      method: payMethod.value,
      paid_on: payDate.value,
      note: payNote.value.trim() || undefined,
    })
    if (res.data?.code !== 0) return toast(res.data?.message || '登记失败', 'error')
    toast('付款已登记')
    payOpen.value = false
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '登记失败', 'error')
  } finally {
    paySaving.value = false
  }
}

// ---- 改调整项（仅老板）----
const adjustOpen = ref(false)
const adjustSaving = ref(false)
const adjustAmount = ref('')
const adjustNote = ref('')

// 入口文案写现值：0 显示「设置调整项」，非 0 直接显示现值（如 调整项 ¥-3.00），
// 老板不用点开也知道这张工资条有没有补差。
const adjustEntryLabel = computed(() => {
  const cents = adjustCents.value
  if (cents === 0) return '设置调整项'
  return `调整项 ${fmtMoney(cents)}`
})

// 输入框里的调整金额（分，有符号）：留空 = 0（撤销调整），格式不对返回 null
const adjustInputCents = computed<number | null>(() => {
  const raw = adjustAmount.value.trim()
  if (raw === '') return 0
  return signedYuanToCents(raw)
})

// 调整后应付 = 记工 − 借支 + 调整（与服务端同一口径，只用于预览，不作数落库）
const adjustPayableCents = computed(() => {
  const sv = s.value
  const adj = adjustInputCents.value ?? 0
  if (!sv) return adj
  return (sv.work_amount_cents ?? 0) - (sv.advance_amount_cents ?? 0) + adj
})

// 服务端会拦的三种情况在弹窗里先说明，省一次失败往返；真被拦时 message 原样 toast
const adjustBadInput = computed(() => {
  if (adjustInputCents.value === null) return '调整金额格式不对，最多两位小数，抹零/罚款用负号（如 -200）'
  if (adjustPayableCents.value < 0) return '调整后应付为负：请核对借支，或改用「撤销结算」重做这张工资条'
  if (adjustPayableCents.value < paidCents.value) {
    return `调整后应付 ${fmtMoney(adjustPayableCents.value)} 小于已付 ${fmtMoney(paidCents.value)}，请先删掉多余的付款记录`
  }
  return ''
})

function openAdjust() {
  const sv = s.value
  if (!store.isBoss || !sv) return
  // 回填当前调整项，改的就是这单现有的值
  adjustAmount.value = centsToYuanInput(sv.adjust_cents)
  adjustNote.value = sv.adjust_note || ''
  adjustOpen.value = true
}

async function saveAdjust() {
  const sv = s.value
  if (!sv) return
  const cents = adjustInputCents.value
  if (cents === null) return toast('调整金额格式不对，最多两位小数，抹零用负号', 'error')
  if (adjustBadInput.value) return toast(adjustBadInput.value, 'error')
  adjustSaving.value = true
  try {
    const res = await updateSettlementAdjust(sv.id, { adjust_cents: cents, adjust_note: adjustNote.value.trim() })
    if (res.data?.code !== 0) return toast(res.data?.message || '保存失败', 'error')
    toast('调整项已更新，应付与付款状态已重算')
    adjustOpen.value = false
    // 保存后只重新拉详情：应付/付款状态一律由服务端算，前端不在本地拼数字
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    adjustSaving.value = false
  }
}

function askDeletePayment(p: SettlementPayment) {
  const sv = s.value
  if (!sv) return
  askConfirm('删除付款流水', `删除 ${p.paid_on} 这笔 ${fmtMoney(p.amount_cents)} 的付款？删除后已付与付款状态会重新计算。`, async () => {
    try {
      const res = await deletePayment(sv.id, p.id)
      if (res.data?.code !== 0) return toast(res.data?.message || '删除失败', 'error')
      toast('已删除')
      await load()
    } catch (e: any) {
      toast(e.response?.data?.message || '删除失败', 'error')
    }
  }, '删除')
}

async function load() {
  try {
    const res = await getSettlement(Number(route.params.id))
    if (res.data?.code === 0) {
      s.value = res.data.data as Settlement
      noteInput.value = res.data.data.note || ''
      try {
        const d = JSON.parse(res.data.data.details || '{}') as Partial<SettlementDetails>
        detail.value = { records: d.records ?? [], advances: d.advances ?? [] }
      } catch {
        detail.value = { records: [], advances: [] }
      }
    }
  } catch {}
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

async function exportPNG() {
  if (!slipRef.value) return
  exporting.value = true
  try {
    const html2canvas = (await import('html2canvas')).default
    const canvas = await html2canvas(slipRef.value, { scale: 2, backgroundColor: null, useCORS: true })
    const link = document.createElement('a')
    link.download = `工资条-${s.value?.worker_name || '工人'}-${s.value?.period_start || ''}.png`
    link.href = canvas.toDataURL('image/png')
    link.click()
    toast('图片已导出')
  } catch (e) {
    toast('导出失败，请重试', 'error')
  } finally {
    exporting.value = false
  }
}

// 复制到微信的对账文本。注意：金额里已含补贴/扣款，
// 所以「数量 × 单价 = 金额」这个等式不再成立——写成 工钱 + 补贴 − 扣款 = 应得；
// 没有补贴/扣款的老记录仍按原等式输出，工人看到的与历史一致。
function recordLine(r: SettlementDetailRecord): string {
  const unit = r.type === 'day' ? '天' : r.type === 'hour' ? '时' : (r.piece_unit || '件')
  const qty = `${r.date} ${r.type_label} ${r.quantity}${unit}`
  const split = recordSplit(r)
  if (!split) return `${qty} × ${fmtYuan(r.unit_price_cents)} = ${fmtYuan(r.amount_cents)}元`
  const sub = r.subsidy_cents ?? 0
  const ded = r.deduct_cents ?? 0
  const wage = typeof r.wage_cents === 'number' ? r.wage_cents : r.amount_cents + ded - sub
  let line = `${qty}（单价 ${fmtYuan(r.unit_price_cents)}） 工钱 ${fmtYuan(wage)}`
  if (sub > 0) line += ` + 补贴 ${fmtYuan(sub)}`
  if (ded > 0) line += ` - 扣款 ${fmtYuan(ded)}`
  return `${line} = 应得 ${fmtYuan(r.amount_cents)}元`
}

function copyText() {
  const sv = s.value
  if (!sv) return
  const lines: string[] = []
  lines.push(`【工资条】${sv.worker_name}${sv.worker_no ? `（${sv.worker_no}）` : ''}`)
  lines.push(`期间：${sv.period_start} ~ ${sv.period_end}（${sv.record_count} 笔）`)
  for (const r of detail.value.records) {
    lines.push(recordLine(r))
    if (r.subsidy_cents && r.subsidy_note) lines.push(`  补贴说明：${r.subsidy_note}`)
  }
  for (const a of detail.value.advances) {
    lines.push(`${a.date} 借支（${a.method_label}${a.project_name ? ` · ${a.project_name}` : ''}） -${fmtYuan(a.amount_cents)}元`)
  }
  lines.push(`应发：${fmtYuan(sv.work_amount_cents)}元`)
  if (sv.advance_amount_cents > 0) lines.push(`借支抵扣：-${fmtYuan(sv.advance_amount_cents)}元`)
  if (adjustCents.value !== 0) {
    lines.push(`调整项：${adjustCents.value > 0 ? '+' : '-'}${fmtYuan(Math.abs(adjustCents.value))}元${sv.adjust_note ? `（${sv.adjust_note}）` : ''}`)
  }
  if (showPaySplit.value) {
    lines.push(`应付：${fmtYuan(sv.payable_cents)}元`)
    lines.push(`已付：${fmtYuan(paidCents.value)}元`)
    lines.push(`尚欠：${fmtYuan(dueCents.value)}元`)
  } else {
    lines.push(`实发：${fmtYuan(sv.payable_cents)}元`)
  }
  if (payments.value.length > 0) {
    lines.push(`付款记录：`)
    for (const p of payments.value) {
      lines.push(`  ${p.paid_on} ${METHOD_LABELS[p.method as keyof typeof METHOD_LABELS] || p.method || ''} ${fmtYuan(p.amount_cents)}元`)
    }
  }
  navigator.clipboard.writeText(lines.join('\n')).then(
    () => toast('已复制，可粘贴发给工人'),
    () => toast('复制失败', 'error'),
  )
}

// 返回结算列表：工资条是二级页面，之前没有任何返回/关闭入口
function goBack() {
  router.push('/admin/settlements')
}

function revert() {
  if (!s.value) return
  askConfirm('撤销结算', '撤销后该结算单删除，相关记工/借支恢复为未结算，可以重新调整后再结算。确定撤销？', async () => {
    try {
      await revertSettlement(s.value!.id)
      toast('已撤销结算')
      router.push('/admin/settlements')
    } catch (e: any) {
      toast(e.response?.data?.message || '撤销失败', 'error')
    }
  }, '撤销')
}

async function saveNote() {
  if (!s.value) return
  try {
    await updateSettlementNote(s.value.id, noteInput.value)
    toast('备注已保存')
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  }
}

onMounted(async () => {
  await load()
  try {
    const res = await fetch(import.meta.env.BASE_URL + 'api/configs/public').then((r) => r.json())
    if (res?.code === 0 && res.data?.site_title) siteTitle.value = res.data.site_title
  } catch {}
})
</script>

<style scoped>
.payslip {
  max-width: 640px;
}
@media print {
  .payslip {
    box-shadow: none;
  }
}
</style>
