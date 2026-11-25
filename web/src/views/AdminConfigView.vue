<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="系统配置" description="管理全局系统级配置，仅管理员可见" />

    <div v-if="errorMsg" class="surface rounded-2xl px-5 py-3 text-sm text-destructive">
      {{ errorMsg }}
    </div>

    <div v-if="loading" class="space-y-3">
      <div v-for="i in 4" :key="i" class="h-20 rounded-xl bg-muted animate-pulse"></div>
    </div>

    <div v-else class="space-y-4">
      <!-- 导出目录（自定义卡片：需要目录选择器，通用配置表单放不下） -->
      <div class="surface rounded-2xl p-4 sm:p-6 space-y-4">
        <div>
          <h3 class="text-sm font-bold text-foreground">导出目录</h3>
          <p class="mt-1 text-sm leading-relaxed text-muted-foreground">
            手动导出与定时导出都保存到这里。飞牛上填共享文件夹路径（需先在应用权限里授权该共享目录）；Docker 上填容器内已挂载的路径。
          </p>
        </div>
        <div class="flex flex-col sm:flex-row gap-2">
          <input v-model="exportDir" class="input-field flex-1" placeholder="绝对路径，如 /vol1/1000/工资表" />
          <button type="button" class="btn-ghost min-h-11 sm:w-28" @click="openPicker">浏览…</button>
        </div>
        <div>
          <label class="text-xs text-muted-foreground">允许选择的根目录（英文逗号分隔；留空则默认导出目录与数据目录）</label>
          <input v-model="allowedRoots" class="input-field mt-1" placeholder="/vol1,/app/exports" />
        </div>
        <div class="flex items-center gap-3">
          <button type="button" class="btn-ghost min-h-11" :disabled="savingCfg" @click="saveExportCfg">保存目录设置</button>
          <span v-if="hint" class="text-xs text-emerald-600 dark:text-emerald-400">{{ hint }}</span>
        </div>
        <div class="border-t border-border pt-4 space-y-2">
          <h4 class="text-sm font-bold text-foreground">立即导出一次（用于验证目录是否可写）</h4>
          <div class="flex flex-wrap gap-2 items-center">
            <SelectMenu v-model="testScope" :options="scopeOptions" class="!py-2 text-sm w-48" />
            <button type="button" class="btn-brand min-h-11" :disabled="running" @click="runNow">{{ running ? '导出中…' : '立即导出' }}</button>
          </div>
          <p v-if="lastFiles.length" class="text-xs text-muted-foreground break-all">已生成：{{ lastFiles.join('、') }}</p>
        </div>
      </div>

      <ConfigGroupList ref="listRef" :items="genericItems" @save="handleSave" />
    </div>

    <Modal v-model="pickerOpen" title="选择导出目录">
      <div class="space-y-3">
        <div class="text-xs text-muted-foreground break-all">当前：{{ pickerPath || '请选择一个根目录' }}</div>
        <div class="flex flex-wrap gap-2">
          <button v-if="pickerPath" type="button" class="btn-ghost !py-1.5 text-xs" @click="enterDir(fsParent)">上一级</button>
          <button type="button" class="btn-ghost !py-1.5 text-xs" @click="loadDirs('')">回到根目录</button>
          <button type="button" class="btn-ghost !py-1.5 text-xs" @click="mkdirMode = !mkdirMode">新建文件夹</button>
        </div>
        <div v-if="mkdirMode" class="flex gap-2 items-center">
          <input v-model="newDirName" class="input-field flex-1 !py-1.5 text-xs" placeholder="新文件夹名，如 工资表" maxlength="40" />
          <button type="button" class="btn-brand !py-1.5 text-xs" @click="doMkdir">创建</button>
          <button type="button" class="btn-ghost !py-1.5 text-xs" @click="mkdirMode = false">取消</button>
        </div>
        <div class="max-h-64 overflow-y-auto divide-y divide-border rounded-xl border border-border">
          <button v-for="d in dirs" :key="d.path" type="button" class="w-full text-left px-3 py-2.5 text-sm hover:bg-muted break-all" @click="enterDir(d.path)">📁 {{ d.name }}</button>
          <p v-if="!dirs.length" class="p-3 text-xs text-muted-foreground">该目录下没有子文件夹</p>
        </div>
        <div class="flex gap-2">
          <button type="button" class="btn-ghost flex-1 min-h-11" @click="pickerOpen = false">取消</button>
          <button type="button" class="btn-brand flex-1 min-h-11" :disabled="!pickerPath" @click="chooseDir">选用此目录</button>
        </div>
      </div>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import ConfigGroupList from '../components/ConfigGroupList.vue'
import Modal from '../components/Modal.vue'
import SelectMenu from '../components/SelectMenu.vue'
import { getSystemConfigMeta, updateConfig } from '../api/config'
import { fsList, fsMkdir, runAutoExportNow, type FsDirEntry } from '../api/worklog'
import type { ConfigMeta } from '../api/config'

