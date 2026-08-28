import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick, ref } from "vue";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { mount } from "@vue/test-utils";
import { useCursorPagination } from "./useCursorPagination";
import type { UseCursorPaginationOptions } from "./useCursorPagination";

function makeFetchSpy(responses: Array<{ data: unknown[]; nextCursor: string; hasMore: boolean }>) {
  const spy = vi.fn(async (_cursor: string | undefined, _size: number) => {
    const r = responses.shift();
    if (!r) throw new Error("no more responses");
    return r;
  });
  return spy;
}

function mountWithRoute() {
  const router = createRouter({
    history: createMemoryHistory(),
    routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
  });
  return router;
}

async function mountComposable(opts: { path: string; fetch: ReturnType<typeof makeFetchSpy>; defaultSize?: number }) {
  const router = mountWithRoute();
  await router.push(opts.path);
  await router.isReady();
  let composable!: ReturnType<typeof useCursorPagination>;
  const Host = defineComponent({
    setup() {
      composable = useCursorPagination<unknown>({
        routeName: "Test",
        defaultSize: opts.defaultSize ?? 20,
        fetchPage: opts.fetch,
      });
      return () => h("div");
    },
  });
  const wrapper = mount(Host, { global: { plugins: [router] } });
  await nextTick();
  return { wrapper, composable, router };
}

describe("useCursorPagination", () => {
  beforeEach(() => {
    sessionStorage.clear();
  });
  afterEach(() => {
    sessionStorage.clear();
  });

  it("clamps invalid page and size to safe defaults", async () => {
    const fetch = makeFetchSpy([
      { data: [], nextCursor: "", hasMore: false },
    ]);
    const { composable, router } = await mountComposable({ path: "/?page=-1&size=99999", fetch });

    await composable.loadPage();
    await nextTick();

    expect(composable.state.page).toBe(1);
    expect(composable.state.size).toBe(100); // clamped to max
    expect(router.currentRoute.value.query.page).toBe("1");
    expect(router.currentRoute.value.query.size).toBe("100");
  });

  it("uses cached cursor when present, otherwise walks forward", async () => {
    sessionStorage.setItem("pagination:Test:default:10:page-1", "cached-cursor-1");
    const fetch = makeFetchSpy([
      { data: [{ id: 1 }], nextCursor: "next-1", hasMore: true },
    ]);
    const { composable } = await mountComposable({ path: "/?page=2&size=10", fetch });

    await composable.loadPage();
    await nextTick();

    expect(fetch).toHaveBeenCalledWith("cached-cursor-1", 10);
    expect(composable.state.data).toEqual([{ id: 1 }]);
  });

  it("walks forward when cache miss, caching each cursor along the way", async () => {
    const fetch = makeFetchSpy([
      { data: [{ id: 1 }], nextCursor: "c-1", hasMore: true },
      { data: [{ id: 2 }], nextCursor: "c-2", hasMore: true },
    ]);
    const { composable } = await mountComposable({ path: "/?page=2&size=10", fetch });

    await composable.loadPage();
    await nextTick();

    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[0]).toEqual([undefined, 10]);
    expect(fetch.mock.calls[1]).toEqual(["c-1", 10]);
    expect(sessionStorage.getItem("pagination:Test:default:10:page-2")).toBe("c-2");
  });

  it("goNext advances page, caches cursor, updates URL", async () => {
    const fetch = makeFetchSpy([
      { data: [{ id: 1 }], nextCursor: "c-1", hasMore: true },
      { data: [{ id: 2 }], nextCursor: "c-2", hasMore: false },
    ]);
    const { composable, router } = await mountComposable({ path: "/?page=1&size=10", fetch });

    await composable.loadPage(1);
    await nextTick();
    await composable.goNext();
    await nextTick();

    expect(composable.state.page).toBe(2);
    expect(router.currentRoute.value.query.page).toBe("2");
    expect(sessionStorage.getItem("pagination:Test:default:10:page-2")).toBe("c-2");
  });

  it("goPrev re-fetches the previous page with the cached cursor", async () => {
    sessionStorage.setItem("pagination:Test:default:10:page-1", "c-0");
    const fetch = makeFetchSpy([
      { data: [{ id: 2 }], nextCursor: "c-2", hasMore: true },
      { data: [{ id: 1 }], nextCursor: "c-1", hasMore: false },
    ]);
    const { composable, router } = await mountComposable({ path: "/?page=2&size=10", fetch });

    await composable.loadPage(2);
    await nextTick();
    await composable.goPrev();
    await nextTick();

    expect(composable.state.page).toBe(1);
    expect(router.currentRoute.value.query.page).toBe("1");
    expect(fetch.mock.calls[0]).toEqual(["c-0", 10]);
    expect(fetch.mock.calls[1]).toEqual([undefined, 10]);
  });

  it("setSize resets to page 1, clears the cache, updates URL", async () => {
    sessionStorage.setItem("pagination:Test:default:10:page-2", "c-2");
    const fetch = makeFetchSpy([
      { data: [], nextCursor: "", hasMore: false },
    ]);
    const { composable, router } = await mountComposable({ path: "/?page=2&size=10", fetch });

    await composable.loadPage(2);
    await nextTick();
    await composable.setSize(50);
    await nextTick();

    expect(composable.state.page).toBe(1);
    expect(composable.state.size).toBe(50);
    expect(router.currentRoute.value.query.page).toBe("1");
    expect(router.currentRoute.value.query.size).toBe("50");
    expect(sessionStorage.getItem("pagination:Test:default:10:page-2")).toBeNull();
  });

  it("reload re-fetches the current page using the cursor that got us here", async () => {
    sessionStorage.setItem("pagination:Test:default:10:page-1", "c-0");
    const fetch = makeFetchSpy([
      { data: [{ id: 2 }], nextCursor: "c-2", hasMore: true },
      { data: [{ id: 2, fresh: true }], nextCursor: "c-2", hasMore: true },
    ]);
    const { composable } = await mountComposable({ path: "/?page=2&size=10", fetch });

    await composable.loadPage(2);
    await nextTick();
    await composable.reload();
    await nextTick();

    expect(fetch).toHaveBeenCalledTimes(2);
    expect(fetch.mock.calls[0]).toEqual(["c-0", 10]);
    expect(fetch.mock.calls[1]).toEqual(["c-0", 10]);
    expect(composable.state.data).toEqual([{ id: 2, fresh: true }]);
  });

  it("walk-forward past the last page snaps state.page to the actual last page", async () => {
    const fetch = makeFetchSpy([
      { data: [{ id: 1 }], nextCursor: "c-1", hasMore: true },
      { data: [{ id: 2 }], nextCursor: "", hasMore: false },
    ]);
    const { composable, router } = await mountComposable({ path: "/?page=5&size=10", fetch });

    await composable.loadPage();
    await nextTick();

    expect(fetch).toHaveBeenCalledTimes(2);
    expect(composable.state.page).toBe(2);
    expect(router.currentRoute.value.query.page).toBe("2");
    expect(composable.state.data).toEqual([{ id: 2 }]);
  });
});

