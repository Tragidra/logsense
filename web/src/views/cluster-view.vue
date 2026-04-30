<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useClustersStore } from '@/stores/clusters.js'
import LevelBadge from '@/components/level-badge.vue'
import FlagBadge from '@/components/flag-badge.vue'
import ClusterTimeline from '@/components/cluster-timeline.vue'
import AnalysisPanel from '@/components/analysis-panel.vue'

const props = defineProps({
  id: { type: String, required: true },
})

const clusters = useClustersStore()
const analyzing = ref(false)

function load() {
  clusters.fetchOne(props.id)
  clusters.fetchEvents(props.id, { limit: 200 })
}

onMounted(load)
watch(() => props.id, load)

async function runAnalysis() {
  analyzing.value = true
  try {
    await clusters.requestAnalysis(props.id)
  } catch (err) {
    clusters.error = err.message || String(err)
  } finally {
    analyzing.value = false
  }
}

const c = computed(() => clusters.current)

const dominantLevel = computed(() => {
  const levels = c.value?.levels || {}
  let best = 'info'
  let max = -1
  for (const [k, v] of Object.entries(levels)) {
    if (v > max) { max = v; best = k }
  }
  return best
})

const levelSummary = computed(() => {
  const levels = c.value?.levels || {}
  return Object.entries(levels)
    .sort((a, b) => b[1] - a[1])
    .map(([k, v]) => `${k}=${v}`)
    .join(' · ')
})

function formatTime(iso) {
  if (!iso) return '—'
  return new Date(iso).toLocaleString('ru-RU')
}

function priorityClass(p) {
  if (p >= 75) return 'priority-critical'
  if (p >= 50) return 'priority-high'
  if (p >= 25) return 'priority-medium'
  return 'priority-low'
}
</script>

<template>
  <section class="cluster-view">
    <div class="nav">
      <RouterLink to="/dashboard">← Dashboard</RouterLink>
    </div>

    <p v-if="!c && !clusters.error">Загрузка…</p>
    <p v-else-if="clusters.error" class="error">Ошибка: {{ clusters.error }}</p>

    <template v-else-if="c">
      <header class="cluster-header">
        <div class="header-top">
          <span :class="['priority-badge', priorityClass(c.priority)]">{{ c.priority }}</span>
          <LevelBadge :level="dominantLevel" />
          <h2 class="template">{{ c.template }}</h2>
        </div>
        <div class="header-meta">
          <span>{{ c.count }} events</span>
          <span>{{ levelSummary }}</span>
          <span>services: {{ (c.services || []).join(', ') || '—' }}</span>
          <span>first: {{ formatTime(c.first_seen) }}</span>
          <span>last: {{ formatTime(c.last_seen) }}</span>
        </div>
        <div v-if="c.anomaly_flags?.length" class="flags">
          <FlagBadge v-for="flag in c.anomaly_flags" :key="flag" :flag="flag" />
        </div>
      </header>

      <section class="block">
        <h3 class="block-title">Timeline</h3>
        <ClusterTimeline
          v-if="clusters.currentEvents.length"
          :events="clusters.currentEvents"
        />
        <p v-else class="muted">Нет событий в выбранном окне.</p>
      </section>

      <section class="block">
        <AnalysisPanel
          :analysis="c.analysis"
          :loading="analyzing"
          @analyze="runAnalysis"
        />
      </section>

      <section class="block">
        <h3 class="block-title">Примеры логов</h3>
        <ul v-if="c.examples?.length" class="examples">
          <li v-for="(ex, i) in c.examples" :key="i">
            <code>{{ ex }}</code>
          </li>
        </ul>
        <p v-else class="muted">Нет примеров.</p>
      </section>

      <section class="block">
        <h3 class="block-title">Последние события ({{ clusters.currentEvents.length }})</h3>
        <div class="events">
          <div v-for="e in clusters.currentEvents.slice(0, 50)" :key="e.id" class="event-row">
            <span class="event-time">{{ formatTime(e.timestamp) }}</span>
            <LevelBadge :level="e.level || 'info'" />
            <span class="event-service">{{ e.service || '—' }}</span>
            <code class="event-msg">{{ e.message }}</code>
          </div>
        </div>
      </section>
    </template>
  </section>
</template>

<style scoped>
.cluster-view {
  padding: 0 4px;
}

.nav {
  font-size: 13px;
  margin-bottom: 8px;
}

.error {
  color: var(--ll-danger);
}

.cluster-header {
  background: var(--ll-bg-1);
  border: 1px solid var(--ll-border);
  border-radius: 4px;
  padding: 14px 16px;
  margin-bottom: 16px;
}

.header-top {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}

.priority-badge {
  font-size: 14px;
  font-weight: 700;
  min-width: 38px;
  text-align: center;
  border-radius: 4px;
  padding: 2px 8px;
}
.priority-critical { background: var(--ll-danger); color: #fff; }
.priority-high     { background: var(--ll-warning); color: #000; }
.priority-medium   { background: var(--ll-accent); color: #fff; }
.priority-low      { background: var(--ll-bg-2); color: var(--ll-text-2); }

.template {
  font-family: 'Menlo', 'Consolas', monospace;
  font-size: 15px;
  margin: 0;
  color: var(--ll-text-1);
  word-break: break-all;
}

.header-meta {
  display: flex;
  gap: 16px;
  flex-wrap: wrap;
  font-size: 12px;
  color: var(--ll-text-2);
  margin-bottom: 6px;
}

.flags {
  display: flex;
  gap: 4px;
  flex-wrap: wrap;
}

.block {
  margin-bottom: 16px;
}
.block-title {
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--ll-text-3);
  margin: 0 0 6px;
}

.muted {
  color: var(--ll-text-3);
  font-size: 13px;
  margin: 0;
}

.examples {
  list-style: none;
  padding: 0;
  margin: 0;
}
.examples li {
  margin-bottom: 4px;
}
.examples code {
  display: block;
  background: var(--ll-bg-1);
  border: 1px solid var(--ll-border);
  border-radius: 3px;
  padding: 4px 8px;
  font-size: 12px;
  color: var(--ll-text-1);
  word-break: break-all;
}

.events {
  background: var(--ll-bg-1);
  border: 1px solid var(--ll-border);
  border-radius: 4px;
  max-height: 420px;
  overflow-y: auto;
}
.event-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 5px 10px;
  border-bottom: 1px solid var(--ll-border);
  font-size: 12px;
}
.event-row:last-child {
  border-bottom: none;
}
.event-time {
  color: var(--ll-text-3);
  font-family: 'Menlo', monospace;
  flex-shrink: 0;
}
.event-service {
  color: var(--ll-text-2);
  flex-shrink: 0;
}
.event-msg {
  color: var(--ll-text-1);
  font-family: 'Menlo', monospace;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex: 1;
}
</style>
