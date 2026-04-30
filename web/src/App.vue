<script setup>
import { RouterView, RouterLink } from 'vue-router'
</script>

<template>
  <div class="app">
    <header class="app-header">
      <RouterLink to="/" class="brand">LogLens</RouterLink>
      <nav class="nav">
        <RouterLink to="/dashboard">Dashboard</RouterLink>
      </nav>
    </header>
    <main class="app-main">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.app {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
.app-header {
  display: flex;
  align-items: center;
  gap: 24px;
  padding: 12px 20px;
  background: var(--ll-bg-1);
  border-bottom: 1px solid var(--ll-border);
}
.brand {
  font-weight: 700;
  font-size: 18px;
  color: var(--ll-accent);
  text-decoration: none;
}
.nav {
  display: flex;
  gap: 16px;
}
.nav a {
  color: var(--ll-text-2);
  text-decoration: none;
  font-size: 14px;
}
.nav a.router-link-active {
  color: var(--ll-text-1);
}
.app-main {
  flex: 1;
  padding: 20px;
}
</style>
