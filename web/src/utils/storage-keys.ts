// 本地存储键统一加应用前缀。本应用与提醒事项、租赁管理等兄弟应用都会跑在飞牛
// 网关域下，同域即共享同一 localStorage/sessionStorage：裸键名会互相覆盖——
// 别的应用写入的 token 会顶掉本应用的登录态，反过来也一样。加前缀后各写各的
// 键，互不干扰。旧裸键不再读取也不再删除（同域上无法分辨归属，删除可能误伤
// 其它应用的会话）。
const P = 'worklog.'

export const SK = {
  token: `${P}token`,
  // 安全提示的「稍后再说」时间戳：也必须是带前缀的键——网关域下与兄弟应用
  // 共享 localStorage，裸键会互相覆盖。
  securityPromptDismissed: `${P}security_prompt_dismissed_at`,
} as const
