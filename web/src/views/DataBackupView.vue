<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="数据备份" description="导出你的记工数据，数据永远在自己设备上" />

    <div class="grid lg:grid-cols-2 gap-4">
      <!-- 数据导出 -->
      <div class="surface rounded-2xl p-5 sm:p-6">
        <h3 class="font-bold text-foreground mb-1">导出数据</h3>
        <p class="text-sm text-muted-foreground mb-4">CSV 可用 Excel / WPS 打开；JSON 为完整结构化备份。</p>
        <div class="space-y-2">
          <a :href="exportUrl('records.csv')" class="btn-ghost w-full !justify-start">
            📝 记工明细 CSV
          </a>
          <a :href="exportUrl('advances.csv')" class="btn-ghost w-full !justify-start">
            💸 借支记录 CSV
          </a>
          <a :href="exportUrl('settlements.csv')" class="btn-ghost w-full !justify-start">
            ✅ 结算记录 CSV
          </a>
          <a :href="exportUrl('json')" class="btn-ghost w-full !justify-start">
            🗂️ 全部数据 JSON 备份
          </a>
        </div>
      </div>

      <!-- 管理员备份与恢复 -->
      <div class="surface rounded-2xl p-5 sm:p-6">
        <h3 class="font-bold text-foreground mb-1">数据库备份与恢复</h3>
        <p class="text-sm text-muted-foreground mb-4">
          仅管理员可见：下载 SQLite 数据库快照（含全部数据），可用于整机迁移与恢复。
        </p>
        <template v-if="isAdmin">
          <a :href="exportUrl('db')" class="btn-brand w-full">⬇️ 下载数据库备份（.db）</a>

          <!-- 恢复进行中：遮住操作区，等重启后自动刷新 -->
          <div v-if="restoring" class="mt-4 rounded-xl border border-amber-400/40 bg-amber-400/10 px-4 py-3 text-sm leading-relaxed text-foreground">
            ⏳ 备份校验通过，服务正在恢复数据并重启，页面将自动刷新，请勿关闭页面…
          </div>
          <div v-else class="mt-6 pt-5 border-t border-border space-y-3">
            <h4 class="text-sm font-bold text-foreground">恢复导入</h4>
            <p class="text-xs text-muted-foreground leading-relaxed">
              上传此前下载的 .db 备份覆盖当前全部数据；恢复前会先把当前数据自动另存一份快照，恢复后服务自动重启。
            </p>
            <label class="btn-ghost w-full !justify-start cursor-pointer">
              📎 {{ restoreFile ? restoreFile.name : '选择备份文件（.db）' }}
              <input type="file" accept=".db,.sqlite,.sqlite3" class="hidden" @change="onFileChange" />
            </label>
            <button
              class="w-full inline-flex items-center justify-center gap-2 px-4 py-2.5 rounded-xl bg-red-500/90 hover:bg-red-500 text-white text-sm font-semibold transition-all disabled:opacity-50 disabled:pointer-events-none"
              :disabled="!restoreFile"
              @click="showConfirm = true"
            >
              ♻️ 恢复导入
            </button>
          </div>
        </template>
        <div v-else class="empty-state"><p>该功能仅管理员可用</p></div>
        <div class="mt-4 rounded-xl bg-muted p-4 text-xs text-muted-foreground leading-relaxed">
          <p class="font-semibold text-foreground mb-1">💡 提示</p>
          数据库文件位于服务端 <code class="px-1 rounded bg-border/50">data/db/worklog.db</code>，
          也可直接在 NAS / 服务器上定时复制该文件进行备份。
        </div>
      </div>
    </div>

    <Modal v-model="showConfirm" title="确认恢复数据" :closable="!uploading">
      <div class="space-y-4 text-sm">
        <p>将使用备份文件 <strong class="break-all">{{ restoreFile?.name }}</strong> 覆盖当前全部数据（包括账号、工人档案、记工、借支与结算记录）。</p>
        <p class="text-destructive">此操作不可撤销；确认前建议先点「下载数据库备份」留好当前数据的备份。</p>
        <div class="flex gap-3">
          <button class="btn-ghost flex-1" :disabled="uploading" @click="showConfirm = false">取消</button>
          <button class="btn-brand flex-1" :disabled="uploading" @click="doRestore">{{ uploading ? '上传中…' : '确认恢复' }}</button>
        </div>
      </div>
    </Modal>

    <Toast :message="toastMsg" :type="toastType" />
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import PageHeader from '../components/PageHeader.vue'
import Toast from '../components/Toast.vue'
import Modal from '../components/Modal.vue'
import { exportUrl, importDB } from '../api/worklog'
import { useAuthStore } from '../stores/auth'

const authStore = useAuthStore()
const isAdmin = computed(() => authStore.isAdmin)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')

const restoreFile = ref<File | null>(null)
const showConfirm = ref(false)
const uploading = ref(false)
const restoring = ref(false)

function showToast(message: string, type: 'success' | 'error' = 'error') {
  toastMsg.value = message
  toastType.value = type
}

function onFileChange(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0] || null
  if (file && !/\.(db|sqlite|sqlite3)$/i.test(file.name)) {
    showToast('请选择 .db / .sqlite 备份文件')
    restoreFile.value = null
  } else {
    restoreFile.value = file
  }
  input.value = ''
}

async function doRestore() {
  if (!restoreFile.value || uploading.value) return
  uploading.value = true
  try {
    const res = await importDB(restoreFile.value)
    if (res.data?.code === 0) {
      showConfirm.value = false
      restoring.value = true
      waitRestartAndReload()
    } else {
      showToast(res.data?.message || '恢复失败')
    }
  } catch (err: any) {
    showToast(err.response?.data?.message || '恢复失败')
  } finally {
    uploading.value = false
  }
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// 恢复接口应答后，服务端会延迟停库、替换文件并重启进程：先等健康检查
// 断开（旧进程退出），再等新进程就绪，最后整页刷新。若一直没观察到断开
// （重启极快的极端情况），超时后也强制刷新。
async function waitRestartAndReload() {
  const health = import.meta.env.BASE_URL.replace(/\/$/, '') + '/api/health'
  let sawDown = false
  const deadline = Date.now() + 10000
  while (Date.now() < deadline) {
    try {
      const res = await fetch(health, { cache: 'no-store' })
      if (!res.ok) { sawDown = true; break }
    } catch {
      sawDown = true
      break
    }
    await sleep(500)
  }
  for (let i = 0; i < 60; i++) {
    try {
      const res = await fetch(health, { cache: 'no-store' })
      if (res.ok) break
    } catch {}
    await sleep(500)
  }
  await sleep(sawDown ? 300 : 2000)
  location.reload()
}
</script>
