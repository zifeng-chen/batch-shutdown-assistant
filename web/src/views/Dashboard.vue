<template>
  <div>
    <el-row :gutter="20" style="margin-bottom: 20px">
      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>设备总数</template>
          <div style="font-size: 32px; font-weight: bold; color: #409eff">{{ stats.total }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>在线</template>
          <div style="font-size: 32px; font-weight: bold; color: #67c23a">{{ stats.online }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>离线</template>
          <div style="font-size: 32px; font-weight: bold; color: #f56c6c">{{ stats.offline }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>服务端最新版本</template>
          <div style="font-size: 32px; font-weight: bold; color: #e6a23c">v{{ serverVersion }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" style="margin-bottom: 20px">
      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>已安装最高版本</template>
          <div style="font-size: 32px; font-weight: bold;" :style="{ color: installedVersion === serverVersion ? '#67c23a' : '#f56c6c' }">v{{ installedVersion }}</div>
        </el-card>
      </el-col>
      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>待升级设备</template>
          <div style="font-size: 32px; font-weight: bold;" :style="{ color: outdatedCount > 0 ? '#f56c6c' : '#67c23a' }">{{ outdatedCount }}</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20">
      <el-col :span="12">
        <el-card>
          <template #header>版本分布</template>
          <el-table :data="versionDist" stripe size="small">
            <el-table-column prop="version" label="版本" width="120">
              <template #default="{ row }">
                <el-tag :type="row.version === latestVersion ? 'success' : 'warning'" size="small">v{{ row.version }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="count" label="数量" width="80" />
            <el-table-column prop="online" label="在线" width="80">
              <template #default="{ row }">
                <span style="color: #67c23a">{{ row.online }}</span>
              </template>
            </el-table-column>
            <el-table-column prop="offline" label="离线" width="80">
              <template #default="{ row }">
                <span style="color: #f56c6c">{{ row.offline }}</span>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
      <el-col :span="12">
        <el-card>
          <template #header>最近操作</template>
          <el-table :data="recentAudits" stripe size="small">
            <el-table-column prop="created_at" label="时间" width="155">
              <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
            </el-table-column>
            <el-table-column prop="username" label="操作人" width="80" />
            <el-table-column prop="action" label="操作" min-width="120">
              <template #default="{ row }">
                <el-tag size="small">{{ actionLabel(row.action) }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="detail" label="详情" min-width="150" show-overflow-tooltip />
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { deviceApi, auditApi, upgradeApi } from '../api'
import { formatTime } from '../utils/time'

const stats = ref({ total: 0, online: 0, offline: 0 })
const devices = ref([])
const recentAudits = ref([])
const serverVersion = ref('?')
const installedVersion = ref('?')

const outdatedCount = computed(() => {
  if (serverVersion.value === '?') return 0
  return devices.value.filter(d => d.agent_version && d.agent_version !== serverVersion.value).length
})

const versionDist = computed(() => {
  const map = {}
  for (const d of devices.value) {
    const v = d.agent_version || '?'
    if (!map[v]) map[v] = { version: v, count: 0, online: 0, offline: 0 }
    map[v].count++
    if (d.status === 'online') map[v].online++
    else map[v].offline++
  }
  return Object.values(map).sort((a, b) => b.count - a.count)
})

const actionLabel = (a) => ({
  login: '登录', device_create: '添加设备', device_update: '更新设备',
  device_delete: '删除设备', device_batch_delete: '批量删除',
  import_csv: 'CSV导入', import_ip_range: 'IP段导入',
  task_create: '任务下发', task_cancel: '任务取消',
  deploy_package: '下载安装包',
}[a] || a)

onMounted(async () => {
  const [s, d, a, v] = await Promise.all([
    deviceApi.stats(),
    deviceApi.list({ size: 200 }),
    auditApi.list({ page: 1, size: 10 }),
    upgradeApi.latest().catch(() => ({ data: { version: '?' } })),
  ])
  stats.value = s.data
  devices.value = d.data
  recentAudits.value = a.data.items || []
  serverVersion.value = v.data.version || '?'

  // 已安装最高版本
  const versions = devices.value.map(d => d.agent_version).filter(Boolean)
  if (versions.length > 0) {
    installedVersion.value = versions.sort((a, b) => {
      const pa = a.split('.').map(Number)
      const pb = b.split('.').map(Number)
      for (let i = 0; i < 3; i++) {
        if ((pa[i] || 0) !== (pb[i] || 0)) return (pb[i] || 0) - (pa[i] || 0)
      }
      return 0
    })[0]
  }
})
</script>
