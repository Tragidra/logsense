import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/', redirect: '/dashboard' },
  {
    path: '/dashboard',
    name: 'dashboard',
    component: () => import('./views/dashboard-view.vue'),
  },
  {
    path: '/clusters/:id',
    name: 'cluster',
    component: () => import('./views/cluster-view.vue'),
    props: true,
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

export default router
