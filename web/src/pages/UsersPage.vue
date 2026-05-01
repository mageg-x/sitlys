<template>
  <section class="admin-shell">
    <article class="panel admin-directory">
      <div class="panel-head">
        <div>
          <h3>{{ app.t("userDirectory") }}</h3>
          <p>{{ app.t("userManagementBriefingText") }}</p>
        </div>
        <button class="primary-button" @click="app.resetUserForm">{{ app.t("createUser") }}</button>
      </div>

      <div class="directory-summary">
        <div class="mini-stat">
          <span>{{ app.t("users") }}</span>
          <strong>{{ app.formatNumber(app.state.users.length) }}</strong>
        </div>
        <div class="mini-stat">
          <span>{{ app.t("websites") }}</span>
          <strong>{{ app.formatNumber(app.state.websites.length) }}</strong>
        </div>
      </div>

      <div v-if="app.state.users.length" class="directory-list">
        <button
          v-for="user in app.state.users"
          :key="user.id"
          class="directory-item"
          :class="{ active: app.userForm.id === user.id }"
          @click="app.editUser(user)"
        >
          <div class="directory-main">
            <strong>{{ user.username }}</strong>
            <span>{{ app.roleLabel(user.role) }}</span>
          </div>
          <span class="directory-state" :class="{ off: !user.enabled }">
            {{ user.enabled ? app.t("enabled") : app.t("disabled") }}
          </span>
        </button>
      </div>
      <p v-else class="empty-note">{{ app.t("noData") }}</p>
    </article>

    <div class="admin-detail-stack">
      <article class="panel admin-explainer">
        <div class="panel-head">
          <div>
            <h3>{{ app.t("roleMatrix") }}</h3>
            <p>{{ app.t("roleText") }}</p>
          </div>
        </div>
        <div class="role-guide">
          <div v-for="card in app.roleCards" :key="card.key" class="role-guide-card">
            <strong>{{ card.label }}</strong>
            <p>{{ card.text }}</p>
          </div>
        </div>
      </article>

      <article class="panel admin-editor">
        <div class="panel-head">
          <div>
            <h3>{{ app.userForm.id ? app.t("updateUser") : app.t("createUser") }}</h3>
            <p>{{ app.t("permissionText") }}</p>
          </div>
          <span class="editor-pill">
            {{ app.userForm.username || app.t("selectedUser") }}
          </span>
        </div>

        <div class="editor-summary">
          <div class="summary-tile">
            <span>{{ app.t("authorizedWebsites") }}</span>
            <strong>{{ app.formatNumber(app.userPermissionSummary.assigned) }}</strong>
          </div>
          <div class="summary-tile">
            <span>{{ app.t("manageWebsites") }}</span>
            <strong>{{ app.formatNumber(app.userPermissionSummary.manage) }}</strong>
          </div>
          <div class="summary-tile">
            <span>{{ app.t("viewWebsites") }}</span>
            <strong>{{ app.formatNumber(app.userPermissionSummary.view) }}</strong>
          </div>
          <div class="summary-tile">
            <span>{{ app.t("noAccessWebsites") }}</span>
            <strong>{{ app.formatNumber(app.userPermissionSummary.none) }}</strong>
          </div>
        </div>

        <form class="stack-form" @submit.prevent="app.saveUser">
          <div class="editor-grid">
            <label>
              <span>{{ app.t("username") }}</span>
              <input v-model.trim="app.userForm.username" required />
            </label>
            <label>
              <span>{{ app.t("password") }}</span>
              <input v-model="app.userForm.password" type="password" :required="!app.userForm.id" minlength="8" />
            </label>
          </div>

          <div class="editor-grid">
            <label>
              <span>{{ app.t("role") }}</span>
              <select v-model="app.userForm.role">
                <option value="super_admin">{{ app.t("superAdmin") }}</option>
                <option value="admin">{{ app.t("admin") }}</option>
                <option value="analyst">{{ app.t("analyst") }}</option>
                <option value="viewer">{{ app.t("viewer") }}</option>
              </select>
            </label>
            <label class="switch-card">
              <span>{{ app.t("status") }}</span>
              <div class="switch-line">
                <input v-model="app.userForm.enabled" type="checkbox" />
                <strong>{{ app.userForm.enabled ? app.t("enabled") : app.t("disabled") }}</strong>
              </div>
            </label>
          </div>

          <section v-if="app.userForm.role !== 'super_admin'" class="permission-board">
            <div class="permission-head">
              <h4>{{ app.t("permissions") }}</h4>
              <p>{{ app.t("permissionText") }}</p>
            </div>
            <div class="permission-summary">
              <span>{{ app.t("selectedUser") }}: <strong>{{ app.userForm.username || app.t("noData") }}</strong></span>
              <span>{{ app.t("permissions") }}: <strong>{{ app.t("authorizedSitesSummary", { count: app.userPermissionSummary.assigned, manage: app.userPermissionSummary.manage, view: app.userPermissionSummary.view }) }}</strong></span>
            </div>
            <div v-if="app.state.websites.length" class="permission-list">
              <div v-for="site in app.state.websites" :key="site.id" class="permission-item">
                <div class="permission-site">
                  <strong>{{ site.name }}</strong>
                  <span>{{ site.domain }}</span>
                </div>
                <div class="permission-actions">
                  <div class="access-segment">
                    <button
                      type="button"
                      class="segment-button"
                      :class="{ active: app.accessLevel(site.id) === 'none' }"
                      @click="app.setAccessLevel(site.id, 'none')"
                    >
                      {{ app.t("noAccess") }}
                    </button>
                    <button
                      type="button"
                      class="segment-button"
                      :class="{ active: app.accessLevel(site.id) === 'view' }"
                      @click="app.setAccessLevel(site.id, 'view')"
                    >
                      {{ app.t("canView") }}
                    </button>
                    <button
                      type="button"
                      class="segment-button"
                      :class="{ active: app.accessLevel(site.id) === 'manage' }"
                      @click="app.setAccessLevel(site.id, 'manage')"
                    >
                      {{ app.t("canManage") }}
                    </button>
                  </div>
                </div>
              </div>
            </div>
            <p v-else class="empty-note">{{ app.t("noData") }}</p>
          </section>

          <div v-else class="super-admin-note">
            <strong>{{ app.t("superAdmin") }}</strong>
            <p>{{ app.t("allWebsites") }}</p>
          </div>

          <div class="form-actions">
            <button class="primary-button">{{ app.userForm.id ? app.t("savePermissions") : app.t("create") }}</button>
            <button type="button" class="ghost-button" @click="app.resetUserForm">{{ app.t("resetForm") }}</button>
          </div>
        </form>
      </article>
    </div>
  </section>
