import request from './request'

export interface Worker {
  id?: number
  name: string
  phone?: string
  note?: string
  daily_wage_cents?: number
  hourly_wage_cents?: number
  monthly_wage_cents?: number
  status?: string
}

export interface Project {
  id?: number
  name: string
  note?: string
  status?: string
  // 工地默认单价（分）：0/未设置 = 不启用，记工时回落到工人自身单价
  day_price_cents?: number
  hour_price_cents?: number
}

export interface PieceItem {
  id?: number
  name: string
  unit?: string
  unit_price_cents?: number
  note?: string
  status?: string
}

export interface WorkRecord {
  id?: number
  worker_id: number
  project_id?: number | null
  piece_item_id?: number | null
  date: string
  type: 'day' | 'hour' | 'piece' | 'rest'
  quantity: string
  unit_price_cents: number
  amount_cents?: number
  note?: string
  settlement_id?: number | null
  worker_name?: string
  project_name?: string
  piece_name?: string
  piece_unit?: string
}

export interface Advance {
  id?: number
  worker_id: number
  date: string
  amount_cents: number
  method?: string
  note?: string
  settlement_id?: number | null
  worker_name?: string
}

export interface Settlement {
  id: number
  worker_id: number
  worker_name: string
  project_id?: number | null
  project_name?: string
  period_start: string
  period_end: string
  work_amount_cents: number
  advance_amount_cents: number
  payable_cents: number
  record_count: number
  details?: string
  note?: string
  settled_at: string
}

export interface DayWorkerRow extends Worker {
  records: WorkRecord[]
  amount_cents: number
  days: string
  hours: string
  pieces: string
  record_count: number
}

// ---- 工人 ----
export const listWorkers = (params?: Record<string, unknown>) => request.get('/api/worklog/workers', { params })
export const createWorker = (data: Worker) => request.post('/api/worklog/workers', data)
export const updateWorker = (id: number, data: Worker) => request.put(`/api/worklog/workers/${id}`, data)
export const deleteWorker = (id: number) => request.delete(`/api/worklog/workers/${id}`)

// ---- 工地 ----
export const listProjects = (params?: Record<string, unknown>) => request.get('/api/worklog/projects', { params })
export const createProject = (data: Project) => request.post('/api/worklog/projects', data)
export const updateProject = (id: number, data: Project) => request.put(`/api/worklog/projects/${id}`, data)
export const deleteProject = (id: number) => request.delete(`/api/worklog/projects/${id}`)

// ---- 工地单价 ----
// 「某工人在某工地」的单独覆盖价：0 = 未覆盖，回落到工地默认价 / 工人自身单价。
export interface ProjectPriceRow {
  worker_id: number
  worker_name: string
  status?: string
  worker_daily_wage_cents: number
  worker_hourly_wage_cents: number
  monthly_wage_cents: number
  daily_wage_cents: number
  hourly_wage_cents: number
  effective_daily_wage_cents: number
  effective_hourly_wage_cents: number
  daily_source: string
  hourly_source: string
}
export interface ProjectPricesData {
  project: Project
  project_daily_price_cents: number
  project_hourly_price_cents: number
  rows: ProjectPriceRow[]
}
export const getProjectPrices = (id: number) => request.get(`/api/worklog/projects/${id}/prices`)
export const saveProjectPrices = (
  id: number,
  rows: { worker_id: number; daily_wage_cents: number; hourly_wage_cents: number }[],
) => request.put(`/api/worklog/projects/${id}/prices`, { rows })

// ---- 计件项目 ----
export const listPieceItems = (params?: Record<string, unknown>) => request.get('/api/worklog/piece-items', { params })
export const createPieceItem = (data: PieceItem) => request.post('/api/worklog/piece-items', data)
export const updatePieceItem = (id: number, data: PieceItem) => request.put(`/api/worklog/piece-items/${id}`, data)
export const deletePieceItem = (id: number) => request.delete(`/api/worklog/piece-items/${id}`)

// ---- 记工 ----
// 记工输入：单价可省略，服务端按工人日薪/时薪/计件项目单价自动带出，
// 月薪工的点工单价按记工日期所在月的天数折算。
export interface RecordInput {
  worker_id: number
  project_id?: number | null
  piece_item_id?: number | null
  date: string
  type: 'day' | 'hour' | 'piece' | 'rest'
  quantity: string
  unit_price_cents?: number
  note?: string
}

// projectId: 不传 = 全部工地；'none' = 未分工地；数字 = 指定工地。
export const recordsByDate = (date: string, projectId?: number | 'none') =>
  request.get('/api/worklog/records', {
    params: projectId === undefined ? { date } : { date, project_id: projectId },
  })
