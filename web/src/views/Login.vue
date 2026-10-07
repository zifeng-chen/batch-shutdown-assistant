<template>
  <div class="login-page">
    <!-- 左侧品牌区 -->
    <div class="login-brand">
      <div class="brand-glow brand-glow--1"></div>
      <div class="brand-glow brand-glow--2"></div>

      <div class="brand-content">
        <div class="brand-logo">
          <el-icon><Monitor /></el-icon>
        </div>
        <h1 class="brand-title">LAN Agent</h1>
        <p class="brand-sub">局域网设备批量管理平台</p>

        <ul class="feature-list">
          <li v-for="f in features" :key="f.title">
            <div class="feature-icon"><el-icon><component :is="f.icon" /></el-icon></div>
            <div>
              <div class="feature-title">{{ f.title }}</div>
              <div class="feature-desc">{{ f.desc }}</div>
            </div>
          </li>
        </ul>
      </div>

      <div class="brand-foot">© 2026 LAN Agent · 企业内网设备运维方案</div>
    </div>

    <!-- 右侧登录表单 -->
    <div class="login-form-area">
      <div class="form-box">
        <h2 class="form-title">欢迎回来</h2>
        <p class="form-sub">请登录以继续管理您的设备</p>

        <el-form :model="form" @submit.prevent="handleLogin" size="large" class="login-form">
          <el-form-item>
            <el-input
              v-model="form.username"
              placeholder="用户名"
              autofocus
              :prefix-icon="User"
            />
          </el-form-item>
          <el-form-item>
            <el-input
              v-model="form.password"
              type="password"
              show-password
              placeholder="密码"
              :prefix-icon="Lock"
              @keyup.enter="handleLogin"
            />
          </el-form-item>

          <el-button
            type="primary"
            class="submit-btn"
            :loading="loading"
            @click="handleLogin"
          >
            {{ loading ? '登录中…' : '登 录' }}
          </el-button>
        </el-form>

        <div class="form-tip">
          <el-icon><InfoFilled /></el-icon>
          首次使用请用默认账号 admin / admin123 登录，并尽快修改密码
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, markRaw } from 'vue'
import { useRouter } from 'vue-router'
import { authApi } from '../api'
import { ElMessage } from 'element-plus'
import { User, Lock, Monitor, Promotion, Document, InfoFilled } from '@element-plus/icons-vue'

const router = useRouter()
const loading = ref(false)
const form = ref({ username: '', password: '' })

const features = markRaw([
  { icon: Monitor, title: '设备集中管理', desc: '一键导入、分组与状态监控' },
  { icon: Promotion, title: '批量指令下发', desc: '重启、锁屏、关机、升级' },
  { icon: Document, title: '全量操作审计', desc: '每一步操作都有迹可循' },
])

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) {
    ElMessage.warning('请输入用户名和密码')
    return
  }
  loading.value = true
  try {
    const res = await authApi.login(form.value)
    localStorage.setItem('token', res.data.access_token)
    ElMessage.success('登录成功')
    router.push('/')
  } catch (e) {
    ElMessage.error(e.response?.data?.detail || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-page {
  display: flex;
  height: 100vh;
  overflow: hidden;
}

/* ---------- 左侧品牌区 ---------- */
.login-brand {
  position: relative;
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: center;
  padding: var(--space-10) 8%;
  background: linear-gradient(145deg, #0f172a 0%, #15213b 55%, #1e3a8a 100%);
  color: #fff;
  overflow: hidden;
}
.brand-glow {
  position: absolute;
  border-radius: 50%;
  filter: blur(90px);
  opacity: 0.5;
}
.brand-glow--1 {
  width: 420px; height: 420px;
  background: var(--brand-primary);
  top: -120px; right: -80px;
}
.brand-glow--2 {
  width: 340px; height: 340px;
  background: var(--brand-accent);
  bottom: -100px; left: -60px;
  opacity: 0.35;
}
.brand-content { position: relative; z-index: 1; max-width: 460px; }

.brand-logo {
  width: 56px;
  height: 56px;
  border-radius: var(--radius-lg);
  background: linear-gradient(135deg, var(--brand-primary) 0%, var(--brand-accent) 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 28px;
  box-shadow: var(--shadow-brand);
  margin-bottom: var(--space-6);
}
.brand-title {
  font-size: 34px;
  font-weight: 800;
  margin: 0 0 var(--space-2);
  letter-spacing: -0.02em;
}
.brand-sub {
  font-size: 15px;
  color: #94a3b8;
  margin: 0 0 var(--space-10);
}

.feature-list {
  list-style: none;
  padding: 0;
  margin: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}
.feature-list li {
  display: flex;
  gap: var(--space-4);
  align-items: flex-start;
}
.feature-icon {
  width: 40px;
  height: 40px;
  flex-shrink: 0;
  border-radius: var(--radius-md);
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.1);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 19px;
  color: var(--brand-accent);
}
.feature-title { font-weight: 600; font-size: 15px; margin-bottom: 2px; }
.feature-desc { font-size: 13px; color: #94a3b8; }

.brand-foot {
  position: absolute;
  bottom: var(--space-6);
  left: 8%;
  font-size: 12px;
  color: #475569;
  z-index: 1;
}

/* ---------- 右侧表单区 ---------- */
.login-form-area {
  width: 480px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-8);
  background: var(--bg-card);
}
.form-box { width: 100%; max-width: 360px; }
.form-title {
  font-size: 26px;
  font-weight: 700;
  color: var(--text-primary);
  margin: 0 0 var(--space-2);
  letter-spacing: -0.01em;
}
.form-sub {
  font-size: 14px;
  color: var(--text-secondary);
  margin: 0 0 var(--space-8);
}
.login-form :deep(.el-form-item) { margin-bottom: var(--space-5); }
.submit-btn {
  width: 100%;
  height: 44px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.05em;
  margin-top: var(--space-2);
}
.form-tip {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-top: var(--space-6);
  padding: var(--space-3) var(--space-4);
  border-radius: var(--radius-sm);
  background: var(--brand-primary-soft);
  color: var(--brand-primary-dark);
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 900px) {
  .login-brand { display: none; }
  .login-form-area { width: 100%; }
}
</style>