</template>

<script setup>
import { useAppController } from "../composables/use-app-controller";

const app = useAppController();
</script>

<style scoped>
.admin-shell {
  display: grid;
  grid-template-columns: minmax(18rem, 23rem) minmax(0, 1fr);
  gap: 1rem;
}

.admin-directory,
.admin-editor,
.admin-explainer {
  display: grid;
  gap: 1rem;
  align-content: start;
}

.admin-detail-stack {
  display: grid;
  gap: 1rem;
}

.directory-summary {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}

.mini-stat {
  padding: 0.9rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.66);
}

.mini-stat span {
  display: block;
  color: var(--muted);
  font-size: 0.74rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.mini-stat strong {
  display: block;
  margin-top: 0.35rem;
  font-size: 1.25rem;
}

.directory-list {
  display: grid;
  gap: 0.55rem;
}

.directory-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  padding: 0.95rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.7);
  text-align: left;
}

.directory-item.active {
  border-color: rgba(99, 102, 241, 0.28);
  background: linear-gradient(135deg, rgba(99, 102, 241, 0.12), rgba(20, 184, 166, 0.06));
}

.directory-main {
  display: grid;
  gap: 0.15rem;
}

.directory-main strong {
  font-size: 0.92rem;
}

.directory-main span {
  color: var(--muted);
  font-size: 0.8rem;
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

.editor-pill {
  padding: 0.38rem 0.7rem;
  border-radius: 999px;
  background: rgba(99, 102, 241, 0.1);
  color: var(--accent-strong);
  font-size: 0.78rem;
  font-weight: 700;
  white-space: nowrap;
}

.role-guide {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.75rem;
}

.role-guide-card {
  padding: 0.9rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.7);
}

.role-guide-card strong {
  display: block;
  margin-bottom: 0.3rem;
  color: var(--accent-strong);
}

.role-guide-card p {
  margin: 0;
  color: var(--muted);
  font-size: 0.8rem;
  line-height: 1.6;
}

.editor-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1rem;
}

.editor-summary {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 0.75rem;
}

.summary-tile {
  padding: 0.95rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.84), rgba(248, 250, 252, 0.76));
}

.summary-tile span {
  display: block;
  color: var(--muted);
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.08em;
}

.summary-tile strong {
  display: block;
  margin-top: 0.35rem;
  font-size: 1.2rem;
  color: var(--ink);
}

.switch-card {
  padding: 0.85rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.66);
}

.permission-board {
  display: grid;
  gap: 1rem;
  padding: 1rem;
  border-radius: var(--radius);
  border: 1px solid var(--line);
  background: rgba(248, 250, 252, 0.76);
}

.permission-head h4 {
  margin: 0;
  font-size: 0.96rem;
}

.permission-head p,
.super-admin-note p {
  margin: 0.2rem 0 0;
  color: var(--muted);
}

.permission-summary {
  display: flex;
  gap: 1rem;
  flex-wrap: wrap;
  color: var(--muted);
  font-size: 0.8rem;
}

.permission-list {
  display: grid;
  gap: 0.75rem;
}

.permission-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 1rem;
  padding: 0.9rem 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.8);
}

.permission-site {
  display: grid;
  gap: 0.12rem;
  min-width: 0;
}

.permission-site span {
  color: var(--muted);
  font-size: 0.78rem;
}

.permission-actions {
  display: flex;
  justify-content: flex-end;
  flex: 1 1 24rem;
}

.access-segment {
  display: inline-flex;
  width: 100%;
  padding: 0.2rem;
  border-radius: 999px;
  border: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.82);
}

.segment-button {
  flex: 1 1 0;
  border: none;
  background: transparent;
  border-radius: 999px;
  padding: 0.45rem 0.7rem;
  color: var(--muted);
  font-size: 0.76rem;
  font-weight: 600;
  text-align: center;
  white-space: nowrap;
}

.segment-button.active {
  background: rgba(99, 102, 241, 0.12);
  color: var(--accent-strong);
}

.super-admin-note {
  padding: 1rem;
  border-radius: var(--radius-sm);
  border: 1px solid rgba(16, 185, 129, 0.18);
  background: rgba(16, 185, 129, 0.06);
}

@media (max-width: 980px) {
  .admin-shell,
  .editor-grid,
  .role-guide,
  .editor-summary {
    grid-template-columns: 1fr;
  }

  .permission-item {
    flex-direction: column;
    align-items: stretch;
  }

  .permission-summary {
    flex-direction: column;
    gap: 0.35rem;
  }

  .permission-actions {
    width: 100%;
    flex: none;
  }

  .access-segment {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    border-radius: var(--radius-sm);
  }
}

@media (max-width: 640px) {
  .segment-button {
    padding: 0.55rem 0.4rem;
    font-size: 0.72rem;
  }
}
</style>