export const listRecords = (params?: Record<string, unknown>) => request.get('/api/worklog/records/list', { params })
export const createRecord = (data: RecordInput) => request.post('/api/worklog/records', data)
export const updateRecord = (id: number, data: WorkRecord) => request.put(`/api/worklog/records/${id}`, data)
export const deleteRecord = (id: number) => request.delete(`/api/worklog/records/${id}`)

// ---- 生效单价 ----
// 记工自动带价用：按「工地单独价 > 工地默认价 > 工人月薪折算/日薪/时薪」算出该工人
// 在该工地、该日期的生效价。source 说明价格来自哪一层。
export type PriceSource = 'project_worker' | 'project' | 'monthly' | 'worker' | 'none'
export interface EffectivePrice {
  worker_id: number
  project_id?: number | null
  date: string
  daily_wage_cents: number
  daily_source: PriceSource
  hourly_wage_cents: number
  hourly_source: PriceSource
  monthly_wage_cents: number
  worker_daily_wage_cents: number
  worker_hourly_wage_cents: number
  project_daily_price_cents: number
  project_hourly_price_cents: number
}
export const getEffectivePrice = (params: Record<string, unknown>) =>
  request.get('/api/worklog/effective-price', { params })

// ---- 借支 ----
export const listAdvances = (params?: Record<string, unknown>) => request.get('/api/worklog/advances', { params })
export const createAdvance = (data: Advance) => request.post('/api/worklog/advances', data)
export const deleteAdvance = (id: number) => request.delete(`/api/worklog/advances/${id}`)

// ---- 结算 ----
export const unsettledList = (params?: Record<string, unknown>) => request.get('/api/worklog/settlements/unsettled', { params })
export const settlementPreview = (params: Record<string, unknown>) => request.get('/api/worklog/settlements/preview', { params })
export const createSettlement = (data: { worker_id: number; start?: string; end?: string; note?: string }) =>
  request.post('/api/worklog/settlements', data)
export const unsettledProjects = () => request.get('/api/worklog/settlements/unsettled-projects')
export const batchSettlement = (data: { start?: string; end?: string; note?: string; project_id?: number }) =>
  request.post('/api/worklog/settlements/batch', data)
export const listSettlements = (params?: Record<string, unknown>) => request.get('/api/worklog/settlements', { params })
export const getSettlement = (id: number) => request.get(`/api/worklog/settlements/${id}`)
export const updateSettlementNote = (id: number, note: string) => request.put(`/api/worklog/settlements/${id}/note`, { note })
export const revertSettlement = (id: number) => request.delete(`/api/worklog/settlements/${id}`)

// ---- 统计 ----
export interface DashboardStats {
  month: string
  worker_count: number
  month_record_count: number
  month_work_cents: number
  unsettled_cents: number
  unsettled_count: number
  advance_cents: number
  settled_cents: number
}

export interface workerUnsettledRow {
  worker_id: number
  worker_name: string
  record_count: number
  work_amount_cents: number
  advance_count: number
  advance_amount_cents: number
  payable_cents: number
  earliest_date: string
  latest_date: string
}

export interface MonthlyStatRow {
  worker_id: number
  worker_name: string
  days: number
  hours: number
  pieces: number
  work_amount_cents: number
  advance_cents: number
  payable_cents: number
}
// 按工地统计的一行（含末行「合计」；未分工地的行 project_name 为「未分工地」）
export interface ProjectStatRow {
  project_id?: number | null
  project_name: string
  record_count: number
  work_amount_cents: number
  advance_cents: number
  payable_cents: number
  settled_cents: number
}
export const statsDashboard = () => request.get('/api/worklog/stats/dashboard')
export const statsMonthly = (params: Record<string, unknown>) => request.get('/api/worklog/stats/monthly', { params })
export const statsRange = (params: Record<string, unknown>) => request.get('/api/worklog/stats/range', { params })
export const statsByProject = (params: Record<string, unknown>) => request.get('/api/worklog/stats/project', { params })

// ---- 导出 ----
// 导出接口需要认证：直连端口只有 Authorization（没有会话 Cookie），而浏览器
// 直接点 <a href> 不会带 Authorization，会 401。统一用 axios 取 blob 再触发
// 下载，直连端口与飞牛网关域（Cookie 会话）都能用。
function filenameFromDisposition(header: string, fallback: string): string {
  if (header) {
    const encoded = /filename\*=UTF-8''([^;]+)/i.exec(header)
    if (encoded) {
      try {
        return decodeURIComponent(encoded[1].trim())
      } catch {
        return encoded[1].trim()
      }
    }
    const plain = /filename="?([^";]+)"?/i.exec(header)
    if (plain) return plain[1].trim()
  }
  return fallback
}

// 浏览器触发文件下载：直连端口与飞牛网关域都走同一条 blob → <a download> 路径。
function triggerBlobDownload(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  try {
    const a = document.createElement('a')
    a.href = url
    a.download = filename
    document.body.appendChild(a)
    a.click()
    a.remove()
  } finally {
    setTimeout(() => URL.revokeObjectURL(url), 1000)
  }
}

