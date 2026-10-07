<template>
  <el-config-provider :locale="zhCn">
    <el-container v-if="$route.path !== '/login'" class="app-shell">
    <!-- 侧边栏 -->
    <aside class="sidebar" :class="{ 'sidebar--collapsed': collapsed }">
      <div class="sidebar__brand">
        <div class="brand-logo">
          <el-icon><Monitor /></el-icon>
        </div>
        <transition name="fade">
          <div v-if="!collapsed" class="brand-text">
            <div class="brand-title">LAN Agent</div>
            <div class="brand-sub">局域网设备管理端</div>
          </div>
        </transition>
      </div>

      <nav class="sidebar__nav">
        <router-link
          v-for="item in menus"
          :key="item.path"
          :to="item.path"
          class="nav-item"
          :class="{ 'nav-item--active': isActive(item.path) }"
        >
          <el-icon class="nav-item__icon"><component :is="item.icon" /></el-icon>
          <span v-if="!collapsed" class="nav-item__label">{{ item.label }}</span>
          <span v-if="!collapsed && item.badge" class="nav-item__badge">{{ item.badge }}</span>
        </router-link>
      </nav>

      <div class="sidebar__foot">
        <button class="foot-btn" @click="showPwd = true" :title="collapsed ? '修改密码' : ''">
          <el-icon><Lock /></el-icon>
          <span v-if="!collapsed">修改密码</span>
        </button>
        <button class="foot-btn foot-btn--danger" @click="logout" :title="collapsed ? '退出登录' : ''">
          <el-icon><SwitchButton /></el-icon>
          <span v-if="!collapsed">退出登录</span>
        </button>
        <div v-if="!collapsed" class="sidebar__version">v{{ appVersion }}</div>
      </div>
    </aside>

    <!-- 主区域 -->
    <div class="main-area">
      <header class="topbar">
        <button class="collapse-btn" @click="collapsed = !collapsed">
          <el-icon><Fold v-if="!collapsed" /><Expand v-else /></el-icon>
        </button>
        <div class="topbar__crumb">
          <el-icon class="crumb-icon"><component :is="currentMenu.icon" /></el-icon>
          <span class="crumb-current">{{ currentMenu.label }}</span>
          <span class="crumb-desc">{{ currentMenu.desc }}</span>
        </div>
        <div class="topbar__right">
          <span class="conn-badge">
            <span class="conn-badge__dot"></span>
            服务运行中
          </span>
        </div>
      </header>

      <main class="content">
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </main>
    </div>
  </el-container>

  <router-view v-else />

  <!-- 修改密码 -->
  <el-dialog v-model="showPwd" title="修改密码" width="420px">
    <el-form :model="pwdForm" label-width="80px">
      <el-form-item label="原密码">
        <el-input v-model="pwdForm.old_password" type="password" show-password />
      </el-form-item>
      <el-form-item label="新密码">
        <el-input v-model="pwdForm.new_password" type="password" show-password placeholder="至少 8 位，需含字母和数字" />
      </el-form-item>
      <el-form-item label="确认密码">
        <el-input v-model="pwdForm.confirm" type="password" show-password />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showPwd = false">取消</el-button>
      <el-button type="primary" :loading="pwdLoading" @click="changePassword">确定</el-button>
    </template>
  </el-dialog>
  </el-config-provider>
</template>

<script setup>
import { ref, computed, onMounted, markRaw } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { authApi, upgradeApi } from './api'
import { ElMessage } from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import {
  Monitor, Lock, SwitchButton, Fold, Expand,
  Odometer, Promotion, Document, QuestionFilled,
} from '@element-plus/icons-vue'

const router = useRouter()
const route = useRoute()

const showPwd = ref(false)
const pwdLoading = ref(false)
const pwdForm = ref({ old_password: '', new_password: '', confirm: '' })
const appVersion = ref('…')
const collapsed = ref(false)

const menus = markRaw([
  { path: '/', icon: Odometer, label: '仪表盘', desc: '设备概览与运行状态' },
  { path: '/devices', icon: Monitor, label: '设备管理', desc: '设备清单与批量操作' },
  { path: '/tasks', icon: Promotion, label: '任务中心', desc: '批量任务下发与追踪' },
  { path: '/audit', icon: Document, label: '审计日志', desc: '操作记录与安全审计' },
  { path: '/help', icon: QuestionFilled, label: '使用帮助', desc: '快速上手与常见问题' },
])

const currentMenu = computed(
  () => menus.find(m => isActive(m.path)) || menus[0]
)

const isActive = (path) =>
  path === '/' ? route.path === '/' : route.path.startsWith(path)

onMounted(async () => {
  try {
    const res = await upgradeApi.latest()
    appVersion.value = res.data.version || '…'
  } catch {
    appVersion.value = '?'
  }
})

const logout = () => {
  localStorage.removeItem('token')
  router.push('/login')
}

