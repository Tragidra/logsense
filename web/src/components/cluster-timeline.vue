<script setup>
import { computed, shallowRef } from 'vue'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { BarChart } from 'echarts/charts'
import {
  GridComponent,
  TooltipComponent,
  TitleComponent,
} from 'echarts/components'
import VChart from 'vue-echarts'

use([CanvasRenderer, BarChart, GridComponent, TooltipComponent, TitleComponent])

const props = defineProps({
  events: { type: Array, required: true },
  bucketSeconds: { type: Number, default: 60 },
})

const option = computed(() => {
  const bucket = props.bucketSeconds * 1000
  const buckets = new Map()
  for (const e of props.events) {
    const ts = new Date(e.timestamp).getTime()
    const b = Math.floor(ts / bucket) * bucket
    buckets.set(b, (buckets.get(b) || 0) + 1)
  }
  const keys = [...buckets.keys()].sort()
  const data = keys.map((k) => [k, buckets.get(k)])

  return {
    grid: { left: 36, right: 12, top: 16, bottom: 24 },
    tooltip: {
      trigger: 'axis',
      backgroundColor: '#1d222d',
      borderColor: '#2a2f3c',
      textStyle: { color: '#e4e6eb' },
      formatter: (p) => {
        const t = new Date(p[0].value[0]).toLocaleString('ru-RU')
        return `${t}<br/>${p[0].value[1]} событий`
      },
    },
    xAxis: {
      type: 'time',
      axisLine: { lineStyle: { color: '#2a2f3c' } },
      axisLabel: { color: '#9aa1b1', fontSize: 10 },
    },
    yAxis: {
      type: 'value',
      axisLine: { lineStyle: { color: '#2a2f3c' } },
      axisLabel: { color: '#9aa1b1', fontSize: 10 },
      splitLine: { lineStyle: { color: '#2a2f3c' } },
    },
    series: [
      {
        type: 'bar',
        data,
        itemStyle: { color: '#5e9cff' },
        barMaxWidth: 16,
      },
    ],
  }
})

const initOpts = shallowRef({ renderer: 'canvas' })
</script>

<template>
  <div class="timeline-wrap">
    <v-chart class="chart" :option="option" :init-options="initOpts" autoresize />
  </div>
</template>

<style scoped>
.timeline-wrap {
  width: 100%;
  height: 180px;
  background: var(--ll-bg-1);
  border: 1px solid var(--ll-border);
  border-radius: 4px;
  padding: 4px;
}
.chart {
  width: 100%;
  height: 100%;
}
</style>
