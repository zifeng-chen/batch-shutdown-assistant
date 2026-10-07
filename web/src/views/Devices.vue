<template>
  <div class="devices-page">
    <!-- 工具栏 -->
    <el-card class="toolbar-card">
      <div class="toolbar">
        <el-input
          v-model="search"
          placeholder="搜索主机名 / IP"
          class="search-input"
          clearable
          :prefix-icon="Search"
          @clear="loadDevices"
          @keyup.enter="loadDevices"
        />
        <el-radio-group v-model="viewMode" class="view-switch">
          <el-radio-button value="table"><el-icon><Grid /></el-icon></el-radio-button>
          <el-radio-button value="card"><el-icon><Menu /></el-icon></el-radio-button>
        </el-radio-group>

        <div class="toolbar--end">
          <el-button type="primary" :icon="Plus" @click="showAdd = true">添加设备</el-button>
          <el-button type="success" :icon="Upload" @click="showImport = true">批量导入</el-button>
          <el-button type="warning" :icon="FolderOpened" :loading="downloading" @click="downloadDeployPackage">U盘安装包</el-button>
          <el-button type="danger" :icon="Top" @click="openUpgrade">推送升级</el-button>
          <el-button type="info" :icon="Files" @click="showFilePush = true">文件分发</el-button>
          <el-button :icon="Refresh" circle @click="loadDevices" />
        </div>
      </div>

      <!-- 批量操作条 -->
      <transition name="fade-slide">
        <div v-if="selectedIds.length > 0" class="batch-bar">
          <div class="batch-bar__info">
            <el-icon><Select /></el-icon>
            已选 <b>{{ selectedIds.length }}</b> 台设备
          </div>
          <div class="batch-bar__actions">
            <el-button size="small" type="success" @click="batchHostMsg">批量弹窗</el-button>
            <el-button size="small" type="info" @click="batchSysinfo">采集设备信息</el-button>
            <el-button size="small" type="primary" @click="batchTask('sysprep')">批量 Sysprep 关机</el-button>
            <el-button size="small" type="warning" @click="batchTask('sysprep_reboot')">批量 Sysprep 重启</el-button>
            <el-button size="small" type="danger" @click="batchDelete">批量删除</el-button>
            <el-button size="small" text @click="clearSelection">取消选择</el-button>
          </div>
        </div>
      </transition>
    </el-card>

    <!-- 表格视图 -->
    <template v-if="viewMode === 'table'">
      <el-card class="section-card">
        <template #header>
          <div class="card-header">
            <span class="section-title">
              <span class="status-dot status-dot--online"></span>
              在线设备
            </span>
            <el-tag type="success" effect="light" round>{{ onlineDevices.length }}</el-tag>
          </div>
        </template>
        <el-table
          ref="tableRef"
          :data="onlineDevices"
          v-loading="loading"
          @selection-change="onSelectionChange"
          empty-text="暂无在线设备"
        >
          <el-table-column type="selection" width="46" />
          <el-table-column label="状态" width="88">
            <template #default><span class="status-dot status-dot--online">在线</span></template>
          </el-table-column>
          <el-table-column prop="ip" label="IP 地址" width="150">
            <template #default="{ row }">
              <span class="ip-text">{{ row.ip || '-' }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="hostname" label="主机名" min-width="140">
            <template #default="{ row }">
              <span class="hostname-edit" @click="editHostname(row)">{{ row.hostname || '点击设置' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="版本" width="96">
            <template #default="{ row }">
              <el-tag size="small" effect="plain">{{ row.agent_version || '?' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="last_heartbeat" label="最后心跳" width="160">
            <template #default="{ row }">
              <span class="muted">{{ formatTime(row.last_heartbeat) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="280" fixed="right">
            <template #default="{ row }">
              <div class="row-actions">
                <el-button size="small" type="info" link @click="showDetail(row)">详情</el-button>
                <el-button size="small" type="success" link @click="showHostMsg(row)">弹窗</el-button>
                <el-button size="small" type="primary" link @click="quickTask(row, 'sysprep')">关机</el-button>
                <el-button size="small" type="warning" link @click="quickTask(row, 'sysprep_reboot')">重启</el-button>
                <el-button size="small" type="danger" link @click="deleteDevice(row)">删除</el-button>
              </div>
            </template>
          </el-table-column>
        </el-table>
      </el-card>

      <el-card class="section-card">
        <template #header>
          <div class="card-header">
            <span class="section-title">
              <span class="status-dot status-dot--offline"></span>
              离线设备
            </span>
            <el-tag type="info" effect="light" round>{{ offlineDevices.length }}</el-tag>
          </div>
        </template>
        <el-table
          :data="offlineDevices"
          v-loading="loading"
          @selection-change="onSelectionChange"
          empty-text="暂无离线设备"
        >
          <el-table-column type="selection" width="46" />
          <el-table-column label="状态" width="88">
            <template #default><span class="status-dot status-dot--offline">离线</span></template>
          </el-table-column>
          <el-table-column prop="ip" label="IP 地址" width="150">
            <template #default="{ row }"><span class="ip-text muted">{{ row.ip || '-' }}</span></template>
          </el-table-column>
          <el-table-column prop="hostname" label="主机名" min-width="140">
            <template #default="{ row }">
              <span class="hostname-edit muted" @click="editHostname(row)">{{ row.hostname || '点击设置' }}</span>
            </template>
          </el-table-column>
          <el-table-column label="版本" width="96">
            <template #default="{ row }">
              <el-tag size="small" effect="plain" type="info">{{ row.agent_version || '?' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="last_heartbeat" label="最后心跳" width="160">
            <template #default="{ row }"><span class="muted">{{ formatTime(row.last_heartbeat) }}</span></template>
          </el-table-column>
          <el-table-column label="操作" width="150" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="info" link @click="showDetail(row)">详情</el-button>
              <el-button size="small" type="danger" link @click="deleteDevice(row)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-card>
    </template>

    <!-- 卡片视图 -->
    <template v-else>
      <div class="section-label">
        <span class="status-dot status-dot--online"></span>
        在线设备 · {{ onlineDevices.length }}
      </div>
      <div class="device-grid">
        <div
          v-for="d in onlineDevices"
          :key="d.id"
          class="device-card"
          :class="{ 'device-card--selected': selectedIds.includes(d.id) }"
          @click="toggleSelect(d.id)"
        >
          <div class="device-card__top">
            <span class="status-dot status-dot--online">在线</span>
            <el-tag size="small" effect="plain">{{ d.agent_version || '?' }}</el-tag>
          </div>
          <div class="device-card__ip">{{ d.ip || '-' }}</div>
          <div class="device-card__host">{{ d.hostname || '未命名设备' }}</div>
          <div class="device-card__foot">
            <el-icon><Clock /></el-icon>
            {{ formatTime(d.last_heartbeat) }}
          </div>
        </div>
        <div v-if="onlineDevices.length === 0" class="empty-state">
          <div class="empty-state__icon"><el-icon><Monitor /></el-icon></div>
          <div class="empty-state__title">暂无在线设备</div>
          <div class="empty-state__hint">部署 Agent 后设备会自动出现在这里</div>
        </div>
      </div>

      <div class="section-label">
        <span class="status-dot status-dot--offline"></span>
        离线设备 · {{ offlineDevices.length }}
      </div>
      <div class="device-grid">
        <div
          v-for="d in offlineDevices"
          :key="d.id"
          class="device-card device-card--offline"
          :class="{ 'device-card--selected': selectedIds.includes(d.id) }"
          @click="toggleSelect(d.id)"
        >
          <div class="device-card__top">
            <span class="status-dot status-dot--offline">离线</span>
            <el-tag size="small" effect="plain" type="info">{{ d.agent_version || '?' }}</el-tag>
          </div>
          <div class="device-card__ip">{{ d.ip || '-' }}</div>
          <div class="device-card__host">{{ d.hostname || '未命名设备' }}</div>
          <div class="device-card__foot">
            <el-icon><Clock /></el-icon>
            {{ formatTime(d.last_heartbeat) }}
          </div>
        </div>
        <div v-if="offlineDevices.length === 0" class="empty-state">
          <div class="empty-state__icon"><el-icon><CircleCheck /></el-icon></div>
          <div class="empty-state__title">没有离线设备</div>
          <div class="empty-state__hint">所有设备均正常运行</div>
        </div>
      </div>
    </template>

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
    <el-dialog v-model="showImport" title="批量导入设备" width="560px">
      <el-tabs v-model="importTab">
        <el-tab-pane label="CSV 文件导入" name="csv">
          <el-upload ref="uploadRef" :auto-upload="false" :limit="1" accept=".csv" :on-change="onFileChange" drag>
            <el-icon class="upload-icon"><UploadFilled /></el-icon>
            <div class="upload-text">拖拽 CSV 文件到此处，或<em>点击选择</em></div>
            <template #tip>
              <div class="upload-tip">格式：hostname,ip（首行为表头）</div>
            </template>
          </el-upload>
        </el-tab-pane>
        <el-tab-pane label="IP 段导入" name="range">
          <el-form label-width="80px" style="margin-top: 10px">
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
    <el-dialog v-model="showUpgrade" title="推送升级" width="650px">
      <el-form label-width="100px">
        <el-form-item label="服务端版本">
          <el-tag size="large" type="success">v{{ upgradeVersion }}</el-tag>
        </el-form-item>
        <el-form-item label="目标设备">
          <el-radio-group v-model="upgradeTarget">
            <el-radio value="all">所有在线设备 ({{ upgradableAllCount }})</el-radio>
            <el-radio value="selected">选中设备 ({{ upgradableSelectedCount }})</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="设备版本">
          <div class="upgrade-list">
            <div v-for="d in upgradeDeviceList" :key="d.id" class="upgrade-item">
              <div>
                <span class="upgrade-item__ip">{{ d.ip }}</span>
                <span class="upgrade-item__host">{{ d.hostname || '-' }}</span>
              </div>
              <div>
                <el-tag v-if="d.agent_version === upgradeVersion" size="small" type="info">v{{ d.agent_version || '?' }} ✓</el-tag>
                <el-tag v-else size="small" type="warning">v{{ d.agent_version || '?' }} → v{{ upgradeVersion }}</el-tag>
              </div>
            </div>
            <div v-if="upgradeDeviceList.length === 0" class="upgrade-empty">
              {{ upgradeTarget === 'selected' ? '请先在列表中勾选要升级的设备' : '没有在线设备' }}
            </div>
          </div>
        </el-form-item>
        <el-form-item v-if="alreadyLatestCount > 0">
          <div class="hint-success">{{ alreadyLatestCount }} 台设备已是最新版本，将自动跳过</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showUpgrade = false">取消</el-button>
        <el-button type="primary" :loading="upgrading" :disabled="upgradableCount === 0" @click="doUpgrade">
          推送升级 ({{ upgradableCount }}台)
        </el-button>
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

    <!-- 设备配置详情 -->
    <el-dialog v-model="showDetailDlg" title="设备配置信息" width="720px">
      <div v-if="detailDevice" class="detail-grid">
        <div class="detail-row">
          <span class="detail-label">主机名</span>
          <span class="detail-value strong">{{ detailDevice.hostname || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">IP 地址</span>
          <span class="detail-value mono">{{ detailDevice.ip || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">状态</span>
          <span class="detail-value">
            <span class="status-dot" :class="detailDevice.status === 'online' ? 'status-dot--online' : 'status-dot--offline'">
              {{ detailDevice.status === 'online' ? '在线' : '离线' }}
            </span>
          </span>
        </div>
        <div class="detail-row">
          <span class="detail-label">Agent 版本</span>
          <span class="detail-value">
            <el-tag size="small" effect="plain">{{ detailDevice.agent_version || '未知' }}</el-tag>
          </span>
        </div>
        <div class="detail-row">
          <span class="detail-label">操作系统</span>
          <span class="detail-value">{{ detailDevice.os_version || detailDevice.os_info || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">CPU</span>
          <span class="detail-value">{{ detailDevice.cpu_model || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">CPU 核心数</span>
          <span class="detail-value">{{ detailDevice.cpu_cores ?? '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">内存</span>
          <span class="detail-value">{{ detailDevice.memory_total_mb ? detailDevice.memory_total_mb + ' MB' : '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">C 盘容量</span>
          <span class="detail-value">
            <template v-if="detailDevice.disk_total_gb">
              {{ detailDevice.disk_total_gb }} GB
              <span class="muted">（剩余 {{ detailDevice.disk_free_gb ?? '?' }} GB）</span>
            </template>
            <template v-else>-</template>
          </span>
        </div>
        <div class="detail-row">
          <span class="detail-label">制造商</span>
          <span class="detail-value">{{ detailDevice.manufacturer || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">型号</span>
          <span class="detail-value">{{ detailDevice.model || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">序列号</span>
          <span class="detail-value mono">{{ detailDevice.serial_number || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">MAC 地址</span>
          <span class="detail-value mono">{{ detailDevice.mac_address || '-' }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">Sysprep 次数</span>
          <span class="detail-value">{{ detailDevice.rearm_count ?? 0 }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">最后心跳</span>
          <span class="detail-value muted">{{ formatTime(detailDevice.last_heartbeat) }}</span>
        </div>
        <div class="detail-row">
          <span class="detail-label">信息更新时间</span>
          <span class="detail-value muted">{{ formatTime(detailDevice.sysinfo_updated_at) }}</span>
        </div>
      </div>

      <div v-if="detailDevice && !detailDevice.cpu_model" class="detail-hint">
        <el-icon><InfoFilled /></el-icon>
        尚未采集到硬件信息。点击下方「采集信息」按钮，设备在线时会自动回传配置。
      </div>

      <template #footer>
        <el-button @click="showDetailDlg = false">关闭</el-button>
        <el-button
          type="primary"
          :loading="collecting"
          :disabled="detailDevice?.status !== 'online'"
          @click="collectFor(detailDevice)"
        >采集信息</el-button>
      </template>
    </el-dialog>

    <!-- 弹窗提示 -->
    <el-dialog v-model="showHostMsgDlg" title="向设备弹出提示" width="560px">
      <el-form label-width="90px">
        <el-form-item label="目标设备">
          <span class="muted">共 {{ hostMsgTargets.length }} 台在线设备</span>
        </el-form-item>
        <el-form-item label="自动关闭">
          <el-input-number v-model="hostMsgTimeout" :min="0" :max="300" />
          <span class="muted" style="margin-left: 8px">秒（0 表示不自动关闭）</span>
        </el-form-item>
        <el-form-item label="提示内容">
          <el-radio-group v-model="hostMsgMode">
            <el-radio value="device">显示该设备自身配置信息（默认）</el-radio>
            <el-radio value="custom">自定义内容</el-radio>
          </el-radio-group>
        </el-form-item>
        <template v-if="hostMsgMode === 'custom'">
          <el-form-item label="标题">
            <el-input v-model="hostMsgTitle" placeholder="设备信息提示" />
          </el-form-item>
          <el-form-item label="正文">
            <el-input v-model="hostMsgBody" type="textarea" :rows="4" placeholder="输入要显示的内容" />
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="showHostMsgDlg = false">取消</el-button>
        <el-button type="primary" :loading="hostMsging" @click="doHostMsg">确认弹窗</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { deviceApi, taskApi, deployApi, upgradeApi } from '../api'
import { formatTime } from '../utils/time'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search, Plus, Upload, FolderOpened, Top, Files, Refresh,
  CircleCheck, Clock, Grid, Menu, Monitor, Select, UploadFilled, InfoFilled } from '@element-plus/icons-vue'

const devices = ref([])
const loading = ref(false)
const search = ref('')
const viewMode = ref('table')
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

// ---- 设备详情 / 弹窗 ----
const showDetailDlg = ref(false)
const detailDevice = ref(null)
const collecting = ref(false)
const showHostMsgDlg = ref(false)
const hostMsgTargets = ref([])
const hostMsgTimeout = ref(15)
const hostMsgMode = ref('device')
const hostMsgTitle = ref('')
const hostMsgBody = ref('')
const hostMsging = ref(false)

const onlineCount = computed(() => devices.value.filter(d => d.status === 'online').length)
const onlineDevices = computed(() => devices.value.filter(d => d.status === 'online'))
const offlineDevices = computed(() => devices.value.filter(d => d.status !== 'online'))
const selectedOnlineDevices = computed(() =>
  devices.value.filter(d => selectedIds.value.includes(d.id) && d.status === 'online')
)

const upgradeDeviceList = computed(() => {
  if (upgradeTarget.value === 'selected') return selectedOnlineDevices.value
  return onlineDevices.value
})
const upgradableAllCount = computed(() => onlineDevices.value.filter(d => d.agent_version !== upgradeVersion.value).length)
const upgradableSelectedCount = computed(() => selectedOnlineDevices.value.filter(d => d.agent_version !== upgradeVersion.value).length)
const alreadyLatestCount = computed(() => upgradeDeviceList.value.filter(d => d.agent_version === upgradeVersion.value).length)
const upgradableCount = computed(() => upgradeDeviceList.value.filter(d => d.agent_version !== upgradeVersion.value).length)

const onSelectionChange = (rows) => {
  selectedIds.value = rows.map(r => r.id)
}

const clearSelection = () => {
  selectedIds.value = []
  tableRef.value?.clearSelection()
}

const toggleSelect = (id) => {
  const i = selectedIds.value.indexOf(id)
  if (i >= 0) selectedIds.value.splice(i, 1)
  else selectedIds.value.push(id)
}

const loadDevices = async () => {
  loading.value = true
  try {
    const res = await deviceApi.list({ search: search.value || undefined, size: 5000 })
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

const showDetail = (row) => {
  detailDevice.value = row
  showDetailDlg.value = true
}

// 采集单台设备信息
const collectFor = async (device) => {
  if (!device || device.status !== 'online') return
  collecting.value = true
  try {
    await taskApi.create({ task_type: 'sysinfo', device_ids: [device.id] })
    ElMessage.success('已下发采集指令，稍后刷新即可看到最新配置')
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '采集指令下发失败')
  } finally {
    collecting.value = false
  }
}

// 打开弹窗对话框（单台）
const showHostMsg = (row) => {
  hostMsgTargets.value = [row]
  resetHostMsgForm()
  showHostMsgDlg.value = true
}

// 打开弹窗对话框（批量，仅在线设备可收到）
const batchHostMsg = () => {
  const targets = selectedOnlineDevices.value
  if (targets.length === 0) {
    ElMessage.warning('请先勾选至少一台在线设备')
    return
  }
  hostMsgTargets.value = targets
  resetHostMsgForm()
  showHostMsgDlg.value = true
}

const resetHostMsgForm = () => {
  hostMsgTimeout.value = 15
  hostMsgMode.value = 'device'
  hostMsgTitle.value = ''
  hostMsgBody.value = ''
}

const doHostMsg = async () => {
  const targets = hostMsgTargets.value.filter(d => d.status === 'online')
  if (targets.length === 0) {
    ElMessage.warning('没有在线设备可接收弹窗')
    return
  }
  if (hostMsgMode.value === 'custom' && !hostMsgBody.value.trim()) {
    ElMessage.warning('请填写要显示的正文内容')
    return
  }

  hostMsging.value = true
  try {
    const params = { timeout: hostMsgTimeout.value }
    if (hostMsgMode.value === 'custom') {
      params.title = hostMsgTitle.value.trim() || '设备信息提示'
      params.message = hostMsgBody.value
    }
    await taskApi.create({
      task_type: 'hostmsg',
      device_ids: targets.map(d => d.id),
      params,
    })
    ElMessage.success(`已向 ${targets.length} 台设备下发弹窗指令`)
    showHostMsgDlg.value = false
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '弹窗指令下发失败')
  } finally {
    hostMsging.value = false
  }
}

// 批量采集设备信息
const batchSysinfo = async () => {
  const targets = selectedOnlineDevices.value
  if (targets.length === 0) {
    ElMessage.warning('请先勾选至少一台在线设备')
    return
  }
  try {
    await taskApi.create({ task_type: 'sysinfo', device_ids: targets.map(d => d.id) })
    ElMessage.success(`已向 ${targets.length} 台设备下发采集指令`)
    clearSelection()
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '采集指令下发失败')
  }
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
  clearSelection()
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
  clearSelection()
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

const openUpgrade = async () => {
  try {
    const res = await upgradeApi.latest()
    upgradeVersion.value = res.data.version
  } catch (e) {
    upgradeVersion.value = '1.4.3'
  }
  showUpgrade.value = true
}

const doUpgrade = async () => {
  const targetDevices = upgradeDeviceList.value.filter(d => d.agent_version !== upgradeVersion.value)

  if (targetDevices.length === 0) {
    ElMessage.warning('所有设备已是最新版本')
    return
  }

  await ElMessageBox.confirm(
    `确认向 ${targetDevices.length} 台设备推送升级到 v${upgradeVersion.value}？\n\n升级过程中设备会短暂断开连接。`,
    '推送升级确认',
    { type: 'warning', confirmButtonText: '确认推送', cancelButtonText: '取消' }
  )
  upgrading.value = true
  try {
    // 直接使用服务器上编译好的 exe，无需再上传
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

<style scoped>
.toolbar-card { margin-bottom: var(--space-5); }
.toolbar-card :deep(.el-card__body) { padding: var(--space-4) var(--space-5); }

.search-input { width: 240px; }
.view-switch { margin-left: var(--space-2); }

.batch-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
  margin-top: var(--space-4);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-md);
  background: linear-gradient(90deg, var(--brand-primary-soft) 0%, #f5f9ff 100%);
  border: 1px solid var(--brand-primary-light);
}
.batch-bar__info {
  display: flex;
  align-items: center;
  gap: 6px;
  color: var(--brand-primary-dark);
  font-size: 13px;
}
.batch-bar__info b { font-size: 15px; }
.batch-bar__actions { display: flex; gap: var(--space-2); }

.section-card { margin-bottom: var(--space-5); }
.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.section-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 15px;
}

.ip-text { font-weight: 600; color: var(--text-primary); font-family: ui-monospace, "SF Mono", Menlo, monospace; }
.hostname-edit {
  cursor: pointer;
  border-bottom: 1px dashed var(--border-color);
  transition: all var(--duration) var(--ease);
}
.hostname-edit:hover { color: var(--brand-primary); border-bottom-color: var(--brand-primary); }
.muted { color: var(--text-secondary); }
.row-actions { white-space: nowrap; }

/* 卡片视图 */
.section-label {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  font-weight: 600;
  color: var(--text-regular);
  margin: var(--space-5) 0 var(--space-3);
}
.device-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(230px, 1fr));
  gap: var(--space-4);
}
.device-card {
  padding: var(--space-4);
  border-radius: var(--radius-lg);
  background: var(--bg-card);
  border: 1px solid var(--border-color);
  box-shadow: var(--shadow-xs);
  cursor: pointer;
  transition: all var(--duration) var(--ease);
}
.device-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--shadow-md);
  border-color: var(--brand-primary-light);
}
.device-card--selected {
  border-color: var(--brand-primary);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.12);
}
.device-card--offline { background: #fafbfc; }
.device-card__top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-3);
}
.device-card__ip {
  font-size: 18px;
  font-weight: 700;
  color: var(--text-primary);
  font-family: ui-monospace, "SF Mono", Menlo, monospace;
  letter-spacing: -0.01em;
}
.device-card__host {
  font-size: 13px;
  color: var(--text-secondary);
  margin-top: 2px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.device-card__foot {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-top: var(--space-3);
  padding-top: var(--space-3);
  border-top: 1px solid var(--border-color-light);
  font-size: 12px;
  color: var(--text-placeholder);
}

/* 上传 */
.upload-icon { font-size: 40px; color: var(--text-placeholder); margin-bottom: 8px; }
.upload-text { font-size: 13px; color: var(--text-secondary); }
.upload-text em { color: var(--brand-primary); font-style: normal; }
.upload-tip { font-size: 12px; color: var(--text-placeholder); }

/* 升级列表 */
.upgrade-list {
  max-height: 250px;
  overflow-y: auto;
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  padding: var(--space-2);
  width: 100%;
}
.upgrade-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 7px var(--space-2);
  border-radius: var(--radius-sm);
  font-size: 13px;
}
.upgrade-item:hover { background: var(--brand-primary-soft); }
.upgrade-item__ip { font-weight: 600; color: var(--text-primary); }
.upgrade-item__host { color: var(--text-secondary); margin-left: 8px; }
.upgrade-empty {
  color: var(--text-placeholder);
  font-size: 12px;
  text-align: center;
  padding: var(--space-4);
}
.hint-success { color: var(--color-success); font-size: 12px; }

/* 设备详情 */
.detail-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  background: var(--border-color);
  border: 1px solid var(--border-color);
  border-radius: var(--radius-md);
  overflow: hidden;
}
.detail-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 10px var(--space-4);
  background: var(--bg-card);
}
.detail-label {
  width: 92px;
  flex-shrink: 0;
  font-size: 13px;
  color: var(--text-secondary);
}
.detail-value {
  flex: 1;
  min-width: 0;
  font-size: 13px;
  color: var(--text-primary);
  word-break: break-all;
}
.detail-value.strong { font-weight: 600; }
.detail-value.mono { font-family: ui-monospace, "SF Mono", Menlo, monospace; }
.detail-hint {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: var(--space-4);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--color-warning-soft);
  color: #92400e;
  font-size: 12px;
}

@media (max-width: 720px) {
  .detail-grid { grid-template-columns: 1fr; }
}
</style>