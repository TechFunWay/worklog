<template>
  <div class="page-container animate-fade-in">
    <!-- Hero -->
    <div class="relative overflow-hidden rounded-2xl bg-brand-gradient p-4 sm:p-8 shadow-glow">
      <div class="absolute -top-16 -right-16 w-64 h-64 rounded-full bg-white/10 blur-2xl"></div>
      <div class="relative flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 sm:gap-4">
        <div class="min-w-0">
          <p class="text-white/80 text-xs sm:text-sm font-medium">{{ greeting }}，{{ authStore.user?.username || '师傅' }} 👷</p>
          <h1 class="text-xl sm:text-3xl font-extrabold text-white mt-0.5 sm:mt-1 font-display leading-tight">
            {{ isWorker ? '我的记工月报' : stats.month + ' 记工月报' }}
          </h1>
          <p class="text-white/85 mt-1 sm:mt-2 text-xs sm:text-sm">
            <template v-if="isWorker">
              出勤 {{ workerAtt?.days ?? 0 }} 个工 · {{ workerAtt?.hours ?? 0 }} 小时 · 休 {{ workerAtt?.rest_count ?? 0 }} 天
            </template>
            <template v-else>本月 {{ stats.month_record_count }} 条记工 · 应付 {{ fmtMoney(stats.month_work_cents) }}</template>
          </p>
        </div>
        <RouterLink to="/admin/records" class="btn-premium !w-auto shrink-0 px-5 py-2.5 text-sm sm:px-6 sm:py-3 sm:text-base">
          <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>
          去记工
        </RouterLink>
      </div>
    </div>

    <!-- Stat cards -->
    <div class="grid grid-cols-2 xl:grid-cols-4 gap-2.5 sm:gap-4">
      <div v-for="card in statCards" :key="card.label" class="surface rounded-2xl p-3 sm:p-5">
        <div class="flex items-center justify-between gap-1.5">
          <span class="text-xs sm:text-sm text-muted-foreground font-medium whitespace-nowrap min-w-0 truncate">{{ card.label }}</span>
          <span class="w-7 h-7 sm:w-9 sm:h-9 rounded-xl flex items-center justify-center text-sm sm:text-lg shrink-0" :class="card.bg">{{ card.icon }}</span>
        </div>
        <div class="text-lg sm:text-2xl font-extrabold text-foreground mt-1.5 sm:mt-2 font-display tabular-nums leading-tight">{{ card.value }}</div>
        <div v-if="card.sub" class="text-xs text-muted-foreground mt-0.5 sm:mt-1">{{ card.sub }}</div>
      </div>
    </div>

    <!-- 未结算提醒 -->
    <div class="surface rounded-2xl p-4 sm:p-6">
      <div class="flex items-center justify-between mb-3 sm:mb-4">
        <h3 class="text-sm sm:text-base font-bold text-foreground flex items-center gap-2">
          <span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span>
          {{ isWorker ? '未结工资明细' : '待结算工资' }}
        </h3>
        <RouterLink to="/admin/settlements" class="text-sm text-brand-600 dark:text-brand-400 font-semibold hover:underline">
          {{ isWorker ? '查看全部 →' : '去结算 →' }}
        </RouterLink>
      </div>
      <div v-if="unsettled.length === 0" class="empty-state !py-6">
        <p>目前没有待结算的记工，干完活记得来结算 👌</p>
      </div>
      <div v-else class="divide-y divide-border">
        <div v-for="row in unsettled" :key="row.worker_id" class="flex items-center justify-between py-2.5 sm:py-3 gap-3">
          <div class="flex items-center gap-2.5 sm:gap-3 min-w-0">
            <div class="w-8 h-8 sm:w-9 sm:h-9 rounded-full bg-brand-gradient text-white text-sm font-bold flex items-center justify-center shrink-0">
              {{ row.worker_name.charAt(0) }}
            </div>
            <div class="min-w-0">
              <div class="text-sm font-semibold text-foreground truncate">{{ row.worker_name }}</div>
              <div class="text-xs text-muted-foreground">{{ row.record_count }} 笔记工 · {{ row.earliest_date }} 起</div>
            </div>
          </div>
          <div class="text-right shrink-0">
            <div class="text-sm font-bold tabular-nums" :class="row.payable_cents < 0 ? 'text-red-500' : 'text-foreground'">
              {{ fmtMoney(row.payable_cents) }}
            </div>
            <div v-if="row.advance_amount_cents > 0" class="text-xs text-muted-foreground">已借支 {{ fmtMoney(row.advance_amount_cents) }}</div>
          </div>
        </div>
      </div>
    </div>

    <!-- Quick actions -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 sm:gap-3">
      <RouterLink v-for="a in quickActions" :key="a.to" :to="a.to" class="surface rounded-2xl px-3 py-2.5 sm:p-4 flex items-center gap-2.5 sm:gap-3 hover:-translate-y-0.5 transition-transform">
        <span class="w-9 h-9 sm:w-10 sm:h-10 rounded-xl bg-muted flex items-center justify-center text-lg sm:text-xl shrink-0">{{ a.icon }}</span>
        <span class="text-sm font-semibold text-foreground">{{ a.label }}</span>
      </RouterLink>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useWorklogStore } from '../stores/worklog'
