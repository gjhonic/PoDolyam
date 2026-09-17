import { createRouter, createWebHashHistory } from 'vue-router'
import HomeView from './views/HomeView.vue'
import NotFoundView from './views/NotFoundView.vue'
import MeetingView from './views/MeetingView.vue'
import FriendView from './views/FriendView.vue'

export const routes = [
  { path: '/', component: HomeView },
  { path: '/meetings/:id', component: MeetingView },
  { path: '/friends/:id', component: FriendView },
  { path: '/:pathMatch(.*)*', component: NotFoundView },
]

export const router = createRouter({
  history: createWebHashHistory(),
  routes,
})
