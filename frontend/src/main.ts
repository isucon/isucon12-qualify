import { createApp } from 'vue'
import AdminApp from './AdminApp.vue'
import TenantApp from './TenantApp.vue'
import adminRouter from './router/admin'
import tenantRouter from './router/tenant'

import '@/assets/style/isuports.css'

const host = location.hostname

if (host.split('.')[0] === 'admin') {
  createApp(AdminApp).use(adminRouter).mount("#app")
} else {
  createApp(TenantApp).use(tenantRouter).mount("#app")
}
