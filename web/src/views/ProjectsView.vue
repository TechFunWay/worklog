<template>
  <div class="page-container animate-fade-in">
    <PageHeader title="工地管理" description="同时干几个工地？记工时选一下，统计可按工地筛选">
      <template #actions>
        <button class="btn-brand" @click="openForm()">添加工地</button>
      </template>
    </PageHeader>

    <div class="surface rounded-2xl overflow-hidden">
      <div v-if="projects.length === 0" class="empty-state"><p>还没有工地（不建工地也可以正常记工）</p></div>
      <div v-else class="divide-y divide-border">
        <div v-for="p in projects" :key="p.id" class="p-3 sm:p-4 flex items-center gap-2.5 sm:gap-3">
          <span class="w-10 h-10 rounded-xl bg-teal-400/15 text-teal-500 flex items-center justify-center text-lg shrink-0">🏗️</span>
          <div class="min-w-0 flex-1">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-foreground truncate">{{ p.name }}</span>
              <span v-if="p.status === 'archived'" class="badge bg-muted text-muted-foreground">已归档</span>
            </div>
            <div v-if="p.note" class="text-xs text-muted-foreground mt-0.5 truncate">{{ p.note }}</div>
          </div>
          <div class="flex gap-1.5 shrink-0">
            <button class="btn-ghost !px-3 !py-2 text-xs" @click="openForm(p)">编辑</button>
            <button class="btn-ghost !px-3 !py-2 text-xs" @click="toggle(p)">{{ p.status === 'active' ? '归档' : '恢复' }}</button>
            <button class="btn-ghost !px-3 !py-2 text-xs !text-red-500" @click="remove(p)">删除</button>
          </div>
        </div>
      </div>
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
import { listProjects, createProject, updateProject, deleteProject, type Project } from '../api/worklog'

const projects = ref<Project[]>([])
const formOpen = ref(false)
const saving = ref(false)
const toastMsg = ref('')
const toastType = ref<'success' | 'error'>('success')
const form = ref<Project>({ name: '' })

async function load() {
  try {
    const res = await listProjects()
    if (res.data?.code === 0) projects.value = res.data.data
  } catch {}
}

function openForm(p?: Project) {
  form.value = p ? { ...p } : { name: '', status: 'active' }
  formOpen.value = true
}

function toast(msg: string, type: 'success' | 'error' = 'success') {
  toastMsg.value = msg
  toastType.value = type
  setTimeout(() => (toastMsg.value = ''), 3000)
}

async function save() {
  if (!form.value.name.trim()) return toast('请填写名称', 'error')
  saving.value = true
  try {
    if (form.value.id) await updateProject(form.value.id, form.value)
    else await createProject(form.value)
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
    await updateProject(p.id!, { ...p, status: p.status === 'active' ? 'archived' : 'active' })
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '操作失败', 'error')
  }
}

async function remove(p: Project) {
  if (!confirm(`确定删除工地「${p.name}」？`)) return
  try {
    await deleteProject(p.id!)
    toast('已删除')
    await load()
  } catch (e: any) {
    toast(e.response?.data?.message || '删除失败', 'error')
  }
}

onMounted(load)
</script>
