// 浮层栈：统一登记「当前打开的浮层」，供 ESC 关闭判定用。
//
// 背景：弹窗与确认框可以叠加（例如在弹窗里再弹出确认框）。若每个浮层各自监听
// document 的 keydown，一次 ESC 会把所有叠加的浮层一起关掉——用户只想关掉最上面
// 那一层，结果连下面的弹窗也没了。
//
// 口径：浮层**打开时**入栈、关闭时出栈；只有栈顶的浮层才响应 ESC。
// 栈是模块级的单例（一个页面只有一个 document）。
//
// 注意：登记必须发生在「打开」时而不是「组件创建」时——弹窗组件在模板里是常驻的，
// 关闭状态也照样会创建实例，若在创建时入栈，栈里会混进一堆并未打开的浮层，
// 导致 ESC 永远落不到真正可见的那一层。

let seq = 0
const stack: number[] = []

/** 浮层打开时入栈，返回它的令牌；关闭时用同一个令牌出栈。 */
export function pushOverlay(): number {
  seq += 1
  stack.push(seq)
  return seq
}

/** 浮层关闭时出栈。令牌不在栈里时忽略（重复出栈不应误伤其它浮层）。 */
export function popOverlay(token: number): void {
  const i = stack.indexOf(token)
  if (i >= 0) {
    stack.splice(i, 1)
  }
}

/** 该浮层是否是最上面的一层。只有栈顶才响应 ESC。 */
export function isTopOverlay(token: number): boolean {
  return stack.length > 0 && stack[stack.length - 1] === token
}

/** 当前栈深（调试用）。 */
export function overlayDepth(): number {
  return stack.length
}
