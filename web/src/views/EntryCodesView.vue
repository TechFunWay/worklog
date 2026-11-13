<template>
  <div class="page-container animate-fade-in">
    <PageHeader
      title="扫码报工"
      description="生成二维码贴到工位，工人扫码只填数量，提交后班组长审核"
    >
      <template #actions>
        <button v-if="ready && store.canManage" type="button" class="btn-brand min-h-11 whitespace-nowrap" @click="openForm()">新建报工码</button>
      </template>
    </PageHeader>

    <div v-if="!ready" class="surface rounded-2xl p-6 text-center text-sm text-muted-foreground">加载中…</div>
    <div v-else-if="!store.canManage" class="surface rounded-2xl">
      <div class="empty-state space-y-2">
        <p class="text-base font-semibold text-foreground">仅老板/班组长可管理报工码</p>
        <p>你的账号是员工，报工码由班组长生成；扫码或让班组长代录即可。</p>
        <RouterLink to="/admin/records" class="inline-flex min-h-11 items-center justify-center px-4 text-sm font-semibold text-brand-600 hover:underline dark:text-brand-400">去我的记工 →</RouterLink>
      </div>
    </div>

    <template v-else>

    <!-- 三步说明 + 地址提示 -->
    <div class="surface rounded-2xl p-4">
      <div class="grid gap-2 sm:grid-cols-3">
        <div class="flex items-start gap-2.5 rounded-xl bg-muted/50 px-3 py-2.5">
          <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-500/15 text-xs font-bold text-brand-600 dark:text-brand-400">1</span>
          <div class="min-w-0 text-xs leading-relaxed">
            <div class="font-semibold text-foreground">新建报工码</div>
            <div class="text-muted-foreground">选产品 / 工人，生成免登录二维码</div>
          </div>
        </div>
        <div class="flex items-start gap-2.5 rounded-xl bg-muted/50 px-3 py-2.5">
          <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-500/15 text-xs font-bold text-brand-600 dark:text-brand-400">2</span>
          <div class="min-w-0 text-xs leading-relaxed">
            <div class="font-semibold text-foreground">打印二维码贴到工位</div>
            <div class="text-muted-foreground">支持打印海报或每人一张专属码</div>
          </div>
        </div>
        <div class="flex items-start gap-2.5 rounded-xl bg-muted/50 px-3 py-2.5">
          <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-brand-500/15 text-xs font-bold text-brand-600 dark:text-brand-400">3</span>
          <div class="min-w-0 text-xs leading-relaxed">
            <div class="font-semibold text-foreground">工人手机扫码填数量</div>
            <div class="text-muted-foreground">提交后你在「报工审核」里确认</div>
          </div>
        </div>
      </div>
      <div class="mt-3 rounded-xl border border-amber-400/30 bg-amber-400/10 px-3 py-2.5">
        <div class="flex items-start gap-2">
          <span class="shrink-0 text-sm leading-5" aria-hidden="true">📶</span>
          <div class="min-w-0 space-y-1 text-[11px] leading-relaxed text-amber-800 sm:text-xs dark:text-amber-200">
            <p>
              二维码里的地址必须工人手机能打开。用飞牛网关地址（含 /app/）时工人需要 NAS 登录，建议用「内网 IP:端口」或「公网直连地址」打开本页再生成；每个报工码可单独填「报工地址」覆盖。
            </p>
            <p class="break-all">当前页面地址：<span class="font-semibold">{{ currentBase }}</span></p>
          </div>
        </div>
      </div>
    </div>

    <!-- 统计 -->
    <div class="grid grid-cols-3 gap-2">
      <div class="surface rounded-2xl p-3">
        <div class="text-[11px] text-muted-foreground">报工码数量</div>
        <div class="mt-0.5 text-lg font-bold tabular-nums text-foreground">{{ codes.length }}</div>
      </div>
      <div class="surface rounded-2xl p-3">
        <div class="text-[11px] text-muted-foreground">今日提交</div>
        <div class="mt-0.5 text-lg font-bold tabular-nums text-foreground">{{ todayTotal }}</div>
      </div>
      <div class="surface rounded-2xl p-3">
        <div class="text-[11px] text-muted-foreground">待审核</div>
        <div class="mt-0.5 text-lg font-bold tabular-nums text-foreground">{{ pendingTotal }}</div>
      </div>
    </div>

    <!-- 报工码列表 -->
    <div class="surface rounded-2xl overflow-hidden">
      <div class="flex items-center justify-between gap-2 border-b border-border px-4 py-3">
        <div class="text-sm font-bold text-foreground">报工码（{{ codes.length }}）</div>
        <button type="button" class="btn-ghost min-h-11 !px-3 text-xs" :disabled="loading" @click="load">
          {{ loading ? '加载中…' : '刷新' }}
        </button>
      </div>

      <div v-if="loading && codes.length === 0" class="py-10 text-center text-sm text-muted-foreground">加载中…</div>

      <div v-else-if="loadError" class="empty-state space-y-3">
        <p>报工码加载失败，请检查网络后重试</p>
        <button type="button" class="btn-ghost min-h-11 !px-4 text-sm" @click="reload">重新加载</button>
      </div>

      <div v-else-if="codes.length === 0" class="empty-state space-y-3">
        <p>还没有报工码。新建一张二维码贴到工位，工人扫码就能报数量。</p>
        <button type="button" class="btn-brand min-h-11 !px-4" @click="openForm()">新建报工码</button>
      </div>

      <template v-else>
        <!-- 桌面：表格 -->
        <div class="hidden md:block overflow-x-auto">
          <table class="w-full text-sm">
            <thead>
              <tr class="bg-muted/60 text-left text-xs uppercase tracking-wider text-muted-foreground">
                <th class="whitespace-nowrap px-4 py-3 font-semibold">名称</th>
                <th class="whitespace-nowrap px-4 py-3 font-semibold">绑定产品</th>
                <th class="whitespace-nowrap px-4 py-3 font-semibold">绑定工人</th>
                <th class="whitespace-nowrap px-4 py-3 font-semibold">今日 · 待审 · 累计</th>
                <th class="whitespace-nowrap px-4 py-3 font-semibold">状态</th>
                <th class="whitespace-nowrap px-4 py-3 text-right font-semibold">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border">
              <tr v-for="c in codes" :key="c.id" class="transition-colors hover:bg-muted/40">
                <td class="px-4 py-3">
                  <div class="max-w-[200px] truncate font-semibold text-foreground">{{ c.name }}</div>
                  <div v-if="c.note" class="max-w-[200px] truncate text-xs text-muted-foreground">{{ c.note }}</div>
                  <div v-if="c.expires_at" class="mt-0.5 text-[11px] text-muted-foreground">截止 {{ c.expires_at }}</div>
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span v-if="c.piece_name" class="text-foreground">{{ c.piece_name }}</span>
                  <span v-else class="text-muted-foreground">—</span>
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span v-if="c.worker_name" class="text-foreground">{{ c.worker_name }}</span>
                  <span v-else class="text-muted-foreground">—</span>
                </td>
                <td class="whitespace-nowrap px-4 py-3 tabular-nums text-muted-foreground">
                  <span class="font-semibold text-foreground">{{ c.today_count || 0 }}</span>
                  <span class="px-1">·</span>
                  <span>{{ c.pending_count || 0 }}</span>
                  <span class="px-1">·</span>
                  <span>{{ c.total_count || 0 }}</span>
                </td>
                <td class="whitespace-nowrap px-4 py-3">
                  <span class="badge" :class="statusClass(c)">{{ statusLabel(c) }}</span>
                </td>
                <td class="px-4 py-3">
                  <div class="flex items-center justify-end gap-1">
                    <button type="button" class="btn-ghost min-h-11 !px-2.5 text-xs whitespace-nowrap" @click="openQr(c)">二维码</button>
                    <button type="button" class="btn-ghost min-h-11 !px-2.5 text-xs whitespace-nowrap" @click="copyLink(c)">复制链接</button>
                    <button type="button" class="btn-ghost min-h-11 !px-2.5 text-xs whitespace-nowrap" @click="openForm(c)">编辑</button>
                    <button type="button" class="btn-ghost min-h-11 !px-2.5 text-xs whitespace-nowrap" @click="toggleEnabled(c)">
                      {{ c.enabled ? '停用' : '启用' }}
                    </button>
                    <button type="button" class="btn-ghost min-h-11 !px-2.5 text-xs whitespace-nowrap !text-red-500" @click="remove(c)">删除</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- 手机：卡片列表，操作按钮内联可见 -->
        <div class="md:hidden p-2 space-y-1.5">
          <div v-for="c in codes" :key="c.id" class="rounded-xl border border-border bg-surface/80 px-3 py-2.5">
            <div class="flex items-start gap-2">
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-brand-500/15 text-sm font-bold text-brand-600 dark:text-brand-400">码</span>
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <span class="truncate font-semibold text-foreground">{{ c.name }}</span>
                  <span class="badge shrink-0" :class="statusClass(c)">{{ statusLabel(c) }}</span>
                </div>
                <div class="mt-0.5 truncate text-[11px] text-muted-foreground">{{ bindingText(c) }}</div>
                <div class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-0.5 text-[11px] text-muted-foreground">
                  <span>今日 <span class="font-semibold text-foreground">{{ c.today_count || 0 }}</span></span>
                  <span>待审 <span class="font-semibold text-foreground">{{ c.pending_count || 0 }}</span></span>
                  <span>累计 <span class="font-semibold text-foreground">{{ c.total_count || 0 }}</span></span>
                  <span v-if="c.expires_at">截止 {{ c.expires_at }}</span>
                </div>
              </div>
            </div>
            <div class="mt-1.5 grid grid-cols-3 gap-1.5">
              <button type="button" class="btn-brand min-h-11 !px-1 text-xs" @click="openQr(c)">二维码</button>
              <button type="button" class="btn-ghost min-h-11 !px-1 text-xs" @click="copyLink(c)">复制</button>
              <button type="button" class="btn-ghost min-h-11 !px-1 text-xs" @click="openForm(c)">编辑</button>
              <button type="button" class="btn-ghost min-h-11 !px-1 text-xs" @click="toggleEnabled(c)">{{ c.enabled ? '停用' : '启用' }}</button>
              <button type="button" class="btn-ghost col-span-2 min-h-11 !px-1 text-xs !text-red-500" @click="remove(c)">删除</button>
            </div>
          </div>
        </div>
      </template>
    </div>

    </template>

    <!-- 新建 / 编辑弹窗 -->
    <Modal v-model="formOpen" :title="form.id ? '编辑报工码' : '新建报工码'">
      <div class="space-y-3.5">
        <div>
          <label class="mb-1.5 block text-sm font-medium text-foreground">名称 *</label>
          <input v-model="form.name" class="input-field" maxlength="40" placeholder="如：3 号楼钢筋组 · 张师傅" />
          <p class="mt-1 text-[11px] text-muted-foreground">2-40 个字，写在二维码上方，工人一眼知道是哪个活。</p>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="mb-1.5 block text-sm font-medium text-foreground">计件项目</label>
            <SelectMenu v-model="form.piece_item_id" :options="pieceOptions" class="w-full" />
            <p class="mt-1 text-[11px] text-muted-foreground">绑定后扫码自动带出产品</p>
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium text-foreground">工人</label>
            <SelectMenu v-model="form.worker_id" :options="workerOptions" class="w-full" />
            <p class="mt-1 text-[11px] text-muted-foreground">绑定后工人扫码只填数量</p>
          </div>
        </div>

        <div>
          <label class="mb-1.5 block text-sm font-medium text-foreground">工地（可选）</label>
          <SelectMenu v-model="form.project_id" :options="projectOptions" class="w-full" />
        </div>

        <div>
          <label class="mb-1.5 block text-sm font-medium text-foreground">报工地址（可选）</label>
          <input v-model="form.base_url" class="input-field" inputmode="url" :placeholder="currentBase" />
          <p class="mt-1 break-all text-[11px] text-muted-foreground">
            二维码里用的地址。留空用当前页面地址 {{ currentBase }}；工人手机打不开飞牛网关地址（含 /app/，需要 NAS 登录）时，填内网 IP:端口 或公网直连地址。
          </p>
        </div>

        <div class="flex items-center justify-between gap-3 rounded-xl border border-border bg-surface/60 px-3 py-2">
          <div class="min-w-0">
            <div class="text-sm font-medium text-foreground">自动通过</div>
            <div class="text-[11px] text-muted-foreground">扫码提交直接生效，不经过审核</div>
          </div>
          <button
            type="button"
            role="switch"
            :aria-checked="form.auto_approve"
            aria-label="自动通过"
            class="inline-flex min-h-11 min-w-11 shrink-0 items-center justify-center"
            @click="form.auto_approve = !form.auto_approve"
          >
            <span
              class="relative block h-6 w-11 rounded-full transition-colors"
              :class="form.auto_approve ? 'bg-brand-500' : 'bg-muted'"
            >
              <span
                class="absolute top-0.5 block h-5 w-5 rounded-full bg-white shadow transition-transform"
                :class="form.auto_approve ? 'translate-x-5' : 'translate-x-0.5'"
              ></span>
            </span>
          </button>
        </div>

        <div>
          <label class="mb-1.5 block text-sm font-medium text-foreground">截止日期（可选）</label>
          <input v-model="form.expires_at" type="date" class="input-field" />
          <p v-if="expiryHint" class="mt-1 text-[11px] text-amber-600 dark:text-amber-300">{{ expiryHint }}</p>
        </div>

        <div>
          <label class="mb-1.5 block text-sm font-medium text-foreground">备注（选填）</label>
          <textarea v-model="form.note" class="input-field" rows="2" maxlength="120" placeholder="会显示在工人扫码页，如：只报今天产量"></textarea>
        </div>

        <div v-if="form.id" class="rounded-xl border border-border bg-muted/40 px-3 py-2.5">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0">
              <div class="text-xs font-medium text-foreground">重置二维码令牌</div>
              <div class="text-[11px] text-muted-foreground">旧二维码立刻失效，需要重新打印张贴。</div>
            </div>
            <button type="button" class="btn-ghost min-h-11 shrink-0 !px-3 text-xs" :disabled="saving" @click="rotateToken">重置</button>
          </div>
        </div>

        <div class="flex gap-3 pt-1">
          <button type="button" class="btn-ghost min-h-11 flex-1" @click="closeForm">取消</button>
          <button type="button" class="btn-brand min-h-11 flex-1" :disabled="saving" @click="save">
            {{ saving ? '保存中…' : '保存' }}
          </button>
        </div>
      </div>
    </Modal>

    <EntryQrDialog v-model="qrOpen" :code="qrCode" :workers="workers" :products="pieces" @notify="toast" />

    <ConfirmDialog
      v-model="confirmOpen"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-text="confirmText"
      confirm-type="danger"
      @confirm="confirmAction()"
    />
    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import PageHeader from '../components/PageHeader.vue'