const items = ref<ConfigMeta[]>([])
const loading = ref(false)
const errorMsg = ref('')
const listRef = ref<InstanceType<typeof ConfigGroupList> | null>(null)

// 导出目录/允许根目录这两项从通用表单里拿出来，用带「浏览」的自定义卡片渲染。
const HIDDEN_KEYS = ['worklog.export_dir', 'worklog.allowed_export_roots']
const genericItems = computed(() => items.value.filter(it => !HIDDEN_KEYS.includes(it.key)))
const exportDir = ref('')
const allowedRoots = ref('')
const hint = ref('')
const savingCfg = ref(false)

function syncFromItems() {
  exportDir.value = items.value.find(it => it.key === 'worklog.export_dir')?.value || ''
  allowedRoots.value = items.value.find(it => it.key === 'worklog.allowed_export_roots')?.value || ''
}

// 目录选择器
const pickerOpen = ref(false)
const pickerPath = ref('')
const fsParent = ref('')
const dirs = ref<FsDirEntry[]>([])

async function loadDirs(path: string) {
  errorMsg.value = ''
  try {
    const res = await fsList(path || undefined)
    if (res.data?.code !== 0) {
      errorMsg.value = res.data?.message || '读取目录失败'
      return
    }
    pickerPath.value = res.data.data.path || ''
    fsParent.value = res.data.data.parent || ''
    dirs.value = res.data.data.dirs || []
  } catch (e: any) {
    errorMsg.value = e.response?.data?.message || '读取目录失败'
  }
}

function openPicker() {
  pickerOpen.value = true
  loadDirs(exportDir.value || '')
}

function enterDir(p: string) {
  if (!p) return
  loadDirs(p)
}

function chooseDir() {
  exportDir.value = pickerPath.value
  pickerOpen.value = false
}

const mkdirMode = ref(false)
const newDirName = ref('')
async function doMkdir() {
  const name = newDirName.value.trim()
  if (!name) { errorMsg.value = '请填写文件夹名'; return }
  try {
    const res = await fsMkdir(pickerPath.value, name)
    if (res.data?.code === 0) {
      newDirName.value = ''
      mkdirMode.value = false
      await loadDirs(pickerPath.value)
    } else {
      errorMsg.value = res.data?.message || '创建失败'
    }
  } catch (e: any) {
    errorMsg.value = e.response?.data?.message || '创建失败'
  }
}

async function saveExportCfg() {
  savingCfg.value = true
  hint.value = ''
  errorMsg.value = ''
  try {
    await updateConfig('worklog.export_dir', exportDir.value.trim())
    await updateConfig('worklog.allowed_export_roots', allowedRoots.value.trim())
    hint.value = '已保存'
    setTimeout(() => (hint.value = ''), 2500)
    await loadMeta()
  } catch (e: any) {
    errorMsg.value = e.response?.data?.message || '保存失败'
  } finally {
    savingCfg.value = false
  }
}

// 立即导出一次（验证目录是否真的可写）
const testScope = ref('attendance')
const scopeOptions = [
  { value: 'attendance', label: '当月考勤与工资条' },
  { value: 'settlements', label: '全部结算单' },
  { value: 'both', label: '两种都导' },
]
const running = ref(false)
const lastFiles = ref<string[]>([])
async function runNow() {
  running.value = true
  errorMsg.value = ''
  try {
    const res = await runAutoExportNow(testScope.value)
    if (res.data?.code === 0) {
      lastFiles.value = res.data.data.files || []
    } else {
      errorMsg.value = res.data?.message || '导出失败'
    }
  } catch (e: any) {
    errorMsg.value = e.response?.data?.message || '导出失败'
  } finally {
    running.value = false
  }
}

onMounted(loadMeta)

async function loadMeta() {
  loading.value = true
  errorMsg.value = ''
  try {
    const res = await getSystemConfigMeta()
    if (res.data?.code === 0) {
      items.value = Array.isArray(res.data.data) ? res.data.data : []
      syncFromItems()
    } else {
      errorMsg.value = res.data?.message || '加载配置失败'
    }
  } catch (err: any) {
    errorMsg.value = err.response?.data?.message || '网络错误'
  } finally {
    loading.value = false
  }
}

async function handleSave(key: string, value: string) {
  errorMsg.value = ''
  try {
    const res = await updateConfig(key, value)
    if (res.data?.code === 0) {
      const idx = items.value.findIndex((it) => it.key === key)
      if (idx >= 0) {
        items.value[idx] = { ...items.value[idx], value }
      }
      listRef.value?.markSaved(key)
    } else {
      errorMsg.value = res.data?.message || '保存失败'
      listRef.value?.markFailed(key)
    }
  } catch (err: any) {
    errorMsg.value = err.response?.data?.message || '网络错误'
    listRef.value?.markFailed(key)
  }
}
</script>