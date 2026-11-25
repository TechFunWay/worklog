import request from './request'

// 补贴/扣款/单价的单笔上限（分）= 100 万元，与服务端 maxSubsidyCents 同口径；
// 前端先拦一道，别让老板填错后要等服务器报错才发现。
export const MAX_SUBSIDY_CENTS = 100000000

export interface Worker {
  id?: number
  name: string
  phone?: string
  note?: string
  daily_wage_cents?: number
  hourly_wage_cents?: number
  monthly_wage_cents?: number
  // 档案级默认补贴（分/出勤日，0=未设置）：记工默认补贴四层链的最底一层
  subsidy_cents?: number
  // 工号（选填）：同姓名工人靠它区分，会显示在记工页与工资条
  worker_no?: string
  // 开户行（选填，明文）
  bank_name?: string
  // 身份证 / 银行卡：明文只在提交时上行；下行一律是掩码 + 是否已填写
  id_card?: string
  bank_card?: string
  id_card_masked?: string
  bank_card_masked?: string
  id_card_set?: boolean
  bank_card_set?: boolean
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
  // 工地默认补贴（分/出勤日，如「饭补 15/天」）：0/未设置 = 不启用
  subsidy_cents?: number
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
  // 纯工钱 = 数量 × 单价（分）。amount_cents 是全应用唯一金额真相，
  // 补贴与扣款已在服务端并进去：amount_cents = wage + subsidy - deduct。
  wage_cents?: number
  subsidy_cents?: number
  subsidy_note?: string
  deduct_cents?: number
  amount_cents?: number
  note?: string
  settlement_id?: number | null
  worker_name?: string
  project_name?: string
  piece_name?: string
  piece_unit?: string
  // 审核流：员工/扫码提交先 pending，班组长审核后 approved 才计入工资。
  status?: 'approved' | 'pending' | 'rejected'
  source?: string
  entry_code_id?: number | null
  entry_code_name?: string
  submitter_name?: string
  reviewed_by?: number | null
  reviewed_at?: string | null
  review_note?: string
  reviewer_name?: string
}

export interface Advance {
  id?: number
  worker_id: number
  // 选填：借支归属工地。null/未传 = 未指定工地，只在按人结算时抵扣（v0.6.0）
  project_id?: number | null
  date: string
  amount_cents: number
  method?: string
  note?: string
  settlement_id?: number | null
  worker_name?: string
  project_name?: string
}

// 结算单付款状态（v0.6.0）：unpaid 分文未付 / partial 部分支付 / paid 已付清。
// 只由服务端按 paid_cents 与 payable_cents 比较得出，前端不自己判。
export type PayStatus = 'unpaid' | 'partial' | 'paid'

export const PAY_STATUS_LABELS: Record<PayStatus, string> = {
  unpaid: '未付款',
  partial: '部分付款',
  paid: '已付清',
}

// 结算单的付款流水：一张结算单可分多笔支付（对应后端 SettlementPayment）。
export interface SettlementPayment {
  id: number
  settlement_id: number
  worker_id: number
  amount_cents: number
  method?: string
  paid_on: string
  note?: string
  created_by?: number
  created_at?: string
  worker_name?: string
}

export interface Settlement {
  id: number
  worker_id: number
  worker_name: string
  // 工号（选填）：同姓名工人区分用
  worker_no?: string
  project_id?: number | null
  project_name?: string
  period_start: string
  period_end: string
  work_amount_cents: number
  advance_amount_cents: number
  // 调整项：抹零/迟到扣款为负，额外奖励为正（有符号）；应付 = 应发 − 借支 + 调整
  adjust_cents?: number
  adjust_note?: string
  payable_cents: number
  // 已付 = 本单付款流水之和（服务端事务内 SUM 重算）；尚欠 = 应付 − 已付（派生）
  paid_cents?: number
  pay_status?: PayStatus
  due_cents?: number
  payment_count?: number
  // 付款流水：只有详情接口返回
  payments?: SettlementPayment[]
  record_count: number
  details?: string
  note?: string
  settled_at: string
}

// 结算单 details JSON 快照里的一行记工（历史快照，字段与记工表同名）。
// v0.6.0 起工资条要拆项展示，所以快照带 wage/subsidy/deduct；老结算单的快照没有
// 这几个字段，前端一律按「没有就不显示」处理。
export interface SettlementDetailRecord {
  record_id: number
  date: string
  type: string
  type_label: string
  quantity: string
  unit_price_cents: number
  amount_cents: number
  wage_cents?: number
  subsidy_cents?: number
  subsidy_note?: string
  deduct_cents?: number
  project_name?: string
  piece_name?: string
  piece_unit?: string
  note?: string
}
// 快照里的一行借支：project_name 是 v0.6.0 新增（历史工资条显示当时的工地名）
export interface SettlementDetailAdvance {
  advance_id: number
  date: string
  amount_cents: number
  method: string
  method_label: string
  project_name?: string
  note?: string
}
export interface SettlementDetails {
  records: SettlementDetailRecord[]
  advances: SettlementDetailAdvance[]
}

