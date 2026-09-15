import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', name: 'Login', component: () => import('../views/Login.vue') },
  { path: '/', name: 'Dashboard', component: () => import('../views/Dashboard.vue'), meta: { auth: true } },
  { path: '/devices', name: 'Devices', component: () => import('../views/Devices.vue'), meta: { auth: true } },
  { path: '/tasks', name: 'Tasks', component: () => import('../views/Tasks.vue'), meta: { auth: true } },
  { path: '/audit', name: 'Audit', component: () => import('../views/Audit.vue'), meta: { auth: true } },
  { path: '/help', name: 'Help', component: () => import('../views/Help.vue'), meta: { auth: true } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  if (to.meta.auth && !localStorage.getItem('token')) {
    return '/login'
  }
})

export default router