import { statsDashboard, unsettledList, getAttendance, type DashboardStats, type workerUnsettledRow } from '../api/worklog'
import { fmtMoney } from '../utils/format'

const authStore = useAuthStore()
const store = useWorklogStore()
const isWorker = computed(() => store.isWorker)

const greeting = computed(() => {
  const h = new Date().getHours()
  if (h < 6) return '凌晨好'
  if (h < 12) return '早上好'
  if (h < 14) return '中午好'
  if (h < 18) return '下午好'
  return '晚上好'
})

const stats = ref<DashboardStats>({
  month: '', worker_count: 0, month_record_count: 0, month_work_cents: 0,
  unsettled_cents: 0, unsettled_count: 0, advance_cents: 0, settled_cents: 0,
})
const unsettled = ref<workerUnsettledRow[]>([])
const workerAtt = ref<{ days: number; hours: number; rest_count: number } | null>(null)

const statCards = computed(() => [
  { label: isWorker.value ? '本月工钱' : '本月应付工资', value: fmtMoney(stats.value.month_work_cents), icon: '💰', bg: 'bg-amber-400/15 text-amber-500', sub: `${stats.value.month_record_count} 条记工` },
  { label: isWorker.value ? '我的未结工资' : '待结算', value: fmtMoney(stats.value.unsettled_cents), icon: '⏳', bg: 'bg-orange-400/15 text-orange-500', sub: `${stats.value.unsettled_count} 笔未结` },
  { label: isWorker.value ? '我的借支' : '借支中', value: fmtMoney(stats.value.advance_cents), icon: '💸', bg: 'bg-red-400/15 text-red-500', sub: '未结算借支' },
  { label: isWorker.value ? '累计已结工资' : '累计已结', value: fmtMoney(stats.value.settled_cents), icon: '✅', bg: 'bg-emerald-400/15 text-emerald-500', sub: '历史结算合计' },
])

const quickActions = computed(() => isWorker.value
  ? [
      { to: '/admin/records', label: '记工', icon: '📝' },
      { to: '/admin/attendance', label: '考勤表', icon: '📅' },
      { to: '/admin/settlements', label: '未结/已结', icon: '🧾' },
      { to: '/admin/team', label: '我的班组', icon: '👷' },
    ]
  : [
      { to: '/admin/records', label: '记工', icon: '📝' },
      { to: '/admin/advances', label: '记借支', icon: '🧾' },
      { to: '/admin/workers', label: '工人管理', icon: '👷' },
      { to: '/admin/data', label: '数据备份', icon: '📦' },
    ])

onMounted(async () => {
  await store.load()
  try {
    const d = await statsDashboard()
    if (d.data?.code === 0) stats.value = d.data.data
    if (store.isWorker) {
      const att = await getAttendance({ year: new Date().getFullYear(), month: new Date().getMonth() + 1 })
      if (att.data?.code === 0) workerAtt.value = att.data.data
    } else {
      const u = await unsettledList()
      if (u.data?.code === 0) unsettled.value = u.data.data.slice(0, 6)
    }
  } catch {}
})
</script>