export interface DayWorkerRow extends Worker {
  records: WorkRecord[]
  amount_cents: number
  days: string
  hours: string
  pieces: string
  record_count: number
  pending_count?: number
  pending_amount_cents?: number
  rejected_count?: number
}

// ---- 工人 ----
export const listWorkers = (params?: Record<string, unknown>) => request.get('/api/worklog/workers', { params })
export const createWorker = (data: Worker) => request.post('/api/worklog/workers', data)
export const updateWorker = (id: number, data: Worker) => request.put(`/api/worklog/workers/${id}`, data)
export const deleteWorker = (id: number) => request.delete(`/api/worklog/workers/${id}`)
// 敏感信息（身份证/银行卡）完整值：仅老板可调用，列表接口只给掩码。
export interface WorkerPII { worker_no?: string; id_card?: string; bank_card?: string; bank_name?: string }
export const getWorkerPII = (id: number) => request.get(`/api/worklog/workers/${id}/pii`)

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
  // 该工人在该工地的默认补贴（分/出勤日）与来源（v0.6.0，与单价同一条覆盖链）
  subsidy_cents?: number
  worker_subsidy_cents?: number
  effective_subsidy_cents?: number
  subsidy_source?: SubsidySource
}
export interface ProjectPricesData {
  project: Project
  project_daily_price_cents: number
  project_hourly_price_cents: number
  // 工地默认补贴（分/出勤日）；老后端没返回时回落到 project.subsidy_cents
  project_subsidy_cents?: number
  rows: ProjectPriceRow[]
}
export const getProjectPrices = (id: number) => request.get(`/api/worklog/projects/${id}/prices`)
// rows 三项（日价 / 时价 / 默认补贴）全 0 才删行，只填补贴也算单独覆盖（v0.6.0）。
export const saveProjectPrices = (
  id: number,
  rows: { worker_id: number; daily_wage_cents: number; hourly_wage_cents: number; subsidy_cents: number }[],
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
  // 补贴/扣款（v0.6.0，单位分）：省略或 null = 服务端按默认链带出（扣款不自动带，等价 0）；
  // 传 0 = 明确不要补贴。员工角色由服务端强制清零，前端不显示输入框。
  subsidy_cents?: number | null
  subsidy_note?: string
  deduct_cents?: number | null
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
// 默认补贴来源：与单价同一条覆盖链，但补贴不按月薪折算，所以没有 monthly 这一层。
export type SubsidySource = 'project_worker' | 'project' | 'worker' | 'none'
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
  // 该工人该工地该日期的默认补贴（分/出勤日）与来源；0 = 没有任何一层设置
  subsidy_cents: number
  subsidy_source: SubsidySource
}
export const getEffectivePrice = (params: Record<string, unknown>) =>
  request.get('/api/worklog/effective-price', { params })

// ---- 借支 ----
// projectId（查询参数 project_id）与记工页同语义：不传 = 全部工地、数字 = 指定工地、
// 'none' = 未分工地（服务端复用 parseProjectFilter）。
export const listAdvances = (params?: Record<string, unknown>) => request.get('/api/worklog/advances', { params })
export const createAdvance = (data: Advance) => request.post('/api/worklog/advances', data)
export const deleteAdvance = (id: number) => request.delete(`/api/worklog/advances/${id}`)

// ---- 结算 ----
export const unsettledList = (params?: Record<string, unknown>) => request.get('/api/worklog/settlements/unsettled', { params })

// 结算预览（确认弹窗的数据源）：summary 是该工人这段时间的未结算汇总，
// loose_advance_* 是「未指定工地」的借支，仅在按项目结算时用于提示（v0.6.0）。
export interface SettlementPreviewSummary extends workerUnsettledRow {
  loose_advance_count?: number
  loose_advance_cents?: number
}
export interface SettlementPreviewData {
  summary: SettlementPreviewSummary
  records: WorkRecord[]
  advances: Advance[]
  loose_advance_count?: number
  loose_advance_cents?: number
}
export const settlementPreview = (params: Record<string, unknown>) => request.get('/api/worklog/settlements/preview', { params })
// 预览响应里的「未指定工地借支」笔数：兼容放在顶层与 summary 两种写法。
export function looseAdvanceCount(data?: SettlementPreviewData | null): number {
  if (!data) return 0
  return Number(data.loose_advance_count ?? data.summary?.loose_advance_count ?? 0) || 0
}

