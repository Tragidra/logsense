<script setup>
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import LevelBadge from './level-badge.vue'
import FlagBadge from './flag-badge.vue'

const props = defineProps({
  cluster: { type: Object, required: true },
})

const router = useRouter()

function open() {
  router.push(`/clusters/${props.cluster.id}`)
}

function priorityClass(p) {
  if (p >= 75) return 'priority-critical'
  if (p >= 50) return 'priority-high'
  if (p >= 25) return 'priority-medium'
  return 'priority-low'
}

function formatTime(iso) {
  if (!iso) return '—'
  const d = new Date(iso)
  return d.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
}

const dominantLevel = computed(() => {
  const levels = props.cluster.levels || {}
  let best = 'info'
  let max = -1
  for (const [k, v] of Object.entries(levels)) {
    if (v > max) { max = v; best = k }
  }
  return best
})
</script>

<template>
  <el-card class="cluster-card" @click="open" shadow="hover">
    <div class="card-header">
      <span :class="['priority-badge', priorityClass(cluster.priority)]">
        {{ cluster.priority }}
      </span>
      <LevelBadge :level="dominantLevel" />
      <span class="service">{{ cluster.services?.[0] || '—' }}</span>
      <span class="count">{{ cluster.count }} events</span>
    </div>

    <p class="template-text">{{ cluster.template }}</p>

    <div v-if="cluster.anomaly_flags?.length" class="flags">
      <FlagBadge v-for="flag in cluster.anomaly_flags" :key="flag" :flag="flag" />
    </div>

    <div class="card-footer">
      <span class="time">{{ formatTime(cluster.last_seen) }}</span>
      <span v-if="cluster.analysis?.severity" :class="['severity', `sev-${cluster.analysis.severity}`]">
        {{ cluster.analysis.severity }}
      </span>
    </div>
  </el-card>
</template>

<style scoped>
.cluster-card {
  cursor: pointer;
  border-left: 3px solid var(--ll-border);
  transition: border-color 0.15s;
}
.cluster-card:hover {
  border-left-color: var(--ll-accent);
}

.card-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.priority-badge {
  font-size: 13px;
  font-weight: 700;
  min-width: 32px;
  text-align: center;
  border-radius: 4px;
  padding: 1px 6px;
}
.priority-critical { background: var(--ll-danger); color: #fff; }
.priority-high     { background: var(--ll-warning); color: #000; }
.priority-medium   { background: var(--ll-accent); color: #fff; }
.priority-low      { background: var(--ll-bg-2); color: var(--ll-text-2); }

.service {
  font-size: 12px;
  color: var(--ll-text-2);
  margin-left: auto;
}
.count {
  font-size: 12px;
  color: var(--ll-text-3);
}

.template-text {
  font-family: 'Menlo', 'Consolas', monospace;
  font-size: 13px;
  color: var(--ll-text-1);
  margin: 4px 0 8px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.flags {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
  margin-bottom: 8px;
}

.card-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.time {
  font-size: 11px;
  color: var(--ll-text-3);
}
.severity {
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
}
.sev-critical { color: var(--ll-danger); }
.sev-high     { color: var(--ll-warning); }
.sev-medium   { color: var(--ll-accent); }
.sev-low      { color: var(--ll-success); }
</style>
