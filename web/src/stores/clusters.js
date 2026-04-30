import { defineStore } from 'pinia'
import {
  listClusters,
  getCluster,
  triggerAnalysis,
  listClusterEvents,
} from '@/api/clusters.js'

export const useClustersStore = defineStore('clusters', {
  state: () => ({
    items: [],
    total: 0,
    loading: false,
    error: null,
    filter: {
      from: null,
      to: null,
      minPriority: 0,
      services: [],
      levels: [],
      search: '',
      orderBy: 'priority_desc',
      limit: 50,
      offset: 0,
    },
    current: null,
    currentEvents: [],
  }),
  actions: {
    async fetchList() {
      this.loading = true
      this.error = null
      try {
        const { items, total } = await listClusters(this.filter)
        this.items = items || []
        this.total = total || 0
      } catch (err) {
        this.error = err.message || String(err)
      } finally {
        this.loading = false
      }
    },
    async fetchOne(id) {
      this.error = null
      try {
        this.current = await getCluster(id)
      } catch (err) {
        this.error = err.message || String(err)
      }
    },
    async fetchEvents(id, params) {
      try {
        const { items } = await listClusterEvents(id, params)
        this.currentEvents = items || []
      } catch (err) {
        this.error = err.message || String(err)
      }
    },
    async requestAnalysis(id) {
      const analysis = await triggerAnalysis(id)
      if (this.current?.id === id) {
        this.current.analysis = analysis
      }
      return analysis
    },
    setFilter(patch) {
      this.filter = { ...this.filter, ...patch, offset: 0 }
      return this.fetchList()
    },
  },
})
