<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="计件项目" description="设置每个工种的计件单价，记计件工时自动带出">
      <template #actions>
        <button class="btn-brand" @click="openForm()">添加计件项目</button>
      </template>
    </PageHeader>

    <div class="surface rounded-2xl overflow-hidden">
      <div v-if="items.length === 0" class="empty-state"><p>还没有计件项目，如：砌砖（块）、扎钢筋（吨）、贴砖（平方）</p></div>
      <div v-else class="divide-y divide-border">
        <div v-for="p in items" :key="p.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
          <span class="w-10 h-10 rounded-xl bg-violet-400/15 text-violet-500 flex items-center justify-center text-lg shrink-0">📦</span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-foreground truncate">{{ p.name }}</span>
              <span v-if="p.status === 'archived'" class="badge bg-muted text-muted-foreground">已归档</span>
            </div>
            <div class="text-xs text-muted-foreground mt-0.5">
              单价 <span class="font-semibold text-foreground">{{ fmtMoney(p.unit_price_cents) }}</span> / {{ p.unit }}
              <span v-if="p.note"> · {{ p.note }}</span>
            </div>
          </div>
          <div class="flex gap-1.5 shrink-0">
            <button class="btn-ghost !px-3 !py-2 text-xs" @click="openForm(p)">编辑</button>
            <button class="btn-ghost !px-3 !py-2 text-xs" @click="toggle(p)">{{ p.status === 'active' ? '归档' : '恢复' }}</button>
            <button class="btn-ghost !px-3 !py-2 text-xs !text-red-500" @click="remove(p)">删除</button>
          </div>
        </div>
      </div>
    </div>

    <Modal v-model="formOpen" :title="form.id ? '编辑计件项目' : '添加计件项目'">
      <div class="space-y-4">
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">名称 *</label>
          <input v-model="form.name" class="input-field" placeholder="如：砌砖" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">计量单位</label>
            <input v-model="form.unit" class="input-field" placeholder="个 / 平方 / 车 / 吨" />
          </div>
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">单价（元）*</label>
            <input v-model="priceInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="0.00" />
          </div>
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
import { listPieceItems, createPieceItem, updatePieceItem, deletePieceItem, type PieceItem } from '../api/worklog'
import { fmtMoney, yuanToCents, centsToYuanInput } from '../utils/format'

const items = ref<PieceItem[]>([])
const formOpen = ref(false)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const form = ref<PieceItem>({ name: '' })
const priceInput = ref('')

async function load() {
  try {
    const res = await listPieceItems()
    if (res.data?.code === 0) items.value = res.data.data
  } catch {}
}

function openForm(p?: PieceItem) {
  form.value = p ? { ...p } : { name: '', unit: '个', status: 'active' }
  priceInput.value = centsToYuanInput(p?.unit_price_cents)
  formOpen.value = true
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

async function save() {
  if (!form.value.name.trim()) return toast('请填写名称', 'error')
  const cents = priceInput.value.trim() ? yuanToCents(priceInput.value) : null
  if (cents === null || cents <= 0) return toast('请填写大于 0 的单价', 'error')
  saving.value = true
  const payload = { ...form.value, unit_price_cents: cents }
  try {
    if (payload.id) await updatePieceItem(payload.id, payload)
    else await createPieceItem(payload)
    toast('保存成功')
    formOpen.value = false
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

async function toggle(p: PieceItem) {
  try {
    await updatePieceItem(p.id!, { ...p, status: p.status === 'active' ? 'archived' : 'active' })
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

async function remove(p: PieceItem) {
  if (!confirm(`确定删除计件项目「${p.name}」？`)) return
  try {
    await deletePieceItem(p.id!)
    toast('已删除')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '删除失败', 'error')
  }
}

onMounted(load)
</script>
