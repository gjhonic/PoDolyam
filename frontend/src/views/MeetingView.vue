<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { onBeforeRouteLeave, useRoute } from 'vue-router'
import { request, message } from '../api'
import { isDesktop, desktop, copyText } from '../desktop'
import { rubles, inputRubles, parseRubles } from '../money'
import { participantFromFriend } from '../participants'
import type { Assignment, Bill, Friend, Item, Meeting } from '../types'

type MeetingSection = 'group' | 'people' | 'bill' | 'result'
const route = useRoute()
const activeSection = ref<MeetingSection>('group')
const base = '/api/meetings/' + String(route.params.id)
const meeting = ref<Meeting | null>(null)
const loading = ref(true), busy = ref(false), dirty = ref(false), error = ref(''), notice = ref('')
const newName = ref(''), receipt = ref(''), paymentText = ref(''), paymentPerson = ref('')
const friends = ref<Friend[]>([])
const prices = ref<Record<string, string>>({}), reasons = ref<Record<string, string>>({}), links = ref<Record<string, string>>({})
const stateNames = { draft: 'Черновик', finalized: 'Зафиксировано', closed: 'Закрыто' }
const blockerNames: Record<string, string> = {
  no_participants: 'Добавьте участников', no_payer: 'Организатор не определён', no_items: 'Добавьте позиции',
  no_receipt: 'Введите итог чека', receipt_mismatch: 'Сумма позиций не совпадает с чеком', unassigned_items: 'Распределите все позиции',
}
const modeNames: Record<Assignment['mode'], string> = {
  single: 'одному', equal: 'поровну', all: 'на всех', units: 'по единицам', weighted: 'по весам',
}
// Эти значения нужны только для представления. Денежные доли не пересчитываются:
// таблица всегда показывает готовый результат, который вернул Go.
const assignedItems = computed(() => meeting.value?.bill.items.filter(item => item.assignment?.weights.length).length ?? 0)
const remainingTotal = computed(() => Object.values(meeting.value?.remaining ?? {}).reduce((sum, value) => sum + value, 0))
const activePayments = computed(() => meeting.value?.payments.filter(payment => !payment.cancelled_at).length ?? 0)
const payerName = computed(() => name(meeting.value?.bill.payer_id ?? ''))
const availableFriends = computed(() => friends.value.filter(friend => !meeting.value?.bill.participants.some(person => person.id === friend.id)))