// 结算入参（v0.6.0）：paid_cents 省略 = 全额（与历史行为一致）；
// adjust_cents 有符号（抹零/罚款为负、奖励为正），仅应付 > 0 时允许分期。
export interface SettlementInput {
  worker_id?: number
  start?: string
  end?: string
  note?: string
  project_id?: number
  paid_cents?: number | null
  adjust_cents?: number
  adjust_note?: string
}
export const createSettlement = (data: SettlementInput) => request.post('/api/worklog/settlements', data)
export const unsettledProjects = () => request.get('/api/worklog/settlements/unsettled-projects')
export const batchSettlement = (data: SettlementInput) => request.post('/api/worklog/settlements/batch', data)
// listSettlements 支持 pay_status 筛选：'unpaid' | 'partial' | 'paid'（v0.6.0）
export const listSettlements = (params?: Record<string, unknown>) => request.get('/api/worklog/settlements', { params })
export const getSettlement = (id: number) => request.get(`/api/worklog/settlements/${id}`)
export const updateSettlementNote = (id: number, note: string) => request.put(`/api/worklog/settlements/${id}/note`, { note })
// 改已结结算单的调整项（仅老板）：服务端按「记工 − 借支 + 调整」重算应付，
// 已付超过新应付会被拒（message 原样提示即可）。原因上限 60 字。
export const updateSettlementAdjust = (id: number, data: { adjust_cents: number; adjust_note?: string }) =>
  request.put(`/api/worklog/settlements/${id}/adjust`, data)
export const revertSettlement = (id: number) => request.delete(`/api/worklog/settlements/${id}`)

// ---- 结算付款流水（v0.6.0，仅老板可登记/删除）----
export interface SettlementPaymentInput {
  amount_cents: number
  method?: string
  paid_on: string
  note?: string
}
export const createPayment = (id: number, data: SettlementPaymentInput) =>
  request.post(`/api/worklog/settlements/${id}/payments`, data)
export const deletePayment = (id: number, paymentId: number) =>
  request.delete(`/api/worklog/settlements/${id}/payments/${paymentId}`)

// 待付款（欠薪）明细：欠薪留在结算单上，绝不退回未结算池。
export interface SettlementDueRow {
  settlement_id: number
  settlement_no?: string
  worker_id: number
  worker_name: string
  project_name?: string
  payable_cents: number
  paid_cents: number
  due_cents: number
  settled_at: string
  period_start: string
  period_end: string
}
export interface SettlementDueData {
  rows: SettlementDueRow[]
  total_due_cents: number
  count: number
}
export const listSettlementDue = () => request.get('/api/worklog/settlements/due')

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
  // 已结算未付（欠薪）：与 unsettled 是两回事，不要混算（v0.6.0）
  due_cents?: number
  due_count?: number
}

export interface workerUnsettledRow {
  worker_id: number
  worker_name: string
  worker_no?: string
  record_count: number
  work_amount_cents: number
  advance_count: number
  advance_amount_cents: number
  payable_cents: number
  earliest_date: string
  latest_date: string
  // 「未指定工地」的借支：按项目结算时不参与本次抵扣，用这两个字段说清去向
  loose_advance_count?: number
  loose_advance_cents?: number
}

