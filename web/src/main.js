import { createApp } from 'vue'
import { createPinia } from 'pinia'

import './styles/design-system.css'
import App from './App.vue'
import router from './router'

// Element Plus 组件与样式由 unplugin-vue-components 按需自动引入，
// 中文 locale 通过 App.vue 的 <el-config-provider> 局部导入。
const app = createApp(App)
app.use(createPinia())
app.use(router)
app.mount('#app')