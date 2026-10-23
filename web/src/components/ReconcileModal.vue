<template>
  <Modal :model-value="modelValue" :title="modalTitle" @update:model-value="close">
    <div v-if="loading" class="py-10 text-center text-sm text-muted-foreground">对工单加载中…</div>

    <div v-else-if="errorMsg" class="py-8 text-center">
      <p class="text-sm text-red-500">{{ errorMsg }}</p>
      <button type="button" class="btn-ghost mt-3 !py-2 text-xs" @click="load">重试</button>
    </div>

    <div v-else-if="data" class="space-y-2">
      <!-- 期间与工地 -->
      <div class="rounded-xl bg-muted/60 px-3 py-2 text-xs text-muted-foreground">
        <div class="flex flex-wrap gap-x-3 gap-y-0.5">
          <span>期间：{{ data.start }} 至 {{ data.end }}</span>
          <span>工地：{{ projectLabel }}</span>
        </div>
      </div>

      <!-- 应发金额 -->
      <div class="flex items-center justify-between gap-2 rounded-xl border border-brand-500/20 bg-brand-500/10 px-3 py-2">
        <div class="min-w-0">
          <div class="text-[11px] text-muted-foreground">应发工钱</div>
          <div class="text-xl sm:text-2xl font-extrabold text-brand-600 dark:text-brand-400 tabular-nums font-display leading-tight">{{ fmtMoney(data.amount_cents) }}</div>
        </div>
        <div class="shrink-0 text-right text-[11px] text-muted-foreground leading-relaxed">
          共 {{ data.record_count }} 笔
        </div>
      </div>

      <!-- 考勤统计：出勤（自然日）+ 点工（折合工天），其余一项一行展示，手机端省高度 -->
      <div class="rounded-xl bg-muted/50 px-2.5 py-1.5 text-[11px] text-muted-foreground leading-relaxed">
        出勤 {{ data.attend_days }} 天（点工 {{ fmtQty(data.days) }} 个工） · 计时 {{ fmtQty(data.hours) }} 小时 · 计件 {{ fmtQty(data.pieces) }} · 休息 {{ data.rest_count }} 天
      </div>

      <!-- 明细：内部滚动，保证底部按钮在手机一屏内可见 -->
      <div>
        <div class="mb-1 text-xs">
          <span class="font-semibold text-muted-foreground">明细（{{ data.items.length }} 笔）</span>
        </div>
        <div class="max-h-[45vh] overflow-y-auto overscroll-contain rounded-xl border border-border">
          <div v-if="data.items.length === 0" class="px-3 py-6 text-center text-xs text-muted-foreground">本期没有记工记录</div>
          <div v-for="it in data.items" :key="it.id" class="flex items-start justify-between gap-2 border-b border-border/60 px-2.5 py-1.5 text-xs last:border-b-0">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-x-1.5 gap-y-0.5 text-foreground">
                <span class="font-semibold tabular-nums">{{ it.date.slice(5) }}</span>
                <span class="badge bg-muted !px-1.5 !py-0 text-[10px] !rounded-md text-muted-foreground">{{ it.type_label }}</span>
                <span class="tabular-nums" v-if="!it.rest">{{ fmtQty(it.quantity) }}{{ unitOf(it) }} × {{ fmtMoney(it.unit_price_cents) }}</span>
                <span v-else class="text-muted-foreground">休息</span>
              </div>
              <div v-if="metaOf(it)" class="mt-0.5 truncate text-[11px] text-muted-foreground">{{ metaOf(it) }}</div>
            </div>
            <div class="shrink-0 font-semibold tabular-nums" :class="it.rest ? 'text-muted-foreground' : 'text-foreground'">
              {{ it.rest ? '—' : fmtMoney(it.amount_cents) }}
            </div>
          </div>
        </div>
      </div>

      <!-- 底部操作：sticky 保证 375×667 一屏内可见 -->
      <div class="sticky bottom-0 z-10 -mx-5 flex gap-2 border-t border-border bg-surface px-5 pb-1 pt-2 sm:-mx-6 sm:px-6">
        <button type="button" class="btn-brand flex-1 !px-2 text-xs sm:text-sm whitespace-nowrap" :disabled="copying" @click="copyText">
          {{ copying ? '复制中…' : '复制对工单' }}
        </button>
        <button type="button" class="btn-ghost flex-1 !px-2 text-xs sm:text-sm whitespace-nowrap" :disabled="downloading" @click="downloadCsv">
          {{ downloading ? '导出中…' : '下载 CSV' }}
        </button>
        <button type="button" class="btn-ghost !px-3 text-xs sm:text-sm whitespace-nowrap" @click="close">关闭</button>
      </div>
    </div>
  </Modal>

  <Toast :message="toastMsg" :type="toastType" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from './Modal.vue'
