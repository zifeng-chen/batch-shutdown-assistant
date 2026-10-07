<template>
  <div class="dashboard">
    <!-- 统计卡片 -->
    <div class="grid-stats">
      <div
        v-for="c in statCards"
        :key="c.label"
        class="stat-card"
        :style="{ '--accent': c.color, '--accent-soft': c.soft }"
      >
        <div class="stat-card__head">
          <span class="stat-card__label">{{ c.label }}</span>
          <span class="stat-card__icon"><el-icon><component :is="c.icon" /></el-icon></span>
        </div>
        <div class="stat-card__value">{{ c.value }}</div>
        <div class="stat-card__foot">{{ c.foot }}</div>
      </div>
    </div>

    <!-- 图表区 -->
    <div class="chart-row">
      <el-card class="chart-card">
        <template #header>
          <div class="card-header">
            <span>设备在线状态</span>
            <el-tag size="small" type="info" effect="plain">实时</el-tag>
          </div>
        </template>
        <div class="donut-wrap">
          <div ref="onlineChartEl" class="chart chart--donut"></div>
          <div class="donut-center">
            <div class="donut-center__value">{{ onlineRate }}%</div>
            <div class="donut-center__label">在线率</div>
          </div>
        </div>
      </el-card>

      <el-card class="chart-card">
        <template #header>
          <div class="card-header">
            <span>版本分布</span>
            <el-tag v-if="outdatedCount > 0" size="small" type="warning">
              {{ outdatedCount }} 台待升级
            </el-tag>
            <el-tag v-else size="small" type="success">全部最新</el-tag>
          </div>
        </template>
        <div ref="versionChartEl" class="chart chart--bar"></div>
      </el-card>
    </div>

    <!-- 列表区 -->
    <el-row :gutter="20">
      <el-col :xs="24" :lg="12">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>版本明细</span>
              <span class="card-header__hint">服务端最新 v{{ serverVersion }}</span>
            </div>
          </template>
          <el-table :data="versionDist" stripe size="default" empty-text="暂无设备数据">
            <el-table-column label="版本" min-width="120">
              <template #default="{ row }">
                <el-tag
                  size="small"
                  :type="row.version === serverVersion ? 'success' : 'warning'"
                  effect="light"
                >
                  v{{ row.version }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="count" label="总量" width="80" align="center" />
            <el-table-column label="在线" width="80" align="center">
              <template #default="{ row }">
                <span class="num-online">{{ row.online }}</span>
              </template>
            </el-table-column>
            <el-table-column label="离线" width="80" align="center">
              <template #default="{ row }">
                <span class="num-offline">{{ row.offline }}</span>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>

      <el-col :xs="24" :lg="12">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>最近操作</span>
              <el-tag size="small" effect="plain" type="info">最近 10 条</el-tag>
            </div>
          </template>
          <div v-if="recentAudits.length === 0" class="empty-state">
            <div class="empty-state__icon"><el-icon><DocumentRemove /></el-icon></div>
            <div class="empty-state__title">暂无操作记录</div>
            <div class="empty-state__hint">执行操作后将在此显示</div>
          </div>
          <div v-else class="timeline">
            <div v-for="a in recentAudits" :key="a.id" class="timeline-item">
              <div class="timeline-item__dot" :class="'dot--' + auditTone(a.action)"></div>
              <div class="timeline-item__body">
                <div class="timeline-item__head">
                  <span class="timeline-item__action">{{ actionLabel(a.action) }}</span>
                  <span class="timeline-item__time">{{ formatTime(a.created_at) }}</span>
                </div>
                <div class="timeline-item__detail">
                  <span class="timeline-item__user">{{ a.username }}</span>
                  {{ a.detail }}
                </div>
              </div>
            </div>
          </div>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, nextTick } from 'vue'
import { deviceApi, auditApi, upgradeApi } from '../api'
import { formatTime } from '../utils/time'
import { Monitor, CircleCheck, CircleClose, Upload, DocumentRemove } from '@element-plus/icons-vue'
import * as echarts from 'echarts/core'
import { PieChart, BarChart } from 'echarts/charts'
import { TooltipComponent, GridComponent } from 'echarts/components'
import { CanvasRenderer } from 'echarts/renderers'

echarts.use([PieChart, BarChart, TooltipComponent, GridComponent, CanvasRenderer])

const stats = ref({ total: 0, online: 0, offline: 0 })
const devices = ref([])
const recentAudits = ref([])
const serverVersion = ref('?')
const installedVersion = ref('?')

const onlineChartEl = ref(null)
const versionChartEl = ref(null)
let onlineChart = null
let versionChart = null

const onlineRate = computed(() => {
  const t = stats.value.total || 0
  if (!t) return 0
  return Math.round((stats.value.online / t) * 100)
})

const outdatedCount = computed(() => {
  if (serverVersion.value === '?') return 0
  return devices.value.filter(d => d.agent_version && d.agent_version !== serverVersion.value).length
})

const statCards = computed(() => [
  {
    label: '设备总数', value: stats.value.total, icon: Monitor,
    color: 'var(--brand-primary)', soft: 'var(--brand-primary-light)',
    foot: '已纳管的全部设备',
  },
  {
    label: '在线设备', value: stats.value.online, icon: CircleCheck,
    color: 'var(--color-success)', soft: 'var(--color-success-soft)',
    foot: `在线率 ${onlineRate.value}%`,
  },
  {
    label: '离线设备', value: stats.value.offline, icon: CircleClose,
    color: 'var(--color-info)', soft: 'var(--color-info-soft)',
    foot: stats.value.offline > 0 ? '需检查网络或服务' : '全部在线',
  },
  {
    label: '待升级', value: outdatedCount.value, icon: Upload,
    color: outdatedCount.value > 0 ? 'var(--color-warning)' : 'var(--color-success)',
    soft: outdatedCount.value > 0 ? 'var(--color-warning-soft)' : 'var(--color-success-soft)',
    foot: `服务端最新 v${serverVersion.value}`,
  },
])

