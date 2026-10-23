<template>
  <div class="w-[1400px] bg-white text-[#111827]" style="font-family: -apple-system, 'PingFang SC', 'Microsoft YaHei', sans-serif">
    <!-- 标题行 -->
    <div class="flex items-end justify-between px-10 pb-3 pt-10">
      <div class="text-[32px] font-bold leading-none">考勤表</div>
      <div class="text-[14px] leading-none text-[#6b7280]">工记 · 导出时间 {{ exportedAt }}</div>
    </div>

    <!-- 副标题 -->
    <div class="px-10 pb-4 text-[16px] text-[#374151]">{{ subtitle }}</div>

    <div class="px-10 pb-10">
      <table class="snap-table">
        <colgroup>
          <col style="width: 130px" />
          <col v-for="d in data.dates" :key="'col-' + d" />
          <col style="width: 70px" />
          <col style="width: 70px" />
          <col style="width: 70px" />
          <col style="width: 60px" />
          <col style="width: 100px" />
        </colgroup>
        <thead>
          <tr>
            <th class="snap-name">姓名</th>
            <th v-for="d in data.dates" :key="d">
              <div class="text-[14px] font-semibold leading-tight">{{ Number(d.slice(8)) }}</div>
              <div class="text-[10px] leading-tight opacity-80">{{ weekdayOf(d) }}</div>
            </th>
            <th>点工</th>
            <th>计时</th>
            <th>计件</th>
            <th>休息</th>
            <th>工钱</th>
          </tr>
        </thead>
        <tbody v-if="rows.length > 0">
          <tr v-for="row in rows" :key="row.worker.worker_id">
            <td class="snap-name">{{ row.worker.worker_name }}</td>
            <td v-for="(cell, i) in row.cells" :key="i" class="snap-cell">
              <template v-if="cell">
                <div v-if="cell.rest" class="font-bold text-[#0d9488]">休</div>
                <template v-else>
                  <div v-if="num(cell.days) > 0">{{ fmtQty(cell.days) }}工</div>
                  <div v-if="num(cell.hours) > 0">{{ fmtQty(cell.hours) }}小时</div>
                  <div v-if="num(cell.pieces) > 0">{{ fmtQty(cell.pieces) }}件</div>
                </template>
              </template>
            </td>
            <td class="snap-num">{{ fmtQty(row.worker.totals.days) }}</td>
            <td class="snap-num">{{ fmtQty(row.worker.totals.hours) }}</td>
            <td class="snap-num">{{ fmtQty(row.worker.totals.pieces) }}</td>
            <td class="snap-num">{{ row.worker.totals.rest_count }}</td>
            <td class="snap-num">{{ fmtYuan(row.worker.totals.amount_cents) }}</td>
          </tr>
        </tbody>
        <tbody v-else>
          <tr>
            <td :colspan="data.dates.length + 6" class="py-10 text-center text-[16px] text-[#6b7280]">
              该期间暂无记工记录
            </td>
          </tr>
        </tbody>
        <tfoot>
          <tr>
            <td class="snap-name">合计（{{ data.workers.length }} 人）</td>
            <td v-for="d in data.dates" :key="'total-' + d"></td>
            <td class="snap-num">{{ fmtQty(data.total.days) }}</td>
            <td class="snap-num">{{ fmtQty(data.total.hours) }}</td>
            <td class="snap-num">{{ fmtQty(data.total.pieces) }}</td>
            <td class="snap-num">{{ data.total.rest_count }}</td>
            <td class="snap-num">{{ fmtYuan(data.total.amount_cents) }}</td>
          </tr>
        </tfoot>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { fmtQty, fmtYuan } from '../utils/format'
import type { AttendanceGridCell, AttendanceGridData } from '../api/worklog'

const props = defineProps<{
  data: AttendanceGridData
}>()

// 表格按 1400px 固定宽度出图：姓名列与合计列定宽，日期列平分剩余宽度，
// 31 天也不会把表格撑出画布。
const weekdayLabels = ['日', '一', '二', '三', '四', '五', '六']

function weekdayOf(date: string): string {
  const d = new Date(date + 'T00:00:00')
  return weekdayLabels[d.getDay()] ?? ''
}

function num(value: number | string | undefined | null): number {
  const n = typeof value === 'string' ? parseFloat(value) : value
  return n && isFinite(n) ? n : 0
}

const cellMap = computed(() => {
  const map = new Map<number, Map<string, AttendanceGridCell>>()
  for (const w of props.data.workers) {
    const byDate = new Map<string, AttendanceGridCell>()
    for (const c of w.cells) byDate.set(c.date, c)
    map.set(w.worker_id, byDate)
  }
  return map
})

const rows = computed(() =>
  props.data.workers.map(w => ({
    worker: w,
    cells: props.data.dates.map(d => cellMap.value.get(w.worker_id)?.get(d) ?? null),
  })),
)

const subtitle = computed(() => {
  const d = props.data
  const period = d.mode === 'month' && d.year && d.month ? `${d.year}年${d.month}月` : `${d.start} ~ ${d.end}`
  return `${period} · ${d.project_name || '全部工地'} · 共 ${d.workers.length} 人`
})

const exportedAt = (() => {
  const d = new Date()
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`
})()
</script>

<style scoped>
/* 图片导出专用样式：白底深色字、固定边框，独立于应用深色主题。 */
.snap-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
}
.snap-table th,
.snap-table td {
  border: 1px solid #d1d5db;
  padding: 3px 2px;
  text-align: center;
  vertical-align: middle;
  color: #111827;
  font-size: 13px;
}
.snap-table thead th {
  background: #0ea5e9;
  color: #fff;
  font-weight: 600;
}
.snap-table .snap-name {
  text-align: left;
  padding-left: 8px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.snap-table .snap-cell {
  font-size: 11px;
  line-height: 1.25;
  white-space: nowrap;
}
.snap-table thead th:not(.snap-name) {
  font-size: 12px;
  padding: 3px 1px;
}
.snap-table .snap-num {
  text-align: right;
  padding-right: 6px;
  font-variant-numeric: tabular-nums;
}
.snap-table tfoot td {
  background: #f3f4f6;
  font-weight: 700;
}
</style>
