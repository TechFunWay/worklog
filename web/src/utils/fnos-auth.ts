// 飞牛授权辅助（与提醒事项等应用同思路，按本应用的网关身份通道做了简化）：
//
// 换账号（startFnOSAccountSwitch）：居中弹窗打开飞牛登录页换号，redirect_uri
// 指回【同源】应用页并带 fnos_switch_close=1 标记；main.ts 接住该标记后
// postMessage 通知打开者并自关，应用主页面全程不跳转、确认弹窗保持打开，
// 收到通知后重新取一次网关注入的 NAS 身份即可刷新账号显示。
// 跳转前不探测可达性：换号入口取当前源（应用正跑在网关域时唯一一定可达），
// 直连端口上身份接口本就 401，弹窗按钮不会出现。
//
// 手机端（飞牛 App 网页视图）：弹窗与跳转都不可靠，调用方应隐藏换号入口，
// 提示用户在飞牛 App 内切换账号后重开本应用（fnOSMobileClient 判定）。

const POPUP_WIDTH = 520
const POPUP_HEIGHT = 700

// 手机端（含飞牛手机 App 的网页视图）判定。iPadOS 13+ 的 Safari 会把自己报成
// Macintosh，靠 maxTouchPoints + 粗指针兜住。
export function fnOSMobileClient(): boolean {
  const ua = navigator.userAgent || ''
  if (/Android|iPhone|iPad|iPod|Mobile|HarmonyOS|Windows Phone/i.test(ua)) return true
  return navigator.maxTouchPoints > 1 && window.matchMedia('(pointer: coarse)').matches
}

// 弹窗打开飞牛登录页换号。成功返回 true（主页面不跳转，等 onSwitched 回调）；
// 返回 false 表示当前环境不支持弹窗（手机端或被拦截），由调用方就地提示。
// onSwitched 在两种情况下触发：换号弹窗回跳自关（postMessage），或用户直接
// 关闭了弹窗——后者身份未必真的变了，调用方重取身份即可，取到什么显示什么。
export function startFnOSAccountSwitch(onSwitched: () => void): boolean {
  if (fnOSMobileClient()) return false
  const backUrl = `${window.location.origin}${import.meta.env.BASE_URL}register?fnos_switch_close=1`
  const loginURL = `/login?redirect_uri=${encodeURIComponent(backUrl)}`
  const screen = window.screen as Screen & { availLeft?: number; availTop?: number }
  const areaLeft = typeof screen.availLeft === 'number' ? screen.availLeft : 0
  const areaTop = typeof screen.availTop === 'number' ? screen.availTop : 0
  const left = Math.max(areaLeft, Math.round(areaLeft + ((screen.availWidth || POPUP_WIDTH) - POPUP_WIDTH) / 2))
  const top = Math.max(areaTop, Math.round(areaTop + ((screen.availHeight || POPUP_HEIGHT) - POPUP_HEIGHT) / 2))
  let popup: Window | null
  try {
    popup = window.open(loginURL, 'fnos_switch', `popup=yes,width=${POPUP_WIDTH},height=${POPUP_HEIGHT},left=${left},top=${top}`)
  } catch {
    return false
  }
  if (!popup) return false

  let settled = false
  const finish = () => {
    if (settled) return
    settled = true
    window.clearInterval(closePoll)
    window.removeEventListener('message', onMessage)
    onSwitched()
  }
  // 弹窗回跳到应用页时：页面在挂载前 postMessage 并自关（见 main.ts）。
  function onMessage(event: MessageEvent) {
    if (event.origin !== window.location.origin) return
    const data = event.data as { type?: unknown } | null
    if (data && data.type === 'fnos_switch_done') {
      try { popup?.close() } catch { /* 自关被忽略时由 openers 关 */ }
      finish()
    }
  }
  window.addEventListener('message', onMessage)
  // 用户直接关窗也要收尾（轮询 closed），此时身份未必变化，重取即可。
  const closePoll = window.setInterval(() => {
    let closed = false
    try { closed = popup.closed } catch { closed = true }
    if (closed) finish()
  }, 500)
  return true
}
