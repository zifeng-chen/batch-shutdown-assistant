import { describe, it, expect, beforeEach, vi } from 'vitest'

const store = {}
globalThis.localStorage = {
  getItem: (k) => (k in store ? store[k] : null),
  setItem: (k, v) => { store[k] = String(v) },
  removeItem: (k) => { delete store[k] },
  clear: () => { Object.keys(store).forEach(k => delete store[k]) },
}

const routePaths = []
let guardFn = null

vi.mock('vue-router', () => ({
  createWebHistory: () => ({}),
  createRouter: ({ routes }) => {
    routes.forEach(r => routePaths.push({ path: r.path, auth: !!r.meta?.auth }))
    return {
      getRoutes: () => routes.map(r => ({ path: r.path, meta: r.meta || {} })),
      beforeEach: (fn) => { guardFn = fn },
    }
  },
}))

await import('../src/router/index.js')

describe('路由定义', () => {
  it('包含全部预期路由', () => {
    const paths = routePaths.map(r => r.path).sort()
    expect(paths).toEqual(['/', '/audit', '/devices', '/help', '/login', '/tasks'])
  })

  it('仅登录页公开，其余均需鉴权', () => {
    const open = routePaths.filter(r => !r.auth).map(r => r.path)
    expect(open).toEqual(['/login'])
  })

  it('根路径与设备页标记为受保护', () => {
    expect(routePaths.find(r => r.path === '/').auth).toBe(true)
    expect(routePaths.find(r => r.path === '/devices').auth).toBe(true)
  })
})

describe('路由守卫', () => {
  beforeEach(() => localStorage.clear())

  it('无 token 访问受保护路由 → 跳登录页', () => {
    expect(guardFn({ meta: { auth: true } })).toBe('/login')
  })

  it('有 token 访问受保护路由 → 放行', () => {
    localStorage.setItem('token', 'tok')
    expect(guardFn({ meta: { auth: true } })).toBeUndefined()
  })

  it('登录页无 token 也放行', () => {
    expect(guardFn({ meta: {} })).toBeUndefined()
  })

  it('token 为空字符串时仍视为未登录', () => {
    localStorage.setItem('token', '')
    expect(guardFn({ meta: { auth: true } })).toBe('/login')
  })
})