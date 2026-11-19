<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="报工审核" description="工人扫码/员工提交的记工先在这里确认，审核通过才计入工资">
      <template #actions>
        <RouterLink v-if="store.canManage" to="/admin/batch" class="btn-brand min-h-11 whitespace-nowrap">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          批量报工
        </RouterLink>
      </template>
    </PageHeader>

    <!-- 角色上下文未就绪：避免员工先看到审核内容再闪成无权限 -->
    <div v-if="!ready" class="surface rounded-2xl p-6 text-center text-sm text-muted-foreground">加载中…</div>

    <!-- 员工无权审核：不请求 summary/list -->
    <div v-else-if="!store.canManage" class="surface rounded-2xl">
      <div class="empty-state space-y-2">
        <p class="text-base font-semibold text-foreground">仅老板/班组长可审核</p>
        <p>你的账号是员工，扫码或提交的记工由班组长统一确认。</p>
        <RouterLink to="/admin/records" class="inline-flex min-h-11 items-center justify-center px-4 text-sm font-semibold text-brand-600 hover:underline dark:text-brand-400">去我的记工 →</RouterLink>
      </div>
    </div>

    <template v-else>
      <!-- 顶部统计：手机两列、桌面四列 -->
      <div class="grid grid-cols-2 gap-2.5 sm:gap-4 xl:grid-cols-4">
        <div class="surface rounded-2xl p-3 sm:p-5">
          <div class="flex items-center justify-between gap-1.5">
            <span class="min-w-0 truncate text-xs font-medium text-muted-foreground sm:text-sm">待审核笔数</span>
            <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-xl bg-amber-400/15 text-sm text-amber-500 sm:h-9 sm:w-9 sm:text-lg">⏳</span>
          </div>
          <div class="mt-1.5 text-lg font-extrabold leading-tight text-foreground tabular-nums font-display sm:mt-2 sm:text-2xl">
            {{ summary.pending_count }}<span class="ml-0.5 text-xs font-semibold text-muted-foreground sm:text-sm">笔</span>
          </div>
          <div class="mt-0.5 text-[11px] text-muted-foreground sm:mt-1 sm:text-xs">已驳回 {{ summary.rejected_count }} 条</div>
        </div>
        <div class="surface rounded-2xl p-3 sm:p-5">
          <div class="flex items-center justify-between gap-1.5">
            <span class="min-w-0 truncate text-xs font-medium text-muted-foreground sm:text-sm">待审金额</span>
            <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-xl bg-orange-400/15 text-sm text-orange-500 sm:h-9 sm:w-9 sm:text-lg">💰</span>
          </div>
          <div class="mt-1.5 break-all text-lg font-extrabold leading-tight text-foreground tabular-nums font-display sm:mt-2 sm:text-2xl">{{ fmtMoney(summary.pending_amount_cents) }}</div>
          <div class="mt-0.5 text-[11px] text-muted-foreground sm:mt-1 sm:text-xs">待审核报工合计</div>
        </div>
        <div class="surface rounded-2xl p-3 sm:p-5">
          <div class="flex items-center justify-between gap-1.5">
            <span class="min-w-0 truncate text-xs font-medium text-muted-foreground sm:text-sm">今日待审</span>
            <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-xl bg-brand-500/15 text-sm text-brand-500 sm:h-9 sm:w-9 sm:text-lg">📝</span>
          </div>
          <div class="mt-1.5 text-lg font-extrabold leading-tight text-foreground tabular-nums font-display sm:mt-2 sm:text-2xl">
            {{ summary.today_pending_count }}<span class="ml-0.5 text-xs font-semibold text-muted-foreground sm:text-sm">笔</span>
          </div>
          <div class="mt-0.5 break-all text-[11px] text-muted-foreground sm:mt-1 sm:text-xs">金额 {{ fmtMoney(summary.today_pending_amount_cents) }}</div>
        </div>
        <div class="surface rounded-2xl p-3 sm:p-5">
          <div class="flex items-center justify-between gap-1.5">
            <span class="min-w-0 truncate text-xs font-medium text-muted-foreground sm:text-sm">今日已通过</span>
            <span class="flex h-7 w-7 shrink-0 items-center justify-center rounded-xl bg-emerald-400/15 text-sm text-emerald-500 sm:h-9 sm:w-9 sm:text-lg">✅</span>
          </div>
          <div class="mt-1.5 text-lg font-extrabold leading-tight text-foreground tabular-nums font-display sm:mt-2 sm:text-2xl">
            {{ summary.today_approved_count }}<span class="ml-0.5 text-xs font-semibold text-muted-foreground sm:text-sm">笔</span>
          </div>
          <div class="mt-0.5 break-all text-[11px] text-muted-foreground sm:mt-1 sm:text-xs">金额 {{ fmtMoney(summary.today_approved_amount_cents) }}</div>
        </div>
      </div>

      <!-- 筛选栏 -->
      <div class="surface rounded-2xl p-2 sm:p-3 space-y-2">
        <div class="flex gap-1 overflow-x-auto">
          <button
            v-for="tab in statusTabs"
            :key="tab.value"
            type="button"
            class="min-h-11 shrink-0 rounded-xl border px-3.5 text-sm font-semibold transition-colors"
            :class="status === tab.value
              ? 'border-transparent bg-brand-gradient text-white shadow-glow'
              : 'border-border text-muted-foreground hover:bg-muted hover:text-foreground'"
            @click="setStatus(tab.value)"
          >{{ tab.label }}</button>
        </div>
        <div class="flex flex-wrap items-center gap-1.5">
          <button
            v-for="d in dateTabs"
            :key="d.value"
            type="button"
            class="min-h-11 shrink-0 rounded-xl border px-3 text-xs font-semibold transition-colors sm:text-sm"
            :class="dateMode === d.value
              ? 'border-brand-500 bg-brand-500/10 text-brand-600 dark:text-brand-400'
              : 'border-border text-muted-foreground hover:bg-muted hover:text-foreground'"
            @click="setDateMode(d.value)"
          >{{ d.label }}</button>
          <input
            v-if="dateMode === 'custom'"
            v-model="customDate"
            type="date"
            class="input-field min-h-11 !w-auto !py-2 text-sm"
            @change="reloadFirstPage"
          />
          <div class="ml-auto flex min-w-0 items-center gap-1.5">
            <SelectMenu
              v-model="filterWorker"
              :options="workerOptions"
              class="max-w-[42vw] sm:max-w-none"
              @change="reloadFirstPage"
            />
            <button
              type="button"
              class="btn-ghost min-h-11 min-w-11 shrink-0 !px-0"
              title="刷新"
              aria-label="刷新审核列表"
              @click="refreshAll"
            >
              <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"/></svg>
            </button>
          </div>
        </div>
      </div>

      <!-- 列表 -->
      <div class="surface rounded-2xl overflow-hidden">
        <!-- 待审核：全选 + 批量动作 -->
        <div v-if="status === 'pending'" class="flex flex-wrap items-center justify-between gap-2 border-b border-border px-3 py-1.5">
          <label class="flex min-h-11 cursor-pointer select-none items-center gap-2">
            <input
              type="checkbox"
              class="h-4 w-4 rounded accent-brand-500"
              :checked="allSelected"
              :disabled="pendingItems.length === 0 || acting"
              @change="toggleSelectAll"
            />
            <span class="text-xs text-muted-foreground sm:text-sm">
              全选<span v-if="selectedIds.length" class="ml-1 font-semibold text-foreground">已选 {{ selectedIds.length }} 条</span>
            </span>
          </label>
          <div class="flex items-center gap-1.5">
            <button
              type="button"
              class="btn-brand min-h-11 !px-3 !py-2 text-xs"
              :disabled="selectedIds.length === 0 || acting"
              @click="askBatchApprove"
            >通过选中</button>
            <button
              type="button"
              class="btn-ghost min-h-11 !px-3 !py-2 text-xs"
              :disabled="selectedIds.length === 0 || acting"
              @click="openReject(selectedIds)"
            >驳回选中</button>
          </div>
        </div>

        <div v-if="loading && items.length === 0" class="empty-state"><p>加载中…</p></div>
        <div v-else-if="items.length === 0" class="empty-state"><p>{{ emptyText }}</p></div>

        <template v-else>
          <!-- 桌面表格 -->
          <div class="hidden overflow-x-auto md:block">
            <table class="w-full text-sm">
              <thead>
                <tr class="border-b border-border bg-muted/60 text-left text-[11px] font-semibold uppercase tracking-wider text-muted-foreground">
                  <th v-if="status === 'pending'" class="w-10 whitespace-nowrap py-2.5 pl-4 pr-1">
                    <input
                      type="checkbox"
                      class="h-4 w-4 rounded accent-brand-500"
                      :checked="allSelected"
                      :disabled="pendingItems.length === 0 || acting"
                      aria-label="全选待审核报工"
                      @change="toggleSelectAll"
                    />
                  </th>
                  <th class="whitespace-nowrap px-3 py-2.5">工人</th>
                  <th class="whitespace-nowrap px-3 py-2.5">产品</th>
                  <th class="whitespace-nowrap px-3 py-2.5">数量</th>
                  <th class="whitespace-nowrap px-3 py-2.5">日期</th>
                  <th class="whitespace-nowrap px-3 py-2.5">来源</th>
                  <th class="whitespace-nowrap px-3 py-2.5">提交人</th>
                  <th class="whitespace-nowrap px-3 py-2.5">金额</th>
                  <th class="whitespace-nowrap px-3 py-2.5">状态</th>
                  <th class="whitespace-nowrap px-3 py-2.5 pr-4">操作</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border">
                <tr v-for="r in items" :key="r.id" class="transition-colors hover:bg-muted/40">
                  <td v-if="status === 'pending'" class="py-2 pl-4 pr-1">
                    <input
                      v-if="r.status === 'pending'"
                      type="checkbox"
                      class="h-4 w-4 rounded accent-brand-500"
                      :checked="isSelected(r.id)"
                      :disabled="acting"
                      :aria-label="`选择 ${r.worker_name || ''} 的报工`"
                      @change="toggleSelect(r.id)"
                    />
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 font-medium text-foreground">{{ reviewWorkerLabel(r.worker_id) }}</td>
                  <td class="max-w-[9rem] truncate px-3 py-2 text-muted-foreground">{{ r.piece_name || '—' }}</td>
                  <td class="whitespace-nowrap px-3 py-2 tabular-nums">
                    {{ fmtQty(r.quantity) }}<span v-if="r.piece_unit" class="ml-0.5 text-xs text-muted-foreground">{{ r.piece_unit }}</span>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2 text-muted-foreground">{{ r.date }}</td>
                  <td class="whitespace-nowrap px-3 py-2">
                    <span class="badge bg-muted !px-2 !py-0.5 text-[11px] font-medium text-muted-foreground">{{ sourceLabel(r) }}</span>
                  </td>
                  <td class="max-w-[7rem] truncate px-3 py-2 text-muted-foreground">{{ r.submitter_name || '—' }}</td>
                  <td class="px-3 py-2 font-semibold tabular-nums">
                    {{ fmtMoney(r.amount_cents) }}
                    <!-- 补贴/扣款要在审核时就看得见：只看总额看不出「这笔为什么多了 15 块」 -->
                    <div v-if="subsidyBadge(r) || deductBadge(r)" class="mt-0.5 flex flex-wrap gap-1">
                      <span v-if="subsidyBadge(r)" class="badge !px-1.5 !py-0 text-[10px] font-medium bg-brand-500/10 text-brand-600 dark:text-brand-400">{{ subsidyBadge(r) }}</span>
                      <span v-if="deductBadge(r)" class="badge !px-1.5 !py-0 text-[10px] font-medium bg-red-500/10 text-red-500">{{ deductBadge(r) }}</span>
                    </div>
                    <div v-if="subsidyNoteOf(r)" class="mt-0.5 max-w-[10rem] whitespace-normal text-[10px] font-normal leading-snug text-muted-foreground">{{ subsidyNoteOf(r) }}</div>
                  </td>
                  <td class="whitespace-nowrap px-3 py-2">
                    <span class="badge !px-2 !py-0.5 text-[11px]" :class="statusClass(r.status)">{{ statusLabel(r.status) }}</span>
                  </td>
                  <td class="px-3 py-2 pr-4">
                    <div v-if="r.status === 'pending'" class="flex gap-1.5">
                      <button type="button" class="btn-brand min-h-11 !px-3 !py-2 text-xs" :disabled="acting" @click="approveOne(r.id)">通过</button>
                      <button type="button" class="btn-ghost min-h-11 !px-3 !py-2 text-xs" :disabled="acting" @click="openReject([r.id])">驳回</button>
                    </div>
                    <span
                      v-else-if="r.status === 'rejected'"
                      class="block max-w-[12rem] truncate text-xs text-red-600 dark:text-red-400"
                      :title="r.review_note || ''"
                    >{{ r.review_note || '已驳回' }}</span>
                    <template v-else>
                      <span class="block text-xs text-muted-foreground">{{ r.reviewer_name ? `审核人 ${r.reviewer_name}` : '已通过' }}</span>
                      <span v-if="r.reviewed_at" class="block text-[11px] text-muted-foreground/80">{{ fmtReviewTime(r.reviewed_at) }}</span>
                    </template>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <!-- 手机卡片：三行信息 + 内联操作 -->
          <div class="space-y-1.5 p-2 md:hidden">
            <div
              v-for="r in items"
              :key="r.id"
              class="space-y-1 rounded-xl border bg-surface/80 px-3 py-2.5"
              :class="status === 'pending' && isSelected(r.id) ? 'border-brand-500/40' : 'border-border'"
            >
              <div class="flex min-w-0 items-center gap-1.5">
                <label
                  v-if="status === 'pending' && r.status === 'pending'"
                  class="-my-2 -ml-2 inline-flex min-h-11 min-w-11 shrink-0 cursor-pointer items-center justify-center"
                >
                  <input
                    type="checkbox"
                    class="h-4 w-4 rounded accent-brand-500"
                    :checked="isSelected(r.id)"
                    :disabled="acting"
                    :aria-label="`选择 ${r.worker_name || ''} 的报工`"
                    @change="toggleSelect(r.id)"
                  />
                </label>
                <span class="min-w-0 flex-1 truncate text-sm font-semibold text-foreground">{{ reviewWorkerLabel(r.worker_id) }}</span>
                <span class="badge shrink-0 !px-2 !py-0.5 text-[10px]" :class="statusClass(r.status)">{{ statusLabel(r.status) }}</span>
              </div>
              <div class="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs text-muted-foreground">
                <span class="min-w-0 break-words font-medium text-foreground">{{ r.piece_name || '—' }}</span>
                <span class="text-muted-foreground/60">·</span>
                <span class="tabular-nums">{{ fmtQty(r.quantity) }}<template v-if="r.piece_unit"> {{ r.piece_unit }}</template></span>
                <span class="text-muted-foreground/60">·</span>
                <span>{{ r.date }}</span>
                <span class="text-muted-foreground/60">·</span>
                <span class="font-semibold tabular-nums text-foreground">{{ fmtMoney(r.amount_cents) }}</span>
                <span v-if="subsidyBadge(r)" class="badge !px-1.5 !py-0 text-[10px] font-medium bg-brand-500/10 text-brand-600 dark:text-brand-400">{{ subsidyBadge(r) }}</span>
                <span v-if="deductBadge(r)" class="badge !px-1.5 !py-0 text-[10px] font-medium bg-red-500/10 text-red-500">{{ deductBadge(r) }}</span>
              </div>
              <p v-if="subsidyNoteOf(r)" class="break-words text-[11px] text-muted-foreground">补贴说明：{{ subsidyNoteOf(r) }}</p>
              <div class="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-[11px] text-muted-foreground">
                <span class="break-words">{{ sourceLabel(r) }}</span>
                <span class="text-muted-foreground/60">·</span>
                <span>提交人 {{ r.submitter_name || '—' }}</span>
                <template v-if="r.status !== 'pending'">
                  <span class="text-muted-foreground/60">·</span>
                  <span>{{ r.reviewer_name ? `审核人 ${r.reviewer_name}` : '已审核' }}</span>
                  <template v-if="r.reviewed_at">
                    <span class="text-muted-foreground/60">·</span>
                    <span>{{ fmtReviewTime(r.reviewed_at) }}</span>
                  </template>
                </template>
              </div>
              <p v-if="r.status === 'rejected' && r.review_note" class="break-words text-[11px] text-red-600 dark:text-red-400">驳回原因：{{ r.review_note }}</p>
              <div v-if="r.status === 'pending'" class="flex gap-2 pt-0.5">
                <button type="button" class="btn-brand min-h-11 flex-1 !py-2 text-xs" :disabled="acting" @click="approveOne(r.id)">通过</button>
                <button type="button" class="btn-ghost min-h-11 flex-1 !py-2 text-xs" :disabled="acting" @click="openReject([r.id])">驳回</button>
              </div>
            </div>
          </div>
        </template>

        <!-- 分页 -->
        <div v-if="total > pageSize" class="flex flex-wrap items-center justify-between gap-2 border-t border-border px-3 py-2">
          <span class="text-[11px] text-muted-foreground sm:text-xs">共 {{ total }} 条 · 第 {{ page }} / {{ totalPages }} 页</span>
          <div class="flex items-center gap-1.5">
            <button type="button" class="btn-ghost min-h-11 !px-3 !py-2 text-xs" :disabled="page <= 1 || loading" @click="goPage(page - 1)">上一页</button>
            <button type="button" class="btn-ghost min-h-11 !px-3 !py-2 text-xs" :disabled="page >= totalPages || loading" @click="goPage(page + 1)">下一页</button>
          </div>
        </div>
      </div>
    </template>

    <ConfirmDialog
      v-model="batchApproveOpen"
      title="批量通过"
      :message="`确定通过选中的 ${approveIds.length} 条报工？通过后计入工资，可在统计与结算中体现。${approveSplitCount > 0 ? `\n其中 ${approveSplitCount} 条含补贴或扣款，金额已并进合计（明细行有标记）。` : ''}`"
      confirm-text="通过"
      @confirm="confirmBatchApprove"
    />

    <Modal v-model="rejectOpen" :title="rejectIds.length > 1 ? `驳回 ${rejectIds.length} 条报工` : '驳回报工'">
      <div class="space-y-3.5">
        <p class="text-sm text-muted-foreground">驳回后这些记工不会计入工资，审核结果与原因会同步给提交人。</p>
        <p v-if="rejectSplitCount > 0" class="rounded-xl bg-amber-400/10 px-3 py-2 text-xs text-amber-700 dark:text-amber-300">
          其中 {{ rejectSplitCount }} 条含补贴或扣款（记工明细行上有标记），驳回会连这部分一起退回未计入工资。
        </p>
        <div>
          <label for="reject-note" class="mb-1.5 block text-sm font-medium text-foreground">驳回原因</label>
          <textarea
            id="reject-note"
            v-model="rejectNote"
            rows="3"
            maxlength="120"
            class="input-field resize-none"
            placeholder="不符合报工要求"
          ></textarea>
          <p class="mt-1 text-[11px] text-muted-foreground">最多 120 字，默认「不符合报工要求」。</p>
        </div>
        <div class="flex gap-2 pt-0.5">
          <button type="button" class="btn-ghost min-h-11 flex-1" :disabled="acting" @click="rejectOpen = false">取消</button>
          <button
            type="button"
            class="inline-flex min-h-11 flex-1 items-center justify-center gap-2 rounded-xl bg-destructive px-4 py-2.5 text-sm font-semibold text-white transition-all hover:brightness-110 active:scale-[0.98] disabled:pointer-events-none disabled:opacity-60"
            :disabled="acting"
            @click="submitReject"
          >{{ acting ? '处理中…' : '确认驳回' }}</button>
        </div>
      </div>
    </Modal>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import Toast from '../components/Toast.vue'