const versionDist = computed(() => {
  const map = {}
  for (const d of devices.value) {
    const v = d.agent_version || '未知'
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
  deploy_package: '下载安装包', password_change: '修改密码',
}[a] || a)

const auditTone = (a) => {
  if (a.includes('delete')) return 'danger'
  if (a.includes('create') || a.includes('import')) return 'success'
  if (a.includes('cancel')) return 'warning'
  return 'info'
}

const renderOnlineChart = () => {
  if (!onlineChartEl.value) return
  onlineChart = echarts.init(onlineChartEl.value)
  onlineChart.setOption({
    tooltip: { trigger: 'item', formatter: '{b}: {c} 台 ({d}%)' },
    series: [{
      type: 'pie',
      radius: ['68%', '88%'],
      avoidLabelOverlap: false,
      itemStyle: { borderRadius: 8, borderColor: '#fff', borderWidth: 3 },
      label: { show: false },
      data: [
        { value: stats.value.online, name: '在线', itemStyle: { color: '#10b981' } },
        { value: stats.value.offline, name: '离线', itemStyle: { color: '#cbd5e1' } },
      ],
    }],
  })
}

const renderVersionChart = () => {
  if (!versionChartEl.value) return
  versionChart = echarts.init(versionChartEl.value)
  const rows = versionDist.value.slice(0, 6).reverse()
  versionChart.setOption({
    grid: { left: 8, right: 24, top: 12, bottom: 8, containLabel: true },
    tooltip: { trigger: 'axis', axisPointer: { type: 'shadow' } },
    xAxis: { type: 'value', splitLine: { lineStyle: { color: '#f1f5f9' } }, axisLabel: { color: '#94a3b8' } },
    yAxis: {
      type: 'category',
      data: rows.map(r => 'v' + r.version),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: { color: '#64748b', fontWeight: 500 },
    },
    series: [
      {
        name: '在线', type: 'bar', stack: 'total', barWidth: 18,
        itemStyle: { color: '#10b981', borderRadius: [0, 0, 0, 0] },
        data: rows.map(r => r.online),
      },
      {
        name: '离线', type: 'bar', stack: 'total', barWidth: 18,
        itemStyle: { color: '#e2e8f0', borderRadius: [0, 6, 6, 0] },
        data: rows.map(r => r.offline),
      },
    ],
  })
}

const resize = () => {
  onlineChart?.resize()
  versionChart?.resize()
}

onMounted(async () => {
  const [s, d, a, v] = await Promise.all([
    deviceApi.stats(),
    deviceApi.list({ size: 5000 }),
    auditApi.list({ page: 1, size: 10 }),
    upgradeApi.latest().catch(() => ({ data: { version: '?' } })),
  ])
  stats.value = s.data
  devices.value = d.data
  recentAudits.value = a.data.items || []
  serverVersion.value = v.data.version || '?'

  const versions = devices.value.map(x => x.agent_version).filter(Boolean)
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

  await nextTick()
  renderOnlineChart()
  renderVersionChart()
  window.addEventListener('resize', resize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', resize)
  onlineChart?.dispose()
  versionChart?.dispose()
})
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.card-header__hint { font-size: 12px; color: var(--text-secondary); font-weight: 400; }

.chart-row {
  display: grid;
  grid-template-columns: minmax(0, 5fr) minmax(0, 7fr);
  gap: var(--space-4);
  margin-bottom: var(--space-5);
}
.chart-card :deep(.el-card__body) { padding: var(--space-4); }

.donut-wrap { position: relative; height: 240px; }
.chart { width: 100%; height: 100%; }
.chart--donut { height: 240px; }
.chart--bar { height: 240px; }
.donut-center {
  position: absolute;
  top: 50%; left: 50%;
  transform: translate(-50%, -50%);
  text-align: center;
  pointer-events: none;
}
.donut-center__value {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-primary);
  line-height: 1;
}
.donut-center__label {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 4px;
}

.num-online { color: var(--color-success); font-weight: 600; }
.num-offline { color: var(--text-secondary); }

/* 时间线 */
.timeline { display: flex; flex-direction: column; gap: var(--space-4); }
.timeline-item { display: flex; gap: var(--space-3); }
.timeline-item__dot {
  width: 9px; height: 9px;
  border-radius: 50%;
  margin-top: 6px;
  flex-shrink: 0;
  background: var(--color-info);
}
.dot--success { background: var(--color-success); }
.dot--danger { background: var(--color-danger); }
.dot--warning { background: var(--color-warning); }
.dot--info { background: var(--brand-primary); }
.timeline-item__body { flex: 1; min-width: 0; }
.timeline-item__head {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: var(--space-3);
}
.timeline-item__action { font-weight: 600; color: var(--text-primary); font-size: 13px; }
.timeline-item__time { font-size: 12px; color: var(--text-placeholder); white-space: nowrap; }
.timeline-item__detail {
  font-size: 12px;
  color: var(--text-secondary);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.timeline-item__user {
  display: inline-block;
  padding: 0 6px;
  border-radius: var(--radius-sm);
  background: var(--brand-primary-soft);
  color: var(--brand-primary-dark);
  margin-right: 6px;
}

@media (max-width: 1100px) {
  .chart-row { grid-template-columns: 1fr; }
}
</style>