import Modal from '../components/Modal.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import EntryQrDialog from '../components/EntryQrDialog.vue'
import SelectMenu from '../components/SelectMenu.vue'
import Toast from '../components/Toast.vue'
import { useWorklogStore } from '../stores/worklog'
import {
  getReviewSummary,
  createEntryCode,
  deleteEntryCode,
  listEntryCodes,
  listPieceItems,
  listProjects,
  listWorkers,
  rotateEntryCodeToken,
  updateEntryCode,
  type EntryCode,
  type EntryCodeInput,
  type PieceItem,
  type Project,
  type ReviewSummary,
  type Worker,
} from '../api/worklog'

interface EntryForm {
  id: number | null
  name: string
  piece_item_id: number | ''
  worker_id: number | ''
  project_id: number | ''
  base_url: string
  auto_approve: boolean
  expires_at: string
  note: string
  enabled: boolean
}

const store = useWorklogStore()
const ready = ref(false)
const codes = ref<EntryCode[]>([])
const pieces = ref<PieceItem[]>([])
const workers = ref<Worker[]>([])
const projects = ref<Project[]>([])
const summary = ref<ReviewSummary | null>(null)
const loading = ref(true)
const loadError = ref(false)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

const form = ref<EntryForm>(emptyForm())
const formOpen = ref(false)

