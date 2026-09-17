<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { message, request } from '../api'
import { rubles } from '../money'
import type { Friend, FriendDetails, FriendMeeting } from '../types'

const route = useRoute()
const id = String(route.params.id)
const loading = ref(true), saving = ref(false), error = ref(''), notice = ref('')
const details = ref<FriendDetails | null>(null)
const form = ref<Friend>({ id, name: '', phone: '', birthday: '', description: '' })
const confirmedMeetings = computed(() => details.value?.stats.meetings.filter(event => event.state !== 'draft') ?? [])
const maxAmount = computed(() => Math.max(1, ...confirmedMeetings.value.map(event => event.amount)))
const confirmedPercent = computed(() => {
  const stats = details.value?.stats
  return stats?.meeting_count ? Math.round(stats.confirmed_count * 100 / stats.meeting_count) : 0
})
function barWidth(event: FriendMeeting) { return Math.max(7, Math.round(event.amount * 100 / maxAmount.value)) + '%' }
function dateLabel(value: string) {
  const date = new Date(value + 'T12:00:00')
  return Number.isNaN(date.getTime()) ? value : new Intl.DateTimeFormat('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' }).format(date)
}
async function load() {
  loading.value = true; error.value = ''
  try { details.value = await request<FriendDetails>('/api/friends/' + encodeURIComponent(id)); form.value = { ...details.value.friend } }
  catch (e) { error.value = message(e) }
  finally { loading.value = false }
}
async function save() {
  if (saving.value) return
  saving.value = true; error.value = ''; notice.value = ''
  try {
    const saved = await request<Friend>('/api/friends/' + encodeURIComponent(id), 'PUT', form.value)
    form.value = { ...saved }
    if (details.value) details.value.friend = saved
    notice.value = 'Карточка друга сохранена.'
  } catch (e) { error.value = message(e) }
  finally { saving.value = false }
}
onMounted(load)
</script>

