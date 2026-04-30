<script setup>
import { reactive } from 'vue'

const emit = defineEmits(['change'])

const TIME_OPTIONS = [
  { label: 'Последние 15 мин', value: 15 },
  { label: 'Последний час', value: 60 },
  { label: 'Последние 6 ч', value: 360 },
  { label: 'Последние 24 ч', value: 1440 },
  { label: 'Все время', value: null },
]

const PRIORITY_OPTIONS = [
  { label: 'Любой', value: 0 },
  { label: '≥ 25', value: 25 },
  { label: '≥ 50', value: 50 },
  { label: '≥ 75', value: 75 },
]

const f = reactive({
  timeRange: null,
  minPriority: 0,
  services: [],
  search: '',
})

function apply() {
  const now = new Date()
  const patch = {
    minPriority: f.minPriority,
    services: f.services,
    search: f.search,
    from: f.timeRange ? new Date(now - f.timeRange * 60000).toISOString() : null,
    to: null,
  }
  emit('change', patch)
}

function reset() {
  f.timeRange = null
  f.minPriority = 0
  f.services = []
  f.search = ''
  apply()
}
</script>

<template>
  <div class="filters-bar">
    <el-select v-model="f.timeRange" placeholder="Период" size="small" style="width:160px" clearable @change="apply">
      <el-option
        v-for="opt in TIME_OPTIONS"
        :key="String(opt.value)"
        :label="opt.label"
        :value="opt.value"
      />
    </el-select>

    <el-select v-model="f.minPriority" placeholder="Приоритет ≥" size="small" style="width:130px" @change="apply">
      <el-option
        v-for="opt in PRIORITY_OPTIONS"
        :key="opt.value"
        :label="opt.label"
        :value="opt.value"
      />
    </el-select>

    <el-input
      v-model="f.search"
      placeholder="Поиск по шаблону…"
      size="small"
      style="width:240px"
      clearable
      @input="apply"
    />

    <el-button size="small" @click="reset">Сбросить</el-button>
  </div>
</template>

<style scoped>
.filters-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 12px 0;
}
</style>
