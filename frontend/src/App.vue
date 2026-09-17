<template>
  <header class="header">
    <RouterLink
      to="/"
      class="brand"
      aria-label="PoDolyam — на главную"
    >
      PoDolyam<span aria-hidden="true"> ↗</span>
    </RouterLink>
    <span class="badge">Windows edition</span>
  </header>
  <main id="main">
    <RouterView :key="$route.fullPath" />
  </main>
  <footer>Общий вечер. Понятный счёт.</footer>
</template>