export async function downloadExport(path: string, params?: Record<string, unknown>): Promise<string> {
  const res = await request.get(`/api/worklog/export/${path}`, { params, responseType: 'blob' })
  const blob = res.data instanceof Blob ? res.data : new Blob([res.data])
  const filename = filenameFromDisposition(
    String(res.headers?.['content-disposition'] || ''),
    path.split('/').pop() || 'export',
  )
  triggerBlobDownload(blob, filename)
  // 返回实际下载文件名：各页面用它提示「文件在浏览器『下载』文件夹」。
  return filename
}

// ---- NAS 导出 ----
// 把考勤表（Excel / 图片）保存到服务端导出目录，管理员可在「系统配置 → 导出目录」
// 改成 NAS 共享文件夹；同时保留浏览器本机下载入口。
// 权限：存到 NAS 与导出列表/信息/下载仅老板/班组长可用，员工只能本机下载。
export interface NASExportResult {
  file: string
  dir: string
  size: number
  kind: 'xlsx' | 'image'
}
export interface NASExportFile {
  name: string
  size: number
  modified: number // Unix 秒
  kind: 'xlsx' | 'image'
  url: string
}
export interface NASExportList {
  enabled: boolean
  dir: string
  error?: string
  files: NASExportFile[]
}
export interface NASExportInfo {
  enabled: boolean
  dir?: string
  default_dir: string
  file_count?: number
  error?: string
}
export interface NASImageMeta {
  start?: string
  end?: string
  project_name?: string
  worker_name?: string
}

// 保存 Excel 到 NAS（无 body，查询参数与考勤导出 CSV 一致）。
export const saveAttendanceXLSX = (params: Record<string, unknown>) =>
  request.post('/api/worklog/attendance/save-xlsx', null, { params })

// 保存图片到 NAS：multipart/form-data，文件字段名固定为 file；start/end/工地/工人
// 只用于生成文件名。图片生成可能较慢，放宽超时。
export const uploadAttendanceImage = (blob: Blob, meta: NASImageMeta = {}) => {
  const form = new FormData()
  form.append('file', blob, 'attendance.png')
  if (meta.start && meta.end) {
    form.append('start', meta.start)
    form.append('end', meta.end)
  }
  if (meta.project_name) form.append('project_name', meta.project_name)
  if (meta.worker_name) form.append('worker_name', meta.worker_name)
  return request.post('/api/worklog/attendance/save-image', form, { timeout: 120000 })
}

// 已保存的导出文件列表（含 enabled：员工为 false）。
export const getNASExportList = () => request.get('/api/worklog/exports')

// 导出目录信息（仅老板/班组长）。
export const getNASExportInfo = () => request.get('/api/worklog/exports/info')

// 下载已保存到导出目录的文件（仅老板/组长）。
export async function downloadNASFile(name: string): Promise<string> {
  const res = await request.get('/api/worklog/exports/download', {
    params: { name },
    responseType: 'blob',
  })
  const blob = res.data instanceof Blob ? res.data : new Blob([res.data])
  const filename = filenameFromDisposition(String(res.headers?.['content-disposition'] || ''), name.split('/').pop() || name)
  triggerBlobDownload(blob, filename)
  return filename
}

// ---- 角色与班组 ----
export interface MeInfo {
  uid: number
  username: string
  role: 'boss' | 'leader' | 'worker'
  in_team: boolean
  team_id?: number | null
  team_name?: string
  self_worker_id?: number | null
  self_worker_name?: string
}

export interface TeamMemberInfo {
  id: number
  team_id?: number
  user_id: number
  role: 'boss' | 'leader' | 'worker'
  worker_id?: number | null
  username: string
  worker_name?: string
  daily_wage_cents?: number
  hourly_wage_cents?: number
}

// 班组列表卡片（GET /worklog/teams）
export interface TeamCard {
  id: number
  name: string
  note?: string
  invite_code?: string
  created_at?: string
  is_owner: boolean
  my_role: 'boss' | 'leader' | 'worker'
  member_count: number
}

export interface TeamInfo {
  id: number
  name: string
  note?: string
  invite_code?: string
  created_at?: string
}

export interface TeamDetail {
  team: TeamInfo
  members: TeamMemberInfo[]
  my_role: 'boss' | 'leader' | 'worker'
  is_owner: boolean
}

export interface TeamListResult {
  teams: TeamCard[]
  joined: TeamCard | null
}

export const getMe = () => request.get('/api/worklog/me')
export const getTeam = () => request.get('/api/worklog/team')
export const getTeamInfo = (teamId?: number) =>
  request.get('/api/worklog/team', { params: teamId ? { team_id: teamId } : undefined })
