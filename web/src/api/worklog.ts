import request from './request'

export interface Worker {
  id?: number
  name: string
  phone?: string
  note?: string
  daily_wage_cents?: number
  hourly_wage_cents?: number
  status?: string
}

export interface Project {
  id?: number
  name: string
  note?: string
  status?: string
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

// ---- 计件项目 ----
export const listPieceItems = (params?: Record<string, unknown>) => request.get('/api/worklog/piece-items', { params })
export const createPieceItem = (data: PieceItem) => request.post('/api/worklog/piece-items', data)
export const updatePieceItem = (id: number, data: PieceItem) => request.put(`/api/worklog/piece-items/${id}`, data)
export const deletePieceItem = (id: number) => request.delete(`/api/worklog/piece-items/${id}`)

// ---- 记工 ----
export const recordsByDate = (date: string) => request.get('/api/worklog/records', { params: { date } })
export const listRecords = (params?: Record<string, unknown>) => request.get('/api/worklog/records/list', { params })
export const createRecord = (data: WorkRecord) => request.post('/api/worklog/records', data)
export const updateRecord = (id: number, data: WorkRecord) => request.put(`/api/worklog/records/${id}`, data)
export const deleteRecord = (id: number) => request.delete(`/api/worklog/records/${id}`)

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
export const statsDashboard = () => request.get('/api/worklog/stats/dashboard')
export const statsMonthly = (params: Record<string, unknown>) => request.get('/api/worklog/stats/monthly', { params })
export const statsRange = (params: Record<string, unknown>) => request.get('/api/worklog/stats/range', { params })

// ---- 导出 ----
export const exportUrl = (path: string, params?: Record<string, unknown>) => {
  const qs = params
    ? '?' +
      Object.entries(params)
        .filter(([, v]) => v !== '' && v !== undefined && v !== null)
        .map(([k, v]) => `${encodeURIComponent(k)}=${encodeURIComponent(String(v))}`)
        .join('&')
    : ''
  return import.meta.env.BASE_URL.replace(/\/$/, '') + `/api/worklog/export/${path}${qs}`
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
  user_id: number
  role: 'boss' | 'leader' | 'worker'
  worker_id?: number | null
  username: string
  worker_name?: string
}

export const getMe = () => request.get('/api/worklog/me')
export const getTeam = () => request.get('/api/worklog/team')
export const createTeam = (data: { name: string; note?: string }) => request.post('/api/worklog/teams', data)
export const joinTeam = (data: { invite_code: string; worker_name: string }) => request.post('/api/worklog/team/join', data)
export const updateMemberRole = (id: number, role: 'leader' | 'worker') => request.put(`/api/worklog/team/members/${id}/role`, { role })
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
  days: number
  hours: number
  pieces: number
  rest_count: number
  amount_cents: number
  cells: AttendanceCell[]
}
export const getAttendance = (params: Record<string, unknown>) => request.get('/api/worklog/attendance', { params })

// ---- 恢复 ----
export const importDB = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return request.post('/api/worklog/import/db', form, { timeout: 120000 })
}
