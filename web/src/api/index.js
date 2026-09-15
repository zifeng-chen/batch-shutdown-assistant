import axios from 'axios'

const api = axios.create({ baseURL: '/api' })

api.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

api.interceptors.response.use(
  (res) => res,
  (err) => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      window.location.href = '/login'
    }
    return Promise.reject(err)
  }
)

export const authApi = {
  login: (data) => api.post('/auth/login', data),
  me: () => api.get('/auth/me'),
  changePassword: (old_password, new_password) =>
    api.post('/auth/change-password', null, { params: { old_password, new_password } }),
}

export const deviceApi = {
  list: (params) => api.get('/devices', { params }),
  stats: () => api.get('/devices/stats'),
  create: (data) => api.post('/devices', data),
  update: (id, data) => api.put(`/devices/${id}`, data),
  delete: (id) => api.delete(`/devices/${id}`),
  batchDelete: (ids) => api.post('/devices/batch-delete', { device_ids: ids }),
  importCsv: (file, groupName) => {
    const fd = new FormData()
    fd.append('file', file)
    fd.append('group_name', groupName || '')
    return api.post('/devices/import/csv', fd)
  },
  importIpRange: (startIp, endIp, groupName) =>
    api.post('/devices/import/ip-range', null, {
      params: { start_ip: startIp, end_ip: endIp, group_name: groupName || '' },
    }),
}

export const taskApi = {
  list: () => api.get('/tasks'),
  create: (data) => api.post('/tasks', data),
  get: (id) => api.get(`/tasks/${id}`),
  cancel: (id) => api.post(`/tasks/${id}/cancel`),
}

export const auditApi = {
  list: (params) => api.get('/audit', { params }),
}

export const deployApi = {
  generateToken: (data) => api.post('/deploy/generate-token', null, { params: data }),
  downloadPackage: (deviceIds) =>
    api.post('/deploy/package', { device_ids: deviceIds || [] }, { responseType: 'blob' }),
}

export const upgradeApi = {
  upload: (file) => {
    const fd = new FormData()
    fd.append('file', file)
    return api.post('/upgrade/upload', fd)
  },
  latest: () => api.get('/upgrade/latest'),
  fileUpload: (file) => {
    const fd = new FormData()
    fd.append('file', file)
    return api.post('/upgrade/file-upload', fd)
  },
}

export default api
