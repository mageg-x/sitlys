<template>
  <section class="admin-workspace">
    <article class="panel analytics-hero">
      <div class="panel-head analytics-head">
        <div>
          <h3>{{ app.t("websiteBriefing") }}</h3>
          <p>{{ app.t("websiteBriefingText") }}</p>
        </div>
      </div>
      <div class="signal-grid compact-signal-grid">
        <div class="signal-card">
          <span>{{ app.t("websites") }}</span>
          <strong>{{ app.formatNumber(app.state.websites.length) }}</strong>
        </div>
        <div class="signal-card teal">
          <span>{{ app.t("selectedWebsite") }}</span>
          <strong>{{ app.selectedWebsite?.name || app.t("chooseWebsite") }}</strong>
        </div>
        <div class="signal-card warm">
          <span>{{ app.t("authorizedUsers") }}</span>
          <strong>{{ app.formatNumber(app.websiteMemberSummary.total) }}</strong>
        </div>
      </div>
    </article>

    <article v-if="!app.canCreateWebsite" class="panel">
      <div class="panel-head">
        <h3>{{ app.t("readOnlyMode") }}</h3>
        <p>{{ app.t("websiteCreatePermissionRequiredText") }}</p>
      </div>
    </article>

    <div class="panel-grid">
      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.websiteForm.id ? app.t("edit") : app.t("createWebsite") }}</h3>
            <p>{{ app.t("websiteListText") }}</p>
          </div>
        </div>

        <form class="stack-form" @submit.prevent="app.saveWebsite">
          <label>
            <span>{{ app.t("websiteName") }}</span>
            <input v-model.trim="app.websiteForm.name" :disabled="!app.canCreateWebsite" required />
          </label>
          <label>
            <span>{{ app.t("domain") }}</span>
            <input v-model.trim="app.websiteForm.domain" :disabled="!app.canCreateWebsite" required />
          </label>
          <div class="form-actions">
            <button class="primary-button" :disabled="!app.canCreateWebsite">{{ app.websiteForm.id ? app.t("save") : app.t("create") }}</button>
            <button v-if="app.websiteForm.id" type="button" class="ghost-button" :disabled="!app.canCreateWebsite" @click="app.deleteWebsite">{{ app.t("deleteAction") }}</button>
            <button type="button" class="ghost-button" @click="app.resetWebsiteForm">{{ app.t("resetForm") }}</button>
          </div>
        </form>

        <section class="setting-card">
          <span>{{ app.t("trackerSnippet") }}</span>
          <pre>{{ app.trackerSnippet }}</pre>
        </section>
      </article>

      <article class="panel workspace-panel">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("websites") }}</h3>
            <p>{{ app.t("installationGuide") }}</p>
          </div>
        </div>

        <div v-if="app.state.websites.length" class="card-list">
          <button
            v-for="site in app.state.websites"
            :key="site.id"
            class="entity-card directory-item"
            :class="{ active: app.editingWebsite?.id === site.id }"
            @click="app.editWebsite(site)"
          >
            <div class="directory-main">
              <strong>{{ site.name }}</strong>
              <span>{{ site.domain }}</span>
            </div>
            <span class="editor-pill">{{ app.state.websiteId === site.id ? app.t("selectedWebsite") : app.t("edit") }}</span>
          </button>
        </div>
        <p v-else class="empty-note">{{ app.t("noData") }}</p>
      </article>
    </div>

    <article v-if="app.canReviewWebsiteMembers" class="panel workspace-panel">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("websiteMembers") }}</h3>
          <p>{{ app.t("websiteMembersText") }}</p>
        </div>
        <span class="editor-pill">
          {{ app.editingWebsite?.name || app.t("chooseWebsite") }}
        </span>
      </div>

      <div class="member-summary">
        <div class="member-stat">
          <span>{{ app.t("authorizedUsers") }}</span>
          <strong>{{ app.formatNumber(app.websiteMemberSummary.total) }}</strong>
        </div>
        <div class="member-stat">
          <span>{{ app.t("canManage") }}</span>
          <strong>{{ app.formatNumber(app.websiteMemberSummary.manage) }}</strong>
        </div>
        <div class="member-stat">
          <span>{{ app.t("canView") }}</span>
          <strong>{{ app.formatNumber(app.websiteMemberSummary.view) }}</strong>
        </div>
      </div>

      <div v-if="app.websiteMembers.length" class="member-list">
        <div v-for="member in app.websiteMembers" :key="member.id" class="member-item">
          <div class="member-main">
            <strong>{{ member.username }}</strong>
            <span>{{ app.roleLabel(member.role) }}</span>
          </div>
          <div class="member-meta">
            <span class="member-access" :class="member.accessLevel">{{ app.accessLevelLabel(member.accessLevel) }}</span>
            <span class="directory-state" :class="{ off: !member.enabled }">
              {{ member.enabled ? app.t("enabled") : app.t("disabled") }}
            </span>
          </div>
        </div>
      </div>
      <p v-else class="empty-note">{{ app.t("noAuthorizedUsers") }}</p>
    </article>
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

.editor-pill {
  padding: 0.38rem 0.7rem;
  border-radius: 999px;
  background: rgba(99, 102, 241, 0.1);
  color: var(--accent-strong);
  font-size: 0.78rem;
  font-weight: 700;
}

.directory-item {
  width: 100%;
  justify-content: space-between;
  text-align: left;
}

.directory-main {
  display: grid;
  gap: 0.15rem;
  min-width: 0;
}

.directory-main span {
  color: var(--muted);
  font-size: 0.8rem;
}

.member-summary {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0.75rem;
}

.member-stat {
  padding: 0.95rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.84), rgba(248, 250, 252, 0.76));
}

.member-stat span {
  display: block;
  color: var(--muted);
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.member-stat strong {
  display: block;
  margin-top: 0.35rem;
  font-size: 1.2rem;
}

.member-list {
  display: grid;
  gap: 0.7rem;
}

.member-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.95rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.78);
}

.member-main {
  display: grid;
  gap: 0.15rem;
}

.member-main span {
  color: var(--muted);
  font-size: 0.8rem;
}

.member-meta {
  display: flex;
  align-items: center;
  gap: 0.6rem;
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
  color: var(--accent-teal-deep);
  background: rgba(20, 184, 166, 0.12);
}

.directory-state {
  padding: 0.28rem 0.55rem;
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.1);
  color: var(--success);
  font-size: 0.74rem;
  font-weight: 700;
}

.directory-state.off {
  background: rgba(239, 68, 68, 0.08);
  color: var(--danger);
}

@media (max-width: 980px) {
  .compact-signal-grid,
  .member-summary {
    grid-template-columns: 1fr;
  }

  .member-item,
  .directory-item {
    flex-direction: column;
    align-items: stretch;
  }

  .member-meta {
    justify-content: flex-start;
  }
}
</style>