const qrOpen = ref(false)
const qrCode = ref<EntryCode | null>(null)

const today = todayStr()

function emptyForm(): EntryForm {
  return {
    id: null,
    name: '',
    piece_item_id: '',
    worker_id: '',
    project_id: '',
    base_url: '',
    auto_approve: false,
    expires_at: '',
    note: '',
    enabled: true,
  }
}

function todayStr(): string {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

const currentBase = (() => {
  let base = window.location.origin + import.meta.env.BASE_URL
  if (!base.endsWith('/')) base += '/'
  return base
})()

const todayTotal = computed(() => codes.value.reduce((sum, c) => sum + (c.today_count || 0), 0))
const pendingTotal = computed(() => summary.value?.pending_count || 0)

const pieceOptions = computed(() => [
  { value: '', label: '不绑定产品（扫码后让工人选）' },
  ...pieces.value.map((p) => ({
    value: p.id as number,
    label: p.unit ? `${p.name}（${p.unit}）` : p.name,
  })),
])

const workerOptions = computed(() => [
  { value: '', label: '不绑定工人（扫码后选姓名）' },
  ...workers.value.map((w) => ({ value: w.id as number, label: w.name })),
])

const projectOptions = computed(() => [
  { value: '', label: '不选工地' },
  ...projects.value.map((p) => ({ value: p.id as number, label: p.name })),
])

const expiryHint = computed(() => {
  if (!form.value.expires_at) return ''
  if (form.value.expires_at < today) return '截止日期已过，请改到未来日期或清空后再保存。'
  return ''
})

function isExpired(code: EntryCode): boolean {
  return !!code.expires_at && code.expires_at < today
}

function statusLabel(code: EntryCode): string {
  if (isExpired(code)) return '已过期'
  return code.enabled ? '启用' : '停用'
}

function statusClass(code: EntryCode): string {
  if (isExpired(code)) return 'bg-rose-100 text-rose-700 dark:bg-rose-500/15 dark:text-rose-300'
  return code.enabled
    ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
    : 'bg-muted text-muted-foreground'
}

function bindingText(code: EntryCode): string {
  const parts: string[] = []
  if (code.piece_name) parts.push(`产品：${code.piece_name}`)
  if (code.worker_name) parts.push(`工人：${code.worker_name}`)
  if (code.project_name) parts.push(`工地：${code.project_name}`)
  return parts.length ? parts.join(' · ') : '产品、工人在扫码页选择'
}

function toast(message: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = message
  toastType.value = type
  window.setTimeout(() => (toastMsg.value = ''), 3000)
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

// 链接构造：base 优先取报工码自己的 base_url，否则当前站点地址；保证以 / 结尾。
// 与 EntryQrDialog.vue 里的同构逻辑保持一致。
function entryBase(code: EntryCode): string {
  let base = (code.base_url || '').trim()
  if (!base) base = currentBase
  if (!base.endsWith('/')) base += '/'
  return base
}

function entryLink(code: EntryCode): string {
  return new URL(`entry/${code.token}`, entryBase(code)).toString()
}

async function copyText(text: string): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // 继续走回退方案
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.top = '0'
    ta.style.left = '0'
    ta.style.opacity = '0'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    ta.remove()
    return ok
  } catch {
    return false
  }
}