import Toast from './Toast.vue'
import { downloadExport, getReconcile, type ReconcileData, type ReconcileItem, type ReconcileParams } from '../api/worklog'
import { fmtMoney, fmtQty } from '../utils/format'

const props = defineProps<{
  modelValue: boolean
  params: ReconcileParams
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const data = ref<ReconcileData | null>(null)
const loading = ref(false)
const errorMsg = ref('')
const copying = ref(false)
const downloading = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

const modalTitle = computed(() => (data.value?.worker?.name ? `对工单 · ${data.value.worker.name}` : '对工单'))
const projectLabel = computed(() => data.value?.project_name || '全部工地')

// 请求参数：空串表示不限制，不能把 '' 传给后端（否则会被当成非法工地 id）。
const query = computed<Record<string, unknown>>(() => {
  const q: Record<string, unknown> = { start: props.params.start, end: props.params.end }
  if (props.params.worker_id !== undefined && props.params.worker_id !== '') q.worker_id = props.params.worker_id
  if (props.params.project_id !== undefined && props.params.project_id !== '') q.project_id = props.params.project_id
  return q
})

function close() {
  emit('update:modelValue', false)
}

async function load() {
  if (!props.params.start || !props.params.end) {
    errorMsg.value = '时间段不完整，请返回考勤表重新选择'
    return
  }
  loading.value = true
  errorMsg.value = ''
  data.value = null
  try {
    const res = await getReconcile(query.value)
    if (res.data?.code === 0) data.value = res.data.data
    else errorMsg.value = res.data?.message || '对工单加载失败'
  } catch (e: any) {
    errorMsg.value = e.response?.data?.message || '对工单加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) load()
  },
)

function unitOf(it: ReconcileItem): string {
  if (it.type === 'day') return ' 天'
  if (it.type === 'hour') return ' 小时'
  return ` ${it.piece_unit || '件'}`
}

// 明细行的辅助信息：工地 / 计件项目 / 备注
function metaOf(it: ReconcileItem): string {
  return [it.project_name, it.piece_name, it.note].filter(Boolean).join(' · ')
}

// 中文对工单文本（微信可直接转发给工人核对）
function buildText(): string {
  const d = data.value
  if (!d) return ''
  const lines: string[] = []
  lines.push(`【对工单】${d.worker?.name ?? ''}`)
  lines.push(`期间：${d.start} 至 ${d.end}`)
  lines.push(`工地：${projectLabel.value}`)
  lines.push(
    `考勤：出勤 ${d.attend_days} 天，点工 ${fmtQty(d.days)} 个工；计时 ${fmtQty(d.hours)} 小时；计件 ${fmtQty(d.pieces)}；休息 ${d.rest_count} 天`,
  )
  lines.push(`应发工钱：${fmtMoney(d.amount_cents)}（共 ${d.record_count} 笔）`)
  if (d.items.length === 0) {
    lines.push('明细：本期无记工记录')
  } else {
    lines.push('明细：')
    for (const it of d.items) {
      const date = it.date.slice(5)
      const meta = metaOf(it)
      const suffix = meta ? `（${meta}）` : ''
      if (it.rest) {
        lines.push(`${date} 休息${suffix}`)
      } else {
        lines.push(
          `${date} ${it.type_label} ${fmtQty(it.quantity)}${unitOf(it)} × ${fmtMoney(it.unit_price_cents)} = ${fmtMoney(it.amount_cents)}${suffix}`,
        )
      }
    }
  }
  lines.push('核对无误请回复确认。')
  return lines.join('\n')
}

async function copyText() {
  if (copying.value) return
  const text = buildText()
  if (!text) return
  copying.value = true
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      throw new Error('clipboard unavailable')
    }
    toast('已复制对工单，可直接粘贴发给工人')
  } catch {
    // 降级：临时 textarea + execCommand（老 WebView / 非安全上下文）
    try {
      const ta = document.createElement('textarea')
      ta.value = text
      ta.setAttribute('readonly', '')
      ta.style.position = 'fixed'
      ta.style.opacity = '0'
      document.body.appendChild(ta)
      ta.select()
      const ok = document.execCommand('copy')
      ta.remove()
      if (!ok) throw new Error('copy failed')
      toast('已复制对工单，可直接粘贴发给工人')
    } catch {
      toast('复制失败，请长按明细手动选择复制', 'error')
    }
  } finally {
    copying.value = false
  }
}

async function downloadCsv() {
  if (downloading.value) return
  downloading.value = true
  try {
    const filename = await downloadExport('reconcile.csv', query.value)
    toast(`已导出：${filename}，文件在浏览器「下载」文件夹`)
  } catch (e: any) {
    toast(e.response?.data?.message || '导出失败，请稍后重试', 'error')
  } finally {
    downloading.value = false
  }
}
</script>
