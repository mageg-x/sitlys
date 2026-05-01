<template>
  <section class="admin-workspace">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("systemSnapshot") }}</h3>
          <p>{{ app.t("systemSnapshotText") }}</p>
        </div>
      </div>
      <div class="signal-grid compact-signal-grid">
        <div class="signal-card">
          <span>{{ app.t("status") }}</span>
          <strong>{{ app.state.version || "0.1.0" }}</strong>
        </div>
        <div class="signal-card teal">
          <span>{{ app.t("websites") }}</span>
          <strong>{{ app.formatNumber(app.state.websites.length) }}</strong>
        </div>
        <div class="signal-card warm">
          <span>{{ app.t("activeShares") }}</span>
          <strong>{{ app.formatNumber(app.state.shares.filter(item => item.enabled).length) }}</strong>
        </div>
      </div>
    </article>

    <div class="panel-grid">
      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("snippetHub") }}</h3>
            <p>{{ app.t("settingsText") }}</p>
          </div>
        </div>
        <div class="settings-grid">
          <div class="setting-card">
            <span>{{ app.t("trackerSnippet") }}</span>
            <pre>{{ app.trackerSnippet }}</pre>
          </div>
          <div class="setting-card">
            <span>{{ app.t("status") }}</span>
            <pre>{{ app.state.version || "0.1.0" }} · SQLite · Embed</pre>
          </div>
          <div class="setting-card">
            <span>{{ app.t("pixelSnippet") }}</span>
            <pre>{{ app.firstPixelSnippet }}</pre>
          </div>
          <div class="setting-card">
            <span>{{ app.t("publicLink") }}</span>
            <pre>{{ app.firstShareLink }}</pre>
          </div>
        </div>
      </article>

      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("accountSecurity") }}</h3>
            <p>{{ app.t("changePasswordText") }}</p>
          </div>
        </div>
        <form class="stack-form" @submit.prevent="app.changePassword">
          <label>
            <span>{{ app.t("currentPassword") }}</span>
            <input v-model="app.passwordForm.currentPassword" type="password" required />
          </label>
          <label>
            <span>{{ app.t("newPassword") }}</span>
            <input v-model="app.passwordForm.newPassword" type="password" minlength="8" required />
          </label>
          <label>
            <span>{{ app.t("confirmPassword") }}</span>
            <input v-model="app.passwordForm.confirmPassword" type="password" minlength="8" required />
          </label>
          <div class="form-actions">
            <button class="primary-button">{{ app.t("save") }}</button>
          </div>
        </form>
      </article>
    </div>

    <div v-if="app.isSuperAdmin" class="panel-grid">
      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("runtimeProfile") }}</h3>
            <p>{{ app.t("systemSettingsText") }}</p>
          </div>
        </div>
        <form class="stack-form" @submit.prevent="app.saveSettings">
          <label>
            <span>{{ app.t("listenAddress") }}</span>
            <input v-model.trim="app.settingsForm.listen_addr" required />
          </label>
          <label>
            <span>{{ app.t("databasePath") }}</span>
            <input v-model.trim="app.settingsForm.database_path" required />
          </label>
          <label>
            <span>{{ app.t("logLevel") }}</span>
            <select v-model="app.settingsForm.log_level">
              <option value="debug">debug</option>
              <option value="info">info</option>
              <option value="warn">warn</option>
              <option value="error">error</option>
            </select>
          </label>
          <div class="form-actions">
            <button class="primary-button">{{ app.t("save") }}</button>
          </div>
        </form>
      </article>

      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("opsActions") }}</h3>
            <p>{{ app.t("backupDatabaseText") }}</p>
          </div>
        </div>

        <div class="ops-stack">
          <button class="primary-button" @click="app.createBackup">{{ app.t("createBackupNow") }}</button>
          <div class="form-block">
            <strong>{{ app.t("latestBackupPath") }}</strong>
            <pre>{{ app.state.backupPath || app.t("noData") }}</pre>
          </div>
        </div>
      </article>
    </div>
  </section>
</template>

<script setup>
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>

<style scoped>
.admin-workspace {
  display: grid;
  gap: 1rem;
}

.workspace-panel {
  display: grid;
  gap: 1rem;
  align-content: start;
}

.workspace-panel :deep(pre) {
  white-space: pre-wrap;
  word-break: break-all;
}

.compact-signal-grid {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}

.signal-card.teal strong {
  color: var(--accent-teal-deep);
}

.signal-card.warm strong {
  color: var(--accent-warm-deep);
}

.ops-stack {
  display: grid;
  gap: 1rem;
}

@media (max-width: 980px) {
  .compact-signal-grid {
    grid-template-columns: 1fr;
  }
}
</style>