async function copyLink(code: EntryCode) {
  let link = ''
  try {
    link = entryLink(code)
  } catch {
    toast('报工地址格式不正确，请编辑后重试', 'error')
    return
  }
  const ok = await copyText(link)
  toast(ok ? '链接已复制' : '复制失败，请手动复制', ok ? 'success' : 'error')
}

async function load() {
  loading.value = true
  loadError.value = false
  // 列表与待审核统计分开取：统计接口失败不应把已经拿到的列表也丢掉。
  const [codesResult, summaryResult] = await Promise.allSettled([listEntryCodes(), getReviewSummary()])
  if (codesResult.status === 'fulfilled') {
    const res = codesResult.value
    if (res.data?.code === 0) {
      const data = res.data.data as { items?: EntryCode[] } | EntryCode[] | undefined
      codes.value = Array.isArray(data) ? data : data?.items || []
      syncQrCode()
    } else {
      loadError.value = true
      toast(res.data?.message || '加载报工码失败', 'error')
    }
  } else {
    loadError.value = true
    const e = codesResult.reason
    toast(e?.response?.data?.message || '加载报工码失败', 'error')
  }
  if (summaryResult.status === 'fulfilled' && summaryResult.value.data?.code === 0) {
    summary.value = summaryResult.value.data.data as ReviewSummary
  }
  loading.value = false
}

