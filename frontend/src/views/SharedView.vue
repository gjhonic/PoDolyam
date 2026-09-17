<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { request, message } from '../api'
import { rubles } from '../money'
const route = useRoute()
const data = ref<{ title: string; name: string; date: string; total: number; debt: number; remaining: number; lines: { name: string; amount: number }[] } | null>(null)
const error = ref(''), loading = ref(true)
async function load() {
  loading.value = true; error.value = ''
  try { data.value = await request('/api/shared/' + String(route.params.token)) }
  catch (e) { error.value = message(e) }
  finally { loading.value = false }
}
onMounted(load)
</script>
<template>
  <p
    v-if="loading"
    role="status"
  >
    Загрузка расчёта…
  </p>
  <div
    v-if="error"
    class="error"
    role="alert"
  >
    {{ error }} <button @click="load">
      Повторить
    </button>
  </div>
  <section
    v-if="data"
    class="card"
  >
    <p class="badge">
      Зафиксированный расчёт
    </p><h1>{{ data.title }}</h1><p>{{ data.date }} · {{ data.name }}</p>
    <ul>
      <li
        v-for="(line, index) in data.lines"
        :key="index"
      >
        {{ line.name }} — {{ rubles(line.amount) }}
      </li>
    </ul>
    <p>Ваша доля: <strong>{{ rubles(data.total) }}</strong></p>
    <p>К возврату плательщику: {{ rubles(data.debt) }}</p>
    <h2>Осталось вернуть: {{ rubles(data.remaining) }}</h2>
    <p>Организатор отмечает полученные переводы вручную.</p>
  </section>
</template>
