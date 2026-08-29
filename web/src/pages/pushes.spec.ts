import { beforeEach, describe, expect, it, vi } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import { createMemoryHistory, createRouter } from "vue-router";
import { createPinia, setActivePinia } from "pinia";
import { useAuthStore } from "../store/pinia/authStore";
import pushes from "./pushes.vue";

/**
 * pushes.vue pulls the user id from the auth store, drives four
 * tabs, and calls the pushes + feature flag services. The spec
 * stubs every service and the auth store so the page can mount
 * without a real backend, then asserts the 4 tabs render, the
 * feature flag loading state shows when the auth store has no
 * user, and toggling the flag actually calls the service.
 */

vi.mock("../store/services", () => ({
  pushesApiService: {
    listSchedules: vi.fn().mockResolvedValue({ data: [], page: { nextCursor: "", hasMore: false } }),
    createSchedule: vi.fn().mockResolvedValue({}),
    updateSchedule: vi.fn().mockResolvedValue({}),
    deleteSchedule: vi.fn().mockResolvedValue({ message: "deleted" }),
    listPushes: vi.fn().mockResolvedValue({ data: [], page: { nextCursor: "", hasMore: false } }),
    getAggregateMetrics: vi.fn().mockResolvedValue({
      messagesSentTotal: 0,
      messagesDeliveredTotal: 0,
      deliveryRateTotal: 0,
      activeSchedules: 0,
      scheduledFires24h: 0,
      scheduledFires7d: 0,
      scheduledFires30d: 0,
      scheduledFailures24h: 0,
      scheduledFailures7d: 0,
      scheduledFailures30d: 0,
      audienceSize: 0,
      audiencePeakToday: 0,
      topPlatforms: [],
      topAppVersions: [],
      hourlyHistogram30d: [],
      lastPushAt: "",
    }),
    getCampaignMetrics: vi.fn().mockResolvedValue({
      pushId: 0,
      targetsTotal: 0,
      delivered: 0,
      notDelivered: 0,
      deliveryRate: 0,
      latencyP50Ms: 0,
      latencyP95Ms: 0,
      failuresByReason: [],
    }),
    createImmediatePush: vi.fn(),
    getPush: vi.fn(),
    getSchedule: vi.fn(),
  },
  featureFlagApiService: {
    setNotificationsEnabled: vi.fn().mockResolvedValue({
      id: 7,
      createdAt: "",
      updatedAt: "",
      email: "admin@example.com",
      role: "admin",
      notificationsEnabled: true,
      notificationsEnabledAt: "",
    }),
  },
  domainsApiService: {
    getAllDomains: vi.fn().mockResolvedValue([]),
  },
  usersApiService: {
    getUser: vi.fn().mockResolvedValue({
      id: 7,
      createdAt: "",
      updatedAt: "",
      email: "admin@example.com",
      role: "admin",
      notificationsEnabled: false,
      notificationsEnabledAt: "",
    }),
  },
}));

const flush = () => new Promise((r) => setTimeout(r, 0));

function buildRouter() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: "/", name: "Pushes", component: { template: "<div />" } },
      { path: "/pushes", name: "Pushes", component: { template: "<div />" } },
    ],
  });
}

describe("pushes.vue", () => {
  beforeEach(() => {
    setActivePinia(createPinia());
    vi.clearAllMocks();
    // Storage is shared across tests; reset it so the auth store
    // starts with an empty user.
    sessionStorage.clear();
    localStorage.clear();
  });

  it("renders all four tabs", async () => {
    const router = buildRouter();
    await router.push("/pushes");
    await router.isReady();
    const wrapper = mount(pushes, { global: { plugins: [router] } });
    await flushPromises();
    const tabs = wrapper.findAll('[data-test^="pushes-tab-"]');
    expect(tabs.length).toBe(4);
    expect(wrapper.find('[data-test="pushes-tab-compose"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="pushes-tab-scheduled"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="pushes-tab-metrics"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="pushes-tab-flag"]').exists()).toBe(true);
    wrapper.unmount();
  });

  it("shows the loading placeholder on the Feature flag tab when no user is loaded", async () => {
    const router = buildRouter();
    await router.push("/pushes");
    await router.isReady();
    const wrapper = mount(pushes, { global: { plugins: [router] } });
    await flushPromises();
    // The auth store has no userData.id, so the toggle must not
    // render at all; the placeholder is the only thing on the
    // feature flag tab.
    expect(useAuthStore().userData.id).toBeUndefined();
    const toggle = wrapper.find('[data-test="pushes-flag-toggle"]');
    expect(toggle.exists()).toBe(false);
    // Switch to the flag tab and re-check.
    await wrapper.find('[data-test="pushes-tab-flag"]').trigger("click");
    await flush();
    expect(wrapper.find('[data-test="pushes-flag-loading"]').exists()).toBe(true);
    expect(wrapper.find('[data-test="pushes-flag-toggle"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it("calls the feature flag service when the toggle is changed", async () => {
    // Hydrate the auth store BEFORE mounting so the page can
    // resolve the current user id on first render.
    const authStore = useAuthStore();
    authStore.setUser(
      { id: 7, email: "admin@example.com", token: "t", refreshToken: "rt" },
      false,
    );

    const { featureFlagApiService } = await import("../store/services");
    const setSpy = featureFlagApiService.setNotificationsEnabled as unknown as ReturnType<typeof vi.fn>;
    setSpy.mockResolvedValue({
      id: 7,
      createdAt: "",
      updatedAt: "",
      email: "admin@example.com",
      role: "admin",
      notificationsEnabled: true,
      notificationsEnabledAt: "",
    });

    const router = buildRouter();
    await router.push("/pushes");
    await router.isReady();
    const wrapper = mount(pushes, { global: { plugins: [router] } });
    await flushPromises();

    // Switch to the feature flag tab.
    await wrapper.find('[data-test="pushes-tab-flag"]').trigger("click");
    await flush();
    await flushPromises();

    // The toggle should now render because the auth store has
    // a user id; flip it.
    const toggle = wrapper.find<HTMLInputElement>('[data-test="pushes-flag-toggle"]');
    expect(toggle.exists()).toBe(true);
    await toggle.setValue(true);
    await flush();
    await flushPromises();

    expect(setSpy).toHaveBeenCalledTimes(1);
    expect(setSpy.mock.calls[0]?.[0]).toBe(7);
    expect(setSpy.mock.calls[0]?.[1]).toEqual({ enabled: true });
    wrapper.unmount();
  });
});