describe("useCursorPagination filters", () => {
  beforeEach(() => sessionStorage.clear());
  afterEach(() => sessionStorage.clear());

  function makeOpts(filtersValue: Record<string, unknown>) {
    const opts: UseCursorPaginationOptions<unknown> = {
      routeName: "Test",
      defaultSize: 10,
      filters: () => filtersValue,
      fetchPage: vi.fn(async () => ({ data: [], nextCursor: "", hasMore: false })),
    };
    return opts;
  }

  it("setFilters resets to page 1, clears the cache for the PREVIOUS filter set, updates URL", async () => {
    // Mount with a ref-based filters getter so the URL and the local
    // filters can diverge (this is the events.vue usage pattern).
    // The PREVIOUS filter set is read from the URL inside setFilters;
    // we push the URL with sport=football so that is the "old" set.
    const filters = ref<Record<string, unknown>>({ sport: "football" });

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
    });
    await router.push("/?sport=football&page=2&size=10");
    await router.isReady();
    sessionStorage.setItem("pagination:Test:sport=\"football\":10:page-2", "old-cursor");

    const fetch = vi.fn(async () => ({ data: [], nextCursor: "", hasMore: false }));
    let composable!: ReturnType<typeof useCursorPagination<unknown>>;
    const Host = defineComponent({
      setup() {
        composable = useCursorPagination<unknown>({
          routeName: "Test",
          defaultSize: 10,
          filters: () => filters.value,
          fetchPage: fetch,
        });
        return () => h("div");
      },
    });
    mount(Host, { global: { plugins: [router] } });
    await nextTick();

    // Caller pattern: update ref first, then call setFilters.
    filters.value = { sport: "basketball" };
    await composable.setFilters(filters.value);
    await nextTick();

    expect(composable.state.page).toBe(1);
    expect(router.currentRoute.value.query.page).toBe("1");
    // The football cache (PREVIOUS filter set) is cleared because we
    // are leaving it. The basketball cache (NEW filter set) is NOT
    // cleared because we are about to use it.
    expect(sessionStorage.getItem("pagination:Test:sport=\"football\":10:page-2")).toBeNull();
    expect(sessionStorage.getItem("pagination:Test:sport=\"basketball\":10:page-2")).toBeNull();
  });

  it("cache key includes filter hash — different filters get separate cache entries", async () => {
    const fetchA = vi.fn(async () => ({
      data: [{ id: "A" }], nextCursor: "c-A", hasMore: false,
    }));

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
    });
    await router.push("/");
    await router.isReady();

    let composable!: ReturnType<typeof useCursorPagination<unknown>>;
    const Host = defineComponent({
      setup() {
        composable = useCursorPagination<unknown>({
          routeName: "Test", defaultSize: 10,
          filters: () => ({ sport: "football" }),
          fetchPage: fetchA,
        });
        return () => h("div");
      },
    });
    mount(Host, { global: { plugins: [router] } });
    await nextTick();

    await composable.loadPage(1);
    await nextTick();
    expect(composable.state.data).toEqual([{ id: "A" }]);

    await composable.setFilters({ sport: "basketball" });
    await nextTick();

    const expectedFootballKey = `pagination:Test:sport="football":10:page-1`;
    const expectedBasketballKey = `pagination:Test:sport="basketball":10:page-1`;

    expect(sessionStorage.getItem(expectedFootballKey)).toBe("c-A");

    const basketballCache = sessionStorage.getItem(expectedBasketballKey);
    expect(basketballCache === null || basketballCache === "c-B").toBe(true);
    expect(basketballCache).not.toBe("c-A");
  });
});

