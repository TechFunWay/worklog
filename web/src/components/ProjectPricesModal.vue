<template>
  <Modal :model-value="modelValue" :title="title" @update:model-value="close">
    <div v-if="loading" class="py-10 text-center text-sm text-muted-foreground">工地单价加载中…</div>

    <div v-else-if="errorMsg" class="py-8 text-center">
      <p class="text-sm text-red-500">{{ errorMsg }}</p>
      <button type="button" class="btn-ghost mt-3 !py-2 text-xs" @click="load">重试</button>
    </div>

    <div v-else-if="data" class="space-y-2">
      <!-- 工地默认单价说明 -->
      <div class="rounded-xl bg-muted/60 px-3 py-2 text-[11px] sm:text-xs text-muted-foreground leading-relaxed">
        工地默认单价：日工 <span class="text-foreground font-semibold">{{ data.project_daily_price_cents ? fmtMoney(data.project_daily_price_cents) + '/个工' : '未设置' }}</span>
        · 小时 <span class="text-foreground font-semibold">{{ data.project_hourly_price_cents ? fmtMoney(data.project_hourly_price_cents) + '/小时' : '未设置' }}</span>
        <br />给某个工人单独定价后，该工人在此工地优先用单独价；输入留空 = 不单独定价（按工地默认或工人单价）。
      </div>

      <!-- 逐工人覆盖价 -->
      <div class="max-h-[52vh] space-y-1.5 overflow-y-auto overscroll-contain">
        <div v-if="data.rows.length === 0" class="empty-state"><p>还没有工人，先去「工人管理」添加工人</p></div>
        <div v-for="r in data.rows" :key="r.worker_id" class="rounded-xl border border-border bg-surface/80 px-3 py-2.5 space-y-1.5">
          <div class="flex items-center justify-between gap-2">
            <span class="min-w-0 truncate text-sm font-semibold text-foreground">{{ r.worker_name }}</span>
            <span v-if="r.status === 'archived'" class="badge bg-muted text-muted-foreground !px-1.5 !py-0 text-[10px]">已归档</span>
          </div>
          <div class="text-[11px] text-muted-foreground">
            <template v-if="r.monthly_wage_cents">月薪 {{ fmtMoney(r.monthly_wage_cents) }}（点工按当月天数折算）</template>
            <template v-else>日薪 {{ r.worker_daily_wage_cents ? fmtMoney(r.worker_daily_wage_cents) : '未设置' }} · 时薪 {{ r.worker_hourly_wage_cents ? fmtMoney(r.worker_hourly_wage_cents) : '未设置' }}</template>
          </div>
          <div class="grid grid-cols-2 gap-2">
            <div class="min-w-0">
              <label class="mb-1 block text-[11px] text-muted-foreground">日工单价（元）</label>
              <input v-model="edits[r.worker_id].daily" type="text" inputmode="decimal"
                     class="input-field !px-2 !py-1.5 text-sm tabular-nums" :placeholder="placeholderOf(r, 'daily')" />
            </div>
            <div class="min-w-0">
              <label class="mb-1 block text-[11px] text-muted-foreground">小时单价（元）</label>
              <input v-model="edits[r.worker_id].hourly" type="text" inputmode="decimal"
                     class="input-field !px-2 !py-1.5 text-sm tabular-nums" :placeholder="placeholderOf(r, 'hourly')" />
            </div>
          </div>
        </div>
      </div>

      <!-- 底部操作：sticky 保证 375×667 一屏内可见 -->
      <div class="sticky bottom-0 z-10 -mx-5 flex gap-2 border-t border-border bg-surface px-5 pb-1 pt-2 sm:-mx-6 sm:px-6">
        <button type="button" class="btn-ghost flex-1 text-sm" @click="close">取消</button>
        <button type="button" class="btn-brand flex-1 text-sm" :disabled="saving" @click="save">{{ saving ? '保存中…' : '保存' }}</button>
      </div>
    </div>
  </Modal>

  <Toast :message="toastMsg" :type="toastType" />
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Modal from './Modal.vue'
import Toast from './Toast.vue'
import {
  getProjectPrices, saveProjectPrices,
  type Project, type ProjectPriceRow, type ProjectPricesData,
} from '../api/worklog'
import { centsToYuanInput, fmtMoney, yuanToCents } from '../utils/format'

const props = defineProps<{
  modelValue: boolean
  project: Project | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const data = ref<ProjectPricesData | null>(null)
const loading = ref(false)
const errorMsg = ref('')
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
// 每个工人的两个输入框（元，字符串；空 = 不覆盖）
const edits = ref<Record<number, { daily: string; hourly: string }>>({})

const title = computed(() => (props.project?.name ? `工地单价 · ${props.project.name}` : '工地单价'))

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

function close() {
  emit('update:modelValue', false)
}

function initEdits(rows: ProjectPriceRow[]) {
  const next: Record<number, { daily: string; hourly: string }> = {}
  for (const r of rows) {
    next[r.worker_id] = {
      daily: centsToYuanInput(r.daily_wage_cents),
      hourly: centsToYuanInput(r.hourly_wage_cents),
    }
  }
  edits.value = next
}

// placeholder 显示不填时的生效价（工地单独价 > 工地默认 > 工人单价）
function placeholderOf(r: ProjectPriceRow, kind: 'daily' | 'hourly'): string {
  const cents = kind === 'daily' ? r.effective_daily_wage_cents : r.effective_hourly_wage_cents
  if (!cents || cents <= 0) return '未设置'
  return (cents / 100).toFixed(2)
}

async function load() {
  const project = props.project
  if (!project?.id) {
    errorMsg.value = '工地信息不完整，请关闭后重试'
    return
  }
  loading.value = true
  errorMsg.value = ''
  data.value = null
  try {
    const res = await getProjectPrices(project.id)
    if (res.data?.code === 0) {
      const d = res.data.data as ProjectPricesData
      data.value = d
      initEdits(d.rows ?? [])
    } else {
      errorMsg.value = res.data?.message || '工地单价加载失败'
    }
  } catch (e: any) {
    errorMsg.value = e.response?.data?.message || '工地单价加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

// 单价输入（元，字符串）→ 分。留空 = 0（不覆盖）；非法/负数返回 null。
function parsePrice(input: string): number | null {
  const raw = (input ?? '').trim()
  if (raw === '') return 0
  return yuanToCents(raw)
}

async function save() {
  const project = props.project
  const d = data.value
  if (!project?.id || !d) return
  const rows: { worker_id: number; daily_wage_cents: number; hourly_wage_cents: number }[] = []
  for (const r of d.rows) {
    const edit = edits.value[r.worker_id] ?? { daily: '', hourly: '' }
    const daily = parsePrice(edit.daily)
    if (daily === null) return toast(`${r.worker_name} 的日工单价格式不正确，请填非负数字`, 'error')
    const hourly = parsePrice(edit.hourly)
    if (hourly === null) return toast(`${r.worker_name} 的小时单价格式不正确，请填非负数字`, 'error')
    rows.push({ worker_id: r.worker_id, daily_wage_cents: daily, hourly_wage_cents: hourly })
  }
  saving.value = true
  try {
    const res = await saveProjectPrices(project.id, rows)
    if (res.data?.code !== 0) {
      toast(res.data?.message || '保存失败', 'error')
      return
    }
    toast('单价已保存')
    close()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) load()
  },
)
</script>
