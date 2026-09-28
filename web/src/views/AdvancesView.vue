<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="借支" description="工人预支工资，结算时自动抵扣">
      <template #actions>
        <button class="btn-brand" @click="openForm">+ 记借支</button>
      </template>
    </PageHeader>

    <!-- 筛选 -->
    <div class="surface rounded-2xl p-3 flex flex-wrap gap-2">
      <SelectMenu v-model="filterWorker" :options="workerFilterOptions" class="!py-2 text-sm" @change="load" />
      <input type="date" v-model="filterStart" class="input-field !w-auto !py-2 text-sm" @change="load" />
      <span class="self-center text-muted-foreground text-sm">至</span>
      <input type="date" v-model="filterEnd" class="input-field !w-auto !py-2 text-sm" @change="load" />
      <div class="flex-1"></div>
      <div class="self-center text-sm text-muted-foreground">
        筛选合计：<span class="font-bold text-foreground tabular-nums">{{ fmtMoney(filteredTotal) }}</span>
      </div>
    </div>

    <div class="surface rounded-2xl overflow-hidden">
      <div v-if="items.length === 0" class="empty-state"><p>暂无借支记录</p></div>
      <div v-else class="divide-y divide-border">
        <div v-for="a in items" :key="a.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
          <span class="w-10 h-10 rounded-xl bg-red-400/15 text-red-500 flex items-center justify-center text-lg shrink-0">💸</span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-foreground">{{ a.worker_name }}</span>
              <span class="badge bg-muted text-muted-foreground">{{ METHOD_LABELS[a.method as keyof typeof METHOD_LABELS] || a.method }}</span>
              <span class="badge" :class="a.settlement_id ? 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400' : 'bg-amber-400/15 text-amber-600 dark:text-amber-500'">
                {{ a.settlement_id ? '已结算' : '未结算' }}
              </span>
            </div>
            <div class="text-xs text-muted-foreground mt-0.5">{{ a.date }}<span v-if="a.note"> · {{ a.note }}</span></div>
          </div>
          <div class="text-right shrink-0 flex items-center gap-2">
            <span class="font-bold text-red-500 tabular-nums">-{{ fmtMoney(a.amount_cents) }}</span>
            <button v-if="!a.settlement_id" class="btn-ghost !px-2.5 !py-1.5 text-xs !text-red-500" @click="remove(a)">删除</button>
          </div>
        </div>
      </div>
      <div v-if="total > pageSize" class="p-3 flex justify-center gap-2 border-t border-border">
        <button class="btn-ghost !py-1.5 text-xs" :disabled="page <= 1" @click="page--; load()">上一页</button>
        <span class="text-xs text-muted-foreground self-center">{{ page }} / {{ Math.ceil(total / pageSize) }}</span>
        <button class="btn-ghost !py-1.5 text-xs" :disabled="page >= Math.ceil(total / pageSize)" @click="page++; load()">下一页</button>
      </div>
    </div>

    <Modal v-model="formOpen" title="记借支">
      <div class="space-y-4">
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">工人 *</label>
          <SelectMenu v-model="form.worker_id" :options="workerFormOptions" class="flex-1" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">日期 *</label>
            <input type="date" v-model="form.date" class="input-field" />
          </div>
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">金额（元）*</label>
            <input v-model="amountInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" />
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">方式</label>
          <div class="grid grid-cols-4 gap-2">
            <button v-for="(label, key) in METHOD_LABELS" :key="key" class="py-2 rounded-xl text-sm font-semibold border transition-all"
                    :class="form.method === key ? 'bg-brand-gradient text-white border-transparent' : 'border-border text-muted-foreground'"
                    @click="form.method = key as string">
              {{ label }}
            </button>
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注</label>
          <input v-model="form.note" class="input-field" placeholder="选填，如：家里急用" />
        </div>
        <button class="btn-brand w-full" :disabled="saving" @click="save">保存</button>
      </div>
    </Modal>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Modal from '../components/Modal.vue'
import Toast from '../components/Toast.vue'
import { listAdvances, createAdvance, deleteAdvance, listWorkers, type Advance, type Worker } from '../api/worklog'
import { fmtMoney, METHOD_LABELS, yuanToCents, todayStr } from '../utils/format'

const items = ref<Advance[]>([])
const workers = ref<Worker[]>([])
// 下拉选项：筛选含「全部工人」空值项，表单含「请选择」空值项
const workerFilterOptions = computed(() => [
  { value: '', label: '全部工人' },
  ...workers.value.map(w => ({ value: w.id as number, label: w.name })),
])
const workerFormOptions = computed(() => [
  { value: '', label: '请选择' },
  ...workers.value.map(w => ({ value: w.id as number, label: w.name })),
])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const filterWorker = ref('')
const filterStart = ref('')
const filterEnd = ref('')
const formOpen = ref(false)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const form = ref<Partial<Advance>>({})
const amountInput = ref('')

const filteredTotal = computed(() => items.value.reduce((s, a) => s + a.amount_cents, 0))

async function load() {
  try {
    const res = await listAdvances({
      page: page.value, page_size: pageSize,
      worker_id: filterWorker.value || undefined,
      start: filterStart.value || undefined,
      end: filterEnd.value || undefined,
    })
    if (res.data?.code === 0) {
      items.value = res.data.data.items
      total.value = res.data.data.total
    }
  } catch {}
}

function openForm() {
  form.value = { date: todayStr(), method: 'cash' }
  amountInput.value = ''
  formOpen.value = true
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

async function save() {
  const cents = amountInput.value.trim() ? yuanToCents(amountInput.value) : null
  if (!form.value.worker_id) return toast('请选择工人', 'error')
  if (cents === null || cents <= 0) return toast('请填写金额', 'error')
  saving.value = true
  try {
    await createAdvance({
      worker_id: Number(form.value.worker_id),
      date: form.value.date || todayStr(),
      amount_cents: cents,
      method: form.value.method || 'cash',
      note: form.value.note,
    })
    toast('借支已记录')
    formOpen.value = false
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

async function remove(a: Advance) {
  if (!confirm(`确定删除「${a.worker_name}」的这笔借支？`)) return
  try {
    await deleteAdvance(a.id!)
    toast('已删除')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '删除失败', 'error')
  }
}

onMounted(async () => {
  await load()
  try {
    const res = await listWorkers()
    if (res.data?.code === 0) workers.value = res.data.data
  } catch {}
})
</script>