const changePassword = async () => {
  if (!pwdForm.value.old_password || !pwdForm.value.new_password) {
    ElMessage.warning('请填写完整')
    return
  }
  if (pwdForm.value.new_password !== pwdForm.value.confirm) {
    ElMessage.warning('两次密码不一致')
    return
  }
  pwdLoading.value = true
  try {
    await authApi.changePassword(pwdForm.value.old_password, pwdForm.value.new_password)
    ElMessage.success('密码修改成功，请重新登录')
    showPwd.value = false
    pwdForm.value = { old_password: '', new_password: '', confirm: '' }
    logout()
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '修改失败')
  } finally {
    pwdLoading.value = false
  }
}
</script>

<style scoped>
.app-shell {
  height: 100vh;
  background: var(--bg-page);
}

/* ---------- 侧边栏 ---------- */
.sidebar {
  width: var(--sidebar-width);
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  background: var(--bg-sidebar);
  background-image: linear-gradient(180deg, #0f172a 0%, #131e33 100%);
  transition: width var(--duration) var(--ease);
  position: relative;
}
.sidebar--collapsed { width: 72px; }

.sidebar__brand {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-5) var(--space-5);
  min-height: var(--header-height);
}
.brand-logo {
  width: 38px;
  height: 38px;
  flex-shrink: 0;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--brand-primary) 0%, var(--brand-accent) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  font-size: 20px;
  box-shadow: var(--shadow-brand);
}
.brand-title {
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  letter-spacing: 0.01em;
  white-space: nowrap;
}
.brand-sub {
  color: #64748b;
  font-size: 11px;
  white-space: nowrap;
  margin-top: 2px;
}

.sidebar__nav {
  flex: 1;
  padding: var(--space-3) var(--space-3);
  display: flex;
  flex-direction: column;
  gap: 4px;
  overflow-y: auto;
}
.nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: 11px var(--space-3);
  border-radius: var(--radius-md);
  color: #94a3b8;
  text-decoration: none;
  font-size: 14px;
  font-weight: 500;
  transition: all var(--duration) var(--ease);
  white-space: nowrap;
}
.nav-item:hover {
  background: var(--bg-sidebar-hover);
  color: #e2e8f0;
}
.nav-item--active {
  background: linear-gradient(90deg, rgba(37, 99, 235, 0.9) 0%, rgba(29, 78, 216, 0.75) 100%);
  color: #fff;
  box-shadow: 0 4px 14px rgba(37, 99, 235, 0.35);
}
.nav-item--active::before {
  content: '';
  position: absolute;
  left: -12px;
  top: 50%;
  transform: translateY(-50%);
  width: 3px;
  height: 20px;
  border-radius: var(--radius-full);
  background: var(--brand-accent);
}
.nav-item__icon { font-size: 18px; flex-shrink: 0; }
.nav-item__label { flex: 1; }
.nav-item__badge {
  font-size: 11px;
  padding: 1px 7px;
  border-radius: var(--radius-full);
  background: rgba(255, 255, 255, 0.15);
}

.sidebar__foot {
  padding: var(--space-3);
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.foot-btn {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  padding: 10px var(--space-3);
  border: none;
  border-radius: var(--radius-md);
  background: transparent;
  color: #94a3b8;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all var(--duration) var(--ease);
  font-family: inherit;
}
.foot-btn:hover { background: var(--bg-sidebar-hover); color: #e2e8f0; }
.foot-btn--danger:hover { background: rgba(239, 68, 68, 0.15); color: #fca5a5; }
.sidebar__version {
  text-align: center;
  color: #475569;
  font-size: 11px;
  padding-top: var(--space-2);
}

/* ---------- 主区域 ---------- */
.main-area {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}
.topbar {
  height: var(--header-height);
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: 0 var(--space-6);
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--border-color);
  position: sticky;
  top: 0;
  z-index: 10;
}
.collapse-btn {
  width: 34px;
  height: 34px;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  font-size: 18px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: all var(--duration) var(--ease);
}
.collapse-btn:hover { background: var(--brand-primary-soft); color: var(--brand-primary); }

.topbar__crumb { display: flex; align-items: baseline; gap: var(--space-3); }
.crumb-icon { color: var(--brand-primary); font-size: 17px; align-self: center; }
.crumb-current {
  font-size: 16px;
  font-weight: 700;
  color: var(--text-primary);
}
.crumb-desc { font-size: 13px; color: var(--text-placeholder); }

.topbar__right { margin-left: auto; }
.conn-badge {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-radius: var(--radius-full);
  background: var(--color-success-soft);
  color: #047857;
  font-size: 12px;
  font-weight: 600;
}
.conn-badge__dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-success);
  animation: pulse-ring 2s var(--ease) infinite;
}

.content {
  flex: 1;
  padding: var(--space-6);
  overflow-y: auto;
}

.fade-enter-active, .fade-leave-active { transition: opacity 0.2s; }
.fade-enter-from, .fade-leave-to { opacity: 0; }

@media (max-width: 768px) {
  .crumb-desc, .topbar__right { display: none; }
}
</style>