import {
  getReviewSummary,
  listReviewRecords,
  approveRecords,
  rejectRecords,
  listWorkers,
  type ReviewSummary,
  type WorkRecord,
  type Worker,
} from '../api/worklog'
import { addDays, fmtMoney, fmtQty, fmtYuan, todayStr } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

type ReviewStatus = 'pending' | 'approved' | 'rejected' | 'all'
type DateMode = 'today' | 'yesterday' | 'all' | 'custom'

const DEFAULT_REJECT_NOTE = '不符合报工要求'
const pageSize = 20

const statusTabs: { value: ReviewStatus; label: string }[] = [
  { value: 'pending', label: '待审核' },
  { value: 'approved', label: '已通过' },
  { value: 'rejected', label: '已驳回' },
  { value: 'all', label: '全部' },
]
const dateTabs: { value: DateMode; label: string }[] = [
  { value: 'today', label: '今天' },
  { value: 'yesterday', label: '昨天' },
  { value: 'all', label: '全部' },
  { value: 'custom', label: '自定义' },
]

const store = useWorklogStore()
const ready = ref(false)
const status = ref<ReviewStatus>('pending')
const dateMode = ref<DateMode>('all')
const customDate = ref(todayStr())
const filterWorker = ref<number | ''>('')
const workers = ref<Worker[]>([])
const items = ref<WorkRecord[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const acting = ref(false)

const summary = ref<ReviewSummary>({
  pending_count: 0,
  pending_amount_cents: 0,
  today_pending_count: 0,
  today_pending_amount_cents: 0,
  today_approved_count: 0,
  today_approved_amount_cents: 0,
  rejected_count: 0,
})

// 同名工人加序号，审核筛选/列表才分得清。
const workerLabelMap = computed(() => {
  const counts = new Map<string, number>()
  for (const w of workers.value) counts.set(w.name, (counts.get(w.name) || 0) + 1)
  const seen = new Map<string, number>()
  const labels = new Map<number, string>()
  for (const w of workers.value) {
    if (typeof w.id !== 'number') continue
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

function reviewWorkerLabel(id?: number): string {
  if (typeof id === 'number' && workerLabelMap.value.has(id)) return workerLabelMap.value.get(id) as string
  return '—'
}

const workerOptions = computed(() => [
  { value: '', label: '全部工人' },
  ...workers.value
    .filter((w) => typeof w.id === 'number')
    .map((w) => ({ value: w.id as number, label: workerLabelMap.value.get(w.id as number) || w.name })),
])

const selectedIds = ref<number[]>([])
const pendingItems = computed(() => items.value.filter((r) => r.status === 'pending'))
const allSelected = computed(
  () => pendingItems.value.length > 0 && pendingItems.value.every((r) => isSelected(r.id)),
)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)))

const emptyText = computed(() => {
  if (filterWorker.value !== '') return '该工人在所选条件下没有报工记录'
  switch (status.value) {
    case 'approved':
      return '还没有已通过的报工'
    case 'rejected':
      return '还没有被驳回的报工'
    case 'all':
      return '所选条件下没有报工记录'
    default:
      return '没有待审核的报工，先歇会儿 ☕'
  }
})

const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const batchApproveOpen = ref(false)
const approveIds = ref<number[]>([])
const rejectOpen = ref(false)
const rejectIds = ref<number[]>([])
const rejectNote = ref('')

// 兼容后端时间字符串（ISO / 带时区），手机卡片第三行只展示到分钟
function fmtReviewTime(iso?: string | null): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso.replace('T', ' ').slice(0, 16)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function statusLabel(s?: string): string {
  if (s === 'approved') return '已通过'
  if (s === 'rejected') return '已驳回'
  return '待审核'
}

function statusClass(s?: string): string {
  if (s === 'approved') return 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400'
  if (s === 'rejected') return 'bg-red-400/15 text-red-600 dark:text-red-400'
  return 'bg-amber-400/15 text-amber-600 dark:text-amber-400'
}

// 来源显示：扫码带报工码名称；手工录入区分「员工提交」与老板直接手工录入。
function sourceLabel(r: WorkRecord): string {
  if (r.source === 'qr') return r.entry_code_name ? `扫码报工 · ${r.entry_code_name}` : '扫码报工'
  if (r.source === 'batch') return '批量代录'
  if (r.source === 'manual') return r.submitter_name ? '员工提交' : '手工录入'
  return r.entry_code_name ? `扫码报工 · ${r.entry_code_name}` : '手工录入'
}

// 补贴/扣款徽标（v0.6.0）：员工与扫码提交的钱由服务端按档案算，但组长/老板代录
// 的记录可以带补贴与扣款。审核只看总额会看不出「这笔为什么多了 15 块」，
// 所以有值就在这行标出来（含说明），没有就一个字符都不加。
function subsidyBadge(r: WorkRecord): string {
  const cents = r.subsidy_cents ?? 0
  return cents > 0 ? `含补贴 ${fmtYuan(cents)}` : ''
}
// 补贴说明单独一小行（不塞进徽标里，否则桌面表格那一列会被 40 字撑得老宽）
function subsidyNoteOf(r: WorkRecord): string {
  return (r.subsidy_cents ?? 0) > 0 ? (r.subsidy_note || '').trim() : ''
}
function deductBadge(r: WorkRecord): string {
  const cents = r.deduct_cents ?? 0
  return cents > 0 ? `含扣款 ${fmtYuan(cents)}` : ''
}
function splitCountOf(ids: number[]): number {
  return items.value.filter((r) => r.id != null && ids.includes(r.id)
    && ((r.subsidy_cents ?? 0) > 0 || (r.deduct_cents ?? 0) > 0)).length
}
// 批量通过 / 驳回的确认文案里也带上笔数，别让带补贴的单子混在总额里过去
const approveSplitCount = computed(() => splitCountOf(approveIds.value))
const rejectSplitCount = computed(() => splitCountOf(rejectIds.value))

function dateParams(): Record<string, string> {
  if (dateMode.value === 'today') return { date: todayStr() }
  if (dateMode.value === 'yesterday') return { date: addDays(todayStr(), -1) }
  if (dateMode.value === 'custom') return customDate.value ? { date: customDate.value } : {}
  return {}
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  // 先清空再赋值：连续两条同样的提示也能重新弹出
  toastMsg.value = ''
  toastType.value = type
  setTimeout(() => {
    toastMsg.value = msg
  }, 20)
}

function isSelected(id?: number): boolean {
  return typeof id === 'number' && selectedIds.value.includes(id)
}

function toggleSelect(id?: number) {
  if (typeof id !== 'number') return
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}

function toggleSelectAll() {
  if (allSelected.value) {
    selectedIds.value = []
    return
  }
  selectedIds.value = pendingItems.value
    .map((r) => r.id)
    .filter((id): id is number => typeof id === 'number')
}

async function loadSummary() {
  try {
    const res = await getReviewSummary()
    if (res.data?.code === 0 && res.data.data) summary.value = res.data.data as ReviewSummary
  } catch {
    /* 统计失败不影响列表，静默即可 */
  }
}

async function loadWorkers() {
  try {
    const res = await listWorkers({ status: 'active' })
    if (res.data?.code === 0) workers.value = res.data.data || []
  } catch {
    /* 工人下拉失败时保留「全部工人」 */
  }
}

// 快速切 tab/日期时只认最后一次响应，避免旧数据覆盖新筛选
let listSeq = 0
async function loadList() {
  const seq = ++listSeq
  loading.value = true
  try {
    const res = await listReviewRecords({
      status: status.value,
      page: page.value,
      page_size: pageSize,
      worker_id: filterWorker.value === '' ? undefined : Number(filterWorker.value),
      project_id: undefined,
      ...dateParams(),
    })
    if (seq !== listSeq) return
    if (res.data?.code === 0) {
      const data = res.data.data || {}
      items.value = data.items || []
      total.value = data.total || 0
      const maxPage = Math.max(1, Math.ceil(total.value / pageSize))
      // 处理完最后一页后总页数变少，自动回退到有效页
      if (page.value > maxPage && page.value > 1) {
        page.value = maxPage
        await loadList()
      }
    }
  } catch (e: any) {
    if (seq === listSeq) toast(e.response?.data?.message || '加载报工记录失败', 'error')
  } finally {
    if (seq === listSeq) loading.value = false
  }
}

function reloadFirstPage() {
  page.value = 1
  selectedIds.value = []
  void loadList()
}

function setStatus(next: ReviewStatus) {
  if (status.value === next) return
  status.value = next
  reloadFirstPage()
}

function setDateMode(next: DateMode) {
  if (dateMode.value === next) return
  dateMode.value = next
  if (next === 'custom' && !customDate.value) customDate.value = todayStr()
  reloadFirstPage()
}

function goPage(next: number) {
  if (next < 1 || next > totalPages.value || loading.value) return
  page.value = next
  selectedIds.value = []
  void loadList()
}

async function refreshAll() {
  if (!store.canManage) return
  await Promise.all([loadSummary(), loadList(), store.refreshReview()])
}

async function doApprove(ids: number[]) {
  if (!ids.length || acting.value) return
  acting.value = true
  try {
    const res = await approveRecords(ids)
    if (res.data?.code !== 0) {
      toast(res.data?.message || '审核失败，请刷新后重试', 'error')
      await loadList()
      return
    }
    const updated = Number(res.data?.data?.updated ?? ids.length)
    toast(`已通过 ${updated} 条`)
    selectedIds.value = []
    approveIds.value = []
    await Promise.all([loadSummary(), loadList(), store.refreshReview()])
  } catch (e: any) {
    toast(e.response?.data?.message || '审核失败，请刷新后重试', 'error')
    await loadList()
  } finally {
    acting.value = false
  }
}

function approveOne(id?: number) {
  if (typeof id === 'number') void doApprove([id])
}

function askBatchApprove() {
  if (!selectedIds.value.length || acting.value) return
  approveIds.value = [...selectedIds.value]
  batchApproveOpen.value = true
}

function confirmBatchApprove() {
  void doApprove(approveIds.value)
}

function openReject(ids: Array<number | undefined>) {
  const clean = ids.filter((id): id is number => typeof id === 'number')
  if (!clean.length || acting.value) return
  rejectIds.value = clean
  rejectNote.value = DEFAULT_REJECT_NOTE
  rejectOpen.value = true
}

async function submitReject() {
  if (!rejectIds.value.length || acting.value) return
  acting.value = true
  try {
    const note = rejectNote.value.trim() || DEFAULT_REJECT_NOTE
    const res = await rejectRecords(rejectIds.value, note)
    if (res.data?.code !== 0) {
      toast(res.data?.message || '驳回失败，请刷新后重试', 'error')
      await loadList()
      return
    }
    const updated = Number(res.data?.data?.updated ?? rejectIds.value.length)
    toast(`已驳回 ${updated} 条`)
    rejectOpen.value = false
    rejectIds.value = []
    selectedIds.value = []
    await Promise.all([loadSummary(), loadList(), store.refreshReview()])
  } catch (e: any) {
    toast(e.response?.data?.message || '驳回失败，请刷新后重试', 'error')
    await loadList()
  } finally {
    acting.value = false
  }
}

onMounted(async () => {
  await store.load()
  ready.value = true
  // 员工无审核权限：不请求统计/列表
  if (!store.canManage) return
  await Promise.all([loadSummary(), loadWorkers(), loadList()])
})
</script>

<style scoped>
/* 共享 SelectMenu 触发器默认约 42px；页面内把输入/选择控件统一补到 44px 触控高度 */
:deep(.input-field) {
  min-height: 2.75rem;
}
</style>
