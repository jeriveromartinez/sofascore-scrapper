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

function cacheKey(routeName: string, page: number): string {
  return `pagination:${routeName}:page-${page}`;
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

  function clearCacheForRoute(): void {
    for (let i = sessionStorage.length - 1; i >= 0; i--) {
      const k = sessionStorage.key(i);
      if (k && k.startsWith(`pagination:${options.routeName}:`)) {
        sessionStorage.removeItem(k);
      }
    }
  }

  function syncSizeFromQuery(): number {
    const raw = route.query.size;
    return clampInt(raw, MIN_SIZE, MAX_SIZE, defaultSize);
  }

  function syncPageFromQuery(): number {
    const raw = route.query.page;
    return clampInt(raw, 1, Number.MAX_SAFE_INTEGER, 1);
  }

  async function writeQuery(page: number, size: number): Promise<void> {
    const next = { ...route.query, page: String(page), size: String(size) };
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
    const rawSize = route.query.size;
    const rawPage = route.query.page;
    const clampedSize = syncSizeFromQuery();
    const targetPage = page ?? syncPageFromQuery();
    state.size = clampedSize;
    state.page = targetPage;

    if (String(rawSize) !== String(clampedSize) || String(rawPage) !== String(targetPage)) {
      await writeQuery(state.page, state.size);
    }
    lastLoadedFullPath = route.fullPath;

    state.loading = true;
    try {
      let cursor: string | undefined;
      let cached: string | null = null;
      if (targetPage > 1) {
        cached = sessionStorage.getItem(cacheKey(options.routeName, targetPage - 1));
      }

      if (cached !== null) {
        const resp = await fetchWith(cached, state.size);
        state.data = resp.data;
        state.hasNext = resp.hasMore;
        if (resp.nextCursor) {
          sessionStorage.setItem(cacheKey(options.routeName, targetPage), resp.nextCursor);
        }
      } else {
        let lastResp: CursorPaginationFetchResult<T> | null = null;
        let reachedPage = 0;
        for (let i = 1; i <= targetPage; i++) {
          lastResp = await fetchWith(cursor, state.size);
          reachedPage = i;
          if (lastResp.nextCursor) {
            sessionStorage.setItem(cacheKey(options.routeName, i), lastResp.nextCursor);
          }
          if (lastResp.hasMore) {
            cursor = lastResp.nextCursor;
          } else {
            break;
          }
        }
        if (reachedPage < targetPage) {
          // Walk-forward hit the end of data before reaching the requested page.
          // Snap state to the actual last page so the URL and table stay in sync.
          state.page = reachedPage;
          await writeQuery(state.page, state.size);
        }
        if (lastResp) {
          state.data = lastResp.data;
          state.hasNext = lastResp.hasMore;
        }
      }
      state.hasPrev = targetPage > 1;
    } catch (err) {
      if ((err as { name?: string })?.name === "AbortError") return;
      state.error = err instanceof Error ? err.message : "Error cargando datos";
      clearCacheForRoute();
    } finally {
      state.loading = false;
    }
  }

  async function goNext(): Promise<void> {
    if (!state.hasNext) return;
    state.page += 1;
    state.hasPrev = true;
    await writeQuery(state.page, state.size);
    await loadPage(state.page);
  }

  async function goPrev(): Promise<void> {
    if (state.page <= 1) return;
    state.page -= 1;
    await writeQuery(state.page, state.size);
    await loadPage(state.page);
  }

  async function setSize(n: number): Promise<void> {
    const clamped = clampInt(n, MIN_SIZE, MAX_SIZE, defaultSize);
    clearCacheForRoute();
    state.size = clamped;
    state.page = 1;
    state.hasPrev = false;
    await writeQuery(state.page, state.size);
    await loadPage(state.page);
  }

  async function reload(): Promise<void> {
    await loadPage(state.page);
  }

  watch(
    () => route.fullPath,
    () => {
      if (route.fullPath === lastLoadedFullPath) return;
      const newPage = syncPageFromQuery();
      const newSize = syncSizeFromQuery();
      if (newPage !== state.page || newSize !== state.size) {
        void loadPage(newPage);
      }
    },
  );

  return { state, loadPage, goNext, goPrev, setSize, reload };
}
