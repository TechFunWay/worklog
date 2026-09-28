import axios from 'axios'
import { useAuthStore } from '../stores/auth'
import router from '../router'
import { onFnOSGatewayOrigin } from '../utils/gateway'

const request = axios.create({
  baseURL: import.meta.env.BASE_URL,
  timeout: 30000,
})

request.interceptors.request.use((config) => {
	// fnOS unified-gateway packages are served below /app/<appname>/.
	// Axios treats a leading slash as host-rooted, so make API clients relative
	// to Vite's build-time base only for that deployment.
	if (import.meta.env.BASE_URL !== '/' && config.url?.startsWith('/')) {
		config.url = config.url.slice(1)
	}
  const authStore = useAuthStore()
  // 飞牛网关域上不带应用自己的 Authorization：接入层会把它当成自己的会话 token，
  // 认不出就直接回 "invalid token"，请求到不了应用（详见 utils/gateway.ts）。
  // 那里的登录态由服务端用网关注入的 X-Trim-* 身份解析；直连端口照旧带 JWT。
  if (authStore.token && !onFnOSGatewayOrigin()) {
    config.headers.Authorization = `Bearer ${authStore.token}`
  }
  return config
})

request.interceptors.response.use(
  (response) => {
    return response
  },
  (error) => {
    // 网关域上的登录态由服务端用网关注入身份解析，401 多半是「该 NAS 用户确实
    // 没有绑定应用账号」，不是应用会话过期：这里只清本地（用户若还绑着别的账号，
    // 下一次请求服务端仍能把他认回来），不跳登录页，让用户留在原页面由路由
    // 守卫处理。直连端口照旧「401 即登出并回登录页」。
    if (error.response?.status === 401) {
      const authStore = useAuthStore()
      authStore.logout()
      if (!onFnOSGatewayOrigin()) {
        router.push('/login')
      }
    }
    return Promise.reject(error)
  }
)

export default request
