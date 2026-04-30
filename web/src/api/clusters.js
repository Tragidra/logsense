import { api } from './client.js'

function buildClusterQuery(filter) {
  const params = {}
  if (filter.from) params.from = filter.from
  if (filter.to) params.to = filter.to
  if (filter.minPriority) params.min_priority = filter.minPriority
  if (filter.services?.length) params.services = filter.services.join(',')
  if (filter.levels?.length) params.levels = filter.levels.join(',')
  if (filter.search) params.search = filter.search
  if (filter.orderBy) params.order_by = filter.orderBy
  if (filter.limit) params.limit = filter.limit
  if (filter.offset) params.offset = filter.offset
  return params
}

export async function listClusters(filter = {}) {
  const { data } = await api.get('/clusters', { params: buildClusterQuery(filter) })
  return data
}

export async function getCluster(id) {
  const { data } = await api.get(`/clusters/${id}`)
  return data
}

export async function listClusterEvents(id, params = {}) {
  const { data } = await api.get(`/clusters/${id}/events`, { params })
  return data
}

export async function triggerAnalysis(id) {
  const { data } = await api.post(`/clusters/${id}/analyze`)
  return data.analysis
}
