<template>
  <div class="min-h-dvh bg-background text-foreground">
    <div class="mx-auto w-full max-w-md px-4 pt-5 pb-32 sm:pb-10">
      <!-- 品牌头：工人扫码后第一眼知道这是哪个厂的报工入口 -->
      <div class="flex items-center gap-3 mb-4">
        <div class="w-11 h-11 rounded-2xl bg-brand-gradient flex items-center justify-center text-white font-display font-extrabold text-lg shadow-glow shrink-0">工</div>
        <div class="min-w-0">
          <p class="text-[11px] text-muted-foreground truncate">{{ info?.site_title || '工记' }}</p>
          <h1 class="text-lg font-extrabold text-foreground leading-tight">扫码报工</h1>
        </div>
      </div>

      <div v-if="loading" class="surface rounded-2xl p-6 text-center text-sm text-muted-foreground">加载中…</div>

      <div v-else-if="error" class="surface rounded-2xl p-6 text-center space-y-3">
        <div class="text-3xl">⚠️</div>
        <p class="text-sm text-foreground font-semibold">{{ error }}</p>
        <p class="text-xs text-muted-foreground">二维码失效、停用或网络不通时请联系班组长重新生成。</p>
        <button class="btn-ghost w-full min-h-11" @click="load">重新加载</button>
      </div>

      <template v-else-if="info">
        <!-- 任务信息 -->
        <div class="surface rounded-2xl p-4 mb-3">
          <div class="text-[11px] font-semibold text-muted-foreground">报工任务</div>
          <div class="mt-0.5 text-xl font-extrabold text-foreground leading-tight">{{ info.code.name }}</div>
          <p v-if="info.code.note" class="mt-1 text-xs text-muted-foreground leading-relaxed break-words">{{ info.code.note }}</p>
          <div class="mt-2 flex flex-wrap gap-1.5 text-[11px] text-muted-foreground">
            <span class="badge bg-brand-500/10 text-brand-600 dark:text-brand-400">{{ info.code.auto_approve ? '提交即生效' : '提交后等班组长确认' }}</span>
            <span class="badge bg-muted text-muted-foreground">只填数量</span>
          </div>
        </div>

        <!-- 成功反馈 -->
        <div v-if="success" class="rounded-2xl border border-emerald-400/40 bg-emerald-400/10 p-4 mb-3">
          <div class="flex items-start gap-3">
            <div class="w-10 h-10 rounded-full bg-emerald-500 text-white flex items-center justify-center text-xl shrink-0">✓</div>
            <div class="min-w-0 flex-1">
              <div class="text-base font-bold text-emerald-700 dark:text-emerald-300">{{ success.duplicate ? '已经提交过了' : '提交成功' }}</div>
              <p class="mt-0.5 text-xs text-emerald-800/80 dark:text-emerald-200/80">{{ success.message }} · {{ successDateLabel }}已报 {{ success.today_count }} 笔</p>
            </div>
          </div>
          <div v-if="todayItems.length" class="mt-3 rounded-xl bg-surface/70 divide-y divide-border/60">
            <div v-for="it in todayItems" :key="it.id" class="flex items-center justify-between gap-2 px-3 py-2 text-xs">
              <span class="min-w-0 truncate text-foreground">{{ it.piece_name || '产品' }}</span>
              <span class="shrink-0 tabular-nums text-foreground font-semibold">{{ it.quantity }}{{ it.piece_unit || '件' }}</span>
              <span class="shrink-0 badge !px-1.5 !py-0 text-[10px]" :class="it.status === 'approved' ? 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-300' : 'bg-amber-400/15 text-amber-600 dark:text-amber-300'">{{ it.status === 'approved' ? '已通过' : '待审核' }}</span>
            </div>
          </div>
          <button class="btn-ghost w-full min-h-11 mt-3" @click="success = null; quantity = ''">再报一笔</button>
        </div>

        <template v-if="!success">
          <!-- 产品 -->
          <div class="surface rounded-2xl p-4 mb-3">
            <div class="text-sm font-bold text-foreground mb-2">产品</div>
            <div v-if="info.product" class="rounded-xl bg-brand-500/10 border border-brand-500/20 px-3 py-2.5">
              <div class="text-lg font-extrabold text-foreground leading-tight break-words">{{ info.product.name }}</div>
              <div class="text-[11px] text-muted-foreground mt-0.5">单位：{{ info.product.unit || '件' }}</div>
            </div>
            <template v-else>
              <div class="grid grid-cols-2 gap-2">
                <button v-for="p in info.products" :key="p.id" class="min-h-12 rounded-xl border px-3 py-2 text-left transition-colors" :class="pieceId === p.id ? 'border-brand-500 bg-brand-500/10' : 'border-border bg-surface/70 hover:bg-muted'" @click="selectProduct(p.id)">
                  <div class="text-sm font-bold text-foreground leading-tight break-words">{{ p.name }}</div>
                  <div class="text-[11px] text-muted-foreground mt-0.5">{{ p.unit || '件' }}</div>
                </button>
              </div>
            </template>
          </div>

          <!-- 姓名 -->
          <div class="surface rounded-2xl p-4 mb-3">
            <div class="text-sm font-bold text-foreground mb-2">姓名</div>
            <div v-if="selectedWorker" class="flex items-center justify-between gap-2 rounded-xl bg-brand-500/10 border border-brand-500/20 px-3 py-2.5">
              <div class="min-w-0">
                <div class="text-lg font-extrabold text-foreground truncate">{{ workerLabel(selectedWorker) }}</div>
                <div class="text-[11px] text-muted-foreground">{{ info.worker_bound ? '姓名由二维码绑定' : '扫码已选好姓名，如需换人点右侧' }}</div>
              </div>
              <button v-if="!info.worker_bound" class="btn-ghost !px-3 min-h-10 text-xs shrink-0" @click="workerId = null">换人</button>
            </div>
            <template v-else>
              <input v-if="info.workers.length > 12" v-model="workerQuery" class="input-field mb-2" placeholder="输入姓名查找" />
              <div class="grid grid-cols-3 gap-1.5 max-h-60 overflow-y-auto overscroll-contain">
                <button v-for="w in filteredWorkers" :key="w.id" class="min-h-11 rounded-lg border border-border bg-surface/70 px-2 py-2 text-sm font-semibold text-foreground truncate hover:bg-muted" @click="selectWorker(w.id)">{{ workerLabel(w) }}</button>
              </div>
              <p v-if="filteredWorkers.length === 0" class="py-4 text-center text-xs text-muted-foreground">没有找到这个姓名</p>
            </template>
          </div>

          <!-- 数量 -->
          <div class="surface rounded-2xl p-4 mb-3">
            <label class="text-sm font-bold text-foreground block mb-2">数量（{{ selectedProduct?.unit || '件' }}）</label>
            <input v-model="quantity" type="text" inputmode="numeric" pattern="[0-9]*" placeholder="0" class="input-field text-center font-extrabold tabular-nums !py-3" style="font-size: 30px !important" />
            <div class="grid grid-cols-4 gap-2 mt-2">
              <button v-for="n in [1, 10, 50, 100]" :key="n" class="min-h-11 rounded-xl border border-border bg-surface/70 text-sm font-bold text-foreground hover:bg-muted" @click="bump(n)">+{{ n }}</button>
            </div>
            <div class="flex items-center justify-between mt-2">
              <div class="text-[11px] text-muted-foreground">只能填整数，单位按产品设置</div>
              <button class="text-xs font-semibold text-muted-foreground hover:text-foreground min-h-10 px-2" @click="quantity = ''">清空</button>
            </div>
          </div>

          <!-- 日期 -->
          <div class="surface rounded-2xl p-4 mb-3">
            <div class="text-sm font-bold text-foreground mb-2">日期</div>
            <div class="grid grid-cols-2 gap-2">
              <button class="min-h-11 rounded-xl border text-sm font-semibold" :class="date === info.today ? 'border-brand-500 bg-brand-500/10 text-brand-600 dark:text-brand-400' : 'border-border text-muted-foreground'" @click="date = info.today">今天</button>
              <button class="min-h-11 rounded-xl border text-sm font-semibold" :class="date === info.yesterday ? 'border-brand-500 bg-brand-500/10 text-brand-600 dark:text-brand-400' : 'border-border text-muted-foreground'" @click="date = info.yesterday">昨天</button>
            </div>
          </div>

          <!-- 手机端提交按钮固定在屏幕底部：表单再长也不用滚到底才能交 -->
          <div class="fixed inset-x-0 bottom-0 z-20 border-t border-border bg-background/95 px-4 py-3 backdrop-blur-sm sm:static sm:mt-3 sm:border-0 sm:bg-transparent sm:p-0 sm:backdrop-blur-none">
            <div class="mx-auto w-full max-w-md">
              <p v-if="formError" class="mb-2 rounded-xl bg-red-500/10 border border-red-500/30 px-3 py-2 text-xs text-red-600 dark:text-red-300">{{ formError }}</p>
              <button class="btn-brand w-full min-h-12 text-base" :disabled="submitting" @click="submit">
                {{ submitting ? '提交中…' : '提交报工' }}
              </button>
              <p class="mt-2 text-center text-[11px] text-muted-foreground">提交后{{ info.code.auto_approve ? '自动生效' : '由班组长确认后计入工资' }}</p>
            </div>
          </div>
        </template>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getPublicEntry, submitPublicEntry, type PublicEntryInfo, type PublicEntrySubmitResult, type PublicEntryTodayItem, type PublicEntryWorker } from '../api/worklog'
