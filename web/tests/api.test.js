import { describe, it, expect, beforeEach, vi } from 'vitest'

// 在导入被测模块前准备 localStorage / window
const store = {}
globalThis.localStorage = {
  getItem: (k) => (k in store ? store[k] : null),
  setItem: (k, v) => { store[k] = String(v) },
  removeItem: (k) => { delete store[k] },
  clear: () => { Object.keys(store).forEach(k => delete store[k]) },
}
globalThis.window = { location: { href: '/' } }

const calls = []
vi.mock('axios', () => {
  const instance = {
    interceptors: {
      request: { use: (fn) => calls.push({ type: 'req', fn }) },
      response: { use: (ok, err) => calls.push({ type: 'res', ok, err }) },
    },
    get: (...a) => Promise.resolve({ data: { args: a } }),
    post: (...a) => Promise.resolve({ data: { args: a } }),
    put: (...a) => Promise.resolve({ data: { args: a } }),
    delete: (...a) => Promise.resolve({ data: { args: a } }),
  }
  return { default: { create: () => instance } }
})

const { default: api, authApi, deviceApi, taskApi, auditApi, upgradeApi } =
  await import('../src/api/index.js')

const reqInterceptor = calls.find(c => c.type === 'req').fn
const resInterceptor = calls.find(c => c.type === 'res')

describe('请求拦截器', () => {
  beforeEach(() => localStorage.clear())

  it('有 token 时注入 Authorization', () => {
    localStorage.setItem('token', 'abc123')
    const cfg = reqInterceptor({ headers: {} })
    expect(cfg.headers.Authorization).toBe('Bearer abc123')
  })

  it('无 token 时不注入', () => {
    const cfg = reqInterceptor({ headers: {} })
    expect(cfg.headers.Authorization).toBeUndefined()
  })
})

describe('响应拦截器', () => {
  beforeEach(() => {
    localStorage.clear()
    window.location.href = '/'
  })

  it('401 时清除 token 并跳转登录', async () => {
    localStorage.setItem('token', 'expired')
    const err = { response: { status: 401 } }
    await expect(resInterceptor.err(err)).rejects.toBe(err)
    expect(localStorage.getItem('token')).toBeNull()
    expect(window.location.href).toBe('/login')
  })

  it('401 以外的错误不改动 token', async () => {
    localStorage.setItem('token', 'keep')
    const err = { response: { status: 500 } }
    await expect(resInterceptor.err(err)).rejects.toBe(err)
    expect(localStorage.getItem('token')).toBe('keep')
  })
})

describe('API 端点拼接', () => {
  it('auth 端点', async () => {
    expect((await authApi.login({ u: 1 })).data.args[0]).toBe('/auth/login')
    expect((await authApi.me()).data.args[0]).toBe('/auth/me')
    expect((await authApi.changePassword('a', 'b')).data.args[0]).toBe('/auth/change-password')
  })

  it('通过旧密码调用时参数命名正确', async () => {
    const res = await authApi.changePassword('old1', 'new1')
    expect(res.data.args[1]).toEqual({ old_password: 'old1', new_password: 'new1' })
  })

  it('device 端点', async () => {
    expect((await deviceApi.stats()).data.args[0]).toBe('/devices/stats')
    expect((await deviceApi.create({ ip: '1.1.1.1' })).data.args[0]).toBe('/devices')
    expect((await deviceApi.update('id1', { hostname: 'h' })).data.args[0]).toBe('/devices/id1')
    expect((await deviceApi.delete('id2')).data.args[0]).toBe('/devices/id2')
  })

  it('批量删除使用 device_ids 字段', async () => {
    const res = await deviceApi.batchDelete(['a', 'b'])
    expect(res.data.args[0]).toBe('/devices/batch-delete')
    expect(res.data.args[1]).toEqual({ device_ids: ['a', 'b'] })
  })

  it('IP 段导入字段名正确', async () => {
    const res = await deviceApi.importIpRange('10.0.0.1', '10.0.0.9', 'grp')
    expect(res.data.args[1]).toEqual({
      start_ip: '10.0.0.1', end_ip: '10.0.0.9', group_name: 'grp',
    })
  })

  it('CSV 导入构造 FormData', async () => {
    const res = await deviceApi.importCsv(new Blob(['x']), 'g1')
    const fd = res.data.args[1]
    expect(res.data.args[0]).toBe('/devices/import/csv')
    expect(fd instanceof FormData).toBe(true)
    expect(fd.get('group_name')).toBe('g1')
  })

  it('task 端点', async () => {
    expect((await taskApi.list()).data.args[0]).toBe('/tasks')
    expect((await taskApi.cancel('t1')).data.args[0]).toBe('/tasks/t1/cancel')
  })

  it('audit 端点透传分页参数', async () => {
    const res = await auditApi.list({ page: 2, size: 50 })
    expect(res.data.args[0]).toBe('/audit')
    expect(res.data.args[1]).toEqual({ params: { page: 2, size: 50 } })
  })

  it('部署包请求为 blob 响应', async () => {
    const res = await deviceApi.importCsv // 占位，防止 lint 未使用
    const pkg = await import('../src/api/index.js').then(m => m.deployApi.downloadPackage(['d1']))
    expect(pkg.data.args[1]).toEqual({ device_ids: ['d1'] })
    expect(pkg.data.args[2]).toEqual({ responseType: 'blob' })
  })

  it('升级接口端点', async () => {
    expect((await upgradeApi.latest()).data.args[0]).toBe('/upgrade/latest')
  })
})