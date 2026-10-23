// 金额与数量格式化工具。服务端金额一律为整数「分」。

export function fmtYuan(cents: number | undefined | null): string {
  if (cents === undefined || cents === null) return '0.00'
  const neg = cents < 0
  const abs = Math.abs(Math.round(cents))
  const s = (abs / 100).toFixed(2)
  const [int, frac] = s.split('.')
  const withSep = int.replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  return (neg ? '-' : '') + withSep + '.' + frac
}

// 「¥1,234.56」带符号版本
export function fmtMoney(cents: number | undefined | null): string {
  return '¥' + fmtYuan(cents)
}

// 统计卡用的紧凑金额：整元省略「.00」，避免 375 宽四列卡里数字撑破卡片。
export function fmtYuanShort(cents: number | undefined | null): string {
  const n = Math.round(cents ?? 0)
  const neg = n < 0
  const abs = Math.abs(n)
  const yuan = Math.floor(abs / 100)
  const fen = abs % 100
  const intStr = String(yuan).replace(/\B(?=(\d{3})+(?!\d))/g, ',')
  const s = fen === 0 ? intStr : `${intStr}.${String(fen).padStart(2, '0')}`
  return (neg ? '-' : '') + s
}

// 用户输入的元金额 → 分（返回 null 表示无效）
export function yuanToCents(input: string): number | null {
  const s = input.trim().replace(/[¥,，\s]/g, '')
  if (!/^\d+(\.\d{1,2})?$/.test(s)) return null
  return Math.round(parseFloat(s) * 100)
}

export function centsToYuanInput(cents: number | undefined | null): string {
  if (cents === undefined || cents === null || cents === 0) return ''
  return (cents / 100).toFixed(2)
}

// 数量格式化：接口里的天数/小时/件数可能是 12.50 这类字符串，展示时去掉多余的 0，
// 最多保留两位小数（点工 0.5 天、计时 0.25 小时都要原样显示）。
export function fmtQty(value: number | string | undefined | null): string {
  const n = typeof value === 'string' ? parseFloat(value) : value
  if (n === undefined || n === null || !isFinite(n)) return '0'
  return String(+n.toFixed(2))
}

export const TYPE_LABELS: Record<string, string> = { day: '点工', hour: '点时', piece: '计件' }
export const TYPE_ICONS: Record<string, string> = { day: '📅', hour: '⏱', piece: '📦' }
export const METHOD_LABELS: Record<string, string> = { cash: '现金', wechat: '微信', alipay: '支付宝', other: '其他' }

export function fmtDateCN(d: string): string {
  // "2026-09-06" → "09-06 周六"
  const date = new Date(d + 'T00:00:00')
  const week = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'][date.getDay()]
  return d.slice(5) + ' ' + week
}

export function todayStr(): string {
  const d = new Date()
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}

export function addDays(dateStr: string, delta: number): string {
  const d = new Date(dateStr + 'T00:00:00')
  d.setDate(d.getDate() + delta)
  const m = String(d.getMonth() + 1).padStart(2, '0')
  const day = String(d.getDate()).padStart(2, '0')
  return `${d.getFullYear()}-${m}-${day}`
}
