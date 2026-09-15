<template>
  <div>
    <el-card style="margin-bottom: 20px">
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <span>设备管理</span>
          <div>
            <el-input v-model="search" placeholder="搜索主机名/IP" style="width: 200px; margin-right: 10px" clearable @clear="loadDevices" @keyup.enter="loadDevices" />
            <el-button type="primary" @click="showAdd = true">添加设备</el-button>
            <el-button type="success" @click="showImport = true">批量导入</el-button>
            <el-button type="warning" :loading="downloading" @click="downloadDeployPackage">生成U盘安装包</el-button>
            <el-button type="danger" @click="openUpgrade">推送升级</el-button>
            <el-button type="info" @click="showFilePush = true">文件分发</el-button>
            <el-button @click="loadDevices">刷新</el-button>
          </div>
        </div>
      </template>

      <!-- 批量操作栏 -->
      <div v-if="selectedIds.length > 0" style="margin-bottom: 15px; padding: 12px 16px; background: #ecf5ff; border-radius: 4px; display: flex; align-items: center; gap: 10px">
        <span style="font-weight: bold; color: #409eff">已选 {{ selectedIds.length }} 台设备</span>
        <el-divider direction="vertical" />
        <el-button size="small" type="primary" @click="batchTask('sysprep')">批量Sysprep关机</el-button>
        <el-button size="small" type="warning" @click="batchTask('sysprep_reboot')">批量Sysprep重启</el-button>
        <el-button size="small" type="danger" @click="batchDelete">批量删除</el-button>
        <el-button size="small" @click="selectedIds = []">取消选择</el-button>
      </div>

      <!-- 在线设备 -->
      <div style="margin-bottom: 8px; font-size: 15px; font-weight: bold; color: #67c23a">在线设备 ({{ onlineDevices.length }})</div>
      <el-table ref="tableRef" :data="onlineDevices" stripe v-loading="loading" @selection-change="onSelectionChange" style="margin-bottom: 25px">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="ip" label="IP地址" width="150">
          <template #default="{ row }">
            <span style="font-weight: bold; color: #303133">{{ row.ip || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="hostname" label="主机名" min-width="120">
          <template #default="{ row }">
            <span style="cursor: pointer; border-bottom: 1px dashed #909399" @click="editHostname(row)">{{ row.hostname || '点击设置' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="agent_version" label="版本" width="70" />
        <el-table-column prop="last_heartbeat" label="最后心跳" width="155">
          <template #default="{ row }">{{ formatTime(row.last_heartbeat) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="240" fixed="right">
          <template #default="{ row }">
            <div style="white-space: nowrap">
              <el-button size="small" type="primary" link @click="quickTask(row, 'sysprep')">Sysprep关机</el-button>
              <el-button size="small" type="warning" link @click="quickTask(row, 'sysprep_reboot')">Sysprep重启</el-button>
              <el-button size="small" type="danger" link @click="deleteDevice(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 离线设备 -->
      <div style="margin-bottom: 8px; font-size: 15px; font-weight: bold; color: #f56c6c">离线设备 ({{ offlineDevices.length }})</div>
      <el-table :data="offlineDevices" stripe v-loading="loading" @selection-change="onSelectionChange">
        <el-table-column type="selection" width="50" />
        <el-table-column prop="ip" label="IP地址" width="150">
          <template #default="{ row }">
            <span style="font-weight: bold; color: #909399">{{ row.ip || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="hostname" label="主机名" min-width="120">
          <template #default="{ row }">
            <span style="cursor: pointer; border-bottom: 1px dashed #dcdfe6; color: #909399" @click="editHostname(row)">{{ row.hostname || '点击设置' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="agent_version" label="版本" width="70" />
        <el-table-column prop="last_heartbeat" label="最后心跳" width="155">
          <template #default="{ row }">{{ formatTime(row.last_heartbeat) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="danger" link @click="deleteDevice(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 单个添加 -->
    <el-dialog v-model="showAdd" title="添加设备" width="450px">
      <el-form :model="newDevice" label-width="80px">
        <el-form-item label="主机名"><el-input v-model="newDevice.hostname" /></el-form-item>
        <el-form-item label="IP地址"><el-input v-model="newDevice.ip" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAdd = false">取消</el-button>
        <el-button type="primary" @click="addDevice">确定</el-button>
      </template>
    </el-dialog>

    <!-- 批量导入 -->
    <el-dialog v-model="showImport" title="批量导入设备" width="550px">
      <el-tabs v-model="importTab">
        <el-tab-pane label="CSV文件导入" name="csv">
          <el-form label-width="80px">
            <el-form-item label="CSV文件">
              <el-upload ref="uploadRef" :auto-upload="false" :limit="1" accept=".csv" :on-change="onFileChange">
                <el-button type="primary">选择文件</el-button>
                <template #tip><div style="color: #909399; font-size: 12px">格式: hostname,ip（首行为表头）</div></template>
              </el-upload>
            </el-form-item>
          </el-form>
        </el-tab-pane>
        <el-tab-pane label="IP段导入" name="range">
          <el-form label-width="80px">
            <el-form-item label="起始IP"><el-input v-model="ipRange.start" placeholder="192.168.1.1" /></el-form-item>
            <el-form-item label="结束IP"><el-input v-model="ipRange.end" placeholder="192.168.1.254" /></el-form-item>
          </el-form>
        </el-tab-pane>
      </el-tabs>
      <template #footer>
        <el-button @click="showImport = false">取消</el-button>
        <el-button type="primary" :loading="importing" @click="doImport">导入</el-button>
      </template>
    </el-dialog>

    <!-- 编辑主机名 -->
    <el-dialog v-model="showEditHost" title="修改主机名" width="400px">
      <el-form label-width="80px">
        <el-form-item label="主机名"><el-input v-model="editHostName" placeholder="输入新主机名" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEditHost = false">取消</el-button>
        <el-button type="primary" @click="saveHostname">确定</el-button>
      </template>
    </el-dialog>

    <!-- 推送升级 -->
    <el-dialog v-model="showUpgrade" title="推送升级" width="600px">
      <el-form label-width="100px">
        <el-form-item label="目标版本">
          <el-tag size="large" type="success">v{{ upgradeVersion }}</el-tag>
        </el-form-item>
        <el-form-item label="目标设备">
          <el-radio-group v-model="upgradeTarget">
            <el-radio value="all">所有在线设备 ({{ onlineCount }})</el-radio>
            <el-radio value="selected">选中设备 ({{ selectedIds.length }})</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="upgradeTarget === 'selected' && selectedIds.length > 0" label="选中列表">
          <div style="max-height: 200px; overflow-y: auto; border: 1px solid #ebeef5; border-radius: 4px; padding: 8px; width: 100%">
            <div v-for="d in selectedOnlineDevices" :key="d.id" style="padding: 4px 0; font-size: 13px">
              <span style="font-weight: bold">{{ d.ip }}</span>
              <span style="color: #909399; margin-left: 8px">{{ d.hostname || '-' }}</span>
              <span style="color: #909399; margin-left: 8px">v{{ d.agent_version || '?' }}</span>
            </div>
          </div>
        </el-form-item>
        <el-form-item v-if="upgradeTarget === 'selected' && selectedIds.length === 0">
          <div style="color: #f56c6c; font-size: 12px">请先在列表中勾选要升级的设备</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showUpgrade = false">取消</el-button>
        <el-button type="primary" :loading="upgrading" @click="doUpgrade">推送升级</el-button>
      </template>
    </el-dialog>

    <!-- 文件分发 -->
    <el-dialog v-model="showFilePush" title="文件分发" width="600px">
      <el-form label-width="100px">
        <el-form-item label="选择文件">
          <el-upload :auto-upload="false" :limit="1" :on-change="(f) => filePushFile = f.raw">
            <el-button type="primary">选择文件</el-button>
          </el-upload>
        </el-form-item>
        <el-form-item label="目标目录">
          <el-input v-model="filePushDir" placeholder="默认 C:\LanAgent\files" />
        </el-form-item>
        <el-form-item label="目标设备">
          <el-radio-group v-model="filePushTarget">
            <el-radio value="all">所有在线设备 ({{ onlineCount }})</el-radio>
            <el-radio value="selected">选中设备 ({{ selectedIds.length }})</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showFilePush = false">取消</el-button>
        <el-button type="primary" :loading="filePushing" @click="doFilePush">分发文件</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { deviceApi, taskApi, deployApi, upgradeApi } from '../api'
import { formatTime } from '../utils/time'
import { ElMessage, ElMessageBox } from 'element-plus'

const devices = ref([])
const loading = ref(false)
const search = ref('')
const showAdd = ref(false)
const newDevice = ref({ hostname: '', ip: '' })
const selectedIds = ref([])

const showImport = ref(false)
const importTab = ref('csv')
const importGroup = ref('')
const importing = ref(false)
const csvFile = ref(null)
const uploadRef = ref(null)
const ipRange = ref({ start: '', end: '' })
const downloading = ref(false)
const tableRef = ref(null)
const showEditHost = ref(false)
const editHostName = ref('')
const editHostId = ref('')
const showUpgrade = ref(false)
const upgrading = ref(false)
const upgradeVersion = ref('')
const upgradeTarget = ref('all')
const showFilePush = ref(false)
const filePushing = ref(false)
const filePushFile = ref(null)
const filePushDir = ref('')
const filePushTarget = ref('all')

const onlineCount = computed(() => devices.value.filter(d => d.status === 'online').length)
const onlineDevices = computed(() => devices.value.filter(d => d.status === 'online'))
const offlineDevices = computed(() => devices.value.filter(d => d.status !== 'online'))
const selectedOnlineDevices = computed(() =>
  devices.value.filter(d => selectedIds.value.includes(d.id) && d.status === 'online')
)

const statusLabel = (s) => ({ online: '在线', offline: '离线', recovering: '恢复中' }[s] || s)

const onSelectionChange = (rows) => {
  selectedIds.value = rows.map(r => r.id)
}

const loadDevices = async () => {
  loading.value = true
  try {
    const res = await deviceApi.list({ search: search.value || undefined })
    devices.value = res.data
    selectedIds.value = []
  } finally {
    loading.value = false
  }
}

const editHostname = (row) => {
  editHostId.value = row.id
  editHostName.value = row.hostname || ''
  showEditHost.value = true
}

const saveHostname = async () => {
  await deviceApi.update(editHostId.value, { hostname: editHostName.value })
  ElMessage.success('主机名已更新')
  showEditHost.value = false
  loadDevices()
}

const addDevice = async () => {
  await deviceApi.create(newDevice.value)
  showAdd.value = false
  newDevice.value = { hostname: '', ip: '' }
  ElMessage.success('添加成功')
  loadDevices()
}

const deleteDevice = async (row) => {
  await ElMessageBox.confirm(`确认删除设备 ${row.hostname || row.ip}？`, '警告', { type: 'warning' })
  await deviceApi.delete(row.id)
  ElMessage.success('已删除')
  tableRef.value?.clearSelection()
  loadDevices()
}

const quickTask = async (row, type) => {
  const labels = { sysprep: 'Sysprep关机', sysprep_reboot: 'Sysprep重启' }
  await ElMessageBox.confirm(`确认对 ${row.hostname || row.ip} 执行${labels[type]}？`, '确认操作', { type: 'warning' })
  await taskApi.create({ task_type: type, device_ids: [row.id] })
  ElMessage.success('任务已下发')
}

const batchTask = async (type) => {
  const labels = { sysprep: 'Sysprep关机', sysprep_reboot: 'Sysprep重启' }
  const count = selectedIds.value.length
  await ElMessageBox.confirm(
    `确认对选中的 ${count} 台设备执行【${labels[type]}】？\n\n此操作不可撤销，请谨慎确认。`,
    '批量操作确认',
    { type: 'warning', confirmButtonText: '确认执行', cancelButtonText: '取消' }
  )
  await taskApi.create({ task_type: type, device_ids: selectedIds.value })
  ElMessage.success(`已向 ${count} 台设备下发${labels[type]}任务`)
  selectedIds.value = []
}

const batchDelete = async () => {
  const count = selectedIds.value.length
  await ElMessageBox.confirm(
    `确认删除选中的 ${count} 台设备？\n\n此操作不可恢复！`,
    '批量删除确认',
    { type: 'error', confirmButtonText: '确认删除', cancelButtonText: '取消' }
  )
  const res = await deviceApi.batchDelete(selectedIds.value)
  ElMessage.success(`批量删除完成: 成功 ${res.data.deleted}/${res.data.requested}`)
  selectedIds.value = []
  tableRef.value?.clearSelection()
  loadDevices()
}

const onFileChange = (file) => {
  csvFile.value = file.raw
}

const doImport = async () => {
  importing.value = true
  try {
    let res
    if (importTab.value === 'csv') {
      if (!csvFile.value) {
        ElMessage.warning('请选择CSV文件')
        return
      }
      res = await deviceApi.importCsv(csvFile.value, importGroup.value)
    } else {
      if (!ipRange.value.start || !ipRange.value.end) {
        ElMessage.warning('请填写起止IP')
        return
      }
      res = await deviceApi.importIpRange(ipRange.value.start, ipRange.value.end)
    }
    const d = res.data
    ElMessage.success(`导入完成: 新增 ${d.added}, 跳过 ${d.skipped}`)
    showImport.value = false
    loadDevices()
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '导入失败')
  } finally {
    importing.value = false
  }
}

const downloadDeployPackage = async () => {
  downloading.value = true
  try {
    const res = await deployApi.downloadPackage(selectedIds.value.length > 0 ? selectedIds.value : undefined)
    const url = window.URL.createObjectURL(new Blob([res.data]))
    const a = document.createElement('a')
    a.href = url
    a.download = 'LanAgent-Deploy.zip'
    a.click()
    window.URL.revokeObjectURL(url)
    ElMessage.success('安装包已下载，拷到U盘后以管理员身份运行 LanAgent.exe')
  } catch (e) {
    ElMessage.error('生成安装包失败')
  } finally {
    downloading.value = false
  }
}

const onUpgradeFileChange = (file) => {
  upgradeFile.value = file.raw
}

const openUpgrade = async () => {
  try {
    const res = await upgradeApi.latest()
    upgradeVersion.value = res.data.version
  } catch (e) {
    upgradeVersion.value = '1.1.0'
  }
  showUpgrade.value = true
}

const doUpgrade = async () => {
  let targetDevices
  if (upgradeTarget.value === 'selected') {
    if (selectedIds.value.length === 0) {
      ElMessage.warning('请先勾选要升级的设备')
      return
    }
    targetDevices = selectedOnlineDevices.value
  } else {
    targetDevices = devices.value.filter(d => d.status === 'online')
  }

  if (targetDevices.length === 0) {
    ElMessage.warning('没有符合条件的在线设备')
    return
  }

  await ElMessageBox.confirm(
    `确认向 ${targetDevices.length} 台设备推送升级到 v${upgradeVersion.value}？\n\n升级过程中设备会短暂断开连接。`,
    '推送升级确认',
    { type: 'warning', confirmButtonText: '确认推送', cancelButtonText: '取消' }
  )
  upgrading.value = true
  try {
    // 使用服务器上的最新exe，不需要上传
    const latestRes = await upgradeApi.latest()
    const upgradeUrl = `${window.location.origin}/api/upgrade/file/${latestRes.data.filename || ''}`

    // 直接用服务器上编译好的exe路径
    const serverExeUrl = `${window.location.origin}/api/upgrade/latest-download`

    const deviceIds = targetDevices.map(d => d.id)
    await taskApi.create({
      task_type: 'upgrade',
      device_ids: deviceIds,
      params: { url: serverExeUrl, version: upgradeVersion.value },
    })
    ElMessage.success(`已向 ${deviceIds.length} 台设备推送升级到 v${upgradeVersion.value}`)
    showUpgrade.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '推送升级失败')
  } finally {
    upgrading.value = false
  }
}

const doFilePush = async () => {
  if (!filePushFile.value) {
    ElMessage.warning('请选择要分发的文件')
    return
  }
  let targetDevices
  if (filePushTarget.value === 'selected') {
    if (selectedIds.value.length === 0) {
      ElMessage.warning('请先勾选目标设备')
      return
    }
    targetDevices = selectedOnlineDevices.value
  } else {
    targetDevices = devices.value.filter(d => d.status === 'online')
  }
  if (targetDevices.length === 0) {
    ElMessage.warning('没有符合条件的在线设备')
    return
  }
  filePushing.value = true
  try {
    const uploadRes = await upgradeApi.fileUpload(filePushFile.value)
    const fileUrl = `${window.location.origin}/api/upgrade/file/${uploadRes.data.filename}`
    const deviceIds = targetDevices.map(d => d.id)
    await taskApi.create({
      task_type: 'file_push',
      device_ids: deviceIds,
      params: {
        url: fileUrl,
        filename: uploadRes.data.original_name,
        dest_dir: filePushDir.value || undefined,
      },
    })
    ElMessage.success(`已向 ${deviceIds.length} 台设备分发文件: ${uploadRes.data.original_name}`)
    showFilePush.value = false
    filePushFile.value = null
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '文件分发失败')
  } finally {
    filePushing.value = false
  }
}

onMounted(loadDevices)
</script>
