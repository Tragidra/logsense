<script setup>
import { onMounted } from 'vue'
import { useClustersStore } from '@/stores/clusters.js'
import FiltersBar from '@/components/filters-bar.vue'
import ClusterCard from '@/components/cluster-card.vue'

const clusters = useClustersStore()

onMounted(() => {
  clusters.fetchList()
})

function onFilterChange(patch) {
  clusters.setFilter(patch)
}
</script>

<template>
  <section class="dashboard">
    <div class="dashboard-header">
      <h2>Dashboard</h2>
      <span class="total" v-if="!clusters.loading">{{ clusters.total }} кластер(ов)</span>
    </div>

    <FiltersBar @change="onFilterChange" />

    <p v-if="clusters.loading" class="state-msg">Загрузка…</p>
    <p v-else-if="clusters.error" class="state-msg error">Ошибка: {{ clusters.error }}</p>
    <p v-else-if="!clusters.items.length" class="state-msg muted">Кластеры не найдены</p>

    <div v-else class="cluster-grid">
      <ClusterCard
        v-for="cluster in clusters.items"
        :key="cluster.id"
        :cluster="cluster"
      />
    </div>
  </section>
</template>

<style scoped>
.dashboard {
  padding: 0 4px;
}

.dashboard-header {
  display: flex;
  align-items: baseline;
  gap: 12px;
}
.dashboard-header h2 {
  margin: 0;
}
.total {
  font-size: 13px;
  color: var(--ll-text-2);
}

.state-msg {
  margin-top: 32px;
  text-align: center;
  font-size: 14px;
}
.error  { color: var(--ll-danger); }
.muted  { color: var(--ll-text-3); }

.cluster-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(360px, 1fr));
  gap: 12px;
  margin-top: 4px;
}
</style>
