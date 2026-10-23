<template>
  <Modal :model-value="modelValue" title="导出考勤表" @update:model-value="close">
    <div class="space-y-2.5">
      <!-- 当前查询条件；NAS 目录仅老板/班组长可见 -->
      <div class="rounded-xl border border-border bg-muted/40 px-3 py-2 text-xs">
        <div class="flex flex-wrap gap-x-3 gap-y-0.5 text-muted-foreground">
          <span>范围：<span class="font-semibold text-foreground">{{ rangeLabel }}</span></span>
          <span>工地：<span class="font-semibold text-foreground">{{ projectLabel }}</span></span>
        </div>
        <div v-if="canManage" class="mt-1.5 flex items-center gap-2">
          <div class="min-w-0 flex-1">
            <div class="text-[10px] text-muted-foreground">NAS 导出目录</div>
            <code class="mt-0.5 block break-all font-mono text-[11px] leading-tight text-foreground">
              {{ dirInfo?.dir || (infoLoading ? '读取中…' : '—') }}
            </code>
          </div>
          <button
            type="button"
            class="btn-ghost shrink-0 !px-2.5 !py-1.5 text-xs whitespace-nowrap"
            :disabled="!dirInfo?.dir"
            @click="copyPath(dirInfo?.dir || '')"
          >复制路径</button>
        </div>
        <div v-if="canManage && infoError" class="mt-1 flex items-start gap-2">
          <p class="min-w-0 flex-1 text-[11px] leading-tight text-red-500">{{ infoError }}</p>
          <button type="button" class="shrink-0 text-[11px] text-brand-600 underline dark:text-brand-400" @click="loadAll()">重试</button>
        </div>
      </div>

      <!-- 导出范围：仅老板/组长 + 月度模式（员工后端强制只导出自己） -->
      <div v-if="canManage && mode === 'month' && monthWorkerId !== ''" class="grid grid-cols-2 gap-1.5">
        <button
          type="button"
          class="rounded-xl py-2 text-xs font-semibold transition-all"
          :class="scope === 'all' ? 'bg-brand-gradient text-white shadow-glow' : 'bg-muted/60 text-muted-foreground hover:bg-muted'"
          @click="scope = 'all'"
        >全部工人</button>
        <button
          type="button"
          class="rounded-xl py-2 text-xs font-semibold transition-all"
          :class="scope === 'current' ? 'bg-brand-gradient text-white shadow-glow' : 'bg-muted/60 text-muted-foreground hover:bg-muted'"
          @click="scope = 'current'"
        >当前工人{{ currentWorkerName ? `（${currentWorkerName}）` : '' }}</button>
      </div>

      <!-- 主操作：手机端全宽、高度 ≥44px，纵向排列保证一屏可见 -->
      <div class="space-y-1.5">
        <template v-if="canManage">
          <button type="button" class="btn-brand w-full min-h-11" :disabled="busy !== ''" @click="saveXlsx">
            {{ busy === 'xlsx' ? '保存中…' : '保存 Excel 到 NAS' }}
          </button>
          <button type="button" class="btn-ghost w-full min-h-11" :disabled="busy !== '' || mode === 'range'" @click="saveImage">
            {{ busy === 'image' ? '生成图片中…' : '保存图片到 NAS' }}
          </button>
          <p v-if="mode === 'range'" class="text-[11px] leading-tight text-muted-foreground">图片导出请先切到月度日历</p>
        </template>
        <button type="button" class="btn-ghost w-full min-h-11" :disabled="busy !== ''" @click="downloadLocal('xlsx')">
          {{ busy === 'local-xlsx' ? '下载中…' : '下载 Excel 到本机' }}
        </button>
        <button type="button" class="btn-ghost w-full min-h-11" :disabled="busy !== ''" @click="downloadLocal('csv')">
          {{ busy === 'local-csv' ? '下载中…' : '下载 CSV 到本机' }}
        </button>
        <p v-if="!canManage" class="text-[11px] leading-tight text-muted-foreground">
          保存到 NAS 仅老板或班组长可用；员工可下载到本机。
        </p>
      </div>

      <!-- 最近一次保存结果 -->
      <div v-if="result" class="rounded-xl border border-brand-500/30 bg-brand-500/10 px-3 py-2">
        <div class="flex items-center justify-between gap-2">
          <span class="text-xs font-semibold text-brand-600 dark:text-brand-400">已保存到 NAS</span>
          <button type="button" class="shrink-0 text-[11px] text-brand-600 underline dark:text-brand-400" @click="copyPath(resultPath)">复制路径</button>
        </div>
        <div class="mt-0.5 break-all font-mono text-[11px] leading-snug text-foreground">{{ result.file }}</div>
        <div class="mt-0.5 break-all text-[10px] leading-snug text-muted-foreground">{{ result.dir }} · {{ fmtSize(result.size) }}</div>
      </div>

      <!-- 最近导出（仅老板/组长） -->
      <div v-if="canManage" class="rounded-xl border border-border">
        <div class="flex items-center justify-between gap-2 border-b border-border/60 px-3 py-1.5">
          <span class="text-xs font-semibold text-foreground">最近导出</span>
          <div class="flex items-center gap-2">
            <span v-if="dirInfo?.file_count !== undefined" class="text-[10px] tabular-nums text-muted-foreground">共 {{ dirInfo.file_count }} 个</span>
            <button
              type="button"
              class="text-[11px] text-brand-600 underline disabled:opacity-50 dark:text-brand-400"
              :disabled="listLoading"
              @click="loadFiles(true)"
            >{{ listLoading ? '刷新中…' : '刷新' }}</button>
          </div>
        </div>
        <div v-if="listError" class="px-3 py-2 text-[11px] text-red-500">{{ listError }}</div>
        <div v-else-if="recentFiles.length === 0" class="px-3 py-3 text-center text-[11px] text-muted-foreground">
          {{ listLoading ? '读取中…' : '还没有导出文件' }}
        </div>
        <div
          v-for="f in recentFiles"
          :key="f.name"
          class="flex items-center gap-2 border-b border-border/50 px-2.5 py-1.5 last:border-b-0"
        >
          <div class="min-w-0 flex-1">
            <div class="truncate font-mono text-[11px] text-foreground" :title="f.name">{{ displayName(f.name) }}</div>
            <div class="text-[10px] tabular-nums text-muted-foreground">
              {{ fmtTime(f.modified) }} · {{ fmtSize(f.size) }} · {{ f.kind === 'image' ? '图片' : 'Excel' }}
            </div>
          </div>
          <button
            type="button"
            class="btn-ghost shrink-0 !px-2.5 !py-1 text-[11px] whitespace-nowrap"
            :disabled="downloadingName !== ''"
            @click="downloadSaved(f)"
          >{{ downloadingName === f.name ? '下载中…' : '下载' }}</button>
        </div>
      </div>

      <p class="text-[10px] leading-relaxed text-muted-foreground">
        Excel 含「考勤表 / 工资条 / 逐日明细」三张工作表；文件保存在应用服务器的「导出目录」，
        管理员可在「系统配置 → 导出目录」改为 NAS 共享文件夹。手机端也可用浏览器下载。
      </p>
    </div>
  </Modal>

  <!-- 离屏考勤表：html2canvas 截图用。不能 display:none / visibility:hidden，否则抓不到内容。 -->
  <Teleport to="body">
    <div
      ref="snapshotWrap"
      class="pointer-events-none fixed top-0 w-[1400px] bg-white"
      style="left: -20000px; z-index: -1"
    >
      <AttendanceSnapshot v-if="snapshotData" :data="snapshotData" />
    </div>
  </Teleport>

  <Toast :message="toastMsg" :type="toastType" />
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import Modal from './Modal.vue'
import Toast from './Toast.vue'
import AttendanceSnapshot from './AttendanceSnapshot.vue'
import {
  downloadExport,
  downloadNASFile,
  getAttendanceGrid,
  getNASExportInfo,
  getNASExportList,
  listWorkers,
  saveAttendanceXLSX,
  uploadAttendanceImage,
  type AttendanceGridData,
  type NASExportFile,
  type NASExportInfo,
  type NASExportList,
  type NASExportResult,
  type Worker,
} from '../api/worklog'

