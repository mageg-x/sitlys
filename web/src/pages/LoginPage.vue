<template>
  <main class="app-frame">
    <div class="ambient ambient-a"></div>
    <div class="ambient ambient-b"></div>
    <div class="ambient ambient-c"></div>
    <div class="entry-decor entry-decor-1"></div>
    <div class="entry-decor entry-decor-2"></div>
    <section class="entry-shell">
      <article class="entry-card">
        <div class="entry-head">
          <div class="entry-logo">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="url(#loginGrad)" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
              <defs><linearGradient id="loginGrad" x1="0%" y1="0%" x2="100%" y2="100%"><stop offset="0%" stop-color="#6366f1"/><stop offset="100%" stop-color="#14b8a6"/></linearGradient></defs>
              <path d="M3 3v18h18"/><path d="m19 9-5 5-4-4-3 3"/>
            </svg>
          </div>
          <p class="eyebrow">{{ app.t("status") }}</p>
          <h1>{{ app.t("loginTitle") }}</h1>
          <p class="entry-copy">{{ app.t("loginText") }}</p>
        </div>
        <form class="entry-form" @submit.prevent="app.submitLogin">
          <label>
            <span>{{ app.t("username") }}</span>
            <input v-model.trim="app.loginForm.username" autocomplete="username" required />
          </label>
          <label>
            <span>{{ app.t("password") }}</span>
            <input v-model="app.loginForm.password" type="password" autocomplete="current-password" required />
          </label>
          <div class="entry-actions">
            <button class="primary-button" :disabled="app.state.submitting">{{ app.t("signIn") }}</button>
            <LocaleSwitch v-model="app.locale" :t="app.t" />
          </div>
        </form>
      </article>
    </section>
  </main>
</template>

<script setup>
import LocaleSwitch from "../components/LocaleSwitch.vue";
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>

<style scoped>
.entry-decor {
  position: absolute;
  border-radius: 999px;
  pointer-events: none;
}

.entry-decor-1 {
  width: 6px;
  height: 80px;
  right: 15%;
  top: 20%;
  background: linear-gradient(180deg, var(--accent), var(--accent-teal));
  opacity: 0.15;
  border-radius: 3px;
}

.entry-decor-2 {
  width: 40px;
  height: 40px;
  left: 12%;
  bottom: 25%;
  border: 2px solid var(--accent-teal);
  opacity: 0.1;
  border-radius: 10px;
  transform: rotate(45deg);
}

.entry-logo {
  width: 48px;
  height: 48px;
  border-radius: 14px;
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.1), rgba(20, 184, 166, 0.08));
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 1.25rem;
  border: 1px solid rgba(99, 102, 241, 0.12);
  box-shadow: var(--shadow-md);
}
</style>
