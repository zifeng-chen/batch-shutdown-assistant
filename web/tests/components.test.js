// @vitest-environment jsdom
import { describe, it, expect, vi } from 'vitest'
import { mount, config } from '@vue/test-utils'
import { h } from 'vue'

// 通配 stub：所有 el-* 组件渲染为带 class 的 div，保留插槽
const Passthrough = {
  inheritAttrs: false,
  setup(_, { slots, attrs }) {
    return () => h('div', { class: attrs.class }, [
      slots.default ? slots.default() : null,
      slots.header ? slots.header() : null,
      slots.footer ? slots.footer() : null,
    ])
  },
}

config.global.stubs = new Proxy({}, {
  get: (_, name) => (String(name).startsWith('el-') || name === 'router-link' || name === 'router-view'
    ? Passthrough
    : undefined),
})

config.global.mocks = {
  $route: { path: '/login' },
  $router: { push: vi.fn() },
}

vi.mock('element-plus', async (importOriginal) => {
  const actual = await importOriginal()
  return {
    ...actual,
    ElMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
    ElMessageBox: { confirm: vi.fn().mockResolvedValue(true) },
  }
})

vi.mock('../src/api/index.js', () => ({
  authApi: { login: vi.fn(), changePassword: vi.fn() },
  upgradeApi: {
    latest: vi.fn().mockResolvedValue({ data: { version: '1.4.1' } }),
    fileUpload: vi.fn(),
  },
  deviceApi: {
    list: vi.fn().mockResolvedValue({ data: [] }),
    stats: vi.fn().mockResolvedValue({ data: { total: 0, online: 0, offline: 0 } }),
    create: vi.fn(), update: vi.fn(), delete: vi.fn(),
    batchDelete: vi.fn(), importCsv: vi.fn(), importIpRange: vi.fn(),
  },
  taskApi: {
    list: vi.fn().mockResolvedValue({ data: [] }),
    create: vi.fn(), cancel: vi.fn(),
  },
  auditApi: { list: vi.fn().mockResolvedValue({ data: { items: [], total: 0 } }) },
  deployApi: { downloadPackage: vi.fn() },
}))

const store = {}
globalThis.localStorage = {
  getItem: (k) => (k in store ? store[k] : null),
  setItem: (k, v) => { store[k] = String(v) },
  removeItem: (k) => { delete store[k] },
  clear: () => { Object.keys(store).forEach(k => delete store[k]) },
}

vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() }),
  useRoute: () => ({ path: '/login' }),
}))

const Login = (await import('../src/views/Login.vue')).default
const Help = (await import('../src/views/Help.vue')).default
const Audit = (await import('../src/views/Audit.vue')).default
const Tasks = (await import('../src/views/Tasks.vue')).default
const Devices = (await import('../src/views/Devices.vue')).default

describe('Login.vue 渲染', () => {
  it('挂载无报错并渲染品牌区', () => {
    const w = mount(Login)
    expect(w.find('.login-page').exists()).toBe(true)
    expect(w.find('.brand-title').text()).toBe('LAN Agent')
  })

  it('渲染三个特性说明', () => {
    expect(mount(Login).findAll('.feature-list li')).toHaveLength(3)
  })

  it('渲染用户名与密码输入框', () => {
    expect(mount(Login).findAll('input')).toHaveLength(2)
  })

  it('渲染默认账号提示', () => {
    expect(mount(Login).find('.form-tip').text()).toContain('admin')
  })

  it('包含提交按钮', () => {
    expect(mount(Login).find('button').exists()).toBe(true)
  })
})

describe('Help.vue 渲染', () => {
  it('挂载无报错', () => {
    expect(mount(Help).exists()).toBe(true)
  })

  it('渲染了实质内容', () => {
    expect(mount(Help).text().length).toBeGreaterThan(50)
  })
})

describe('Audit.vue 渲染', () => {
  it('挂载无报错并渲染页面标题', () => {
    const w = mount(Audit)
    expect(w.find('.page-title').text()).toBe('审计日志')
  })

  it('空数据时展示空状态', async () => {
    const w = mount(Audit)
    await new Promise(r => setTimeout(r, 10))
    expect(w.find('.empty-state').exists()).toBe(true)
  })
})

describe('Tasks.vue 渲染', () => {
  it('挂载无报错并渲染页面标题', () => {
    const w = mount(Tasks)
    expect(w.find('.page-title').text()).toBe('任务中心')
  })

  it('空数据时展示空状态', async () => {
    const w = mount(Tasks)
    await new Promise(r => setTimeout(r, 10))
    expect(w.find('.empty-state').exists()).toBe(true)
  })
})