const props = defineProps<{
  modelValue: boolean
  mode: 'month' | 'range'
  year: number
  month: number
  start: string
  end: string
  projectId: number | 'none' | ''
  projectName: string
  monthWorkerId: number | ''
  rangeWorkerId: number | ''
  isWorker: boolean
  canManage: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const dirInfo = ref<NASExportInfo | null>(null)
const infoLoading = ref(false)
const infoError = ref('')
const files = ref<NASExportFile[]>([])
const listLoading = ref(false)
const listError = ref('')
const result = ref<NASExportResult | null>(null)
const busy = ref<'' | 'xlsx' | 'image' | 'local-xlsx' | 'local-csv'>('')
const downloadingName = ref('')
// 月度导出范围：全部工人 / 当前工人（老板/组长可选；员工后端强制自己）
const scope = ref<'all' | 'current'>('all')
const currentWorkerName = ref('')
const snapshotData = ref<AttendanceGridData | null>(null)
const snapshotWrap = ref<HTMLElement | null>(null)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

const rangeLabel = computed(() =>
  props.mode === 'month' ? `${props.year}年${props.month}月` : `${props.start || '—'} ~ ${props.end || '—'}`,
)
const projectLabel = computed(() => props.projectName || '全部工地')
const recentFiles = computed(() => files.value.slice(0, 5))
const resultPath = computed(() => (result.value ? `${result.value.dir}/${result.value.file}` : ''))

const monthStart = computed(() => `${props.year}-${String(props.month).padStart(2, '0')}-01`)
const monthEnd = computed(() => {
  const last = new Date(props.year, props.month, 0).getDate()
  return `${props.year}-${String(props.month).padStart(2, '0')}-${String(last).padStart(2, '0')}`
})

// 当前导出参数：与考勤导出 CSV 完全一致（员工不传 worker_id，后端强制只看自己）。
const exportQuery = computed<Record<string, unknown>>(() => {
  const q: Record<string, unknown> = {}
  if (props.mode === 'month') {
    q.year = props.year
    q.month = props.month
  } else {
    q.start = props.start
    q.end = props.end
  }
  if (props.projectId !== '') q.project_id = props.projectId
  if (!props.isWorker) {
    const wid = props.mode === 'month'
      ? (scope.value === 'current' ? Number(props.monthWorkerId) : 0)
      : Number(props.rangeWorkerId || 0)
    if (wid > 0) q.worker_id = wid
  }
  return q
})

// 图片文件名信息：后端只用来拼「考勤表-起至止-工地-工人_时间戳.png」。
const imageMeta = computed(() => {
  const start = props.mode === 'month' ? monthStart.value : props.start
  const end = props.mode === 'month' ? monthEnd.value : props.end
  const worker = props.mode === 'month' && scope.value === 'current' ? currentWorkerName.value : ''
  return { start, end, project_name: props.projectName, worker_name: worker || undefined }
})

function close() {
  emit('update:modelValue', false)
}

function checkRange(): boolean {
  if (props.mode !== 'range') return true
  if (!props.start || !props.end) {
    toast('请先选择起止日期', 'error')
    return false
  }
  if (props.start > props.end) {
    toast('开始日期不能晚于结束日期', 'error')
    return false
  }
  return true
}

// 导出目录信息 + 已保存文件列表；员工无权访问，只在 canManage 时调用。
async function loadAll() {
  if (!props.canManage) return
  infoLoading.value = true
  infoError.value = ''
  try {
    const res = await getNASExportInfo()
    if (res.data?.code === 0) {
      dirInfo.value = res.data.data as NASExportInfo
      if (dirInfo.value.error) infoError.value = dirInfo.value.error
    } else {
      infoError.value = res.data?.message || '导出目录信息加载失败'
    }
  } catch (e: any) {
    infoError.value = e?.response?.data?.message || '导出目录信息加载失败，仍可继续导出'
  } finally {
    infoLoading.value = false
  }
  void loadFiles()
}

async function loadFiles(silent = false) {
  if (!props.canManage || listLoading.value) return
  listLoading.value = true
  if (!silent) listError.value = ''
  try {
    const res = await getNASExportList()
    if (res.data?.code === 0) {
      const data = res.data.data as NASExportList
      files.value = data.files ?? []
      if (data.error) listError.value = data.error
      else if (data.dir) {
        if (!dirInfo.value) dirInfo.value = { enabled: true, dir: data.dir, default_dir: data.dir }
        else if (!dirInfo.value.dir) dirInfo.value = { ...dirInfo.value, dir: data.dir }
      }
    } else {
      listError.value = res.data?.message || '最近导出加载失败'
    }
  } catch (e: any) {
    listError.value = e?.response?.data?.message || '最近导出加载失败'
  } finally {
    listLoading.value = false
  }
}

// 弹窗需要显示「当前工人（张三）」；老板/组长可读工人列表。
async function loadCurrentWorkerName() {
  if (!props.canManage || props.mode !== 'month' || !props.monthWorkerId) return
  try {
    const res = await listWorkers({ status: 'active' })
    if (res.data?.code === 0) {
      const list = res.data.data as Worker[]
      currentWorkerName.value = list.find(w => w.id === Number(props.monthWorkerId))?.name ?? ''
    }
  } catch {}
}

// 保存成功后同时刷新文件列表与目录信息：文件数（共 N 个）从 info 接口来，
// 只刷新列表会让结果卡上方的计数停在旧值。
function refreshAfterSave() {
  void loadFiles(true)
  void getNASExportInfo()
    .then((res) => {
      if (res.data?.code === 0 && !res.data.data?.error) {
        const next = res.data.data as NASExportInfo
        dirInfo.value = { ...(dirInfo.value || { enabled: true }), ...next }
      }
    })
    .catch(() => {})
}

async function saveXlsx() {
  if (busy.value) return
  if (!checkRange()) return
  busy.value = 'xlsx'
  try {
    const res = await saveAttendanceXLSX(exportQuery.value)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '保存失败')
    result.value = res.data.data as NASExportResult
    toast(`已保存到 NAS：${result.value.file}`)
    refreshAfterSave()
  } catch (e: any) {
    toast(e?.response?.data?.message || e?.message || '保存 Excel 到 NAS 失败，请稍后重试', 'error')
  } finally {
    busy.value = ''
  }
}

