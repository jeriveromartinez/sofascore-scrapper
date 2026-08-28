import { reactive, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

const MIN_SIZE = 1;
const MAX_SIZE = 100;
const DEFAULT_SIZE = 20;

export interface CursorPaginationFetchResult<T> {
  data: T[];
  nextCursor: string;
  hasMore: boolean;
}

export interface UseCursorPaginationOptions<T> {
  routeName: string;
  defaultSize?: number;
  filters?: () => Record<string, unknown>;
  fetchPage: (cursor: string | undefined, size: number) => Promise<CursorPaginationFetchResult<T>>;
}

export interface CursorPaginationState<T> {
  data: T[];
  loading: boolean;
  error: string;
  page: number;
  size: number;
  hasNext: boolean;
  hasPrev: boolean;
}

function stableHash(obj: Record<string, unknown>): string {
  const keys = Object.keys(obj).sort();
  return keys.map((k) => `${k}=${JSON.stringify(obj[k])}`).join("&") || "default";
}

function cacheKey(routeName: string, filterHash: string, size: number, page: number): string {
  return `pagination:${routeName}:${filterHash}:${size}:page-${page}`;
}

function cacheKeyPrefix(routeName: string, filterHash: string, size: number): string {
  return `pagination:${routeName}:${filterHash}:${size}:page-`;
}

function clampInt(value: unknown, min: number, max: number, fallback: number): number {
  const n = Number(value);
  if (!Number.isFinite(n)) return fallback;
  const i = Math.trunc(n);
  if (i < min) return min;
  if (i > max) return max;
  return i;
}

export function useCursorPagination<T>(options: UseCursorPaginationOptions<T>) {
  const route = useRoute();
  const router = useRouter();
  const defaultSize = options.defaultSize ?? DEFAULT_SIZE;

  const state = reactive<CursorPaginationState<T>>({
    data: [],
    loading: false,
    error: "",
    page: 1,
    size: defaultSize,
    hasNext: false,
    hasPrev: false,
  });

  let abortController: AbortController | null = null;
  let lastLoadedFullPath = "";

  function getFilters(): Record<string, unknown> {
    return options.filters ? options.filters() : {};
  }

  function clearCacheForFilterSet(filterHash: string): void {
    const prefix = cacheKeyPrefix(options.routeName, filterHash, state.size);
    for (let i = sessionStorage.length - 1; i >= 0; i--) {
      const k = sessionStorage.key(i);
      if (k && k.startsWith(prefix)) {
        sessionStorage.removeItem(k);
      }
    }
  }

  function syncSizeFromQuery(): number {
    return clampInt(route.query.size, MIN_SIZE, MAX_SIZE, defaultSize);
  }

  function syncPageFromQuery(): number {
    return clampInt(route.query.page, 1, Number.MAX_SAFE_INTEGER, 1);
  }

  // Read filter values from the current URL query (excluding page and
  // size). Used by setFilters to compute the cache key for the
  // PREVIOUS filter set — the caller updates its local filters ref
  // before invoking setFilters, so getFilters() no longer reflects
  // the old value. Also used by the URL watcher to detect
  // filter-only changes (deep-link, back/forward).
  function readFiltersFromQuery(): Record<string, unknown> {
    const filters: Record<string, unknown> = {};
    for (const [k, v] of Object.entries(route.query)) {
      if (k === "page" || k === "size") continue;
      if (typeof v === "string") filters[k] = v;
    }
    return filters;
  }

  async function writeQuery(page: number, size: number, filters: Record<string, unknown>): Promise<void> {
    const next: Record<string, string> = {};
    for (const [k, v] of Object.entries(route.query)) {
      if (typeof v === "string") next[k] = v;
    }
    next.page = String(page);
    next.size = String(size);
    for (const [k, v] of Object.entries(filters)) {
      if (v === undefined || v === null || v === "") continue;
      next[k] = String(v);
    }
    await router.replace({ query: next });
  }

  async function fetchWith(cursor: string | undefined, size: number): Promise<CursorPaginationFetchResult<T>> {
    abortController?.abort();
    abortController = new AbortController();
    try {
      return await options.fetchPage(cursor, size);
    } finally {
      abortController = null;
    }
  }

  async function loadPage(page?: number): Promise<void> {
    state.error = "";
    const clampedSize = syncSizeFromQuery();
    const targetPage = page ?? syncPageFromQuery();
    const filters = getFilters();
    state.size = clampedSize;
    state.page = targetPage;

    const rawSize = String(route.query.size ?? "");
    const rawPage = String(route.query.page ?? "");
    if (rawSize !== String(clampedSize) || rawPage !== String(targetPage)) {
      await writeQuery(state.page, state.size, filters);
    }
    lastLoadedFullPath = route.fullPath;

    state.loading = true;
    try {
      const filterHash = stableHash(filters);
      const k = (n: number) => cacheKey(options.routeName, filterHash, state.size, n);

      let cursor: string | undefined;
      let cached: string | null = null;
      if (targetPage > 1) {
        cached = sessionStorage.getItem(k(targetPage - 1));
      }

      if (cached !== null) {
        const resp = await fetchWith(cached, state.size);
        state.data = resp.data as never;
        state.hasNext = resp.hasMore;
        if (resp.nextCursor) {
          sessionStorage.setItem(k(targetPage), resp.nextCursor);
        }
      } else {
        let lastResp: CursorPaginationFetchResult<T> | null = null;
        let reachedPage = 0;
        for (let i = 1; i <= targetPage; i++) {
          lastResp = await fetchWith(cursor, state.size);
          reachedPage = i;
          if (lastResp.nextCursor) {
            sessionStorage.setItem(k(i), lastResp.nextCursor);
          }
          if (lastResp.hasMore) {
            cursor = lastResp.nextCursor;
          } else {
            break;
          }
        }
        if (reachedPage < targetPage) {
          state.page = reachedPage;
          await writeQuery(state.page, state.size, filters);
        }
        if (lastResp) {
          state.data = lastResp.data as never;
          state.hasNext = lastResp.hasMore;
        }
      }
      state.hasPrev = targetPage > 1;
    } catch (err) {
      if ((err as { name?: string })?.name === "AbortError") return;
      state.error = err instanceof Error ? err.message : "Error cargando datos";
      clearCacheForFilterSet(stableHash(filters));
    } finally {
      state.loading = false;
    }
  }

  async function goNext(): Promise<void> {
    if (!state.hasNext) return;
    state.page += 1;
    state.hasPrev = true;
    await writeQuery(state.page, state.size, getFilters());
    await loadPage(state.page);
  }

  async function goPrev(): Promise<void> {
    if (state.page <= 1) return;
    state.page -= 1;
    await writeQuery(state.page, state.size, getFilters());
    await loadPage(state.page);
  }

  async function setSize(n: number): Promise<void> {
    const clamped = clampInt(n, MIN_SIZE, MAX_SIZE, defaultSize);
    clearCacheForFilterSet(stableHash(getFilters()));
    state.size = clamped;
    state.page = 1;
    state.hasPrev = false;
    await writeQuery(state.page, state.size, getFilters());
    await loadPage(state.page);
  }

  async function reload(): Promise<void> {
    await loadPage(state.page);
  }

  async function setFilters(newFilters: Record<string, unknown>): Promise<void> {
    // The caller (e.g. events.vue:48-50) updates its local filters
    // ref BEFORE invoking setFilters, so getFilters() now returns the
    // NEW filter set. The cache we need to clear belongs to the OLD
    // filter set, which is still in the URL.
    const oldFilters = readFiltersFromQuery();
    clearCacheForFilterSet(stableHash(oldFilters));
    state.page = 1;
    state.hasPrev = false;
    state.hasNext = false;
    await writeQuery(state.page, state.size, newFilters);
    await loadPage(1);
  }

  watch(
    () => route.fullPath,
    () => {
      if (route.fullPath === lastLoadedFullPath) return;
      const newPage = syncPageFromQuery();
      const newSize = syncSizeFromQuery();
      const urlFilters = readFiltersFromQuery();
      const currentFilters = getFilters();
      const filtersChanged = stableHash(urlFilters) !== stableHash(currentFilters);
      if (newPage !== state.page || newSize !== state.size || filtersChanged) {
        void loadPage(newPage);
      }
    },
  );

  return { state, loadPage, goNext, goPrev, setSize, reload, setFilters };
}