async function reload() {
  await Promise.all([load(), loadRefs()])
}

async function loadRefs() {
  try {
    const [pieceRes, workerRes, projectRes] = await Promise.all([
      listPieceItems({ status: 'active' }),
      listWorkers({ status: 'active' }),
      listProjects({ status: 'active' }),
    ])
    if (pieceRes.data?.code === 0) pieces.value = pieceRes.data.data || []
    if (workerRes.data?.code === 0) workers.value = workerRes.data.data || []
    if (projectRes.data?.code === 0) projects.value = projectRes.data.data || []
  } catch (e: any) {
    toast(e.response?.data?.message || '加载产品 / 工人 / 工地失败', 'error')
  }
}

function syncQrCode() {
  const current = qrCode.value
  if (!current) return
  const fresh = codes.value.find((c) => c.id === current.id)
  if (fresh) qrCode.value = fresh
}

function openForm(code?: EntryCode) {
  form.value = code
    ? {
        id: code.id,
        name: code.name,
        piece_item_id: code.piece_item_id ?? '',
        worker_id: code.worker_id ?? '',
        project_id: code.project_id ?? '',
        base_url: code.base_url || '',
        auto_approve: !!code.auto_approve,
        expires_at: code.expires_at || '',
        note: code.note || '',
        enabled: code.enabled !== false,
      }
    : emptyForm()
  formOpen.value = true
}

