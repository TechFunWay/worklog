import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getMe, getReviewSummary, type MeInfo } from '../api/worklog'

// 工记角色状态：boss（老板/个人模式）/ leader（组长）/ worker（员工）
export const useWorklogStore = defineStore('worklog', () => {
  const me = ref<MeInfo | null>(null)
  const loaded = ref(false)
  // 待审核笔数：导航红点与审核页共用，页面动作后调用 refreshReview() 刷新。
  const pendingReview = ref(0)

  const role = computed(() => me.value?.role ?? 'boss')
  const isBoss = computed(() => role.value === 'boss')
  const isLeader = computed(() => role.value === 'leader')
  const isWorker = computed(() => role.value === 'worker')
  const canManage = computed(() => isBoss.value || isLeader.value)
  const reviewRequired = computed(() => me.value?.review_required ?? false)
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

  async function refreshReview() {
    if (!canManage.value) {
      pendingReview.value = 0
      return
    }
    try {
      const res = await getReviewSummary()
      if (res.data?.code === 0) pendingReview.value = res.data.data?.pending_count ?? 0
    } catch {}
  }

  function reset() {
    me.value = null
    loaded.value = false
    pendingReview.value = 0
  }

  return { me, loaded, pendingReview, role, isBoss, isLeader, isWorker, canManage, reviewRequired, inTeam, load, refreshReview, reset }
})
