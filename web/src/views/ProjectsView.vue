<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="工地管理" description="同时干几个工地？记工时选一下，统计可按工地筛选">
      <template #actions>
        <button class="btn-brand" @click="openForm()">添加工地</button>
      </template>
    </PageHeader>

    <div class="surface rounded-2xl overflow-hidden">
      <div v-if="projects.length === 0" class="empty-state"><p>还没有工地（不建工地也可以正常记工）</p></div>

      <template v-else>
        <!-- 桌面：行布局 -->
        <div class="hidden md:block divide-y divide-border">
          <div v-for="p in projects" :key="p.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
            <span class="w-10 h-10 rounded-xl bg-teal-400/15 text-teal-500 flex items-center justify-center text-lg shrink-0">🏗️</span>
            <div class="min-w-0 flex-1">
              <div class="flex items-center gap-2">
                <span class="font-semibold text-foreground truncate">{{ p.name }}</span>
                <span v-if="p.status === 'archived'" class="badge bg-muted text-muted-foreground">已归档</span>
              </div>
              <div v-if="p.note" class="text-xs text-muted-foreground mt-0.5 truncate">{{ p.note }}</div>
              <div v-if="hasDefaultPrice(p)" class="mt-0.5 text-xs text-muted-foreground">
                默认单价：<template v-if="p.day_price_cents">日工 ¥{{ fmtYuanShort(p.day_price_cents) }}/个工</template>
                <template v-if="p.day_price_cents && p.hour_price_cents"> · </template>
                <template v-if="p.hour_price_cents">小时 ¥{{ fmtYuanShort(p.hour_price_cents) }}/小时</template>
              </div>
            </div>
            <div class="flex gap-1.5 shrink-0">
              <button class="btn-ghost !px-3 !py-2 text-xs" @click="openForm(p)">编辑</button>
              <button class="btn-ghost !px-3 !py-2 text-xs whitespace-nowrap" @click="openPrices(p)">工人单价</button>
              <button class="btn-ghost !px-3 !py-2 text-xs" @click="toggle(p)">{{ p.status === 'active' ? '归档' : '恢复' }}</button>
              <button class="btn-ghost !px-3 !py-2 text-xs !text-red-500" @click="remove(p)">删除</button>
            </div>
          </div>
        </div>

        <!-- 手机：卡片列表（操作按钮卡片内一行可见，不横向溢出） -->
        <div class="md:hidden p-2 space-y-1.5">
          <div v-for="p in projects" :key="p.id" class="rounded-xl border border-border bg-surface/80 px-3 py-2.5">
            <div class="flex items-start gap-2">
              <span class="w-9 h-9 rounded-xl bg-teal-400/15 text-teal-500 flex items-center justify-center text-base shrink-0">🏗️</span>
              <div class="min-w-0 flex-1">
                <div class="flex items-center gap-1.5">
                  <span class="truncate font-semibold text-foreground">{{ p.name }}</span>
                  <span v-if="p.status === 'archived'" class="badge bg-muted text-muted-foreground !px-1.5 !py-0 text-[10px]">已归档</span>
                </div>
                <div v-if="p.note" class="mt-0.5 truncate text-[11px] text-muted-foreground">{{ p.note }}</div>
                <div v-if="hasDefaultPrice(p)" class="mt-0.5 text-[11px] text-muted-foreground">
                  默认单价：<template v-if="p.day_price_cents">日工 ¥{{ fmtYuanShort(p.day_price_cents) }}/个工</template>
                  <template v-if="p.day_price_cents && p.hour_price_cents"> · </template>
                  <template v-if="p.hour_price_cents">小时 ¥{{ fmtYuanShort(p.hour_price_cents) }}/小时</template>
                </div>
              </div>
            </div>
            <div class="mt-1.5 flex gap-1.5">
              <button class="btn-ghost flex-1 !px-1 !py-1.5 text-xs whitespace-nowrap" @click="openForm(p)">编辑</button>
              <button class="btn-ghost flex-1 !px-1 !py-1.5 text-xs whitespace-nowrap" @click="openPrices(p)">工人单价</button>
              <button class="btn-ghost flex-1 !px-1 !py-1.5 text-xs whitespace-nowrap" @click="toggle(p)">{{ p.status === 'active' ? '归档' : '恢复' }}</button>
              <button class="btn-ghost flex-1 !px-1 !py-1.5 text-xs whitespace-nowrap !text-red-500" @click="remove(p)">删除</button>
            </div>
          </div>
        </div>
      </template>
    </div>

    <Modal v-model="formOpen" :title="form.id ? '编辑工地' : '添加工地'">
      <div class="space-y-4">
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">名称 *</label>
          <input v-model="form.name" class="input-field" placeholder="如：城东安置房 3 号楼" />
        </div>
        <div>
          <label class="text-sm font-medium text-foreground block mb-1.5">备注</label>
          <input v-model="form.note" class="input-field" placeholder="选填" />
        </div>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">默认日工单价（元/个工）</label>
            <input v-model="dayPriceInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="留空 = 按工人单价" />
          </div>
          <div>
            <label class="text-sm font-medium text-foreground block mb-1.5">默认小时单价（元/小时）</label>
            <input v-model="hourPriceInput" type="text" inputmode="decimal" class="input-field tabular-nums" placeholder="留空 = 按工人单价" />
          </div>
        </div>
        <p class="text-xs text-muted-foreground">留空或填 0 表示不设置，记工时按工人自身日薪/时薪（月薪工按当月天数折算）。</p>
        <div class="flex gap-3">
          <button type="button" class="btn-ghost min-h-11 flex-1" @click="closeForm">取消</button>
          <button type="button" class="btn-brand min-h-11 flex-1" :disabled="saving" @click="save">保存</button>
        </div>
      </div>
    </Modal>

    <ProjectPricesModal v-model="pricesOpen" :project="pricesProject" />

    <ConfirmDialog v-model="confirmOpen" :title="confirmTitle" :message="confirmMessage" :confirm-text="confirmText" confirm-type="danger" @confirm="confirmAction()" />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import ProjectPricesModal from '../components/ProjectPricesModal.vue'
