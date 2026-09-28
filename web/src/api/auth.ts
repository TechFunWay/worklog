import request from './request'

export function login(username: string, password: string) {
  return request.post('/api/auth/login', {
    username,
    password,
  })
}

export function fnosLogin() {
  return request.post('/api/auth/fnos/login')
}

export function getFnOSIdentity() {
  return request.get('/api/auth/fnos/identity')
}

export function bindFnOSAccount(mode: 'register' | 'bind', username: string, password: string) {
  return request.post('/api/auth/fnos/bind', { mode, username, password })
}

export function register(username: string, password: string) {
  return request.post('/api/auth/register', {
    username,
    password,
  })
}

export function checkAuth() {
  return request.get('/api/auth/check')
}

// 用户主动退出：服务端据此落「网关隐式登录抑制」标记（见 stores/auth.ts
// endSession），网关域上不落这条标记的话，下一个请求又会被自动认回登录态。
export function logout() {
  return request.post('/api/auth/logout')
}

export function getCurrentUser() {
  return request.get('/api/auth/me')
}

export function checkSetupRequired() {
  return request.get('/api/auth/setup-required')
}

export function changePassword(oldPassword: string, newPassword: string) {
  return request.put('/api/auth/password', {
    old_password: oldPassword,
    new_password: newPassword,
  })
}

export function regenerateAPIKey() {
  return request.post('/api/auth/apikey')
}
