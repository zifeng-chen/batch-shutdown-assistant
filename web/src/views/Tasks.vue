<template>
  <div class="tasks-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <div>
            <div class="page-title">任务中心</div>
            <div class="page-subtitle">批量任务下发记录与执行结果追踪</div>
          </div>
          <el-button type="primary" :icon="Plus" @click="showCreate = true">新建批量任务</el-button>
        </div>
      </template>

      <div v-if="tasks.length === 0 && !loading" class="empty-state">
        <div class="empty-state__icon"><el-icon><Promotion /></el-icon></div>
        <div class="empty-state__title">暂无任务记录</div>
        <div class="empty-state__hint">点击右上角「新建批量任务」开始下发指令</div>
      </div>

      <el-table v-else :data="tasks" v-loading="loading" empty-text="暂无任务">
        <el-table-column prop="id" label="任务 ID" width="250" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono">{{ row.id }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="task_type" label="类型" width="130">
          <template #default="{ row }">
            <el-tag effect="light">{{ typeLabel(row.task_type) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="description" label="操作内容" min-width="140" show-overflow-tooltip />
        <el-table-column label="目标设备" min-width="200">
          <template #default="{ row }">
            <div v-if="row.device_ips && row.device_ips.length" class="ip-tags">
              <el-tag v-for="ip in row.device_ips.slice(0, 3)" :key="ip" size="small" effect="plain" type="info">{{ ip }}</el-tag>
              <el-tag v-if="row.device_ips.length > 3" size="small" effect="plain" type="info">
                +{{ row.device_ips.length - 3 }}
              </el-tag>
            </div>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <span class="status-dot" :class="'status-dot--' + statusTone(row.status)">
              {{ statusLabel(row.status) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="170">
          <template #default="{ row }"><span class="muted">{{ formatTime(row.created_at) }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="140" fixed="right">
          <template #default="{ row }">
            <el-button size="small" link type="primary" @click="viewTask(row)">详情</el-button>
            <el-button
              v-if="row.status === 'running' || row.status === 'pending'"
              size="small" type="danger" link
              @click="cancelTask(row)"
            >取消</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 新建任务对话框 -->
    <el-dialog v-model="showCreate" title="新建批量任务" width="600px">
      <el-form label-width="100px">
        <el-form-item label="任务类型">
          <el-select v-model="createForm.task_type" style="width: 100%">
            <el-option label="Sysprep 关机" value="sysprep" />
            <el-option label="Sysprep 重启" value="sysprep_reboot" />
            <el-option label="弹窗提示" value="hostmsg" />
            <el-option label="采集设备信息" value="sysinfo" />
          </el-select>
        </el-form-item>
        <el-form-item label="选择设备">
          <div class="device-picker">
            <el-checkbox-group v-model="createForm.device_ids">
              <el-checkbox v-for="d in onlineDevices" :key="d.id" :label="d.id" class="device-check">
                <span class="mono">{{ d.ip }}</span>
                <span class="muted"> · {{ d.hostname || '未命名' }}</span>
              </el-checkbox>
            </el-checkbox-group>
            <div v-if="onlineDevices.length === 0" class="empty-hint">暂无在线设备</div>
          </div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="submitTask" :disabled="createForm.device_ids.length === 0">
          下发任务 ({{ createForm.device_ids.length }})
        </el-button>
      </template>
    </el-dialog>

    <!-- 任务详情对话框 -->
    <el-dialog v-model="showDetail" title="任务详情" width="820px">
      <el-descriptions :column="2" border class="detail-desc">
        <el-descriptions-item label="任务 ID"><span class="mono">{{ detailTask?.id }}</span></el-descriptions-item>
        <el-descriptions-item label="类型">{{ typeLabel(detailTask?.task_type) }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <span class="status-dot" :class="'status-dot--' + statusTone(detailTask?.status)">
            {{ statusLabel(detailTask?.status) }}
          </span>
        </el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatTime(detailTask?.created_at) }}</el-descriptions-item>
      </el-descriptions>

      <div class="detail-list-title">执行明细</div>
      <el-table :data="detailTask?.results || []" size="small" empty-text="暂无执行明细">
        <el-table-column label="目标设备" width="170">
          <template #default="{ row }">
            <span class="mono">{{ row.device_ip || row.device_id }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="110">
          <template #default="{ row }">
            <span class="status-dot" :class="'status-dot--' + statusTone(row.status)">
              {{ statusLabel(row.status) }}
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="output" label="输出" show-overflow-tooltip />
        <el-table-column prop="finished_at" label="完成时间" width="170">
          <template #default="{ row }"><span class="muted">{{ formatTime(row.finished_at) }}</span></template>
        </el-table-column>
      </el-table>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, nextTick } from 'vue'
import { taskApi, deviceApi } from '../api'
import { formatTime } from '../utils/time'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Plus, Promotion } from '@element-plus/icons-vue'

const tasks = ref([])
const loading = ref(false)
const showCreate = ref(false)
const showDetail = ref(false)
const detailTask = ref(null)
const onlineDevices = ref([])

const createForm = ref({
  task_type: 'sysprep',
  device_ids: [],
})

const statusTone = (s) => ({
  completed: 'online', success: 'online', failed: 'danger', running: 'upgrading',
  pending: 'offline', cancelled: 'offline', sent: 'upgrading',
}[s] || 'offline')

const statusLabel = (s) => ({
  completed: '已完成', success: '成功', failed: '失败', running: '执行中',
  pending: '待执行', cancelled: '已取消', sent: '已下发',
}[s] || s || '-')

const typeLabel = (t) => ({
  sysprep: 'Sysprep 关机', sysprep_reboot: 'Sysprep 重启',
  upgrade: '推送升级', file_push: '文件分发', command: '远程指令',
  hostmsg: '弹窗提示', sysinfo: '采集信息',
}[t] || t || '-')

const loadTasks = async () => {
  loading.value = true
  try {
    const res = await taskApi.list()
    tasks.value = res.data
  } finally {
    loading.value = false
  }
}

const loadOnlineDevices = async () => {
  const res = await deviceApi.list({ status: 'online' })
  onlineDevices.value = res.data
}

const submitTask = async () => {
  await taskApi.create({
    task_type: createForm.value.task_type,
    device_ids: createForm.value.device_ids,
  })
  showCreate.value = false
  createForm.value = { task_type: 'sysprep', device_ids: [] }
  ElMessage.success('任务已下发')
  loadTasks()
}

let taskWs = null

const viewTask = async (row) => {
  const res = await taskApi.get(row.id)
  detailTask.value = res.data
  showDetail.value = true

  // WebSocket 实时监听任务结果
  if (taskWs) taskWs.close()
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  taskWs = new WebSocket(`${protocol}//${window.location.host}/ws/tasks/${row.id}`)
  taskWs.onmessage = (event) => {
    const data = JSON.parse(event.data)
    if (data.type === 'result' && detailTask.value) {
      const results = detailTask.value.results || []
      const idx = results.findIndex(r => r.id === data.command_id)
      if (idx >= 0) {
        results[idx].status = data.status
        results[idx].output = data.output
        results[idx].finished_at = new Date().toISOString()
      }
      // 检查是否全部完成
      const allDone = results.every(r => ['success', 'failed', 'cancelled'].includes(r.status))
      if (allDone) {
        detailTask.value.status = results.every(r => r.status === 'success') ? 'completed' : 'failed'
      }
    }
  }
}

// 关闭对话框时断开 WebSocket
watch(showDetail, (val) => {
  if (!val && taskWs) {
    taskWs.close()
    taskWs = null
  }
})

const cancelTask = async (row) => {
  await ElMessageBox.confirm('确认取消该任务？', '警告', { type: 'warning' })
  await taskApi.cancel(row.id)
  ElMessage.success('已取消')
  loadTasks()
}

onMounted(() => {
  loadTasks()
  loadOnlineDevices()
})
</script>

<style scoped>
.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
}
.mono { font-family: ui-monospace, "SF Mono", Menlo, monospace; font-size: 12px; }
.muted { color: var(--text-secondary); }
.ip-tags { display: flex; flex-wrap: wrap; gap: 4px; }

.device-picker {
  max-height: 260px;
  overflow-y: auto;
  width: 100%;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}
.device-check { display: flex; width: 100%; margin-right: 0; }
.empty-hint { color: var(--text-placeholder); font-size: 13px; padding: var(--space-2); }

.detail-desc { margin-bottom: var(--space-5); }
.detail-list-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--text-regular);
  margin-bottom: var(--space-3);
}
</style>
