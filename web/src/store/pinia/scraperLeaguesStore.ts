import { defineStore } from 'pinia'
import {
  ScraperLeagueService,
  type ScraperLeague,
  type ListFilters,
} from '../services/ScraperLeagueService'

export type { ScraperLeague, ListFilters }

export const useScraperLeaguesStore = defineStore('scraperLeagues', {
  state: () => ({
    items: [] as ScraperLeague[],
    total: 0,
    loading: false,
    error: null as string | null,
    filters: { page: 1, limit: 50 } as ListFilters,
    _service: null as ScraperLeagueService | null,
    // Monotonic counter: only the newest list request may write to the
    // store, so a slow response cannot overwrite a newer one.
    _requestSeq: 0,
  }),
  actions: {
    setService(svc: ScraperLeagueService) {
      this._service = svc
    },
    async fetch(filters: Partial<ListFilters> = {}) {
      this.filters = { ...this.filters, ...filters }
      this.loading = true
      this.error = null
      const seq = ++this._requestSeq
      try {
        const svc = this._service ?? new ScraperLeagueService()
        const res = await svc.list(this.filters)
        if (seq !== this._requestSeq) return
        this.items = res.data
        this.total = res.total
      } catch (e: any) {
        if (seq !== this._requestSeq) return
        this.error = e.message
      } finally {
        if (seq === this._requestSeq) this.loading = false
      }
    },
    async create(payload: Partial<ScraperLeague>) {
      const svc = this._service ?? new ScraperLeagueService()
      const created = await svc.create(payload)
      await this.fetch()
      return created
    },
    async update(id: number, fields: Partial<ScraperLeague>) {
      const svc = this._service ?? new ScraperLeagueService()
      await svc.update(id, fields)
      await this.fetch()
    },
    async remove(id: number) {
      const svc = this._service ?? new ScraperLeagueService()
      await svc.delete(id)
      await this.fetch()
    },
    async searchLeagues(q: string) {
      const svc = this._service ?? new ScraperLeagueService()
      return await svc.search(q)
    },
  },
})
