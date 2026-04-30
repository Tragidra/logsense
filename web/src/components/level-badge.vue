<script setup>
defineProps({
  level: { type: String, required: true },
})

const COLOR_MAP = {
  fatal: 'danger',
  error: 'danger',
  warn: 'warning',
  info: 'success',
  debug: 'info',
  trace: 'info',
}
</script>

<template>
  <el-tag :type="COLOR_MAP[level] || 'info'" size="small" effect="dark">
    {{ level.toUpperCase() }}
  </el-tag>
</template>
