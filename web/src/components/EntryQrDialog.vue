<template>
  <Modal
    :model-value="modelValue"
    title="报工码二维码"
    @update:model-value="onModelValue"
  >
    <div v-if="code" class="space-y-3.5">
      <!-- 大二维码 + 码信息 -->
      <div class="text-center">
        <div class="mx-auto inline-flex items-center justify-center rounded-2xl border border-border bg-white p-2">
          <img
            v-if="qrSrc"
            :src="qrSrc"
            :alt="`${code.name}的报工二维码`"
            class="block h-[240px] w-[240px] sm:h-[280px] sm:w-[280px]"
          />
        </div>
        <div class="mt-2.5 break-words text-base font-bold text-foreground">{{ code.name }}</div>
        <div class="mt-1 flex flex-wrap items-center justify-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span v-if="code.piece_name">产品：{{ code.piece_name }}</span>
          <span v-if="code.worker_name">工人：{{ code.worker_name }}</span>
          <span v-if="!code.piece_name && !code.worker_name">产品、工人在扫码页选择</span>
        </div>
        <div class="mt-1.5">
          <span v-if="code.auto_approve" class="badge bg-emerald-500/15 text-emerald-600 dark:text-emerald-300">提交即生效</span>
          <span v-else class="badge bg-amber-400/15 text-amber-600 dark:text-amber-300">提交后需审核</span>
        </div>
      </div>

      <!-- 完整链接 / 复制 / 下载 -->
      <div class="rounded-xl border border-border bg-muted/40 px-3 py-2.5">
        <div class="text-[11px] font-medium text-muted-foreground">完整链接（工人手机用浏览器 / 微信打开）</div>
        <div class="mt-1 select-all break-all text-xs leading-relaxed text-foreground">{{ fullLink }}</div>
        <div class="mt-2 flex flex-wrap gap-2">
          <button type="button" class="btn-ghost min-h-11 !px-3 text-xs" @click="copyLink">复制链接</button>
          <a class="btn-ghost min-h-11 !px-3 text-xs" :href="qrSrc" :download="downloadName">下载 PNG</a>
        </div>
      </div>

      <!-- 打印 -->
      <div class="grid grid-cols-2 gap-2">
        <button type="button" class="btn-brand min-h-11 text-xs sm:text-sm" @click="printPoster">打印海报</button>
        <button type="button" class="btn-ghost min-h-11 text-xs sm:text-sm" @click="workerPanelOpen = !workerPanelOpen">
          打印工人专属码
        </button>
      </div>

      <!-- 工人专属码：每人一张，链接带 ?w=工人 id -->
      <div v-if="workerPanelOpen" class="space-y-3 rounded-xl border border-border bg-muted/30 p-3">
        <div class="flex items-start justify-between gap-2">
          <div class="min-w-0">
            <div class="text-sm font-semibold text-foreground">打印工人专属码</div>
            <p class="mt-0.5 text-[11px] leading-relaxed text-muted-foreground">
              每名在职工人一张卡片，二维码自动带出姓名，工人扫码后只填数量。最多 50 人。
            </p>
          </div>
          <span class="badge shrink-0 bg-muted text-muted-foreground">{{ printableWorkers.length }} 人</span>
        </div>
        <p v-if="workers.length > 50" class="text-[11px] text-amber-600 dark:text-amber-300">
          在职工人超过 50 人，这里只打印前 50 张；其余工人请用「不绑定工人」的产品码或在班组里分批生成。
        </p>
        <div>
          <label class="mb-1.5 block text-xs font-medium text-foreground">产品（可选，仅打印在卡片上）</label>
          <SelectMenu v-model="printProductId" :options="productOptions" class="w-full" />
          <p class="mt-1 text-[11px] text-muted-foreground">只影响卡片上印的产品名，不改变手机扫码可选的产品。</p>
        </div>
        <div v-if="printableWorkers.length === 0" class="py-2 text-center text-xs text-muted-foreground">
          还没有在职工人，先去「工人」里添加。
        </div>
        <button
          v-else
          type="button"
          class="btn-brand w-full min-h-11 text-sm"
          @click="printWorkerCards"
        >
          打印 {{ printableWorkers.length }} 张专属码
        </button>
      </div>
    </div>
    <div v-else class="py-10 text-center text-sm text-muted-foreground">请选择报工码</div>
  </Modal>

  <!-- 只用于打印的内容：屏幕上 display:none，@media print 时只显示这一块 -->
  <Teleport to="body">
    <div ref="printArea" class="entry-print-area">
      <template v-if="printMode === 'poster' && code">
        <div class="entry-print-poster">
          <div class="entry-print-kicker">扫码报工</div>
          <div class="entry-print-title">{{ code.name }}</div>
          <div class="entry-print-sub">{{ posterSub }}</div>
          <img class="entry-print-poster-qr" :src="qrSrc" :alt="code.name" />
          <div class="entry-print-steps">微信扫一扫 → 填数量 → 提交</div>
          <div v-if="code.note" class="entry-print-note">{{ code.note }}</div>
        </div>
      </template>

      <template v-else-if="printMode === 'workers' && code">
        <div class="entry-print-cards-title">{{ code.name }}</div>
        <div v-if="printProductLabel" class="entry-print-cards-product">产品：{{ printProductLabel }}</div>
        <div class="entry-print-cards">
          <div v-for="w in printableWorkers" :key="w.id" class="entry-print-card">
            <div class="entry-print-card-name">{{ cardWorkerName(w) }}</div>
            <div v-if="printProductLabel" class="entry-print-card-product">{{ printProductLabel }}</div>
            <img :src="qrFor(workerLink(w.id))" :alt="w.name" />
            <div class="entry-print-card-tip">微信扫一扫 → 填数量 → 提交</div>
          </div>
        </div>
      </template>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import Modal from './Modal.vue'