import Toast from '../components/Toast.vue'
import { listProjects, createProject, updateProject, deleteProject, type Project } from '../api/worklog'
import { centsToYuanInput, fmtYuanShort, yuanToCents } from '../utils/format'

const projects = ref<Project[]>([])
const formOpen = ref(false)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const form = ref<Project>({ name: '' })
// 默认单价输入框：元为单位，留空/0 = 不设置
const dayPriceInput = ref('')
const hourPriceInput = ref('')

// 工人单价弹窗（逐工人在本工地的覆盖价）
const pricesOpen = ref(false)
const pricesProject = ref<Project | null>(null)

async function load() {
  try {
    const res = await listProjects()
    if (res.data?.code === 0) projects.value = res.data.data
  } catch {}
}

function hasDefaultPrice(p: Project): boolean {
  return !!(p.day_price_cents || p.hour_price_cents)
}

function openForm(p?: Project) {
  form.value = p ? { ...p } : { name: '', status: 'active' }
  dayPriceInput.value = centsToYuanInput(p?.day_price_cents)
  hourPriceInput.value = centsToYuanInput(p?.hour_price_cents)
  formOpen.value = true
}

// 取消：只关闭弹窗，不提交表单。
// 与右上角 X 保持一致——保存中 X 仍可关，这里也不该把用户堵在弹窗里；
// 保存请求会自行结束（save() 的 finally 会把 saving 复位），关掉弹窗不影响结果。
function closeForm() {
  formOpen.value = false
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

function openPrices(p: Project) {
  pricesProject.value = p
  pricesOpen.value = true
}

// 单价输入（元，字符串）→ 分。留空 = 0（不设置）；非法/负数返回 null。
function parsePrice(input: string): number | null {
  const raw = input.trim()
  if (raw === '') return 0
  return yuanToCents(raw)
}

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

async function save() {
  if (!form.value.name.trim()) return toast('请填写名称', 'error')
  const dayCents = parsePrice(dayPriceInput.value)
  if (dayCents === null) return toast('默认日工单价格式不正确，请填非负数字', 'error')
  const hourCents = parsePrice(hourPriceInput.value)
  if (hourCents === null) return toast('默认小时单价格式不正确，请填非负数字', 'error')
  const payload: Project = { ...form.value, day_price_cents: dayCents, hour_price_cents: hourCents }
  saving.value = true
  try {
    if (payload.id) await updateProject(payload.id, payload)
    else await createProject(payload)
    toast('保存成功')
    formOpen.value = false
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

async function toggle(p: Project) {
  try {
    // 带上默认单价一起更新，避免旧后端/网关丢弃字段时把已设置的单价清空
    await updateProject(p.id!, { ...p, status: p.status === 'active' ? 'archived' : 'active' })
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

function remove(p: Project) {
  askConfirm('删除工地', `确定删除工地「${p.name}」？`, async () => {
    try {
      await deleteProject(p.id!)
      toast('已删除')
      await load()
    } catch (e: any) {
      toast(e.response?.data?.message || '删除失败', 'error')
    }
  }, '删除')
}

onMounted(load)
</script>
