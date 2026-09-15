<template>
  <div>
    <el-card style="margin-bottom: 20px">
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <span>任务中心</span>
          <el-button type="primary" @click="showCreate = true">新建批量任务</el-button>
        </div>
      </template>

      <el-table :data="tasks" stripe v-loading="loading">
        <el-table-column prop="id" label="任务ID" width="300" show-overflow-tooltip />
        <el-table-column prop="task_type" label="类型" width="120">
          <template #default="{ row }">
            <el-tag>{{ row.task_type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="statusType(row.status)">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150">
          <template #default="{ row }">
            <el-button size="small" link @click="viewTask(row)">详情</el-button>
            <el-button v-if="row.status === 'running' || row.status === 'pending'" size="small" type="danger" link @click="cancelTask(row)">取消</el-button>
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
          </el-select>
        </el-form-item>
        <el-form-item label="选择设备">
          <el-checkbox-group v-model="createForm.device_ids">
            <el-checkbox v-for="d in onlineDevices" :key="d.id" :label="d.id">
              {{ d.hostname || d.ip }} ({{ d.ip }})
            </el-checkbox>
          </el-checkbox-group>
          <div v-if="onlineDevices.length === 0" style="color: #909399">暂无在线设备</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" @click="submitTask" :disabled="createForm.device_ids.length === 0">下发任务</el-button>
      </template>
    </el-dialog>

    <!-- 任务详情对话框 -->
    <el-dialog v-model="showDetail" title="任务详情" width="800px">
      <el-descriptions :column="2" border style="margin-bottom: 20px">
        <el-descriptions-item label="任务ID">{{ detailTask?.id }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ detailTask?.task_type }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="statusType(detailTask?.status)">{{ detailTask?.status }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatTime(detailTask?.created_at) }}</el-descriptions-item>
      </el-descriptions>

      <el-table :data="detailTask?.results || []" stripe size="small">
        <el-table-column prop="device_id" label="设备ID" width="300" show-overflow-tooltip />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'success' ? 'success' : row.status === 'failed' ? 'danger' : 'info'" size="small">
              {{ row.status }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="output" label="输出" show-overflow-tooltip />
        <el-table-column prop="finished_at" label="完成时间" width="170">
          <template #default="{ row }">{{ formatTime(row.finished_at) }}</template>
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

const statusType = (s) => ({
  completed: 'success', success: 'success', failed: 'danger', running: 'warning',
  pending: 'info', cancelled: 'info', sent: 'warning',
}[s] || 'info')

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
