<template>
  <el-card>
    <template #header>
      <div style="display: flex; justify-content: space-between; align-items: center">
        <span>审计日志</span>
        <div>
          <el-select v-model="filter.action" placeholder="操作类型" clearable style="width: 150px; margin-right: 10px" @change="loadLogs">
            <el-option label="登录" value="login" />
            <el-option label="设备创建" value="device_create" />
            <el-option label="设备更新" value="device_update" />
            <el-option label="设备删除" value="device_delete" />
            <el-option label="批量导入CSV" value="import_csv" />
            <el-option label="批量导入IP段" value="import_ip_range" />
            <el-option label="任务创建" value="task_create" />
            <el-option label="任务取消" value="task_cancel" />
          </el-select>
          <el-button @click="loadLogs">刷新</el-button>
        </div>
      </div>
    </template>

    <el-table :data="logs" stripe v-loading="loading">
      <el-table-column prop="created_at" label="时间" width="180">
        <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
      </el-table-column>
      <el-table-column prop="username" label="操作人" width="120" />
      <el-table-column prop="action" label="操作" width="140">
        <template #default="{ row }">
          <el-tag size="small">{{ actionLabel(row.action) }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column prop="target" label="目标" min-width="200" show-overflow-tooltip />
      <el-table-column prop="detail" label="详情" min-width="250" show-overflow-tooltip />
      <el-table-column prop="ip_address" label="来源IP" width="130" />
    </el-table>

    <div style="margin-top: 15px; text-align: right">
      <el-pagination
        v-model:current-page="page"
        :page-size="50"
        :total="total"
        layout="total, prev, pager, next"
        @current-change="loadLogs"
      />
    </div>
  </el-card>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { auditApi } from '../api'
import { formatTime } from '../utils/time'

const logs = ref([])
const loading = ref(false)
const page = ref(1)
const total = ref(0)
const filter = ref({ action: '' })

const actionLabel = (a) => ({
  login: '登录', device_create: '设备创建', device_update: '设备更新',
  device_delete: '设备删除', import_csv: 'CSV导入', import_ip_range: 'IP段导入',
  task_create: '任务创建', task_cancel: '任务取消',
}[a] || a)

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