// 取消：只关闭弹窗，不提交表单（保存中的请求会自行结束）。
function closeForm() {
  formOpen.value = false
}

function openQr(code: EntryCode) {
  qrCode.value = code
  qrOpen.value = true
}

// 关联 id 留空 = 不绑定；后端用 0 表示清空，这里统一发 0（JSON null 会被当作「没改」）。
function refId(value: number | ''): number {
  return value === '' ? 0 : Number(value)
}

async function save() {
  const name = form.value.name.trim()
  const nameLength = Array.from(name).length
  if (nameLength < 2 || nameLength > 40) {
    toast('名称请填 2-40 个字', 'error')
    return
  }
  if (form.value.expires_at && form.value.expires_at < today) {
    toast('截止日期不能早于今天，可直接清空', 'error')
    return
  }
  const payload: EntryCodeInput = {
    name,
    piece_item_id: refId(form.value.piece_item_id),
    worker_id: refId(form.value.worker_id),
    project_id: refId(form.value.project_id),
    base_url: form.value.base_url.trim(),
    auto_approve: form.value.auto_approve,
    expires_at: form.value.expires_at.trim(),
    note: form.value.note.trim(),
  }
  saving.value = true
  try {
    if (form.value.id) {
      const res = await updateEntryCode(form.value.id, { ...payload, enabled: form.value.enabled })
      if (res.data?.code !== 0) {
        toast(res.data?.message || '保存失败', 'error')
        return
      }
      formOpen.value = false
      toast('已保存')
      await load()
    } else {
      const res = await createEntryCode({ ...payload, enabled: true })
      if (res.data?.code !== 0) {
        toast(res.data?.message || '创建失败', 'error')
        return
      }
      const created = res.data?.data as EntryCode | undefined
      formOpen.value = false
      toast('报工码已创建')
      await load()
      if (created?.token) {
        const fresh = codes.value.find((c) => c.id === created.id) || created
        openQr(fresh)
      }
    }
  } catch (e: any) {
    toast(e.response?.data?.message || '保存失败', 'error')
  } finally {
    saving.value = false
  }
}

async function toggleEnabled(code: EntryCode) {
  try {
    const res = await updateEntryCode(code.id, { enabled: !code.enabled })
    if (res.data?.code !== 0) {
      toast(res.data?.message || '操作失败', 'error')
      return
    }
    toast(code.enabled ? '已停用' : '已启用')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

function remove(code: EntryCode) {
  askConfirm('删除报工码', `确定删除报工码「${code.name}」？删除后已经贴出去的二维码会立刻失效。`, async () => {
    try {
      await deleteEntryCode(code.id)
      if (qrCode.value?.id === code.id) qrOpen.value = false
      toast('已删除')
      await load()
    } catch (e: any) {
      toast(e.response?.data?.message || '删除失败', 'error')
    }
  }, '删除')
}

function rotateToken() {
  const id = form.value.id
  if (!id) return
  askConfirm(
    '重置二维码令牌',
    '重置后旧二维码立刻失效，工人扫码会提示报工码无效，需要重新打印张贴。确定重置？',
    async () => {
      try {
        const res = await rotateEntryCodeToken(id)
        if (res.data?.code !== 0) {
          toast(res.data?.message || '重置失败', 'error')
          return
        }
        const updated = res.data?.data as EntryCode | undefined
        formOpen.value = false
        toast('令牌已重置，请重新打印二维码')
        await load()
        const fresh = codes.value.find((c) => c.id === id) || updated
        if (fresh) openQr(fresh)
      } catch (e: any) {
        toast(e.response?.data?.message || '重置失败', 'error')
      }
    },
    '重置',
  )
}

onMounted(async () => {
  await store.load()
  ready.value = true
  if (!store.canManage) return
  await Promise.all([load(), loadRefs()])
})
</script>
