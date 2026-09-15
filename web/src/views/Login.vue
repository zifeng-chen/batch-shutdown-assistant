<template>
  <div style="display: flex; justify-content: center; align-items: center; height: 100vh; background: #2d3a4b">
    <el-card style="width: 400px" shadow="always">
      <template #header>
        <h2 style="text-align: center; margin: 0">LAN Agent 管理端</h2>
      </template>
      <el-form :model="form" @submit.prevent="handleLogin" label-position="top">
        <el-form-item label="用户名">
          <el-input v-model="form.username" placeholder="admin" autofocus />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="form.password" type="password" show-password placeholder="admin123" @keyup.enter="handleLogin" />
        </el-form-item>
        <el-button type="primary" style="width: 100%" :loading="loading" @click="handleLogin">登 录</el-button>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { authApi } from '../api'
import { ElMessage } from 'element-plus'

const router = useRouter()
const loading = ref(false)
const form = ref({ username: '', password: '' })

const handleLogin = async () => {
  if (!form.value.username || !form.value.password) return
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
