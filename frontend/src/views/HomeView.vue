<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { currentUser, loadSession, request, message } from '../api'
import { isDesktop } from '../desktop'
import { organizerParticipant } from '../participants'
import type { Friend, Meeting, TransferProfile } from '../types'

const router = useRouter()
const loading = ref(true), busy = ref(false), error = ref(''), notice = ref('')
const email = ref(''), password = ref(''), title = ref(''), date = ref(new Date().toLocaleDateString('en-CA'))
const list = ref<{ id: string; title: string; date: string; state: string }[]>([])
const mainSection = ref<'meetings' | 'friends' | 'profile'>('meetings')
const profile = ref<TransferProfile>({ name: '', phone: '', bank: '' })
const friends = ref<Friend[]>([])
const friendForm = ref<Friend>({ id: '', name: '', phone: '', birthday: '' })
const stateNames: Record<string, string> = { draft: 'Черновик', finalized: 'Зафиксировано', closed: 'Закрыто' }

async function refresh() {
  error.value = ''; loading.value = true
  try {
    await loadSession()
    if (currentUser.value) {
      list.value = await request('/api/meetings')
      if (isDesktop) {
        profile.value = await request<TransferProfile>('/api/profile')
        friends.value = await request<Friend[]>('/api/friends')
      }
    }
  } catch (e) { error.value = message(e) }
  finally { loading.value = false }
}
async function login(register: boolean) {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    await request(register ? '/api/auth/register' : '/api/auth/login', 'POST', { email: email.value, password: password.value })
    password.value = ''; await refresh()
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
async function create() {
  if (busy.value) return
  busy.value = true; error.value = ''
  try {
    if (isDesktop && !profile.value.name.trim()) {
      mainSection.value = 'profile'
      throw new Error('Сначала укажите своё имя в разделе «Я». Организатор автоматически оплачивает чек.')
    }
    const organizer = isDesktop ? [organizerParticipant(profile.value.name)] : []
    const meeting = await request<Meeting>('/api/meetings', 'POST', {
      title: title.value, description: '', date: date.value, venue: '',
      bill: { participants: organizer, payer_id: isDesktop ? 'me' : '', receipt_total: null, items: [] },
    })
    await router.push('/meetings/' + meeting.id)
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
function editFriend(friend: Friend) { friendForm.value = { ...friend }; mainSection.value = 'friends' }
async function saveFriend() {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    const saved = await request<Friend>('/api/friends', 'POST', friendForm.value)
    const index = friends.value.findIndex(friend => friend.id === saved.id)
    if (index === -1) friends.value.push(saved)
    else friends.value[index] = saved
    friends.value.sort((left, right) => left.name.localeCompare(right.name, 'ru'))
    friendForm.value = { id: '', name: '', phone: '', birthday: '' }
    notice.value = 'Друг сохранён и доступен при добавлении участников.'
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
async function deleteFriend(friend: Friend) {
  if (busy.value || !window.confirm(`Удалить ${friend.name} из списка друзей? В существующих встречах участник останется.`)) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    await request('/api/friends/' + encodeURIComponent(friend.id), 'DELETE')
    friends.value = friends.value.filter(item => item.id !== friend.id)
    if (friendForm.value.id === friend.id) friendForm.value = { id: '', name: '', phone: '', birthday: '' }
    notice.value = 'Друг удалён из справочника.'
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
function birthdayLabel(value: string) {
  if (!value) return 'День рождения не указан'
  return new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long' }).format(new Date(value + 'T12:00:00'))
}
async function saveProfile() {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try {
    profile.value = await request<TransferProfile>('/api/profile', 'PUT', profile.value)
    notice.value = 'Профиль сохранён. Вы автоматически станете плательщиком в новых встречах.'
  } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
async function logout() {
  busy.value = true
  try { await request('/api/auth/logout', 'POST'); await refresh() }
  catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
onMounted(refresh)
</script>

<template>
  <p
    v-if="loading"
    role="status"
    class="loading-card"
  >
    Загрузка…
  </p>
  <div
    v-if="error"
    class="error"
    role="alert"
  >
    {{ error }} <button
      type="button"
      @click="refresh"
    >
      Повторить загрузку
    </button>
  </div>
  <p
    v-if="notice"
    class="notice"
    role="status"
  >
    {{ notice }}
  </p>
  <template v-if="!loading">
    <section
      v-if="!currentUser"
      class="card narrow"
    >
      <h1>Разделим счёт</h1><p>Войдите, чтобы сохранить встречу. Участникам аккаунт не нужен.</p>
      <form @submit.prevent="login(false)">
        <fieldset :disabled="busy">
          <label for="email">Email</label><input
            id="email"
            v-model="email"
            type="email"
            autocomplete="username"
            required
            maxlength="254"
          >
          <label for="password">Пароль</label><input
            id="password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            required
            minlength="12"
            aria-describedby="password-hint"
          >
          <small id="password-hint">От 12 символов, не более 128 байт.</small>
          <div class="actions">
            <button type="submit">
              {{ busy ? 'Подождите…' : 'Войти' }}
            </button><button
              type="button"
              class="secondary"
              @click="login(true)"
            >
              Создать аккаунт
            </button>
          </div>
        </fieldset>
      </form>
    </section>
    <template v-else>
      <div class="home-heading">
        <div>
          <p class="spray-label">
            Локально на вашем компьютере
          </p><h1>{{ mainSection === 'meetings' ? 'Мои встречи' : mainSection === 'friends' ? 'Друзья' : 'Я' }}</h1>
        </div><button
          v-if="!isDesktop"
          class="secondary"
          :disabled="busy"
          @click="logout"
        >
          Выйти
        </button>
      </div>
      <nav
        v-if="isDesktop"
        class="main-tabs"
        aria-label="Главное меню"
      >
        <button
          type="button"
          :aria-pressed="mainSection === 'meetings'"
          @click="mainSection = 'meetings'"
        >
          Встречи
        </button>
        <button
          type="button"
          :aria-pressed="mainSection === 'friends'"
          @click="mainSection = 'friends'"
        >
          Друзья
        </button>
        <button
          type="button"
          :aria-pressed="mainSection === 'profile'"
          @click="mainSection = 'profile'"
        >
          Я
        </button>
      </nav>

      <section v-show="mainSection === 'meetings'">
        <form
          class="card new-meeting-card"
          @submit.prevent="create"
        >
          <fieldset :disabled="busy">
            <legend>Новая встреча</legend><p
              v-if="isDesktop"
              class="hint"
            >
              Вы автоматически станете участником и плательщиком.
            </p>
            <div class="columns">
              <div>
                <label for="title">Название</label><input
                  id="title"
                  v-model="title"
                  required
                  maxlength="120"
                  placeholder="Ужин с друзьями"
                >
              </div><div>
                <label for="date">Дата</label><input
                  id="date"
                  v-model="date"
                  type="date"
                  required
                >
              </div>
            </div>
            <button type="submit">
              {{ busy ? 'Создаём…' : 'Создать встречу' }}
            </button>
          </fieldset>
        </form>
        <p
          v-if="list.length === 0"
          class="empty-state"
        >
          Пока нет встреч. Создайте первую и добавьте участников.
        </p>
        <ul class="meeting-list">
          <li
            v-for="meeting in list"
            :key="meeting.id"
            class="card"
          >
            <RouterLink :to="'/meetings/' + meeting.id">
              {{ meeting.title }}
            </RouterLink><p>{{ meeting.date }} · {{ stateNames[meeting.state] }}</p>
          </li>
        </ul>
      </section>

      <section
        v-if="isDesktop"
        v-show="mainSection === 'friends'"
        class="friends-layout"
      >
        <form
          class="card friend-form"
          @submit.prevent="saveFriend"
        >
          <fieldset :disabled="busy">
            <p class="eyebrow">
              Люди рядом
            </p><h2>{{ friendForm.id ? 'Изменить друга' : 'Новый друг' }}</h2>
            <label for="friend-name">Имя</label><input
              id="friend-name"
              v-model="friendForm.name"
              required
              maxlength="120"
              placeholder="Дима"
            >
            <label for="friend-phone">Номер телефона</label><input
              id="friend-phone"
              v-model="friendForm.phone"
              type="tel"
              autocomplete="tel"
              required
              maxlength="40"
              placeholder="+7 999 123-45-67"
            >
            <label for="friend-birthday">День рождения</label><input
              id="friend-birthday"
              v-model="friendForm.birthday"
              type="date"
            >
            <div class="actions">
              <button type="submit">
                {{ busy ? 'Сохраняем…' : 'Сохранить друга' }}
              </button><button
                v-if="friendForm.id"
                type="button"
                class="secondary"
                @click="friendForm = { id: '', name: '', phone: '', birthday: '' }"
              >
                Отмена
              </button>
            </div>
          </fieldset>
        </form>
        <div class="friend-list">
          <p
            v-if="friends.length === 0"
            class="empty-state"
          >
            Список пока пуст. Сохранённые друзья появятся здесь и в форме встречи.
          </p>
          <article
            v-for="friend in friends"
            :key="friend.id"
            class="card friend-card"
          >
            <div class="friend-avatar">
              {{ friend.name.slice(0, 1).toUpperCase() }}
            </div>
            <div><h3>{{ friend.name }}</h3><p>{{ friend.phone }}</p><small>{{ birthdayLabel(friend.birthday) }}</small></div>
            <div class="friend-actions">
              <button
                type="button"
                class="mini-button"
                @click="editFriend(friend)"
              >
                Изменить
              </button><button
                type="button"
                class="mini-button danger-button"
                @click="deleteFriend(friend)"
              >
                Удалить
              </button>
            </div>
          </article>
        </div>
      </section>

      <section
        v-if="isDesktop"
        v-show="mainSection === 'profile'"
        class="profile-layout"
      >
        <form
          class="card profile-card"
          @submit.prevent="saveProfile"
        >
          <fieldset :disabled="busy">
            <p class="eyebrow">
              Организатор и плательщик
            </p><h2>Мой профиль</h2><p>Вы автоматически добавляетесь в новые встречи и оплачиваете общий чек.</p>
            <label for="profile-name">Ваше имя</label><input
              id="profile-name"
              v-model="profile.name"
              autocomplete="name"
              required
              maxlength="120"
              placeholder="Женя"
            >
            <label for="profile-phone">Номер телефона</label><input
              id="profile-phone"
              v-model="profile.phone"
              type="tel"
              autocomplete="tel"
              required
              maxlength="40"
              placeholder="+7 999 123-45-67"
            >
            <label for="profile-bank">Название банка</label><input
              id="profile-bank"
              v-model="profile.bank"
              required
              maxlength="120"
              placeholder="Т-Банк"
            >
            <button type="submit">
              {{ busy ? 'Сохраняем…' : 'Сохранить данные' }}
            </button>
          </fieldset>
        </form>
        <aside class="profile-note">
          <span>Лично</span><h3>Данные остаются на этом компьютере</h3><p>PoDolyam хранит имя, телефон и банк в локальной SQLite-базе. При экспорте реквизиты добавляются только в выбранный PDF-файл.</p>
        </aside>
      </section>
    </template>
  </template>
</template>