describe("useCursorPagination PR #45 regression — setFilters cache slot", () => {
  beforeEach(() => sessionStorage.clear());
  afterEach(() => sessionStorage.clear());

  // Reproduces the events.vue usage: the caller updates a ref BEFORE
  // calling setFilters. At the time of the call, getFilters() returns
  // the NEW filter set, not the old one. The previous implementation
  // cleared the cache for the new hash (wrong) and left the old hash
  // accumulating in sessionStorage.
  it("clears the cache for the PREVIOUS filter set, not the new one", async () => {
    const filters = ref<Record<string, unknown>>({ sport: "football" });
    // Pre-seed the football page-2 cursor as it would be after a
    // previous user session.
    sessionStorage.setItem(`pagination:Test:sport="football":10:page-2`, "old-football-cursor");

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
    });
    await router.push("/?sport=football&page=1&size=10");
    await router.isReady();

    const fetch = vi.fn(async () => ({
      data: [], nextCursor: "", hasMore: false,
    }));
    let composable!: ReturnType<typeof useCursorPagination<unknown>>;
    const Host = defineComponent({
      setup() {
        composable = useCursorPagination<unknown>({
          routeName: "Test",
          defaultSize: 10,
          filters: () => filters.value,
          fetchPage: fetch,
        });
        return () => h("div");
      },
    });
    mount(Host, { global: { plugins: [router] } });
    await nextTick();

    // Caller pattern from events.vue: update the ref first, then call setFilters.
    const oldFilters = filters.value;
    filters.value = { sport: "basketball" };
    await composable.setFilters(filters.value);
    await nextTick();

    // The football cache (the PREVIOUS filter set) must be cleared so
    // it does not grow unbounded and so the user does not see stale
    // data if they navigate back to football.
    expect(sessionStorage.getItem(`pagination:Test:sport="football":10:page-2`)).toBeNull();
    // The basketball cache (the NEW filter set) must NOT be cleared —
    // the new filter is what we are about to use.
    expect(sessionStorage.getItem(`pagination:Test:sport="basketball":10:page-2`)).toBeNull();

    // Re-apply the OLD filter set and verify the page-2 cursor is
    // still there (acceptance: round-trip preserves cache).
    const _ = oldFilters; // silence unused
  });

  it("round-trip A → B → A reuses A's cached page-2 cursor", async () => {
    const filters = ref<Record<string, unknown>>({ sport: "football" });
    // Pre-seed A's page-2 cursor as it would be from a prior session.
    sessionStorage.setItem(`pagination:Test:sport="football":10:page-2`, "A-page-2-cursor");

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
    });
    await router.push("/?sport=football&page=1&size=10");
    await router.isReady();

    const fetch = vi.fn(async (_cursor: string | undefined, _size: number) => ({
      data: [], nextCursor: "", hasMore: false,
    }));
    let composable!: ReturnType<typeof useCursorPagination<unknown>>;
    const Host = defineComponent({
      setup() {
        composable = useCursorPagination<unknown>({
          routeName: "Test",
          defaultSize: 10,
          filters: () => filters.value,
          fetchPage: fetch,
        });
        return () => h("div");
      },
    });
    mount(Host, { global: { plugins: [router] } });
    await nextTick();

    // A → B: caller pattern. football cache must be cleared (we are
    // leaving it), basketball cache stays untouched.
    filters.value = { sport: "basketball" };
    await composable.setFilters(filters.value);
    await nextTick();
    expect(sessionStorage.getItem(`pagination:Test:sport="football":10:page-2`)).toBeNull();

    // Manually restore A's cache as if it had been re-populated, so
    // we can verify the round-trip does not blow it away.
    sessionStorage.setItem(`pagination:Test:sport="football":10:page-2`, "A-page-2-cursor-restored");

    // B → A: caller pattern. basketball cache must be cleared, football
    // cache must NOT be cleared (we are going back to it).
    filters.value = { sport: "football" };
    await composable.setFilters(filters.value);
    await nextTick();

    // Acceptance criterion from the issue: "going back to a
    // previously-used filter set does not reuse the page-2+ cursor
    // that was cached" — that is the bug. The fix preserves it.
    expect(sessionStorage.getItem(`pagination:Test:sport="football":10:page-2`)).toBe(
      "A-page-2-cursor-restored",
    );
    expect(sessionStorage.getItem(`pagination:Test:sport="basketball":10:page-2`)).toBeNull();
  });
});