import SelectMenu from './SelectMenu.vue'
import type { EntryCode, PieceItem, Worker } from '../api/worklog'

const props = withDefaults(defineProps<{
  modelValue: boolean
  code: EntryCode | null
  workers?: Worker[]
  products?: PieceItem[]
}>(), {
  workers: () => [],
  products: () => [],
})

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  notify: [message: string, type: 'success' | 'error']
}>()

const workerPanelOpen = ref(false)
const printMode = ref<'' | 'poster' | 'workers'>('')
const printProductId = ref<number | ''>('')
const printArea = ref<HTMLElement | null>(null)

// 报工链接：优先用报工码自己的 base_url，否则用当前站点地址；保证 base 以 / 结尾，
// 这样 new URL('entry/<token>', base) 在带子路径（如飞牛 /app/）时也能拼对。
function baseFor(code: EntryCode): string {
  let base = (code.base_url || '').trim()
  if (!base) base = window.location.origin + import.meta.env.BASE_URL
  if (!base.endsWith('/')) base += '/'
  return base
}

function workerLink(workerId: number): string {
  if (!props.code) return ''
  try {
    const url = new URL(`entry/${props.code.token}`, baseFor(props.code))
    url.searchParams.set('w', String(workerId))
    // 工人码同时预设产品：扫码后产品也自动带出，只剩数量一个输入框。
    const productId = printProductId.value !== '' ? Number(printProductId.value) : (props.code.piece_item_id ?? 0)
    if (productId > 0) url.searchParams.set('p', String(productId))
    return url.toString()
  } catch {
    return ''
  }
}

const fullLink = computed(() => {
  if (!props.code) return ''
  try {
    return new URL(`entry/${props.code.token}`, baseFor(props.code)).toString()
  } catch {
    return ''
  }
})

function qrFor(content: string): string {
  // 用 Vite base 而不是根路径：飞牛网关部署时 API 在 /app/<应用>/api 下。
  const base = import.meta.env.BASE_URL
  return `${base}api/qrcode?content=${encodeURIComponent(content)}&size=600`
}

const qrSrc = computed(() => (fullLink.value ? qrFor(fullLink.value) : ''))

const downloadName = computed(() => {
  const raw = props.code?.name || '报工码'
  return `${raw.replace(/[\\/:*?"<>|\s]+/g, '_')}-二维码.png`
})

const printableWorkers = computed(() =>
  props.workers
    .filter((w): w is Worker & { id: number } => typeof w.id === 'number')
    .slice(0, 50),
)

// 同名工人加序号，打印出来的专属码才分得清是谁。
function cardWorkerName(w: Worker): string {
  const list = props.workers.filter((x): x is Worker & { id: number } => typeof x.id === 'number')
  const same = list.filter((x) => x.name === w.name)
  if (same.length <= 1) return w.name
  const index = same.findIndex((x) => x.id === w.id) + 1
  return `${w.name}（${index}）`
}

const productOptions = computed(() => {
  const opts: { value: string | number; label: string }[] = [{ value: '', label: '不指定产品' }]
  for (const p of props.products) {
    if (p.id) opts.push({ value: p.id, label: p.unit ? `${p.name}（${p.unit}）` : p.name })
  }
  const code = props.code
  if (code?.piece_item_id && !opts.some((o) => o.value === code.piece_item_id)) {
    opts.push({ value: code.piece_item_id, label: code.piece_name || `产品 #${code.piece_item_id}` })
  }
  return opts
})

// 打印卡片上显示的产品名：优先弹窗里选的产品，否则回退到报工码绑定的产品。
const printProductLabel = computed(() => {
  if (!props.code) return ''
  if (printProductId.value !== '') {
    const opt = productOptions.value.find((o) => o.value === printProductId.value)
    if (opt) return opt.label
  }
  return props.code.piece_name || ''
})