function itemName(itemID: string) {
  return meeting.value?.bill.items.find(item => item.id === itemID)?.name || 'Позиция'
}
function matrixAmount(participantID: string, itemID: string) {
  return meeting.value?.calculation.lines.find(line => line.item_id === itemID)?.shares.find(share => share.participant_id === participantID)?.amount ?? null
}
function itemAmount(itemID: string) {
  return meeting.value?.calculation.lines.find(line => line.item_id === itemID)?.amount ?? 0
}
function participantItemCount(participantID: string) {
  return meeting.value?.calculation.lines.filter(line => line.shares.some(share => share.participant_id === participantID)).length ?? 0
}
function shareText(itemID: string) {
  const line = meeting.value?.calculation.lines.find(item => item.item_id === itemID)
  if (!line?.shares.length) return 'Ещё не распределено'
  return line.shares.map(share => `${name(share.participant_id)} - ${rubles(share.amount)}`).join(' · ')
}
async function load() {
  loading.value = true
  try {
    meeting.value = await request<Meeting>(base)
    if (isDesktop) friends.value = await request<Friend[]>('/api/friends')
    meeting.value.description ??= ''
    receipt.value = inputRubles(meeting.value.bill.receipt_total)
    prices.value = Object.fromEntries(meeting.value.bill.items.map(i => [i.id, inputRubles(i.unit_price)]))
    dirty.value = false
  } finally { loading.value = false }
}
async function perform(action: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await action() } catch (e) { error.value = message(e) }
  finally { busy.value = false }
}
function nextParticipantOrder() {
  return Math.max(-1, ...(meeting.value?.bill.participants.map(person => person.order) ?? [])) + 1
}
function addPerson() {
  if (!meeting.value || !newName.value.trim()) return
  meeting.value.bill.participants.push({ id: crypto.randomUUID(), name: newName.value.trim(), order: nextParticipantOrder() })
  newName.value = ''; dirty.value = true
}
function addFriend(friend: Friend) {
  if (!meeting.value) return
  const participant = participantFromFriend(meeting.value.bill.participants, friend)
  if (!participant) return
  meeting.value.bill.participants.push(participant)
  dirty.value = true
}function removePerson(id: string) {
  const bill = meeting.value!.bill
  if (bill.payer_id === id) { error.value = 'Организатор остаётся участником встречи.'; return }
  if (bill.items.some(i => i.assignment?.weights.some(w => w.participant_id === id))) {
    error.value = 'Сначала снимите назначения этого участника и сохраните чек.'; return
  }
  bill.participants = bill.participants.filter(p => p.id !== id)
  dirty.value = true
}
function addItem() {
  const id = crypto.randomUUID()
  meeting.value!.bill.items.push({ id, name: '', quantity: 1, unit_price: null, assignment: null })
  prices.value[id] = ''; dirty.value = true
}
function mode(item: Item, event: Event) {
  const selected = (event.target as HTMLSelectElement).value
  if (!selected) item.assignment = null
  else if (selected === 'all') all(item)
  else item.assignment = { mode: selected as Assignment['mode'], weights: [] }
  dirty.value = true
}
function all(item: Item) {
  // Копируем текущие ID. Новые участники не попадут в старое назначение.
  item.assignment = { mode: 'all', weights: meeting.value!.bill.participants.map(p => ({ participant_id: p.id, value: 1 })) }
  dirty.value = true
}
function toggle(item: Item, id: string) {
  if (!item.assignment) item.assignment = { mode: 'equal', weights: [] }
  const a = item.assignment
  if (a.mode === 'all') a.mode = 'equal'
  if (a.mode === 'single') a.weights = [{ participant_id: id, value: 1 }]
  else if (a.weights.some(w => w.participant_id === id)) a.weights = a.weights.filter(w => w.participant_id !== id)
  else a.weights.push({ participant_id: id, value: 1 })
  dirty.value = true
}
async function save() {
  await perform(async () => {
    const m = meeting.value!
    const bill = JSON.parse(JSON.stringify(m.bill)) as Bill
    // Vue хранит reactive proxy; JSON-копия отделяет входной документ.
    bill.receipt_total = receipt.value.trim() === '' ? null : parseRubles(receipt.value)
    for (const item of bill.items) {
      try { item.unit_price = parseRubles(prices.value[item.id] ?? '') }
      catch { throw new Error('Проверьте цену позиции <' + (item.name || 'Без названия') + '>: введите рубли и до двух знаков копеек.') }
    }
    await request(base, 'PUT', { title: m.title, description: m.description, date: m.date, venue: m.venue, bill, version: m.version })
    await load(); notice.value = 'Изменения сохранены'
  })
}
function name(id: string) { return meeting.value?.bill.participants.find(p => p.id === id)?.name ?? id }
async function finalize() {
  await perform(async () => { await request(base + '/finalize', 'POST', { version: meeting.value!.version }); await load() })
}
async function pay() {
  await perform(async () => { await request(base + '/payments', 'POST', { participant_id: paymentPerson.value, amount: parseRubles(paymentText.value) }); paymentText.value = ''; await load() })
}
async function cancel(id: string) {
  await perform(async () => { await request(base + '/payments/' + id + '/cancel', 'POST', { reason: reasons.value[id] }); await load() })
}
async function personalLink(id: string, revoke: boolean) {
  await perform(async () => {
    const result = await request<{ token: string }>(base + '/links', 'POST', { participant_id: id, revoke })
    if (revoke) { delete links.value[id]; notice.value = 'Ссылки участника отозваны' }
    else { links.value[id] = location.origin + '/s/' + result.token; notice.value = 'Новая ссылка создана; предыдущая отозвана' }
  })
}
async function exportPersonal(id: string) {
  await perform(async () => {
    const path = await desktop().ExportParticipantPDF(meeting.value!.id, id)
    notice.value = path ? 'Расчёт сохранён: ' + path : 'Экспорт отменён'
  })
}
async function copyMessage(id: string) {
  await perform(async () => {
    const m = meeting.value!, total = m.calculation.totals.find(t => t.participant_id === id)!
    const lines = m.calculation.lines.flatMap(line => {
      const share = line.shares.find(s => s.participant_id === id)
      return share ? [itemName(line.item_id) + ': ' + rubles(share.amount)] : []
    })
    await copyText([m.title, name(id), ...lines, 'Всего: ' + rubles(total.amount), 'Осталось вернуть: ' + rubles(m.remaining[id] ?? 0), links.value[id] ?? ''].filter(Boolean).join('\n'))
    notice.value = 'Сообщение скопировано. Отправьте его участнику самостоятельно.'
  })
}
onMounted(() => perform(load))
onBeforeRouteLeave(() => !dirty.value || window.confirm('Есть несохранённые изменения. Уйти со страницы?'))
</script>

