<template>
  <el-card>
    <template #header>
      <div class="card-header">
        <div>
          <div class="page-title">审计日志</div>
          <div class="page-subtitle">记录所有管理操作，可追溯每一步变更</div>
        </div>
        <div class="toolbar">
          <el-select
            v-model="filter.action"
            placeholder="全部操作类型"
            clearable
            class="action-select"
            @change="onFilterChange"
          >
            <el-option label="登录" value="login" />
            <el-option label="设备创建" value="device_create" />
            <el-option label="设备更新" value="device_update" />
            <el-option label="设备删除" value="device_delete" />
            <el-option label="批量导入 CSV" value="import_csv" />
            <el-option label="批量导入 IP 段" value="import_ip_range" />
            <el-option label="任务创建" value="task_create" />
            <el-option label="任务取消" value="task_cancel" />
          </el-select>
          <el-button :icon="Refresh" @click="loadLogs">刷新</el-button>
        </div>
      </div>
    </template>

    <div v-if="logs.length === 0 && !loading" class="empty-state">
      <div class="empty-state__icon"><el-icon><DocumentRemove /></el-icon></div>
      <div class="empty-state__title">暂无审计记录</div>
      <div class="empty-state__hint">系统操作将自动记录在此</div>
    </div>

    <el-table v-else :data="logs" v-loading="loading" empty-text="暂无记录">
      <el-table-column prop="created_at" label="时间" width="170">
        <template #default="{ row }"><span class="muted">{{ formatTime(row.created_at) }}</span></template>
      </el-table-column>
      <el-table-column prop="username" label="操作人" width="120">
        <template #default="{ row }">
          <span class="user-chip">{{ row.username }}</span>
        </template>
      </el-table-column>
      <el-table-column prop="action" label="操作" width="150">
        <template #default="{ row }">
          <el-tag size="small" effect="light" :type="actionTone(row.action)">
            {{ actionLabel(row.action) }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="target" label="目标" min-width="180" show-overflow-tooltip />
      <el-table-column prop="detail" label="详情" min-width="240" show-overflow-tooltip />
      <el-table-column prop="ip_address" label="来源 IP" width="140">
        <template #default="{ row }"><span class="mono">{{ row.ip_address }}</span></template>
      </el-table-column>
    </el-table>

    <div class="pager">
      <el-pagination
        v-model:current-page="page"
        :page-size="50"
        :total="total"
        layout="total, prev, pager, next"
        background
        @current-change="loadLogs"
      />
    </div>
  </el-card>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { auditApi } from '../api'
import { formatTime } from '../utils/time'
import { Refresh, DocumentRemove } from '@element-plus/icons-vue'

const logs = ref([])
const loading = ref(false)
const page = ref(1)
const total = ref(0)
const filter = ref({ action: '' })

const actionLabel = (a) => ({
  login: '登录', device_create: '设备创建', device_update: '设备更新',
  device_delete: '设备删除', import_csv: 'CSV导入', import_ip_range: 'IP段导入',
  task_create: '任务创建', task_cancel: '任务取消',
  device_batch_delete: '批量删除', deploy_package: '下载安装包',
  password_change: '修改密码',
}[a] || a)

const actionTone = (a) => {
  if (a.includes('delete')) return 'danger'
  if (a.includes('create') || a.includes('import')) return 'success'
  if (a.includes('cancel')) return 'warning'
  return 'primary'
}

const onFilterChange = () => {
  page.value = 1
  loadLogs()
}

const loadLogs = async () => {
  loading.value = true
  try {
    const res = await auditApi.list({
      page: page.value,
      size: 50,
      action: filter.value.action || undefined,
    })
    logs.value = res.data.items
    total.value = res.data.total
  } finally {
    loading.value = false
  }
}

onMounted(loadLogs)
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
}
.action-select { width: 180px; }
.mono { font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 12px; }
.muted { color: var(--text-secondary); }
.user-chip {
  display: inline-block;
  padding: 1px 8px;
  border-radius: var(--radius-sm);
  background: var(--brand-primary-soft);
  color: var(--brand-primary-dark);
  font-size: 12px;
  font-weight: 500;
}
.pager {
  margin-top: var(--space-5);
  display: flex;
  justify-content: flex-end;
}
</style>