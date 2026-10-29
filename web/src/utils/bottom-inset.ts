// 底部避让（--app-bottom-inset）
//
// 背景（Gitee issue IKNAOC）：飞牛手机 App 用内嵌 WebView 打开应用时，
// 安卓三键导航条（返回/Home/无障碍）直接压在页面底部导航上——iOS 的刘海
// 与横条会进 env(safe-area-inset-bottom)，但安卓 WebView 基本恒报 0，
// 靠 env() 挡不住，必须自己留出一段底部空间。
//
// 做法：
// - 自动档：安卓触屏环境默认预留 48px（三键导航条的常见高度），
//   并取 env(safe-area-inset-bottom) 的实测值兜底（iOS 上 env 有值就用它）；
// - 手动档：偏好设置里可切 0 / 24 / 48 / 72px（不同 ROM 导航条高度不同，
//   全面屏手势条很薄，用户可自己调）；
// - 结果写进 <html> 的 CSS 变量 --app-bottom-inset，底部导航、main 的
//   底部内边距、贴底弹窗（Modal / ConfirmDialog）全部读同一个变量。
// 设置存 localStorage——这是「本机这台设备」的观感参数，不进服务端配置。

const STORAGE_KEY = 'workl…set'

/** 'auto' 或手动档位对应的 px 数 */
export type InsetMode = 'auto' | number

export const INSET_PRESETS: { value: InsetMode; label: string }[] = [
  { value: 'auto', label: '自动' },
  { value: 0, label: '不预留' },
  { value: 24, label: '较窄' },
  { value: 48, label: '标准' },
  { value: 72, label: '较宽' },
]

/** 安卓三键导航条的常见高度（px），自动档在安卓上的默认预留 */
const ANDROID_NAV_PX = 48

export function readInsetMode(): InsetMode {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === null || raw === 'auto') return 'auto'
  const n = Number(raw)
  return Number.isFinite(n) && n >= 0 && n <= 160 ? n : 'auto'
}

/** 实测 env(safe-area-inset-bottom)：探针元素量计算值，iOS/安卓 WebView 都准 */
function measureEnvInset(): number {
  const probe = document.createElement('div')
  probe.style.cssText =
    'position:fixed;bottom:0;left:0;width:0;height:0;padding-bottom:env(safe-area-inset-bottom);visibility:hidden;pointer-events:none'
  document.body.appendChild(probe)
  const v = parseFloat(getComputedStyle(probe).paddingBottom) || 0
  probe.remove()
  return v
}

function isAndroidTouch(): boolean {
  return /Android|HarmonyOS/i.test(navigator.userAgent || '')
}

/** 当前模式换算出的实际 px */
export function resolveInsetPx(mode: InsetMode): number {
  const env = measureEnvInset()
  if (mode !== 'auto') return Math.max(mode, env)
  return Math.max(env, isAndroidTouch() ? ANDROID_NAV_PX : 0)
}

/** 把当前档位写进 --app-bottom-inset；主题/旋转/档位变化后都应调用 */
export function applyBottomInset(mode: InsetMode = readInsetMode()): number {
  const px = resolveInsetPx(mode)
  document.documentElement.style.setProperty('--app-bottom-inset', `${px}px`)
  return px
}

export function setBottomInset(mode: InsetMode): void {
  if (mode === 'auto') localStorage.removeItem(STORAGE_KEY)
  else localStorage.setItem(STORAGE_KEY, String(mode))
  applyBottomInset(mode)
}

let wired = false
/** 应用启动时调用一次：立即生效 + 视口变化（旋转、折叠屏展开）后重新量测 */
export function watchBottomInset(): void {
  if (wired) return
  wired = true
  applyBottomInset()
  const reapply = () => applyBottomInset()
  window.addEventListener('resize', reapply)
  window.addEventListener('orientationchange', reapply)
}
