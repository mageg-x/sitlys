<template>
  <section class="admin-workspace">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("shares") }}</h3>
          <p>{{ app.t("shareWorkspaceText") }}</p>
        </div>
      </div>
      <div class="insight-band">
        <div class="insight-pill">
          <span>{{ app.t("selectedWebsite") }}</span>
          <strong>{{ app.selectedWebsite?.name || app.t("chooseWebsite") }}</strong>
          <small>{{ app.selectedWebsite?.domain || app.t("websiteRequired") }}</small>
        </div>
        <div class="insight-pill">
          <span>{{ app.t("activeShares") }}</span>
          <strong>{{ app.formatNumber(app.state.shares.filter(item => item.enabled).length) }}</strong>
          <small>{{ app.t("shares") }} · {{ app.formatNumber(app.state.shares.length) }}</small>
        </div>
        <div class="insight-pill">
          <span>{{ app.t("publicLink") }}</span>
          <strong>{{ app.t("shareSummary") }}</strong>
          <small>{{ app.origin }}/share/&lt;slug&gt;</small>
        </div>
      </div>
    </article>

    <article v-if="!app.canManageSelectedWebsite" class="panel">
      <div class="panel-head">
        <h3>{{ app.t("readOnlyMode") }}</h3>
        <p>{{ app.t("managePermissionRequiredText") }}</p>
      </div>
    </article>

    <div class="panel-grid">
      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("createShare") }}</h3>
            <p>{{ app.t("installationGuide") }}</p>
          </div>
        </div>

        <div class="share-cta-block">
          <p>{{ app.t("shareWorkspaceText") }}</p>
          <button class="primary-button" :disabled="!app.canManageSelectedWebsite" @click="app.saveShare">{{ app.t("create") }}</button>
        </div>

        <div class="snippet-stack">
          <section class="setting-card">
            <span>{{ app.t("publicLink") }}</span>
            <pre>{{ app.firstShareLink }}</pre>
          </section>
          <section class="setting-card">
            <span>{{ app.t("snippetHub") }}</span>
            <pre>{{ app.t("shareSummary") }}</pre>
          </section>
        </div>
      </article>

      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("shareDirectory") }}</h3>
            <p>{{ app.t("copyReady") }}</p>
          </div>
        </div>

        <div v-if="app.state.shares.length" class="card-list">
          <div v-for="share in app.state.shares" :key="share.id" class="entity-card static asset-card">
            <div class="asset-main">
              <strong>{{ app.t("publicLink") }}</strong>
              <span>{{ app.origin }}/share/{{ share.slug }}</span>
            </div>
            <div class="asset-actions">
              <span class="member-access" :class="share.enabled ? 'manage' : 'view'">
                {{ share.enabled ? app.t("enabled") : app.t("disabled") }}
              </span>
              <label class="switch-line">
                <input :checked="share.enabled" type="checkbox" :disabled="!app.canManageSelectedWebsite" @change="app.toggleShare(share, $event.target.checked)" />
                <span>{{ app.t("enabled") }}</span>
              </label>
            </div>
          </div>
        </div>
        <p v-else class="empty-note">{{ app.t("noData") }}</p>
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

.share-cta-block {
  display: grid;
  gap: 0.85rem;
  padding: 1rem;
  border: 1px solid var(--line);
  border-radius: var(--radius-sm);
  background: linear-gradient(180deg, rgba(255,255,255,0.9), rgba(248,250,252,0.82));
}

.share-cta-block p {
  margin: 0;
  color: var(--muted);
}

.snippet-stack {
  display: grid;
  gap: 0.75rem;
}

.asset-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  width: 100%;
}

.asset-main {
  display: grid;
  gap: 0.16rem;
  min-width: 0;
}

.asset-main span {
  word-break: break-all;
}

.asset-actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex-wrap: wrap;
  justify-content: flex-end;
}

.member-access {
  padding: 0.32rem 0.65rem;
  border-radius: 999px;
  font-size: 0.74rem;
  font-weight: 700;
}

.member-access.manage {
  color: var(--accent-strong);
  background: rgba(99, 102, 241, 0.12);
}

.member-access.view {
  color: var(--muted);
  background: rgba(148, 163, 184, 0.16);
}

@media (max-width: 980px) {
  .asset-card {
    flex-direction: column;
    align-items: stretch;
  }

  .asset-actions {
    justify-content: flex-start;
  }
}
</style>