async function saveImage() {
  if (busy.value) return
  if (props.mode !== 'month') {
    toast('图片导出请先切到月度日历', 'error')
    return
  }
  busy.value = 'image'
  try {
    const res = await getAttendanceGrid(exportQuery.value)
    if (res.data?.code !== 0) throw new Error(res.data?.message || '考勤数据加载失败')
    snapshotData.value = res.data.data as AttendanceGridData
    // 等离屏表挂载 + 字体就绪再截图，避免首次出图字体回退或空白。
    await nextTick()
    await document.fonts.ready
    const el = snapshotWrap.value
    if (!el) throw new Error('考勤表渲染失败，请重试')
    const html2canvas = (await import('html2canvas')).default
    const canvas = await html2canvas(el, { scale: 2, backgroundColor: '#fff', logging: false, useCORS: true })
    const blob = await new Promise<Blob>((resolve, reject) => {
      canvas.toBlob(b => (b ? resolve(b) : reject(new Error('图片生成失败，请重试'))), 'image/png')
    })
    const up = await uploadAttendanceImage(blob, imageMeta.value)
    if (up.data?.code !== 0) throw new Error(up.data?.message || '图片保存失败')
    result.value = up.data.data as NASExportResult
    toast(`已保存到 NAS：${result.value.file}`)
    refreshAfterSave()
  } catch (e: any) {
    toast(e?.response?.data?.message || e?.message || '保存图片到 NAS 失败，请稍后重试', 'error')
  } finally {
    snapshotData.value = null
    busy.value = ''
  }
}

