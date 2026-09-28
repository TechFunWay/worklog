<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="工资条" :description="`单号 JS${serial}`">
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
            <div class="mt-1 inline-flex items-center gap-1 px-2 py-0.5 rounded-full bg-emerald-400/15 text-emerald-600 dark:text-emerald-400 font-semibold">已结清</div>
          </div>
        </div>

        <!-- 工人信息 -->
        <div class="grid grid-cols-3 gap-3 py-4">
          <div>
            <div class="text-xs text-muted-foreground">工人</div>
            <div class="font-bold text-foreground text-base mt-0.5">{{ s.worker_name }}</div>
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
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 text-right tabular-nums font-semibold whitespace-nowrap">{{ fmtYuan(r.amount_cents) }}</td>
              </tr>
              <tr v-for="a in detail.advances" :key="'a' + a.advance_id" class="bg-red-400/5">
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 tabular-nums text-muted-foreground whitespace-nowrap">{{ a.date.slice(5) }}</td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2"><span class="badge bg-red-400/15 text-red-500">借支 · {{ a.method_label }}</span></td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2"></td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2"></td>
                <td class="px-2 py-1.5 sm:px-3 sm:py-2 text-right tabular-nums text-red-500 font-semibold whitespace-nowrap">-{{ fmtYuan(a.amount_cents) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 汇总 -->
        <div class="mt-4 space-y-2 text-sm">
          <div class="flex justify-between"><span class="text-muted-foreground">应发合计</span><span class="tabular-nums font-semibold">{{ fmtMoney(s.work_amount_cents) }}</span></div>
          <div v-if="s.advance_amount_cents > 0" class="flex justify-between"><span class="text-muted-foreground">借支抵扣</span><span class="tabular-nums font-semibold text-red-500">-{{ fmtMoney(s.advance_amount_cents) }}</span></div>
          <div class="flex justify-between items-center border-t-2 border-dashed border-border pt-3">
            <span class="font-bold text-foreground">实发工资</span>
            <span class="text-2xl font-extrabold tabular-nums font-display" :class="s.payable_cents < 0 ? 'text-red-500' : 'text-emerald-600 dark:text-emerald-400'">
              {{ fmtMoney(s.payable_cents) }}
            </span>
          </div>
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

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import Toast from '../components/Toast.vue'
import { getSettlement, revertSettlement, updateSettlementNote, type Settlement } from '../api/worklog'
import { fmtMoney, fmtYuan } from '../utils/format'
import { useWorklogStore } from '../stores/worklog'

const store = useWorklogStore()

const route = useRoute()
const router = useRouter()
const s = ref<Settlement | null>(null)
const detail = ref<{ records: any[]; advances: any[] }>({ records: [], advances: [] })
const noteInput = ref('')
const exporting = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const slipRef = ref<HTMLElement | null>(null)
const siteTitle = ref('工记')

const serial = computed(() => String(route.params.id).padStart(6, '0'))

async function load() {
  try {
    const res = await getSettlement(Number(route.params.id))
    if (res.data?.code === 0) {
      s.value = res.data.data
      noteInput.value = res.data.data.note || ''
      try {
        detail.value = JSON.parse(res.data.data.details || '{}')
      } catch {}
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

function copyText() {
  if (!s.value) return
  const sv = s.value
  if (!sv) return
  const lines: string[] = []
  lines.push(`【工资条】${sv.worker_name}`)
  lines.push(`期间：${sv.period_start} ~ ${sv.period_end}（${sv.record_count} 笔）`)
  for (const r of detail.value.records) {
    lines.push(`${r.date} ${r.type_label} ${r.quantity}${r.type === 'day' ? '天' : r.type === 'hour' ? '时' : (r.piece_unit || '件')} × ${fmtYuan(r.unit_price_cents)} = ${fmtYuan(r.amount_cents)}元`)
  }
  for (const a of detail.value.advances) {
    lines.push(`${a.date} 借支（${a.method_label}） -${fmtYuan(a.amount_cents)}元`)
  }
  lines.push(`应发：${fmtYuan(sv.work_amount_cents)}元`)
  if (sv.advance_amount_cents > 0) lines.push(`借支抵扣：-${fmtYuan(sv.advance_amount_cents)}元`)
  lines.push(`实发：${fmtYuan(sv.payable_cents)}元`)
  navigator.clipboard.writeText(lines.join('\n')).then(
    () => toast('已复制，可粘贴发给工人'),
    () => toast('复制失败', 'error'),
  )
}

async function revert() {
  if (!s.value) return
  if (!confirm('撤销后该结算单删除，相关记工/借支恢复为未结算，可以重新调整后再结算。确定撤销？')) return
  try {
    await revertSettlement(s.value.id)
    toast('已撤销结算')
    router.push('/admin/settlements')
  } catch (e: any) {
    toast(e.response?.data?.message || '撤销失败', 'error')
  }
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
