import { isDesktop } from './desktop'
import { createRouter, createWebHistory, createWebHashHistory } from 'vue-router'
import HomeView from './views/HomeView.vue'
import NotFoundView from './views/NotFoundView.vue'
import MeetingView from './views/MeetingView.vue'
import SharedView from './views/SharedView.vue'

export const routes = [
  { path: '/', component: HomeView },
  { path: '/meetings/:id', component: MeetingView },
  { path: '/s/:token', component: SharedView },
  { path: '/:pathMatch(.*)*', component: NotFoundView },
]

export const router = createRouter({
  history: isDesktop ? createWebHashHistory() : createWebHistory(),
  routes,
})