export const listTeams = () => request.get('/api/worklog/teams')
export const createTeam = (data: { name: string; note?: string }) => request.post('/api/worklog/teams', data)
export const updateTeam = (id: number, data: { name?: string; note?: string }) => request.put(`/api/worklog/teams/${id}`, data)
export const deleteTeam = (id: number) => request.delete(`/api/worklog/teams/${id}`)
export const joinTeam = (data: { invite_code: string; worker_name: string }) => request.post('/api/worklog/team/join', data)
export const addTeamMember = (data: {
  team_id: number
  user_id: number
  role: 'leader' | 'worker'
  worker_name?: string
  daily_wage_cents?: number
  hourly_wage_cents?: number
}) => request.post('/api/worklog/team/members', data)
export const updateMemberRole = (id: number, role: 'leader' | 'worker') => request.put(`/api/worklog/team/members/${id}/role`, { role })
export const updateMemberWage = (id: number, data: { daily_wage_cents: number; hourly_wage_cents: number }) =>
  request.put(`/api/worklog/team/members/${id}/wage`, data)
export const removeMember = (id: number) => request.delete(`/api/worklog/team/members/${id}`)
export const leaveTeam = () => request.delete('/api/worklog/team/leave')
export const disbandTeam = () => request.delete('/api/worklog/team')

// ---- 考勤 ----
export interface AttendanceCell {
  date: string
  days: number
  hours: number
  pieces: number
  rest: boolean
  record_count: number
  amount_cents: number
}
export interface AttendanceData {
  worker: Worker | null
  year: number
  month: number
  // 工地筛选回显：null = 全部工地或未分工地（未分工地时记录已按 IS NULL 过滤）
  project_id?: number | null
  days: number
  hours: number
  pieces: number
  rest_count: number
  amount_cents: number
  cells: AttendanceCell[]
}
export const getAttendance = (params: Record<string, unknown>) => request.get('/api/worklog/attendance', { params })

// 自定义时间段的按工人汇总行（GET /attendance/range）
export interface AttendanceRangeRow {
  worker_id: number
  worker_name: string
  attend_days: number
  days: number
  hours: number
  pieces: number
  rest_count: number
  amount_cents: number
  record_count: number
}
export interface AttendanceRangeData {
  start: string
  end: string
  project_id?: number | null
  worker: Worker | null
  rows: AttendanceRangeRow[]
  total: AttendanceRangeRow
}
export const getAttendanceRange = (params: Record<string, unknown>) =>
  request.get('/api/worklog/attendance/range', { params })

// 考勤网格（GET /attendance/grid）：导出图片时离屏渲染考勤表用。
// cells 是稀疏的——只含当天有记录的日期，前端按 date 建 Map 回填到完整日期序列。
export interface AttendanceGridCell {
  date: string
  days: number
  hours: number
  pieces: number
  rest: boolean
  amount_cents: number
  record_count: number
  notes?: string[]
}
export interface AttendanceGridTotals {
  attend_days: number
  days: number
  hours: number
  pieces: number
  rest_count: number
  amount_cents: number
  record_count: number
}
export interface AttendanceGridWorker {
  worker_id: number
  worker_name: string
  cells: AttendanceGridCell[]
  totals: AttendanceGridTotals
}
export interface AttendanceGridData {
  mode: 'month' | 'range'
  start: string
  end: string
  year?: number
  month?: number
  project_id?: number | 'none' | null
  project_name: string
  dates: string[]
  workers: AttendanceGridWorker[]
  total: AttendanceGridTotals
}
export const getAttendanceGrid = (params: Record<string, unknown>) =>
  request.get('/api/worklog/attendance/grid', { params })

// 一键对工（GET /attendance/reconcile）
export interface ReconcileItem {
  id: number
  date: string
  type: string
  type_label: string
  quantity: string
  unit_price_cents: number
  amount_cents: number
  project_name?: string
  piece_name?: string
  piece_unit?: string
  note?: string
  rest: boolean
}
export interface ReconcileData {
  worker: Worker
  start: string
  end: string
  project_id?: number | null
  project_name?: string
  attend_days: number
  days: number
  hours: number
  pieces: number
  rest_count: number
  amount_cents: number
  record_count: number
  items: ReconcileItem[]
  generated_at: string
}
// 对工弹窗的入参：worker_id 省略/空串时由服务端按员工角色强制自己；
// project_id 空串 = 全部工地，'none' = 未分工地。
export interface ReconcileParams {
  worker_id?: number | ''
  start: string
  end: string
  project_id?: number | 'none' | ''
}
export const getReconcile = (params: Record<string, unknown>) =>
  request.get('/api/worklog/attendance/reconcile', { params })

// ---- 恢复 ----
export const importDB = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return request.post('/api/worklog/import/db', form, { timeout: 120000 })
}
