<template>
  <el-container v-if="$route.path !== '/login'" style="height: 100vh">
    <el-aside width="220px" style="background: #304156">
      <div style="padding: 20px; color: #fff; font-size: 18px; font-weight: bold; text-align: center">
        LAN Agent 管理端
      </div>
      <div style="text-align: center; color: #bfcbd9; font-size: 12px; margin-bottom: 10px">v1.2.0</div>
      <el-menu
        :default-active="$route.path"
        router
        background-color="#304156"
        text-color="#bfcbd9"
        active-text-color="#409eff"
      >
        <el-menu-item index="/">
          <span>仪表盘</span>
        </el-menu-item>
        <el-menu-item index="/devices">
          <span>设备管理</span>
        </el-menu-item>
        <el-menu-item index="/tasks">
          <span>任务中心</span>
        </el-menu-item>
        <el-menu-item index="/audit">
          <span>审计日志</span>
        </el-menu-item>
        <el-menu-item index="/help">
          <span>使用帮助</span>
        </el-menu-item>
      </el-menu>
      <div style="position: absolute; bottom: 20px; width: 220px; text-align: center; display: flex; gap: 8px; justify-content: center">
        <el-button type="primary" size="small" @click="showPwd = true">修改密码</el-button>
        <el-button type="danger" size="small" @click="logout">退出登录</el-button>
      </div>
    </el-aside>
    <el-main style="background: #f0f2f5; padding: 20px">
      <router-view />
    </el-main>
  </el-container>
  <router-view v-else />

  <!-- 修改密码对话框 -->
  <el-dialog v-model="showPwd" title="修改密码" width="400px">
    <el-form :model="pwdForm" label-width="80px">
      <el-form-item label="原密码"><el-input v-model="pwdForm.old_password" type="password" show-password /></el-form-item>
      <el-form-item label="新密码"><el-input v-model="pwdForm.new_password" type="password" show-password /></el-form-item>
      <el-form-item label="确认密码"><el-input v-model="pwdForm.confirm" type="password" show-password /></el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="showPwd = false">取消</el-button>
      <el-button type="primary" :loading="pwdLoading" @click="changePassword">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { authApi } from './api'
import { ElMessage } from 'element-plus'

const router = useRouter()
const showPwd = ref(false)
const pwdLoading = ref(false)
const pwdForm = ref({ old_password: '', new_password: '', confirm: '' })

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

<style>
body { margin: 0; }
.el-menu { border-right: none; }
</style>
