<template>
  <p v-if="!rows.length" class="empty-note">{{ emptyText }}</p>
  <div v-else class="table-wrap">
    <table class="data-table">
      <thead>
        <tr>
          <th v-for="column in columns" :key="column.key">{{ column.label }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(row, index) in rows" :key="row.id || row.path || row.referrer || row.name || row.source || index">
          <td v-for="column in columns" :key="column.key">
            {{ column.format ? column.format(row) : row[column.key] ?? "-" }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup>
defineProps({
  rows: { type: Array, default: () => [] },
  columns: { type: Array, default: () => [] },
  emptyText: { type: String, default: "" },
});
</script>

<style scoped>
.table-wrap {
  overflow-x: auto;
  border-radius: var(--radius);
  border: 1px solid var(--line);
  box-shadow: var(--shadow-xs);
  background: rgba(255, 255, 255, 0.86);
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  min-width: 34rem;
}

.data-table th, .data-table td {
  padding: 0.85rem 1rem;
  border-bottom: 1px solid var(--line);
  text-align: left;
  vertical-align: middle;
  font-size: 0.875rem;
}

.data-table th {
  color: var(--muted);
  font-size: 0.72rem;
  font-weight: 700;
  text-transform: uppercase;
  letter-spacing: 0.06em;
  background: linear-gradient(180deg, var(--bg-strong) 0%, var(--bg) 100%);
  padding: 0.75rem 1rem;
  position: sticky;
  top: 0;
  z-index: 1;
}

.data-table tbody tr:nth-child(even) td {
  background: rgba(99, 102, 241, 0.015);
}

.data-table tbody tr {
  transition: all var(--transition-fast) ease;
}

.data-table tbody tr:hover td {
  background: rgba(99, 102, 241, 0.05);
}

.data-table tbody tr:last-child td {
  border-bottom: none;
}

.empty-note {
  margin: 0;
  color: var(--muted-light);
  font-size: 0.85rem;
  padding: 1.75rem 1.25rem;
  text-align: center;
  background: linear-gradient(180deg, rgba(248,250,252,0.9), rgba(255,255,255,0.78));
  border-radius: var(--radius-xs);
  border: 1px dashed var(--line-strong);
}
</style>
