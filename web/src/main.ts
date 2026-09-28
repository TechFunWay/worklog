import { createApp } from 'vue'
import { createPinia } from 'pinia'
import router from './router'
import App from './App.vue'
import './style.css'

// 换号弹窗的回跳落点：register?fnos_switch_close=1。在应用挂载前通知打开者
// 「飞牛账号已切换完成」并自关，不在弹窗里渲染完整应用。window.close() 在
// 个别环境会被忽略，兜底显示一句提示，避免用户对着白屏。
if (new URLSearchParams(window.location.search).get('fnos_switch_close') === '1') {
  try {
    window.opener?.postMessage({ type: 'fnos_switch_done' }, window.location.origin)
  } catch { /* opener 已关闭等情况忽略 */ }
  window.close()
  document.title = '换号完成'
  document.body.innerHTML =
    '<p style="font:14px/1.6 system-ui,sans-serif;color:#666;text-align:center;padding-top:40vh">飞牛账号已切换，请关闭本窗口返回应用</p>'
} else {
  const app = createApp(App)
  app.use(createPinia())
  app.use(router)
  app.mount('#app')
}