// 按项目未结算汇总的一行（GET /settlements/unsettled-projects）。
// amount_cents = 记工 − 本项目借支；loose_advance_* 只用于提示「按人结算时才抵」。
export interface UnsettledProjectRow {
  project_id: number
  project_name: string
  worker_count: number
  record_count: number
  work_amount_cents: number
  advance_cents: number
  amount_cents: number
  loose_advance_count?: number
  loose_advance_cents?: number
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

// ---- 定时导出与目录选择（v0.6.1）----
export interface FsDirEntry { name: string; path: string }
export interface FsListData { path: string; parent?: string; roots: string[]; dirs: FsDirEntry[] }
export const fsList = (path?: string) => request.get('/api/worklog/fs/list', { params: path ? { path } : undefined })
export const fsMkdir = (path: string, name: string) => request.post('/api/worklog/fs/mkdir', { path, name })
export const runAutoExportNow = (scope?: string) =>
  request.post('/api/worklog/auto-export/run', null, { params: scope ? { scope } : undefined })

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
  review_required?: boolean
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
  review_required?: boolean
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
export const createTeam = (data: { name: string; note?: string; review_required?: boolean }) => request.post('/api/worklog/teams', data)
export const updateTeam = (id: number, data: { name?: string; note?: string; review_required?: boolean }) => request.put(`/api/worklog/teams/${id}`, data)
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

// ---- 扫码报工：报工码管理 ----
// 报工码是给工人手机扫的免登录入口。产品/工人可单独绑定或都绑定：
// 都绑定时扫码后只剩数量一个输入框（可打印成每人每产品一张）。
export interface EntryCode {
  id: number
  name: string
  token: string
  type?: string
  piece_item_id?: number | null
  worker_id?: number | null
  project_id?: number | null
  base_url?: string
  auto_approve: boolean
  enabled: boolean
  expires_at?: string | null
  note?: string
  created_at?: string
  piece_name?: string
  piece_unit?: string
  worker_name?: string
  project_name?: string
  today_count?: number
  pending_count?: number
  total_count?: number
}
export interface EntryCodeInput {
  name?: string
  piece_item_id?: number | null
  worker_id?: number | null
  project_id?: number | null
  base_url?: string
  auto_approve?: boolean
  enabled?: boolean
  expires_at?: string
  note?: string
}
export const listEntryCodes = (params?: Record<string, unknown>) =>
  request.get('/api/worklog/entry-codes', { params })
export const createEntryCode = (data: EntryCodeInput) => request.post('/api/worklog/entry-codes', data)
export const updateEntryCode = (id: number, data: EntryCodeInput) => request.put(`/api/worklog/entry-codes/${id}`, data)
export const deleteEntryCode = (id: number) => request.delete(`/api/worklog/entry-codes/${id}`)
export const rotateEntryCodeToken = (id: number) => request.post(`/api/worklog/entry-codes/${id}/rotate-token`)

// ---- 报工审核 ----
export interface ReviewSummary {
  pending_count: number
  pending_amount_cents: number
  today_pending_count: number
  today_pending_amount_cents: number
  today_approved_count: number
  today_approved_amount_cents: number
  rejected_count: number
}
export const getReviewSummary = () => request.get('/api/worklog/review/summary')
export const listReviewRecords = (params?: Record<string, unknown>) =>
  request.get('/api/worklog/review/records', { params })
export const approveRecords = (ids: number[]) => request.post('/api/worklog/review/approve', { ids })
export const rejectRecords = (ids: number[], note: string) => request.post('/api/worklog/review/reject', { ids, note })

// ---- 批量报工（班组长代录整班产量，直接生效）----
export interface BatchRecordInput {
  date: string
  piece_item_id: number
  project_id?: number | null
  note?: string
  rows: { worker_id: number; quantity: string }[]
}
export const batchCreateRecords = (data: BatchRecordInput) => request.post('/api/worklog/records/batch', data)

// ---- 公开扫码报工（免登录，工人手机用）----
export interface PublicEntryProduct {
  id: number
  name: string
  unit?: string
}
export interface PublicEntryWorker {
  id: number
  name: string
}
export interface PublicEntryInfo {
  site_title?: string
  code: { name: string; auto_approve: boolean; note?: string }
  product: PublicEntryProduct | null
  products: PublicEntryProduct[]
  worker: PublicEntryWorker | null
  // 报工码本身绑定了工人（true）；?w= 只是预设，不是绑定。
  worker_bound: boolean
  workers: PublicEntryWorker[]
  today: string
  yesterday: string
  allow_yesterday: boolean
}
export interface PublicEntrySubmitInput {
  worker_id?: number
  piece_item_id?: number
  quantity: string
  date?: string
  note?: string
}
export interface PublicEntryTodayItem {
  id: number
  piece_name?: string
  piece_unit?: string
  quantity: string
  status: string
  created_at: string
}
export interface PublicEntrySubmitResult {
  record: Pick<WorkRecord, 'id' | 'status' | 'quantity' | 'date'>
  duplicate: boolean
  message: string
  status: string
  // 本次提交对应的日期（今天/昨天），用于成功页文案与已报明细口径。
  date: string
  today_count: number
  today_items: PublicEntryTodayItem[]
}
export const getPublicEntry = (token: string, workerId?: number) =>
  request.get(`/api/worklog/public/entry/${encodeURIComponent(token)}`, {
    params: workerId ? { w: workerId } : undefined,
  })
export const submitPublicEntry = (token: string, data: PublicEntrySubmitInput) =>
  request.post(`/api/worklog/public/entry/${encodeURIComponent(token)}/records`, data)
