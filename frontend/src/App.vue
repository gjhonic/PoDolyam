<script setup lang="ts">
const appVersion = import.meta.env.VITE_APP_VERSION || 'dev'
</script>

<template>
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
