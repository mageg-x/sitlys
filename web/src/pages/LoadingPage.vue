<template>
  <main class="app-frame">
    <div class="ambient ambient-a"></div>
    <div class="ambient ambient-b"></div>
    <div class="ambient ambient-c"></div>
    <section class="entry-shell">
      <article class="entry-card">
        <div class="entry-logo">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="url(#loadGrad)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
            <defs><linearGradient id="loadGrad" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0%" stop-color="#6366f1"/><stop offset="100%" stop-color="#14b8a6"/></linearGradient></defs>
            <path d="M3 3v18h18"/><path d="m19 9-5 5-4-4-3 3"/>
          </svg>
        </div>
        <p class="eyebrow">{{ app.t("appTagline") }}</p>
        <h1>{{ app.t("appName") }}</h1>
        <p class="entry-copy">{{ app.t("loading") }}</p>
        <div class="loading-bar">
          <div class="loading-bar-fill"></div>
        </div>
      </article>
    </section>
  </main>
</template>

<script setup>
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>

<style scoped>
.entry-logo {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.1), rgba(20, 184, 166, 0.08));
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 1rem;
  border: 1px solid rgba(99, 102, 241, 0.12);
  box-shadow: var(--shadow-sm);
}

.loading-bar {
  margin-top: 1.5rem;
  height: 3px;
  border-radius: 999px;
  background: var(--bg-deep);
  overflow: hidden;
}

.loading-bar-fill {
  height: 100%;
  width: 40%;
  border-radius: inherit;
  background: linear-gradient(90deg, var(--accent), var(--accent-teal));
  animation: loadingSlide 1.5s ease-in-out infinite;
}

@keyframes loadingSlide {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(350%); }
}
</style>
