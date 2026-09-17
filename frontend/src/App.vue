<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const appVersion = import.meta.env.VITE_APP_VERSION || 'dev'
const showSplash = ref(true)
let splashTimer: ReturnType<typeof setTimeout> | undefined
onMounted(() => { splashTimer = setTimeout(() => { showSplash.value = false }, 1200) })
onBeforeUnmount(() => clearTimeout(splashTimer))
</script>

<template>
  <Transition name="splash">
    <section
      v-if="showSplash"
      class="splash-screen"
      role="status"
      aria-label="PoDolyam запускается"
    >
      <div
        class="splash-art"
        aria-hidden="true"
      >
        <strong class="splash-brand">PoDolyam<span> ↗</span></strong>
      </div>
      <p>Общий вечер. Понятный счёт.</p>
      <div
        class="splash-loader"
        aria-hidden="true"
      >
        <span />
      </div>
    </section>
  </Transition>
  <header class="header">
    <RouterLink
      to="/"
      class="brand"
      aria-label="PoDolyam — на главную"
    >
      PoDolyam<span aria-hidden="true"> ↗</span>
    </RouterLink>
    <span class="badge">Windows edition · v{{ appVersion }}</span>
  </header>
  <main id="main">
    <RouterView :key="$route.fullPath" />
  </main>
  <footer>Общий вечер. Понятный счёт.</footer>
</template>
