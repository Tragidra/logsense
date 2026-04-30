import axios from 'axios'

export const api = axios.create({
  baseURL: import.meta.env.VITE_API_BASE_URL || '/api',
  timeout: 30000,
})

api.interceptors.response.use(
  (r) => r,
  (err) => {
    // eslint-disable-next-line no-console
    console.error('[api]', err.response?.data || err.message)
    return Promise.reject(err)
  },
)
