import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { defineComponent, h, nextTick } from "vue";
import { createMemoryHistory, createRouter, RouterView } from "vue-router";
import { mount } from "@vue/test-utils";
import { useCursorPagination } from "./useCursorPagination";

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
    sessionStorage.setItem("pagination:Test:page-1", "cached-cursor-1");
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
    expect(sessionStorage.getItem("pagination:Test:page-2")).toBe("c-2");
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
    expect(sessionStorage.getItem("pagination:Test:page-2")).toBe("c-2");
  });

  it("goPrev re-fetches the previous page with the cached cursor", async () => {
    sessionStorage.setItem("pagination:Test:page-1", "c-0");
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
    sessionStorage.setItem("pagination:Test:page-2", "c-2");
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
    expect(sessionStorage.getItem("pagination:Test:page-2")).toBeNull();
  });

  it("reload re-fetches the current page using the cursor that got us here", async () => {
    sessionStorage.setItem("pagination:Test:page-1", "c-0");
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