import { todayStr } from '../utils/format'

const route = useRoute()
const token = computed(() => String(route.params.token || ''))

const loading = ref(true)
const error = ref('')
const info = ref<PublicEntryInfo | null>(null)
const workerId = ref<number | null>(null)
const pieceId = ref<number | null>(null)
const quantity = ref('')
const date = ref(todayStr())
const workerQuery = ref('')
const submitting = ref(false)
const formError = ref('')
const success = ref<PublicEntrySubmitResult | null>(null)
const todayItems = ref<PublicEntryTodayItem[]>([]);

const successDateLabel = computed(() => {
  if (!success.value || !info.value) return '当天'
  if (success.value.date === info.value.today) return '今天'
  if (success.value.date === info.value.yesterday) return '昨天'
  return success.value.date
})

const selectedWorker = computed<PublicEntryWorker | null>(() => {
  if (!info.value) return null
  const id = workerId.value
  if (!id) return null
  return (info.value.workers || []).find((w) => w.id === id) || (info.value.worker?.id === id ? info.value.worker : null)
});
const selectedProduct = computed(() => {
  if (!info.value) return null
  if (info.value.product) return info.value.product
  return (info.value.products || []).find((p) => p.id === pieceId.value) || null
});
// 同名工人加序号，避免免登录页出现两个一模一样的姓名按钮。
const workerLabelMap = computed(() => {
  const counts = new Map<string, number>()
  for (const w of info.value?.workers || []) counts.set(w.name, (counts.get(w.name) || 0) + 1)
  const seen = new Map<string, number>()
  const labels = new Map<number, string>()
  for (const w of info.value?.workers || []) {
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

function workerLabel(w: PublicEntryWorker): string {
  return workerLabelMap.value.get(w.id) || w.name
}

const filteredWorkers = computed(() => {
  const list = info.value?.workers || []
  const q = workerQuery.value.trim()
  if (!q) return list
  return list.filter((w) => w.name.includes(q))
});

function workerStorageKey(): string {
  return `worklog-entry-worker-${token.value}`
}

async function load() {
  loading.value = true
  error.value = ''
  formError.value = ''
  try {
    const wParam = Number(route.query.w || 0)
    const res = await getPublicEntry(token.value, Number.isInteger(wParam) && wParam > 0 ? wParam : undefined)
    if (res.data?.code !== 0 || !res.data.data) {
      error.value = res.data?.message || '报工码无效'
      return
    }
    info.value = res.data.data as PublicEntryInfo
    if (info.value.site_title) document.title = `扫码报工 · ${info.value.site_title}`
    else document.title = '扫码报工 · 工记'
    date.value = info.value.today
    // 产品：绑定的直接带出；打印的工人专属码会带 ?p=产品 id 预设；
    // 未绑定时只有一个产品也自动选中，少一步操作
    const pParam = Number(route.query.p || 0)
    if (info.value.product) pieceId.value = info.value.product.id
    else if (Number.isInteger(pParam) && pParam > 0 && info.value.products.some((p) => p.id === pParam)) pieceId.value = pParam
    else if (info.value.products.length === 1) pieceId.value = info.value.products[0].id
    // 姓名：?w= / 绑定工人优先；否则用本机上次选的（同码）
    if (info.value.worker) {
      workerId.value = info.value.worker.id
      rememberWorker(info.value.worker.id)
    } else {
      let saved = 0
      try { saved = Number(localStorage.getItem(workerStorageKey()) || 0) } catch { /* 隐私模式忽略 */ }
      if (saved > 0 && info.value.workers.some((w) => w.id === saved)) workerId.value = saved
    }
  } catch (e: any) {
    error.value = e.response?.data?.message || '网络不通，请重新扫码或联系班组长'
  } finally {
    loading.value = false
  }
}

function rememberWorker(id: number) {
  try { localStorage.setItem(workerStorageKey(), String(id)) } catch { /* 隐私模式忽略 */ }
}

function selectWorker(id: number) {
  workerId.value = id;
  rememberWorker(id);
  formError.value = '';
}

function selectProduct(id: number) {
  pieceId.value = id;
  formError.value = '';
}

function bump(n: number) {
  const cur = parseInt(quantity.value || '0', 10) || 0
  quantity.value = String(Math.max(0, cur + n));
}

function fail(msg: string) {
  formError.value = msg;
}

async function submit() {
  formError.value = '';
  if (!selectedWorker.value) return fail('请选择你的姓名');
  if (!selectedProduct.value) return fail('请选择产品');
  const qty = Number(quantity.value);
  if (!Number.isInteger(qty) || qty <= 0) return fail('请填写整数数量');
  submitting.value = true;
  try {
    const res = await submitPublicEntry(token.value, {
      worker_id: selectedWorker.value.id,
      piece_item_id: selectedProduct.value.id,
      quantity: String(qty),
      date: date.value,
    });
    if (res.data?.code !== 0 || !res.data.data) {
      fail(res.data?.message || '提交失败，请重试');
      return;
    }
    const result = res.data.data as PublicEntrySubmitResult;
    success.value = result;
    todayItems.value = result.today_items || [];
    quantity.value = '';
  } catch (e: any) {
    fail(e.response?.data?.message || '提交失败，请检查网络后重试');
  } finally {
    submitting.value = false;
  }
}

onMounted(load);
</script>