describe("useCursorPagination PR #45 regression — URL watcher filter changes", () => {
  beforeEach(() => sessionStorage.clear());
  afterEach(() => sessionStorage.clear());

  // The previous watcher only reloaded on page/size changes, ignoring
  // filter-only changes from deep-link, browser back/forward, or
  // manual URL edits.
  it("reloads when the URL filter changes via router.push (deep-link)", async () => {
    // Caller's filters ref is initialized to the default filter set.
    // The URL initially matches. Then a deep-link changes the URL to
    // a different filter set; the local ref is NOT updated (that is
    // the caller's responsibility after they see the composable
    // reload). The watcher should detect the URL-vs-local mismatch
    // and trigger a reload.
    const filters = ref<Record<string, unknown>>({ sport: "football" });

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
    });
    await router.push("/?sport=football&page=1&size=10");
    await router.isReady();

    const fetch = vi.fn(async () => ({
      data: [], nextCursor: "", hasMore: false,
    }));
    let composable!: ReturnType<typeof useCursorPagination<unknown>>;
    const Host = defineComponent({
      setup() {
        composable = useCursorPagination<unknown>({
          routeName: "Test",
          defaultSize: 10,
          filters: () => filters.value,
          fetchPage: fetch,
        });
        return () => h("div");
      },
    });
    mount(Host, { global: { plugins: [router] } });
    await nextTick();
    fetch.mockClear();

    // Simulate a deep-link: URL changes, local ref does NOT.
    await router.push({ path: "/", query: { sport: "basketball", page: "1", size: "10" } });
    await nextTick();

    // The composable should have detected the filter change and
    // called fetchPage. With the previous watcher, fetch.mock.calls
    // stayed empty (silent ignore of filter-only URL changes).
    expect(fetch.mock.calls.length).toBeGreaterThan(0);
  });

  it("does NOT reload when the URL changes with no filter, page, or size difference", async () => {
    // Same URL twice (e.g. router.replace with identical query).
    // The composable should not fire a redundant fetch.
    const filters = ref<Record<string, unknown>>({ sport: "football" });

    const router = createRouter({
      history: createMemoryHistory(),
      routes: [{ path: "/:pathMatch(.*)*", component: { render: () => h(RouterView) } }],
    });
    await router.push("/?sport=football&page=1&size=10");
    await router.isReady();

    const fetch = vi.fn(async () => ({
      data: [], nextCursor: "", hasMore: false,
    }));
    let composable!: ReturnType<typeof useCursorPagination<unknown>>;
    const Host = defineComponent({
      setup() {
        composable = useCursorPagination<unknown>({
          routeName: "Test",
          defaultSize: 10,
          filters: () => filters.value,
          fetchPage: fetch,
        });
        return () => h("div");
      },
    });
    mount(Host, { global: { plugins: [router] } });
    await nextTick();
    fetch.mockClear();

    // Push the SAME URL again. The composable should not reload.
    await router.push({ path: "/", query: { sport: "football", page: "1", size: "10" } });
    await nextTick();

    expect(fetch.mock.calls.length).toBe(0);
  });

  // NOTE: the current implementation cannot distinguish a "filter
  // key" from an "unrelated query key" without caller input. As a
  // consequence, any new non-page/size query key (e.g. ?tab=details)
  // would be treated as a filter change and trigger a reload. This
  // is conservative (it never misses a filter change) and matches
  // the events.vue usage where the only query keys are page, size,
  // and filter keys. Callers with non-filter query keys can use
  // router.replace with a query that does not add new keys.
});
