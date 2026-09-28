<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="工人管理" description="日薪 / 时薪设置后，记工时自动带出">
      <template #actions>
        <button class="btn-brand" @click="openForm()">
          <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          添加工人
        </button>
      </template>
    </PageHeader>

    <div class="surface rounded-2xl overflow-hidden">
      <div v-if="workers.length === 0" class="empty-state"><p>还没有工人，点右上角「添加工人」</p></div>
      <div v-else class="divide-y divide-border">
        <div v-for="w in workers" :key="w.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
          <div class="w-10 h-10 sm:w-11 sm:h-11 rounded-full bg-brand-gradient flex items-center justify-center text-white font-bold shrink-0">{{ w.name.charAt(0) }}</div>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-foreground truncate">{{ w.name }}</span>
              <span class="badge" :class="w.status === 'active' ? 'bg-emerald-400/15 text-emerald-600 dark:text-emerald-400' : 'bg-muted text-muted-foreground'">
                {{ w.status === 'active' ? '在工' : '已归档' }}
              </span>
            </div>
            <div class="text-xs text-muted-foreground mt-0.5">
              <span v-if="w.phone">{{ w.phone }} · </span>日薪 {{ fmtMoney(w.daily_wage_cents) }} · 时薪 {{ fmtMoney(w.hourly_wage_cents) }}
            </div>
            <div v-if="w.note" class="text-xs text-muted-foreground/80 mt-0.5 truncate">{{ w.note }}</div>
          </div>
          <div class="flex gap-1 shrink-0 sm:gap-1.5">
            <button class="btn-ghost !px-2 !py-1.5 sm:!px-3 sm:!py-2 text-xs" @click="openForm(w)">编辑</button>
            <button class="btn-ghost !px-2 !py-1.5 sm:!px-3 sm:!py-2 text-xs" @click="toggleStatus(w)">{{ w.status === 'active' ? '归档' : '恢复' }}</button>
            <button v-if="w.status !== 'active'" class="btn-ghost !px-2 !py-1.5 sm:!px-3 sm:!py-2 text-xs !text-red-500" @click="remove(w)">删除</button>
          </div>
        </div>
      </div>
    </div>

    <Modal v-model="formOpen" :title="form.id ? '编辑工人' : '添加工人'">
      <div class="space-y-4">
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">姓名 *</label>
          <input v-model="form.name" class="input-field" placeholder="工人姓名" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">日薪（元）</label>
            <input v-model="dailyInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" />
          </div>
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">时薪（元）</label>
            <input v-model="hourlyInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" />
          </div>
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">电话</label>
          <input v-model="form.phone" class="input-field" placeholder="选填" />
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注</label>
          <input v-model="form.note" class="input-field" placeholder="选填" />
        </div>
        <button class="btn-brand w-full" :disabled="saving" @click="save">保存</button>
      </div>
    </Modal>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import Modal from '../components/Modal.vue'
import Toast from '../components/Toast.vue'
import { listWorkers, createWorker, updateWorker, deleteWorker, type Worker } from '../api/worklog'
import { fmtMoney, yuanToCents, centsToYuanInput } from '../utils/format'

const workers = ref<Worker[]>([])
const formOpen = ref(false)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const form = ref<Worker>({ name: '' })
const dailyInput = ref('')
const hourlyInput = ref('')

async function load() {
  try {
    const res = await listWorkers()
    if (res.data?.code === 0) workers.value = res.data.data
  } catch {}
}

function openForm(w?: Worker) {
  form.value = w ? { ...w } : { name: '', status: 'active' }
  dailyInput.value = centsToYuanInput(w?.daily_wage_cents)
  hourlyInput.value = centsToYuanInput(w?.hourly_wage_cents)
  formOpen.value = true
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

function parseCents(input: string): number | null {
  if (!input.trim()) return 0
  return yuanToCents(input)
}

async function save() {
  if (!form.value.name.trim()) return toast('请填写姓名', 'error')
  const daily = parseCents(dailyInput.value)
  const hourly = parseCents(hourlyInput.value)
  if (daily === null || hourly === null) return toast('薪资格式不正确', 'error')
  saving.value = true
  const payload: Worker = { ...form.value, daily_wage_cents: daily, hourly_wage_cents: hourly }
  try {
    if (payload.id) await updateWorker(payload.id, payload)
    else await createWorker(payload)
    toast('保存成功')
    formOpen.value = false
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

async function toggleStatus(w: Worker) {
  try {
    await updateWorker(w.id!, { ...w, status: w.status === 'active' ? 'archived' : 'active' })
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

async function remove(w: Worker) {
  if (!confirm(`确定删除工人「${w.name}」？`)) return
  try {
    await deleteWorker(w.id!)
    toast('已删除')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '删除失败', 'error')
  }
}

onMounted(load)
</script>