const posterSub = computed(() => {
  if (!props.code) return ''
  const parts: string[] = []
  if (props.code.piece_name) parts.push(`产品：${props.code.piece_name}`)
  if (props.code.worker_name) parts.push(`工人：${props.code.worker_name}`)
  if (props.code.project_name) parts.push(`工地：${props.code.project_name}`)
  if (parts.length === 0) parts.push('产品、工人在手机上选择')
  return parts.join('　')
})

function onModelValue(value: boolean) {
  emit('update:modelValue', value)
}

// 复制：http 内网地址下 navigator.clipboard 不可用（非安全上下文），
// 回退到临时 textarea + execCommand，保证局域网里也能复制。
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

async function copyLink() {
  if (!fullLink.value) {
    emit('notify', '报工地址格式不正确，请先编辑报工地址', 'error')
    return
  }
  const ok = await copyText(fullLink.value)
  emit('notify', ok ? '链接已复制' : '复制失败，请长按链接手动复制', ok ? 'success' : 'error')
}

// 打印：先渲染打印区，等二维码图片全部加载完再调 window.print()，
// 否则工人专享码几十张图可能只打印出一半空白。
async function waitForPrintImages() {
  const root = printArea.value
  if (!root) return
  const images = Array.from(root.querySelectorAll('img'))
  await Promise.all(images.map((img) => new Promise<void>((resolve) => {
    if (img.complete) {
      resolve()
      return
    }
    const done = () => resolve()
    img.addEventListener('load', done, { once: true })
    img.addEventListener('error', done, { once: true })
    // 事件注册前刚好加载完的极窄竞态：补一次检查，避免白等超时。
    if (img.complete) done()
    window.setTimeout(done, 4000)
  })))
}

async function runPrint(mode: 'poster' | 'workers') {
  if (!props.code) return
  printMode.value = mode
  await nextTick()
  await waitForPrintImages()
  window.print()
}

function printPoster() {
  void runPrint('poster')
}

function printWorkerCards() {
  if (printableWorkers.value.length === 0) return
  void runPrint('workers')
}

function onAfterPrint() {
  printMode.value = ''
}

watch(
  () => [props.modelValue, props.code?.id] as const,
  () => {
    if (props.modelValue) {
      printProductId.value = props.code?.piece_item_id ?? ''
    } else {
      printMode.value = ''
    }
    workerPanelOpen.value = false
  },
)

onMounted(() => {
  window.addEventListener('afterprint', onAfterPrint)
})

onBeforeUnmount(() => {
  window.removeEventListener('afterprint', onAfterPrint)
})
</script>

<style>
.entry-print-area {
  display: none;
}

@page {
  margin: 10mm;
}

@media print {
  body {
    overflow: visible !important;
    background: #fff !important;
  }

  body * {
    visibility: hidden !important;
  }

  .entry-print-area,
  .entry-print-area * {
    visibility: visible !important;
  }

  .entry-print-area {
    display: block !important;
    position: absolute !important;
    top: 0;
    left: 0;
    width: 100%;
    background: #fff !important;
    color: #111 !important;
  }

  .entry-print-poster {
    padding: 6mm 8mm;
    text-align: center;
  }

  .entry-print-kicker {
    font-size: 12pt;
    font-weight: 600;
    letter-spacing: 0.4em;
    color: #666;
  }

  .entry-print-title {
    margin-top: 4mm;
    font-size: 28pt;
    font-weight: 800;
    line-height: 1.15;
  }

  .entry-print-sub {
    margin-top: 2mm;
    font-size: 15pt;
    color: #333;
  }

  .entry-print-poster-qr {
    display: block;
    width: 90mm;
    height: 90mm;
    margin: 8mm auto 7mm;
  }

  .entry-print-steps {
    display: inline-block;
    padding: 3mm 6mm;
    border: 2px solid #111;
    border-radius: 3mm;
    font-size: 16pt;
    font-weight: 700;
  }

  .entry-print-note {
    margin-top: 6mm;
    font-size: 11pt;
    color: #555;
  }

  .entry-print-cards-title {
    margin-bottom: 3mm;
    text-align: center;
    font-size: 16pt;
    font-weight: 700;
  }

  .entry-print-cards-product {
    margin-bottom: 4mm;
    text-align: center;
    font-size: 11pt;
    color: #555;
  }

  .entry-print-cards {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 5mm;
  }

  .entry-print-card {
    break-inside: avoid;
    page-break-inside: avoid;
    border: 1px solid #999;
    border-radius: 3mm;
    padding: 4mm 3mm;
    text-align: center;
  }

  .entry-print-card-name {
    font-size: 18pt;
    font-weight: 700;
    line-height: 1.15;
  }

  .entry-print-card-product {
    margin-top: 1mm;
    font-size: 11pt;
    color: #555;
  }

  .entry-print-card img {
    display: block;
    width: 56mm;
    height: 56mm;
    margin: 3mm auto;
  }

  .entry-print-card-tip {
    font-size: 10pt;
    color: #333;
  }
}
</style>
