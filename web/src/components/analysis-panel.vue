<script setup>
defineProps({
  analysis: { type: Object, default: null },
  loading: { type: Boolean, default: false },
})
defineEmits(['analyze'])

function formatTime(iso) {
  if (!iso) return ''
  return new Date(iso).toLocaleString('ru-RU')
}
</script>

<template>
  <div class="analysis-panel">
    <div class="panel-header">
      <h3>AI-анализ</h3>
      <el-button
        type="primary"
        size="small"
        :loading="loading"
        @click="$emit('analyze')"
      >
        {{ analysis ? 'Переанализировать' : 'Анализировать' }}
      </el-button>
    </div>

    <div v-if="!analysis && !loading" class="empty">
      Анализ ещё не запускался для этого кластера.
    </div>

    <div v-else-if="analysis" class="content">
      <div class="meta">
        <span :class="['severity', `sev-${analysis.severity}`]">
          {{ analysis.severity }}
        </span>
        <span class="confidence">уверенность: {{ Math.round(analysis.confidence * 100) }}%</span>
        <span class="model">{{ analysis.model_used }}</span>
        <span class="time">{{ formatTime(analysis.created_at) }}</span>
      </div>

      <section>
        <h4>Сводка</h4>
        <p>{{ analysis.summary }}</p>
      </section>

      <section v-if="analysis.root_cause_hypothesis">
        <h4>Гипотеза причины</h4>
        <p>{{ analysis.root_cause_hypothesis }}</p>
      </section>

      <section v-if="analysis.suggested_actions?.length">
        <h4>Рекомендуемые действия</h4>
        <ol>
          <li v-for="(a, i) in analysis.suggested_actions" :key="i">{{ a }}</li>
        </ol>
      </section>
    </div>
  </div>
</template>

<style scoped>
.analysis-panel {
  background: var(--ll-bg-1);
  border: 1px solid var(--ll-border);
  border-radius: 4px;
  padding: 14px 16px;
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.panel-header h3 {
  margin: 0;
  font-size: 15px;
}

.empty {
  color: var(--ll-text-3);
  font-size: 13px;
  padding: 20px 0;
  text-align: center;
}

.meta {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 12px;
  color: var(--ll-text-2);
  margin-bottom: 12px;
  flex-wrap: wrap;
}

.severity {
  font-weight: 700;
  text-transform: uppercase;
  padding: 2px 8px;
  border-radius: 3px;
}
.sev-critical { background: var(--ll-danger); color: #fff; }
.sev-high     { background: var(--ll-warning); color: #000; }
.sev-medium   { background: var(--ll-accent); color: #fff; }
.sev-low      { background: var(--ll-bg-2); color: var(--ll-text-1); }

section {
  margin-bottom: 12px;
}
section h4 {
  font-size: 12px;
  text-transform: uppercase;
  color: var(--ll-text-3);
  letter-spacing: 0.5px;
  margin: 0 0 4px;
}
section p {
  margin: 0;
  color: var(--ll-text-1);
  font-size: 14px;
  line-height: 1.5;
}
section ol {
  margin: 0;
  padding-left: 20px;
  color: var(--ll-text-1);
  font-size: 14px;
}
section ol li {
  margin-bottom: 4px;
}
</style>
