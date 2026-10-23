// 工地上下文（筛选值）的统一解析与本地记忆。
//
// 三处共用同一套语义：考勤表的工地筛选、记工页的工地上下文、记工弹窗的默认工地。
//   ''     全部工地（不写 localStorage）
//   'none' 未分工地（project_id IS NULL）
//   number 指定工地 id
// 本地键见 SK.lastProjectId：只记「上次具体选择」，选「全部工地」不清除它——
// 记工弹窗在全部工地上下文下仍要默认上次记过的工地。
import { SK } from './storage-keys'

export type ProjectFilter = number | 'none' | ''

/** 解析 URL 查询参数 / localStorage 原值；识别不了返回 ''（全部工地）。 */
export function parseProjectFilter(raw: unknown): ProjectFilter {
  if (raw === 'none') return 'none'
  const s = typeof raw === 'number' ? String(raw) : typeof raw === 'string' ? raw.trim() : ''
  if (/^\d+$/.test(s) && Number(s) > 0) return Number(s)
  return ''
}

/** 读上次工地（供初始默认值用，是否仍在工地列表里由调用方校验）。 */
export function readLastProjectFilter(): ProjectFilter {
  try {
    return parseProjectFilter(localStorage.getItem(SK.lastProjectId))
  } catch {
    return ''
  }
}

/** 写回上次工地；选「全部工地」时保留原值（见文件头说明）。 */
export function saveLastProjectFilter(filter: ProjectFilter): void {
  if (filter === '') return
  try {
    localStorage.setItem(SK.lastProjectId, String(filter))
  } catch {
    // localStorage 不可用（隐私模式等）时静默降级：只是不再记住上次工地。
  }
}

/** 校验筛选值是否仍然可用：'none' 始终可用，具体工地须在当前工地列表里。 */
export function isProjectFilterAvailable(filter: ProjectFilter, projectIds: number[]): boolean {
  if (filter === '' || filter === 'none') return true
  return projectIds.includes(filter)
}
