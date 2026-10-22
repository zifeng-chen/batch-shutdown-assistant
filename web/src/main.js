import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'element-plus/dist/index.css'

import './styles/design-system.css'
import App from './App.vue'
import router from './router'

// Element Plus 组件由 unplugin-vue-components 按需自动引入，
// 但按需模式不会为 ElMessageBox / ElNotification 等函数式调用的组件注入样式，
// 故此处统一引入完整样式表，确保弹窗遮罩、居中定位与主题变量齐全。
const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')