describe('Devices.vue 渲染', () => {
  it('挂载无报错', () => {
    expect(mount(Devices).exists()).toBe(true)
  })

  it('渲染工具栏与搜索框', () => {
    const w = mount(Devices)
    expect(w.find('.toolbar').exists()).toBe(true)
    expect(w.find('.search-input').exists()).toBe(true)
  })

  it('渲染表格/卡片视图切换', () => {
    const w = mount(Devices)
    expect(w.find('.view-switch').exists()).toBe(true)
  })

  it('默认渲染在线/离线两个分区标题', () => {
    const w = mount(Devices)
    const titles = w.findAll('.section-title').map(t => t.text())
    expect(titles.some(t => t.includes('在线'))).toBe(true)
    expect(titles.some(t => t.includes('离线'))).toBe(true)
  })

  it('选中设备后出现批量弹窗/采集按钮', async () => {
    const w = mount(Devices)
    await new Promise(r => setTimeout(r, 10))
    expect(w.html()).not.toContain('批量弹窗')

    w.vm.selectedIds = ['d1']
    await w.vm.$nextTick()
    const html = w.html()
    expect(html).toContain('批量弹窗')
    expect(html).toContain('采集设备信息')
  })
})

describe('设备详情与弹窗逻辑', () => {
  const openDetail = async (device) => {
    const w = mount(Devices)
    await new Promise(r => setTimeout(r, 10))
    w.vm.showDetail(device)
    await w.vm.$nextTick()
    return w
  }

  const fullDevice = {
    id: 'd1', ip: '192.168.1.10', hostname: 'PC-A', status: 'online',
    agent_version: '1.4.1', cpu_model: 'Intel i7', cpu_cores: 8,
    memory_total_mb: 16384, disk_total_gb: 512, disk_free_gb: 300,
    os_version: 'Windows 10 Pro', serial_number: 'SN123',
    mac_address: '00:11:22:33:44:55', manufacturer: 'Lenovo',
    model: 'M720', rearm_count: 2, sysinfo_updated_at: '2026-10-06T10:00:00',
  }

  it('showDetail 打开详情对话框', async () => {
    const w = await openDetail(fullDevice)
    expect(w.vm.showDetailDlg).toBe(true)
    expect(w.vm.detailDevice.cpu_model).toBe('Intel i7')
  })

  it('详情渲染全部硬件字段', async () => {
    const w = await openDetail(fullDevice)
    const html = w.html()
    for (const text of ['Intel i7', '16384 MB', '512 GB', 'SN123',
                        '00:11:22:33:44:55', 'Lenovo', 'M720', 'Windows 10 Pro']) {
      expect(html).toContain(text)
    }
  })

  it('无硬件信息时展示采集提示', async () => {
    const bare = { id: 'd2', ip: '10.0.0.1', hostname: 'x', status: 'online' }
    const w = await openDetail(bare)
    expect(w.find('.detail-hint').exists()).toBe(true)
  })

  it('有硬件信息时不展示采集提示', async () => {
    const w = await openDetail(fullDevice)
    expect(w.find('.detail-hint').exists()).toBe(false)
  })

  it('showHostMsg 设置单个目标并重置表单', async () => {
    const w = mount(Devices)
    await new Promise(r => setTimeout(r, 10))
    w.vm.hostMsgTimeout = 99
    w.vm.showHostMsg(fullDevice)
    await w.vm.$nextTick()
    expect(w.vm.showHostMsgDlg).toBe(true)
    expect(w.vm.hostMsgTargets).toHaveLength(1)
    expect(w.vm.hostMsgTimeout).toBe(15)   // 已重置
    expect(w.vm.hostMsgMode).toBe('device')
  })

  it('batchHostMsg 未选设备时提示且不打开对话框', async () => {
    const w = mount(Devices)
    await new Promise(r => setTimeout(r, 10))
    w.vm.selectedIds = []
    w.vm.batchHostMsg()
    await w.vm.$nextTick()
    expect(w.vm.showHostMsgDlg).toBe(false)
  })

  it('batchHostMsg 选中在线设备时打开对话框', async () => {
    const w = mount(Devices)
    await new Promise(r => setTimeout(r, 10))
    w.vm.devices = [fullDevice]
    w.vm.selectedIds = ['d1']
    await w.vm.$nextTick()
    w.vm.batchHostMsg()
    await w.vm.$nextTick()
    expect(w.vm.showHostMsgDlg).toBe(true)
    expect(w.vm.hostMsgTargets).toHaveLength(1)
  })
})