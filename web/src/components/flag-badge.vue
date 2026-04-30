<script setup>
defineProps({
  flag: { type: String, required: true },
})

const LABEL_MAP = {
  burst: 'Burst',
  novel: 'New',
  rare: 'Rare',
  'cross-service': 'Cross-svc',
}
</script>

<template>
  <el-tag type="warning" size="small" effect="plain" class="flag-badge">
    {{ LABEL_MAP[flag] || flag }}
  </el-tag>
</template>

<style scoped>
.flag-badge {
  font-size: 11px;
}
</style>
