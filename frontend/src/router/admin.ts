import { createRouter, createWebHistory, RouteRecordRaw } from 'vue-router'
import HomeView from '../views/admin/HomeView.vue'
import AdminView from '../views/admin/AdminView.vue'

const routes: Array<RouteRecordRaw> = [
  {
    path: '/',
    name: 'home',
    component: HomeView,
  },
  {
    path: '/admin/',
    name: 'admin',
    component: AdminView,
  },
  {
    path: '/:catchall(.*)',
    name: 'notfound',
    redirect: '/',
  },
]

const router = createRouter({
  history: createWebHistory(process.env.BASE_URL),
  routes,
})

export default router
