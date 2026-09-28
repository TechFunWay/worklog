import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getMe, type MeInfo } from '../api/worklog'

// 工记角色状态：boss（老板/个人模式）/ leader（组长）/ worker（员工）
export const useWorklogStore = defineStore('worklog', () => {
  const me = ref<MeInfo | null>(null)
  const loaded = ref(false)

  const role = computed(() => me.value?.role ?? 'boss')
  const isBoss = computed(() => role.value === 'boss')
  const isLeader = computed(() => role.value === 'leader')
  const isWorker = computed(() => role.value === 'worker')
  const canManage = computed(() => isBoss.value || isLeader.value)
  const inTeam = computed(() => me.value?.in_team ?? false)

  async function load(force = false) {
    if (loaded.value && !force) return
    try {
      const res = await getMe()
      if (res.data?.code === 0) {
        me.value = res.data.data
        loaded.value = true
      }
    } catch {}
  }

  function reset() {
    me.value = null
    loaded.value = false
  }

  return { me, loaded, role, isBoss, isLeader, isWorker, canManage, inTeam, load, reset }
})