async function downloadLocal(kind: 'xlsx' | 'csv') {
  if (busy.value) return
  if (!checkRange()) return
  busy.value = kind === 'xlsx' ? 'local-xlsx' : 'local-csv'
  try {
    const filename = await downloadExport(kind === 'xlsx' ? 'attendance.xlsx' : 'attendance.csv', exportQuery.value)
    toast(`已导出：${filename}，文件在浏览器「下载」文件夹`)
  } catch (e: any) {
    toast(e?.response?.data?.message || '导出失败，请稍后重试', 'error')
  } finally {
    busy.value = ''
  }
}

async function downloadSaved(file: NASExportFile) {
  if (downloadingName.value) return
  downloadingName.value = file.name
  try {
    const name = await downloadNASFile(file.name)
    toast(`已下载：${name}，文件在浏览器「下载」文件夹`)
  } catch {
    // 下载走 blob，服务端 400/404 的 JSON 错误体拿不到 message，给通用提示。
    toast('下载失败，文件可能已被移出导出目录', 'error')
  } finally {
    downloadingName.value = ''
  }
}

// 列表里图片文件名带 pictures/ 前缀，展示时只显示文件名。
function displayName(name: string): string {
  return name.startsWith('pictures/') ? name.slice('pictures/'.length) : name
}

async function copyPath(text: string) {
  if (!text) return
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text)
    } else {
      throw new Error('clipboard unavailable')
    }
    toast('已复制路径')
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
      toast('已复制路径')
    } catch {
      toast('复制失败，请长按目录手动复制', 'error')
    }
  }
}

function fmtSize(bytes: number | undefined | null): string {
  const n = Number(bytes ?? 0)
  if (!isFinite(n) || n <= 0) return '0 KB'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1).replace(/\.0$/, '')} KB`
  return `${(n / 1024 / 1024).toFixed(1).replace(/\.0$/, '')} MB`
}

// 列表时间是 Unix 秒；兼容 ISO 字符串。
function fmtTime(value: number | string | undefined): string {
  if (value === undefined || value === null || value === '') return ''
  const d = typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  if (isNaN(d.getTime())) return typeof value === 'string' ? value.slice(0, 16).replace('T', ' ') : ''
  const p = (n: number) => String(n).padStart(2, '0')
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
}

// 每次打开重置结果与范围选择；目录信息与工人名异步加载，不阻塞导出按钮。
watch(
  () => props.modelValue,
  (open) => {
    if (!open) return
    result.value = null
    scope.value = 'all'
    snapshotData.value = null
    busy.value = ''
    downloadingName.value = ''
    infoError.value = ''
    listError.value = ''
    if (props.canManage) {
      void loadAll()
      void loadCurrentWorkerName()
    }
  },
  { immediate: true },
)
</script>
