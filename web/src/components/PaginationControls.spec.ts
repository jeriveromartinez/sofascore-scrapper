import { describe, expect, it } from "vitest";
import { mount } from "@vue/test-utils";
import PaginationControls from "./PaginationControls.vue";
import type { CursorPaginationState } from "../composables/useCursorPagination";

function makeState(overrides: Partial<CursorPaginationState<unknown>> = {}): CursorPaginationState<unknown> {
  return {
    data: [],
    loading: false,
    error: "",
    page: 1,
    size: 20,
    hasNext: false,
    hasPrev: false,
    ...overrides,
  };
}

describe("PaginationControls", () => {
  it("renders prev and next buttons", () => {
    const wrapper = mount(PaginationControls, {
      props: { state: makeState() },
    });
    expect(wrapper.find('[data-test="pagination-prev"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="pagination-next"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("disables prev when hasPrev is false, enables when true", () => {
    const disabledWrapper = mount(PaginationControls, {
      props: { state: makeState({ hasPrev: false }) },
    });
    expect(
      disabledWrapper.find('[data-test="pagination-prev"]').attributes("disabled"),
    ).toBeDefined();

    const enabledWrapper = mount(PaginationControls, {
      props: { state: makeState({ hasPrev: true }) },
    });
    expect(
      enabledWrapper.find('[data-test="pagination-prev"]').attributes("disabled"),
    ).toBeUndefined();
    enabledWrapper.unmount();
  });

  it("disables next when hasNext is false, enables when true", () => {
    const disabledWrapper = mount(PaginationControls, {
      props: { state: makeState({ hasNext: false }) },
    });
    expect(
      disabledWrapper.find('[data-test="pagination-next"]').attributes("disabled"),
    ).toBeDefined();

    const enabledWrapper = mount(PaginationControls, {
      props: { state: makeState({ hasNext: true }) },
    });
    expect(
      enabledWrapper.find('[data-test="pagination-next"]').attributes("disabled"),
    ).toBeUndefined();
    enabledWrapper.unmount();
  });

  it("disables both buttons while loading", () => {
    const wrapper = mount(PaginationControls, {
      props: {
        state: makeState({ hasPrev: true, hasNext: true, loading: true }),
      },
    });
    expect(
      wrapper.find('[data-test="pagination-prev"]').attributes("disabled"),
    ).toBeDefined();
    expect(
      wrapper.find('[data-test="pagination-next"]').attributes("disabled"),
    ).toBeDefined();
    wrapper.unmount();
  });

  it("emits prev/next on click and reuses the page's goPrev/goNext handlers", async () => {
    const wrapper = mount(PaginationControls, {
      props: { state: makeState({ hasPrev: true, hasNext: true }) },
    });
    await wrapper.find('[data-test="pagination-prev"]').trigger("click");
    await wrapper.find('[data-test="pagination-next"]').trigger("click");
    expect(wrapper.emitted("prev")?.length).toBe(1);
    expect(wrapper.emitted("next")?.length).toBe(1);
    wrapper.unmount();
  });

  it("does not render the reload button by default", () => {
    const wrapper = mount(PaginationControls, {
      props: { state: makeState() },
    });
    expect(wrapper.find('[data-test="pagination-reload"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("renders the reload button when withReload=true and emits on click", async () => {
    const wrapper = mount(PaginationControls, {
      props: { state: makeState(), withReload: true },
    });
    const reload = wrapper.find('[data-test="pagination-reload"]');
    expect(reload.exists()).toBe(true);
    expect(reload.attributes("disabled")).toBeUndefined();
    await reload.trigger("click");
    expect(wrapper.emitted("reload")?.length).toBe(1);
    wrapper.unmount();
  });

  it("disables the reload button while loading when withReload=true", () => {
    const wrapper = mount(PaginationControls, {
      props: { state: makeState({ loading: true }), withReload: true },
    });
    const reload = wrapper.find('[data-test="pagination-reload"]');
    expect(reload.exists()).toBe(true);
    expect(reload.attributes("disabled")).toBeDefined();
    wrapper.unmount();
  });

  it("exposes a11y labels for screen readers", () => {
    const wrapper = mount(PaginationControls, {
      props: { state: makeState() },
    });
    expect(
      wrapper.find('[data-test="pagination-prev"]').attributes("aria-label"),
    ).toBe("Previous page");
    expect(
      wrapper.find('[data-test="pagination-next"]').attributes("aria-label"),
    ).toBe("Next page");
    wrapper.unmount();
  });
});