<template>
  <RouterLink
    to="/"
    class="back-link"
  >
    ← К списку друзей
  </RouterLink>
  <p
    v-if="loading"
    class="loading-card"
    role="status"
  >
    Загружаем карточку…
  </p>
  <div
    v-if="error"
    class="error"
    role="alert"
  >
    {{ error }} <button
      type="button"
      @click="load"
    >
      Повторить
    </button>
  </div>
  <p
    v-if="notice"
    class="notice"
    role="status"
  >
    {{ notice }}
  </p>
  <template v-if="details && !loading">
    <header class="friend-hero">
      <div
        class="friend-hero-avatar"
        aria-hidden="true"
      >
        {{ details.friend.name.slice(0, 1).toUpperCase() }}
      </div>
      <div>
        <p class="spray-label">
          Личная карточка
        </p><h1>{{ details.friend.name }}</h1><p>{{ details.friend.description || 'Добавьте пару слов об этом человеке.' }}</p>
      </div>
      <span class="friend-hero-tag">{{ details.stats.meeting_count }} встреч</span>
    </header>
    <div class="friend-profile-grid">
      <form
        class="card friend-details-form"
        @submit.prevent="save"
      >
        <fieldset :disabled="saving">
          <p class="eyebrow">
            Контакты и заметки
          </p><h2>О друге</h2>
          <label for="detail-name">Имя</label><input
            id="detail-name"
            v-model="form.name"
            required
            maxlength="120"
          >
          <label for="detail-phone">Номер телефона</label><input
            id="detail-phone"
            v-model="form.phone"
            type="tel"
            autocomplete="tel"
            maxlength="40"
            placeholder="Можно оставить пустым"
          >
          <label for="detail-birthday">День рождения</label><input
            id="detail-birthday"
            v-model="form.birthday"
            type="date"
          >
          <label for="detail-description">Описание</label><textarea
            id="detail-description"
            v-model="form.description"
            rows="5"
            maxlength="2000"
            placeholder="Что любит, где познакомились, полезные заметки…"
          />
          <button type="submit">
            {{ saving ? 'Сохраняем…' : 'Сохранить карточку' }}
          </button>
        </fieldset>
      </form>
      <section
        class="friend-dashboard"
        aria-labelledby="friend-stat-title"
      >
        <div class="friend-dashboard-heading">
          <div>
            <p class="eyebrow">
              Общая история
            </p><h2 id="friend-stat-title">
              Статистика
            </h2>
          </div>
          <div
            class="friend-ring"
            :style="{ '--progress': confirmedPercent + '%' }"
          >
            <strong>{{ confirmedPercent }}%</strong><span>готово</span>
          </div>
        </div>
        <div class="friend-stat-grid">
          <article class="friend-stat yellow">
            <small>Потрачено</small><strong>{{ rubles(details.stats.total_spent) }}</strong><span>по зафиксированным встречам</span>
          </article>
          <article class="friend-stat pink">
            <small>Средний вечер</small><strong>{{ rubles(details.stats.average_spent) }}</strong><span>{{ details.stats.confirmed_count }} завершённых расчётов</span>
          </article>
          <article class="friend-stat blue">
            <small>Самый дорогой</small><strong>{{ rubles(details.stats.biggest_spent) }}</strong><span>максимальная личная доля</span>
          </article>
          <article class="friend-stat white">
            <small>Осталось вернуть</small><strong>{{ rubles(details.stats.outstanding) }}</strong><span>по текущим долгам</span>
          </article>
        </div>
      </section>
    </div>
    <section class="friend-charts">
      <header class="section-heading">
        <span>01</span><div>
          <p class="eyebrow">
            Динамика расходов
          </p><h2>Вечера в цифрах</h2>
        </div>
      </header>
      <div
        v-if="confirmedMeetings.length"
        class="spending-chart"
        role="img"
        :aria-label="'Расходы по ' + confirmedMeetings.length + ' встречам'"
      >
        <div
          v-for="event in confirmedMeetings"
          :key="event.id"
          class="spending-row"
        >
          <div>
            <RouterLink :to="'/meetings/' + event.id">
              {{ event.title }}
            </RouterLink><small>{{ dateLabel(event.date) }}</small>
          </div>
          <div class="spending-track">
            <span :style="{ width: barWidth(event) }" />
          </div><strong>{{ rubles(event.amount) }}</strong>
        </div>
      </div>
      <p
        v-else
        class="empty-state"
      >
        График появится после первой зафиксированной встречи.
      </p>
      <div class="attendance-chart">
        <div class="attendance-caption">
          <strong>Статусы встреч</strong><span>{{ details.stats.confirmed_count }} зафиксировано · {{ details.stats.meeting_count - details.stats.confirmed_count }} черновиков</span>
        </div>
        <div
          class="attendance-track"
          aria-hidden="true"
        >
          <span :style="{ width: confirmedPercent + '%' }" /><i />
        </div>
      </div>
    </section>
    <section class="friend-history">
      <header class="section-heading">
        <span>02</span><div>
          <p class="eyebrow">
            Хронология
          </p><h2>Все встречи</h2>
        </div>
      </header>
      <p
        v-if="!details.stats.meetings.length"
        class="empty-state"
      >
        Друг ещё не участвовал во встречах.
      </p>
      <div
        v-else
        class="friend-event-list"
      >
        <RouterLink
          v-for="event in details.stats.meetings"
          :key="event.id"
          :to="'/meetings/' + event.id"
          class="friend-event-card"
        >
          <span :class="['event-state', 'event-' + event.state]">{{ event.state === 'draft' ? 'Черновик' : event.state === 'closed' ? 'Закрыта' : 'Зафиксирована' }}</span>
          <div>
            <h3>{{ event.title }}</h3><p>
              {{ dateLabel(event.date) }}<template v-if="event.venue">
                · {{ event.venue }}
              </template>
            </p>
          </div>
          <div class="event-amount">
            <strong>{{ rubles(event.amount) }}</strong><small>{{ event.item_count }} позиций</small>
          </div>
        </RouterLink>
      </div>
    </section>
  </template>
</template>