<template>
  <RouterLink
    to="/"
    class="back-link"
  >
    ← Все встречи
  </RouterLink>
  <p
    v-if="loading"
    class="loading-card"
    role="status"
  >
    Загружаем вашу тусовку:
  </p>
  <p
    v-if="error"
    class="error"
    role="alert"
  >
    {{ error }}
  </p>
  <p
    v-if="notice"
    class="notice"
    role="status"
  >
    {{ notice }}
  </p>
  <button
    v-if="!meeting && !loading"
    @click="perform(load)"
  >
    Повторить загрузку
  </button>

  <template v-if="meeting && !loading">
    <section class="meeting-cover">
      <div>
        <p class="spray-label">
          Группа / {{ meeting.date }}
        </p>
        <h1>{{ meeting.title }}</h1>
        <p class="cover-description">
          {{ meeting.description || 'Добавьте описание: повод, настроение или важную заметку для компании.' }}
        </p>
        <div class="cover-meta">
          <span>{{ meeting.venue || 'Место ещё не выбрано' }}</span>
          <span>{{ meeting.bill.participants.length }} участников</span>
          <span>Платит: {{ payerName || 'не выбран' }}</span>
        </div>
      </div>
      <span :class="['state-sticker', 'state-' + meeting.state]">{{ stateNames[meeting.state] }}</span>
    </section>

    <nav
      class="section-nav"
      aria-label="Разделы встречи"
      role="tablist"
    >
      <button
        v-for="section in ([['group', '01 Группа'], ['people', '02 Участники'], ['bill', '03 Чек'], ['result', '04 Итог']] as const)"
        :key="section[0]"
        type="button"
        role="tab"
        :aria-selected="activeSection === section[0]"
        @click="activeSection = section[0]"
      >
        {{ section[1] }}
      </button>
    </nav>

    <form
      v-if="meeting.state === 'draft'"
      @submit.prevent="save"
      @input="dirty = true"
      @change="dirty = true"
    >
      <fieldset :disabled="busy">
        <section
          v-show="activeSection === 'group'"
          id="group"
          class="street-section"
        >
          <header class="section-heading">
            <span>01</span><div>
              <p class="eyebrow">
                Карточка ужина
              </p><h2>Группа</h2>
            </div>
          </header>
          <div class="card group-card">
            <div class="columns">
              <div>
                <label for="meeting-title">Название встречи</label><input
                  id="meeting-title"
                  v-model="meeting.title"
                  required
                  maxlength="120"
                >
              </div>
              <div>
                <label for="meeting-date">Дата</label><input
                  id="meeting-date"
                  v-model="meeting.date"
                  type="date"
                  required
                >
              </div>
            </div>
            <label for="meeting-description">Описание встречи</label>
            <textarea
              id="meeting-description"
              v-model="meeting.description"
              maxlength="1000"
              rows="4"
              placeholder="Например: пятничный ужин после релиза. Без спешки, с десертом!"
            />
            <label for="venue">Где собираемся</label>
            <input
              id="venue"
              v-model="meeting.venue"
              maxlength="200"
              placeholder="Название кафе или адрес"
            >
          </div>
        </section>

        <section
          v-show="activeSection === 'people'"
          id="people"
          class="street-section"
        >
          <header class="section-heading">
            <span>02</span><div>
              <p class="eyebrow">
                Вся банда
              </p><h2>Участники</h2>
            </div>
          </header>
          <div class="card people-card">
            <p
              v-if="!meeting.bill.participants.length"
              class="empty-state"
            >
              Здесь пока тихо. Добавьте первого участника.
            </p>
            <div class="people-grid">
              <div
                v-for="(p, index) in meeting.bill.participants"
                :key="p.id"
                class="person-card"
              >
                <span class="avatar">{{ p.name.trim().slice(0, 1).toUpperCase() || index + 1 }}</span>
                <div>
                  <label :for="'person-' + p.id">Имя участника</label><input
                    :id="'person-' + p.id"
                    v-model="p.name"
                    required
                    maxlength="80"
                  >
                </div>
                <button
                  v-if="p.id !== meeting.bill.payer_id"
                  type="button"
                  class="icon-button"
                  :aria-label="'Удалить ' + p.name"
                  title="Удалить участника"
                  @click="removePerson(p.id)"
                >
                  ?
                </button>
              </div>
            </div>
            <div class="quick-add">
              <div>
                <label for="new-person">Новый участник</label><input
                  id="new-person"
                  v-model="newName"
                  maxlength="80"
                  placeholder="Имя друга"
                  @keydown.enter.prevent="addPerson"
                >
              </div>
              <button
                type="button"
                class="accent-pink"
                @click="addPerson"
              >
                + Добавить
              </button>
            </div>
            <div
              v-if="isDesktop"
              class="friend-picker"
            >
              <div>
                <p class="eyebrow">
                  Из списка друзей
                </p><p class="hint">
                  ID друга сохранится во встрече для будущей общей статистики.
                </p>
              </div>
              <div
                v-if="availableFriends.length"
                class="actions"
              >
                <button
                  v-for="friend in availableFriends"
                  :key="friend.id"
                  type="button"
                  class="friend-chip"
                  @click="addFriend(friend)"
                >
                  <span>{{ friend.name }}</span><small>{{ friend.phone }}</small>
                </button>
              </div>
              <p
                v-else
                class="hint"
              >
                Все сохранённые друзья уже добавлены или список пока пуст.
              </p>
            </div>
            <p class="organizer-note">
              <strong>{{ payerName }}</strong> — организатор и плательщик этой встречи.
            </p>
          </div>
        </section>

        <section
          v-show="activeSection === 'bill'"
          id="bill"
          class="street-section"
        >
          <header class="section-heading">
            <span>03</span><div>
              <p class="eyebrow">
                Кто что ел
              </p><h2>Чек и назначения</h2>
            </div>
          </header>
          <p
            v-if="!meeting.bill.items.length"
            class="empty-state card"
          >
            Чек пуст. Добавьте первое блюдо - и начнём делить.
          </p>
          <div class="dish-grid">
            <article
              v-for="(item, index) in meeting.bill.items"
              :key="item.id"
              class="card dish-card"
            >
              <div class="dish-topline">
                <span class="dish-number">#{{ String(index + 1).padStart(2, '0') }}</span>
                <span :class="['assignment-status', item.assignment?.weights.length ? 'ready' : 'waiting']">{{ item.assignment?.weights.length ? 'Распределено' : 'Ждёт назначения' }}</span>
              </div>
              <label :for="'dish-' + item.id">Название позиции</label>
              <input
                :id="'dish-' + item.id"
                v-model="item.name"
                required
                maxlength="160"
                placeholder="Например, большая пицца"
              >
              <div class="columns">
                <div>
                  <label :for="'qty-' + item.id">Количество</label><input
                    :id="'qty-' + item.id"
                    v-model.number="item.quantity"
                    type="number"
                    min="1"
                    max="1000000"
                    step="1"
                    required
                  >
                </div>
                <div>
                  <label :for="'price-' + item.id">Цена за единицу, ?</label><input
                    :id="'price-' + item.id"
                    v-model="prices[item.id]"
                    inputmode="decimal"
                    required
                    placeholder="250,00"
                  >
                </div>
              </div>
              <label :for="'mode-' + item.id">Как делим</label>
              <select
                :id="'mode-' + item.id"
                :value="item.assignment?.mode ?? ''"
                @change="mode(item, $event)"
              >
                <option value="">
                  Не распределено
                </option><option value="single">
                  Одному
                </option><option value="equal">
                  Поровну выбранным
                </option>
                <option value="all">
                  На всех сейчас
                </option><option value="units">
                  По единицам
                </option><option value="weighted">
                  По весам долей
                </option>
              </select>
              <div
                class="actions assignment-chips"
                aria-label="Выбор участников"
              >
                <button
                  v-for="p in meeting.bill.participants"
                  :key="p.id"
                  type="button"
                  class="chip"
                  :aria-pressed="item.assignment?.weights.some(w => w.participant_id === p.id) ?? false"
                  @click="toggle(item, p.id)"
                >
                  {{ p.name }}
                </button>
                <button
                  type="button"
                  class="secondary"
                  :disabled="!meeting.bill.participants.length"
                  @click="all(item)"
                >
                  На всех
                </button>
              </div>
              <template v-if="item.assignment?.mode === 'units' || item.assignment?.mode === 'weighted'">
                <div
                  v-for="w in item.assignment.weights"
                  :key="w.participant_id"
                >
                  <label :for="item.id + w.participant_id">{{ name(w.participant_id) }} - {{ item.assignment.mode === 'units' ? 'единиц' : 'вес' }}</label>
                  <input
                    :id="item.id + w.participant_id"
                    v-model.number="w.value"
                    type="number"
                    min="1"
                    max="1000000"
                    step="1"
                    required
                  >
                </div>
              </template>
              <button
                type="button"
                class="text-button danger-button"
                @click="meeting.bill.items = meeting.bill.items.filter(i => i.id !== item.id); dirty = true"
              >
                Удалить позицию
              </button>
            </article>
          </div>
          <button
            type="button"
            class="add-dish"
            @click="addItem"
          >
            + Добавить позицию
          </button>
          <section class="card receipt-card">
            <div>
              <p class="eyebrow">
                Контрольная сумма
              </p><label for="receipt">Итог на ресторанном чеке, ?</label><p class="hint">
                Сверим его с суммой всех позиций перед фиксацией.
              </p>
            </div>
            <div class="receipt-action">
              <input
                id="receipt"
                v-model="receipt"
                inputmode="decimal"
                placeholder="8820,00"
              ><button type="submit">
                {{ busy ? 'Сохраняем:' : 'Сохранить и пересчитать' }}
              </button>
            </div>
          </section>
        </section>
      </fieldset>
    </form>

    <section
      v-else
      v-show="activeSection === 'group'"
      id="group"
      class="street-section readonly-group"
    >
      <header class="section-heading">
        <span>01</span><div>
          <p class="eyebrow">
            Карточка ужина
          </p><h2>Группа</h2>
        </div>
      </header>
      <div class="card group-card">
        <p>{{ meeting.description || 'Описание не добавлено.' }}</p><div class="cover-meta dark">
          <span>{{ meeting.date }}</span><span>{{ meeting.venue || 'Место не указано' }}</span><span>Плательщик: {{ payerName }}</span>
        </div>
      </div>
    </section>

    <section
      v-if="meeting.state !== 'draft'"
      v-show="activeSection === 'people'"
      id="people"
      class="street-section readonly-people"
    >
      <header class="section-heading">
        <span>02</span><div>
          <p class="eyebrow">
            Вся банда
          </p><h2>Участники</h2>
        </div>
      </header>
      <div class="participant-strip">
        <span
          v-for="p in meeting.bill.participants"
          :key="p.id"
          class="participant-sticker"
        >{{ p.name }}<small v-if="p.id === meeting.bill.payer_id">плательщик</small></span>
      </div>
    </section>

    <section
      v-if="meeting.state !== 'draft'"
      v-show="activeSection === 'bill'"
      id="bill"
      class="street-section readonly-bill"
    >
      <header class="section-heading">
        <span>03</span>
        <div>
          <p class="eyebrow">
            Сохранённый чек
          </p>
          <h2>Чек и назначения</h2>
        </div>
      </header>
      <div class="dish-grid">
        <article
          v-for="(item, index) in meeting.bill.items"
          :key="item.id"
          class="card dish-card readonly-dish"
        >
          <div class="dish-topline">
            <span class="dish-number">#{{ String(index + 1).padStart(2, '0') }}</span>
            <strong>{{ rubles(meeting.calculation.lines.find(line => line.item_id === item.id)?.amount ?? 0) }}</strong>
          </div>
          <h3>{{ item.name }}</h3>
          <p>{{ item.quantity }} × {{ rubles(item.unit_price ?? 0) }} · {{ item.assignment ? modeNames[item.assignment.mode] : 'не распределено' }}</p>
          <div class="assignment-preview">
            <span
              v-for="weight in item.assignment?.weights ?? []"
              :key="weight.participant_id"
            >
              {{ name(weight.participant_id) }}<small v-if="item.assignment?.mode === 'units' || item.assignment?.mode === 'weighted'"> × {{ weight.value }}</small>
            </span>
          </div>
        </article>
      </div>
    </section>

    <section
      v-show="activeSection === 'result'"
      id="result"
      class="street-section result-section"
    >
      <header class="section-heading inverse">
        <span>04</span><div>
          <p class="eyebrow">
            Финальная раскладка
          </p><h2>{{ meeting.state === 'draft' ? 'Предварительный итог' : 'Итог' }}</h2>
        </div>
      </header>
      <p
        v-if="dirty"
        class="notice"
      >
        Есть несохранённые изменения. Ниже показан последний расчёт Go.
      </p>
      <div class="stats-grid">
        <article class="stat-card yellow">
          <span>Общий чек</span><strong>{{ rubles(meeting.calculation.total) }}</strong><small>{{ meeting.bill.items.length }} позиций</small>
        </article>
        <article class="stat-card pink">
          <span>В компании</span><strong>{{ meeting.bill.participants.length }}</strong><small>участников</small>
        </article>
        <article class="stat-card blue">
          <span>Назначено</span><strong>{{ assignedItems }}/{{ meeting.bill.items.length }}</strong><small>позиций чека</small>
        </article>
        <article class="stat-card white">
          <span>{{ meeting.state === 'draft' ? 'Вернуть плательщику' : 'Осталось вернуть' }}</span><strong>{{ rubles(meeting.state === 'draft' ? (meeting.calculation.to_repay ?? 0) : remainingTotal) }}</strong><small>{{ activePayments }} активных переводов</small>
        </article>
      </div>
      <div
        v-if="meeting.calculation.unassigned_ids.length"
        class="unassigned-banner"
      >
        <strong>Не всё поделено!</strong> {{ meeting.calculation.unassigned_ids.length }} поз. на {{ rubles(meeting.calculation.unassigned_amount) }}
      </div>

      <div class="result-panel">
        <div class="panel-heading">
          <div>
            <p class="eyebrow">
              Кому сколько
            </p><h3>Таблица расчёта</h3>
          </div><span class="algorithm-tag">largest remainder v1</span>
        </div>
        <div class="table-scroll matrix-scroll">
          <table class="result-table matrix-table">
            <caption>Доли участников по каждой позиции чека</caption>
            <thead>
              <tr>
                <th class="sticky-participant">
                  Участник
                </th>
                <th
                  v-for="item in meeting.bill.items"
                  :key="item.id"
                  class="product-heading"
                >
                  <span>{{ item.name || 'Без названия' }}</span>
                </th>
                <th>Позиций</th><th>Доля</th><th>{{ meeting.state === 'draft' ? 'К возврату' : 'Осталось' }}</th><th>Действия</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="total in meeting.calculation.totals"
                :key="total.participant_id"
                :class="{ 'payer-row': total.participant_id === meeting.bill.payer_id }"
              >
                <th
                  class="sticky-participant participant-cell"
                  scope="row"
                >
                  <strong>{{ name(total.participant_id) }}</strong>
                  <small v-if="total.participant_id === meeting.bill.payer_id">плательщик</small>
                </th>
                <td
                  v-for="item in meeting.bill.items"
                  :key="item.id"
                  class="matrix-money"
                >
                  {{ matrixAmount(total.participant_id, item.id) === null ? '—' : rubles(matrixAmount(total.participant_id, item.id)!) }}
                </td>
                <td class="matrix-count">
                  {{ participantItemCount(total.participant_id) }}
                </td>
                <td class="money-cell">
                  {{ rubles(total.amount) }}
                </td>
                <td class="money-cell remaining-cell">
                  {{ total.debt === null ? '—' : rubles(meeting.state === 'draft' ? total.debt : (meeting.remaining[total.participant_id] ?? 0)) }}
                </td>
                <td class="matrix-actions">
                  <details>
                    <summary>Лог доли</summary>
                    <ul class="share-list">
                      <template
                        v-for="line in meeting.calculation.lines"
                        :key="line.item_id"
                      >
                        <li
                          v-for="share in line.shares.filter(s => s.participant_id === total.participant_id)"
                          :key="line.item_id + share.participant_id"
                        >
                          <span>{{ itemName(line.item_id) }}</span><strong>{{ rubles(share.amount) }}</strong>
                        </li>
                      </template>
                    </ul>
                  </details>
                  <div
                    v-if="meeting.state !== 'draft'"
                    class="row-actions"
                  >
                    <button
                      type="button"
                      :disabled="busy"
                      class="mini-button"
                      @click="copyMessage(total.participant_id)"
                    >
                      Копировать
                    </button>
                    <button
                      type="button"
                      :disabled="busy"
                      class="mini-button receipt-button"
                      @click="isDesktop ? exportPersonal(total.participant_id) : personalLink(total.participant_id, false)"
                    >
                      {{ isDesktop ? 'Сформировать чек PDF' : 'Новая ссылка' }}
                    </button>
                    <button
                      v-if="!isDesktop"
                      type="button"
                      :disabled="busy"
                      class="mini-button"
                      @click="personalLink(total.participant_id, true)"
                    >
                      Отозвать
                    </button>
                    <a
                      v-if="links[total.participant_id]"
                      :href="links[total.participant_id]"
                      target="_blank"
                      rel="noreferrer"
                    >Открыть расчёт</a>
                  </div>
                  <small v-else>PDF доступен после фиксации</small>
                </td>
              </tr>
            </tbody>
            <tfoot>
              <tr>
                <th class="sticky-participant">
                  Стоимость позиции
                </th>
                <th
                  v-for="item in meeting.bill.items"
                  :key="item.id"
                  class="matrix-money"
                >
                  {{ rubles(itemAmount(item.id)) }}
                </th>
                <th>{{ meeting.bill.items.length }}</th><th>{{ rubles(meeting.calculation.total) }}</th><th>{{ rubles(meeting.state === 'draft' ? (meeting.calculation.to_repay ?? 0) : remainingTotal) }}</th><th />
              </tr>
            </tfoot>
          </table>
        </div>
      </div>

      <div class="result-columns">
        <article class="result-panel calculation-log">
          <div class="panel-heading">
            <div>
              <p class="eyebrow">
                Как получились суммы
              </p><h3>Лог подсчёта</h3>
            </div>
          </div>
          <ol v-if="meeting.calculation.lines.length">
            <li
              v-for="(line, index) in meeting.calculation.lines"
              :key="line.item_id"
            >
              <span class="log-index">{{ String(index + 1).padStart(2, '0') }}</span>
              <div><strong>{{ itemName(line.item_id) }} - {{ rubles(line.amount) }}</strong><p>{{ meeting.bill.items.find(item => item.id === line.item_id)?.assignment ? modeNames[meeting.bill.items.find(item => item.id === line.item_id)!.assignment!.mode] : 'не распределено' }} · {{ shareText(line.item_id) }}</p></div>
            </li>
          </ol>
          <p
            v-else
            class="empty-state"
          >
            Добавьте и сохраните позиции - здесь появится прозрачная расшифровка.
          </p>
        </article>
        <article class="result-panel calculation-status">
          <p class="eyebrow">
            Готовность
          </p><h3>Статус расчёта</h3>
          <template v-if="meeting.state === 'draft'">
            <ul
              v-if="meeting.calculation.blockers.length"
              class="blocker-list"
            >
              <li
                v-for="b in meeting.calculation.blockers"
                :key="b"
              >
                {{ blockerNames[b] ?? b }}
              </li>
            </ul>
            <p
              v-else
              class="ready-message"
            >
              Всё сошлось. Можно фиксировать!
            </p>
            <p class="hint">
              После фиксации изменить чек нельзя.
            </p>
            <button
              :disabled="busy || dirty || !!meeting.calculation.blockers.length"
              @click="finalize"
            >
              Зафиксировать расчёт
            </button>
          </template>
          <template v-else>
            <p class="ready-message">
              Расчёт сохранён как неизменяемый снимок.
            </p><p>Плательщик: <strong>{{ payerName }}</strong></p><p>К возврату: <strong>{{ rubles(meeting.calculation.to_repay ?? 0) }}</strong></p>
          </template>
        </article>
      </div>
    </section>

    <section
      v-if="meeting.state !== 'draft'"
      v-show="activeSection === 'result'"
      class="street-section payments-section"
    >
      <header class="section-heading">
        <span>05</span><div>
          <p class="eyebrow">
            Деньги назад
          </p><h2>Возвраты</h2>
        </div>
      </header>
      <div class="result-columns">
        <form
          v-if="meeting.state === 'finalized'"
          class="card payment-form"
          @submit.prevent="pay"
        >
          <fieldset :disabled="busy">
            <label for="payment-person">Кто перевёл</label><select
              id="payment-person"
              v-model="paymentPerson"
              required
            >
              <option value="">
                Выберите участника
              </option><option
                v-for="p in meeting.bill.participants.filter(p => p.id !== meeting!.bill.payer_id)"
                :key="p.id"
                :value="p.id"
              >
                {{ p.name }}
              </option>
            </select>
            <label for="payment-amount">Получено, ?</label><input
              id="payment-amount"
              v-model="paymentText"
              inputmode="decimal"
              required
              placeholder="500,00"
            ><button type="submit">
              Записать перевод
            </button>
          </fieldset>
        </form>
        <div class="payment-log">
          <p
            v-if="!meeting.payments.length"
            class="empty-state card"
          >
            Переводов ещё нет.
          </p>
          <article
            v-for="p in meeting.payments"
            :key="p.id"
            :class="['payment-row', { cancelled: p.cancelled_at }]"
          >
            <p><strong>{{ name(p.participant_id) }}</strong><span>{{ rubles(p.amount) }}</span></p><small v-if="p.cancelled_at">Отменён: {{ p.cancellation_reason }}</small>
            <template v-if="!p.cancelled_at && meeting.state === 'finalized'">
              <label :for="'reason-' + p.id">Причина отмены ошибочной записи</label><input
                :id="'reason-' + p.id"
                v-model="reasons[p.id]"
                maxlength="500"
              ><button
                class="secondary"
                :disabled="busy || !reasons[p.id]?.trim()"
                @click="cancel(p.id)"
              >
                Отменить запись
              </button>
            </template>
          </article>
        </div>
      </div>
      <button
        v-if="meeting.state === 'finalized'"
        :disabled="busy || Object.values(meeting.remaining).some(v => v > 0)"
        @click="perform(async () => { await request(base + '/close', 'POST'); await load() })"
      >
        Закрыть встречу
      </button>
    </section>
  </template>
</template>
