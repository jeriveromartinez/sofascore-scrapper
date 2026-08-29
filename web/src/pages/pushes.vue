<script setup lang="ts">
/**
 * Pushes dashboard.
 *
 * Single page with four tabs that exercises the PushesApiService
 * and FeatureFlagApiService singletons. The service contract is
 * the source of truth (see ../store/services/PushesApiService.ts);
 * this SFC only renders forms/tables and wires callbacks to the
 * service methods.
 *
 * Tab map:
 *   compose    - send an immediate push
 *   scheduled  - list + create / toggle / delete scheduled pushes
 *   metrics    - aggregate metrics + recent pushes (with per-campaign drilldown)
 *   flag       - per-user `notifications_enabled` toggle
 *
 * The Feature flag tab is gated on the auth store having a user
 * id (the only piece of the user the dashboard needs for this
 * surface). All other tabs run regardless.
 */
import { computed, onMounted, reactive, ref, watch } from "vue";
import { toast } from "vue3-toastify";
import { useAuthStore } from "../store/pinia/authStore";
import {
  domainsApiService,
  featureFlagApiService,
  pushesApiService,
  usersApiService,
} from "../store/services";
import {
  PushCategory,
  PushPriority,
  PushScheduleType,
} from "../proto/api";
import type {
  CreateImmediatePushPayload,
  CreateSchedulePayload,
  Domain,
  PushMessage,
  PushMessagePageResponse,
  PushMetricsAggregate,
  PushMetricsByCampaign,
  ScheduledPush,
  ScheduledPushPageResponse,
  UpdateSchedulePayload,
} from "../store/services/models";

type TabKey = "compose" | "scheduled" | "metrics" | "flag";

const activeTab = ref<TabKey>("compose");

const authStore = useAuthStore();

/**
 * The auth store holds the user identity under `userData` (id,
 * email, token, refreshToken). The brief described it as
 * `useAuthStore().user`; the real field is `userData`, so we
 * project it here once and use the projected value across the
 * template.
 */
const currentUserId = computed<number | null>(() => {
  const u = authStore.userData as { id?: number };
  return typeof u.id === "number" ? u.id : null;
});

// ------------------------------------------------------------------
// Domain catalog
// ------------------------------------------------------------------

const domains = ref<Domain[]>([]);
const domainsLoading = ref(false);
const domainsError = ref("");

async function loadDomains(): Promise<void> {
  domainsLoading.value = true;
  domainsError.value = "";
  try {
    domains.value = await domainsApiService.getAllDomains();
  } catch (err) {
    domainsError.value =
      err instanceof Error ? err.message : "No domains available";
  } finally {
    domainsLoading.value = false;
  }
}

function toggleDomain(domainId: number, checked: boolean): void {
  const idx = compose.domainIds.indexOf(domainId);
  if (checked && idx === -1) compose.domainIds.push(domainId);
  if (!checked && idx !== -1) compose.domainIds.splice(idx, 1);
}

// ------------------------------------------------------------------
// Compose tab
// ------------------------------------------------------------------

const compose = reactive({
  domainIds: [] as number[],
  category: PushCategory.PUSH_CATEGORY_ADMIN_MESSAGE,
  priority: PushPriority.PUSH_PRIORITY_NORMAL,
  title: "",
  body: "",
  imageUrl: "",
  deepLink: "",
  ttlSeconds: 0,
  loading: false,
  error: "",
});

const composeDisabled = computed<boolean>(
  () =>
    compose.loading ||
    currentUserId.value === null ||
    compose.title.trim() === "" ||
    compose.body.trim() === "" ||
    compose.domainIds.length === 0,
);

function resetCompose(): void {
  compose.domainIds = [];
  compose.category = PushCategory.PUSH_CATEGORY_ADMIN_MESSAGE;
  compose.priority = PushPriority.PUSH_PRIORITY_NORMAL;
  compose.title = "";
  compose.body = "";
  compose.imageUrl = "";
  compose.deepLink = "";
  compose.ttlSeconds = 0;
  compose.error = "";
}

async function sendCompose(): Promise<void> {
  if (composeDisabled.value) return;
  compose.loading = true;
  compose.error = "";
  const payload: CreateImmediatePushPayload = {
    domainIds: [...compose.domainIds],
    payload: {
      category: compose.category,
      priority: compose.priority,
      title: compose.title.trim(),
      body: compose.body.trim(),
      imageUrl: compose.imageUrl.trim(),
      deepLink: compose.deepLink.trim(),
      ttlSeconds: Number(compose.ttlSeconds) || 0,
      data: {},
    },
  };
  try {
    await pushesApiService.createImmediatePush(payload);
    toast.success("Push sent");
    resetCompose();
  } catch (err) {
    compose.error = err instanceof Error ? err.message : "Could not send push";
    toast.error(compose.error);
  } finally {
    compose.loading = false;
  }
}

const PUSH_CATEGORY_OPTIONS: ReadonlyArray<{ value: PushCategory; label: string }> = [
  { value: PushCategory.PUSH_CATEGORY_EVENT_ALERT, label: "Event alert" },
  { value: PushCategory.PUSH_CATEGORY_APK_UPDATE, label: "APK update" },
  { value: PushCategory.PUSH_CATEGORY_ADMIN_MESSAGE, label: "Admin message" },
  { value: PushCategory.PUSH_CATEGORY_SCHEDULED, label: "Scheduled" },
];

const PUSH_PRIORITY_OPTIONS: ReadonlyArray<{ value: PushPriority; label: string }> = [
  { value: PushPriority.PUSH_PRIORITY_NORMAL, label: "Normal" },
  { value: PushPriority.PUSH_PRIORITY_HIGH, label: "High" },
];

function categoryLabel(value: PushCategory): string {
  const match = PUSH_CATEGORY_OPTIONS.find((o) => o.value === value);
  return match?.label ?? "Unspecified";
}

function priorityLabel(value: PushPriority): string {
  const match = PUSH_PRIORITY_OPTIONS.find((o) => o.value === value);
  return match?.label ?? "Unspecified";
}

// ------------------------------------------------------------------
// Scheduled tab
// ------------------------------------------------------------------

const newScheduleOpen = ref(false);
const scheduleForm = reactive({
  domainIds: [] as number[],
  category: PushCategory.PUSH_CATEGORY_ADMIN_MESSAGE,
  priority: PushPriority.PUSH_PRIORITY_NORMAL,
  title: "",
  body: "",
  scheduleType: PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT,
  runAt: "",
  cronExpr: "",
  loading: false,
  error: "",
});

const schedulePagination = reactive({
  data: [] as ScheduledPush[],
  cursor: "",
  hasMore: false,
  loading: false,
  error: "",
});

function scheduleTypeLabel(value: PushScheduleType): string {
  switch (value) {
    case PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT:
      return "One-shot";
    case PushScheduleType.PUSH_SCHEDULE_TYPE_RECURRING:
      return "Recurring";
    default:
      return "Unspecified";
  }
}

function toggleScheduleDomain(domainId: number, checked: boolean): void {
  const idx = scheduleForm.domainIds.indexOf(domainId);
  if (checked && idx === -1) scheduleForm.domainIds.push(domainId);
  if (!checked && idx !== -1) scheduleForm.domainIds.splice(idx, 1);
}

function resetScheduleForm(): void {
  scheduleForm.domainIds = [];
  scheduleForm.category = PushCategory.PUSH_CATEGORY_ADMIN_MESSAGE;
  scheduleForm.priority = PushPriority.PUSH_PRIORITY_NORMAL;
  scheduleForm.title = "";
  scheduleForm.body = "";
  scheduleForm.scheduleType = PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT;
  scheduleForm.runAt = "";
  scheduleForm.cronExpr = "";
  scheduleForm.error = "";
}

async function loadSchedules(reset: boolean = true): Promise<void> {
  schedulePagination.loading = true;
  schedulePagination.error = "";
  try {
    const cursor = reset ? undefined : schedulePagination.cursor || undefined;
    const page: ScheduledPushPageResponse = await pushesApiService.listSchedules(
      cursor,
      20,
    );
    schedulePagination.data = reset
      ? page.data
      : schedulePagination.data.concat(page.data);
    schedulePagination.cursor = page.page?.nextCursor ?? "";
    schedulePagination.hasMore = page.page?.hasMore ?? false;
  } catch (err) {
    schedulePagination.error =
      err instanceof Error ? err.message : "Could not load schedules";
  } finally {
    schedulePagination.loading = false;
  }
}

async function loadMoreSchedules(): Promise<void> {
  if (!schedulePagination.hasMore || schedulePagination.loading) return;
  await loadSchedules(false);
}

async function createSchedule(): Promise<void> {
  if (scheduleForm.title.trim() === "" || scheduleForm.body.trim() === "") {
    scheduleForm.error = "Title and body are required";
    return;
  }
  if (scheduleForm.domainIds.length === 0) {
    scheduleForm.error = "Select at least one domain";
    return;
  }
  if (
    scheduleForm.scheduleType === PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT &&
    scheduleForm.runAt.trim() === ""
  ) {
    scheduleForm.error = "runAt is required for one-shot schedules";
    return;
  }
  if (
    scheduleForm.scheduleType === PushScheduleType.PUSH_SCHEDULE_TYPE_RECURRING &&
    scheduleForm.cronExpr.trim() === ""
  ) {
    scheduleForm.error = "cronExpr is required for recurring schedules";
    return;
  }
  scheduleForm.loading = true;
  scheduleForm.error = "";
  const payload: CreateSchedulePayload = {
    domainIds: [...scheduleForm.domainIds],
    payload: {
      category: scheduleForm.category,
      priority: scheduleForm.priority,
      title: scheduleForm.title.trim(),
      body: scheduleForm.body.trim(),
      imageUrl: "",
      deepLink: "",
      ttlSeconds: 0,
      data: {},
    },
    scheduleType: scheduleForm.scheduleType,
    runAt:
      scheduleForm.scheduleType === PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT
        ? new Date(scheduleForm.runAt).toISOString()
        : "",
    cronExpr:
      scheduleForm.scheduleType === PushScheduleType.PUSH_SCHEDULE_TYPE_RECURRING
        ? scheduleForm.cronExpr.trim()
        : "",
  };
  try {
    await pushesApiService.createSchedule(payload);
    toast.success("Schedule created");
    resetScheduleForm();
    newScheduleOpen.value = false;
    await loadSchedules(true);
  } catch (err) {
    scheduleForm.error =
      err instanceof Error ? err.message : "Could not create schedule";
    toast.error(scheduleForm.error);
  } finally {
    scheduleForm.loading = false;
  }
}

async function toggleScheduleActive(s: ScheduledPush): Promise<void> {
  if (!s.payload) return;
  const payload: UpdateSchedulePayload = {
    id: s.id,
    isActive: !s.isActive,
    payload: s.payload,
  };
  try {
    await pushesApiService.updateSchedule(s.id, payload);
    toast.success(s.isActive ? "Schedule paused" : "Schedule activated");
    await loadSchedules(true);
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "Could not update schedule");
  }
}

async function deleteSchedule(id: number): Promise<void> {
  if (!window.confirm("Delete this schedule? This cannot be undone.")) return;
  try {
    await pushesApiService.deleteSchedule(id);
    toast.success("Schedule deleted");
    await loadSchedules(true);
  } catch (err) {
    toast.error(err instanceof Error ? err.message : "Could not delete schedule");
  }
}

function formatTimestamp(value: string | undefined): string {
  if (!value) return "-";
  return new Date(value).toLocaleString();
}

// ------------------------------------------------------------------
// Metrics tab
// ------------------------------------------------------------------

const aggregate = ref<PushMetricsAggregate | null>(null);
const aggregateLoading = ref(false);
const aggregateError = ref("");

const recentPushes = ref<PushMessage[]>([]);
const recentPushesLoading = ref(false);
const recentPushesError = ref("");
const expandedPushId = ref<number | null>(null);
const campaignMetricsById = reactive<Record<number, PushMetricsByCampaign | null>>({});
const campaignMetricsLoading = reactive<Record<number, boolean>>({});

function formatPercent(value: number): string {
  if (!Number.isFinite(value)) return "-";
  return `${(value * 100).toFixed(1)}%`;
}

function formatCount(value: number | undefined): string {
  if (value === undefined || value === null) return "-";
  return new Intl.NumberFormat("en-US").format(value);
}

async function loadAggregate(): Promise<void> {
  aggregateLoading.value = true;
  aggregateError.value = "";
  try {
    aggregate.value = await pushesApiService.getAggregateMetrics();
  } catch (err) {
    aggregateError.value =
      err instanceof Error ? err.message : "Could not load aggregate metrics";
  } finally {
    aggregateLoading.value = false;
  }
}

async function loadRecentPushes(): Promise<void> {
  recentPushesLoading.value = true;
  recentPushesError.value = "";
  try {
    const page: PushMessagePageResponse = await pushesApiService.listPushes(
      undefined,
      50,
    );
    recentPushes.value = page.data;
  } catch (err) {
    recentPushesError.value =
      err instanceof Error ? err.message : "Could not load recent pushes";
  } finally {
    recentPushesLoading.value = false;
  }
}

async function togglePushExpansion(p: PushMessage): Promise<void> {
  if (expandedPushId.value === p.id) {
    expandedPushId.value = null;
    return;
  }
  expandedPushId.value = p.id;
  if (campaignMetricsById[p.id] !== undefined) return;
  campaignMetricsLoading[p.id] = true;
  try {
    campaignMetricsById[p.id] = await pushesApiService.getCampaignMetrics(p.id);
  } catch {
    campaignMetricsById[p.id] = null;
    toast.error("Could not load campaign metrics");
  } finally {
    campaignMetricsLoading[p.id] = false;
  }
}

function failureReasonLabel(reason: number): string {
  // Mirror the proto enum names without importing the full enum
  // (the proto3 numeric value is the only thing the backend sends).
  const map: Record<number, string> = {
    0: "Unspecified",
    1: "Device offline",
    2: "Send timeout",
    3: "WS disconnected",
    4: "Domain mismatch",
    5: "Expired token",
    6: "Internal error",
  };
  return map[reason] ?? `Reason ${reason}`;
}

async function refreshMetrics(): Promise<void> {
  await Promise.all([loadAggregate(), loadRecentPushes()]);
}

// ------------------------------------------------------------------
// Feature flag tab
// ------------------------------------------------------------------

const flagState = reactive({
  enabled: false,
  initialLoaded: false,
  loading: false,
  error: "",
  toggling: false,
});

const flagPlaceholder = computed<string>(() =>
  currentUserId.value === null ? "Loading..." : "Loading flag state...",
);

async function loadFlag(): Promise<void> {
  if (currentUserId.value === null) return;
  flagState.loading = true;
  flagState.error = "";
  try {
    const u = await usersApiService.getUser(currentUserId.value);
    flagState.enabled = !!u.notificationsEnabled;
    flagState.initialLoaded = true;
  } catch (err) {
    flagState.error =
      err instanceof Error ? err.message : "Could not load flag state";
  } finally {
    flagState.loading = false;
  }
}

async function onFlagChange(event: Event): Promise<void> {
  if (currentUserId.value === null) return;
  const target = event.target as HTMLInputElement | null;
  if (!target) return;
  const next = target.checked;
  const previous = flagState.enabled;
  flagState.toggling = true;
  flagState.error = "";
  // Optimistically reflect the new value; revert on failure.
  flagState.enabled = next;
  try {
    const u = await featureFlagApiService.setNotificationsEnabled(
      currentUserId.value,
      { enabled: next },
    );
    flagState.enabled = !!u.notificationsEnabled;
    toast.success(
      u.notificationsEnabled
        ? "Push notifications enabled for your account"
        : "Push notifications disabled for your account",
    );
  } catch (err) {
    flagState.enabled = previous;
    flagState.error =
      err instanceof Error ? err.message : "Could not update flag";
    toast.error(flagState.error);
  } finally {
    flagState.toggling = false;
  }
}

// ------------------------------------------------------------------
// Lifecycle
// ------------------------------------------------------------------

onMounted(async () => {
  // Load domains up front so both the compose and the schedule
  // form have them when the user clicks into the tab.
  await loadDomains();
});

watch(activeTab, (tab) => {
  if (tab === "scheduled" && schedulePagination.data.length === 0 && !schedulePagination.loading) {
    void loadSchedules(true);
  } else if (tab === "metrics" && aggregate.value === null) {
    void refreshMetrics();
  } else if (tab === "flag" && !flagState.initialLoaded && currentUserId.value !== null) {
    void loadFlag();
  }
});
</script>

<template>
  <div class="card">
    <div class="card-header">
      <h5 class="mb-0">Push notifications</h5>
    </div>
    <div class="card-body">
      <ul class="nav nav-tabs" role="tablist">
        <li class="nav-item" role="presentation">
          <button
            type="button"
            class="nav-link"
            :class="{ active: activeTab === 'compose' }"
            data-test="pushes-tab-compose"
            @click="activeTab = 'compose'"
          >
            Compose
          </button>
        </li>
        <li class="nav-item" role="presentation">
          <button
            type="button"
            class="nav-link"
            :class="{ active: activeTab === 'scheduled' }"
            data-test="pushes-tab-scheduled"
            @click="activeTab = 'scheduled'"
          >
            Scheduled
          </button>
        </li>
        <li class="nav-item" role="presentation">
          <button
            type="button"
            class="nav-link"
            :class="{ active: activeTab === 'metrics' }"
            data-test="pushes-tab-metrics"
            @click="activeTab = 'metrics'"
          >
            Metrics
          </button>
        </li>
        <li class="nav-item" role="presentation">
          <button
            type="button"
            class="nav-link"
            :class="{ active: activeTab === 'flag' }"
            data-test="pushes-tab-flag"
            @click="activeTab = 'flag'"
          >
            Feature flag
          </button>
        </li>
      </ul>

      <div class="tab-content pt-3">
        <!-- Compose ------------------------------------------------- -->
        <div v-show="activeTab === 'compose'">
          <form class="row g-3" @submit.prevent="sendCompose">
            <div class="col-12">
              <label class="form-label">Domains *</label>
              <div v-if="domainsLoading" class="text-muted small">Loading domains...</div>
              <div v-else-if="domainsError" class="alert alert-warning py-2 mb-2">
                {{ domainsError }}
              </div>
              <div v-else-if="domains.length === 0" class="text-muted small">
                No domains available. Create one in the Domains page first.
              </div>
              <div v-else class="border rounded p-2" style="max-height: 180px; overflow-y: auto;">
                <div
                  v-for="d in domains"
                  :key="d.id"
                  class="form-check"
                >
                  <input
                    :id="`compose-domain-${d.id}`"
                    type="checkbox"
                    class="form-check-input"
                    :checked="compose.domainIds.includes(d.id)"
                    @change="toggleDomain(d.id, ($event.target as HTMLInputElement).checked)"
                  />
                  <label class="form-check-label" :for="`compose-domain-${d.id}`">
                    {{ d.domain }}
                    <small v-if="d.user" class="text-muted ms-2">
                      ({{ d.user.email }})
                    </small>
                  </label>
                </div>
              </div>
            </div>

            <div class="col-md-6">
              <label class="form-label" for="pushes-compose-category">Category *</label>
              <select
                id="pushes-compose-category"
                v-model.number="compose.category"
                class="form-select"
                required
              >
                <option
                  v-for="opt in PUSH_CATEGORY_OPTIONS"
                  :key="opt.value"
                  :value="opt.value"
                >
                  {{ opt.label }}
                </option>
              </select>
            </div>

            <div class="col-md-6">
              <label class="form-label" for="pushes-compose-priority">Priority</label>
              <select
                id="pushes-compose-priority"
                v-model.number="compose.priority"
                class="form-select"
              >
                <option
                  v-for="opt in PUSH_PRIORITY_OPTIONS"
                  :key="opt.value"
                  :value="opt.value"
                >
                  {{ opt.label }}
                </option>
              </select>
            </div>

            <div class="col-12">
              <label class="form-label" for="pushes-compose-title">Title *</label>
              <input
                id="pushes-compose-title"
                v-model="compose.title"
                type="text"
                class="form-control"
                maxlength="120"
                required
              />
            </div>

            <div class="col-12">
              <label class="form-label" for="pushes-compose-body">Body *</label>
              <textarea
                id="pushes-compose-body"
                v-model="compose.body"
                class="form-control"
                rows="3"
                maxlength="2000"
                required
              ></textarea>
            </div>

            <div class="col-md-6">
              <label class="form-label" for="pushes-compose-image">Image URL</label>
              <input
                id="pushes-compose-image"
                v-model="compose.imageUrl"
                type="url"
                class="form-control"
                placeholder="https://..."
              />
            </div>

            <div class="col-md-6">
              <label class="form-label" for="pushes-compose-deeplink">Deep link</label>
              <input
                id="pushes-compose-deeplink"
                v-model="compose.deepLink"
                type="text"
                class="form-control"
                placeholder="/events/123"
              />
            </div>

            <div class="col-md-4">
              <label class="form-label" for="pushes-compose-ttl">TTL (seconds)</label>
              <input
                id="pushes-compose-ttl"
                v-model.number="compose.ttlSeconds"
                type="number"
                min="0"
                step="1"
                class="form-control"
              />
              <small class="text-muted">0 = no expiry</small>
            </div>

            <div v-if="compose.error" class="col-12">
              <div class="alert alert-danger mb-0">{{ compose.error }}</div>
            </div>

            <div class="col-12 d-flex justify-content-end">
              <button
                type="submit"
                class="btn btn-primary"
                :disabled="composeDisabled"
                data-test="pushes-compose-send"
              >
                <span
                  v-if="compose.loading"
                  class="spinner-border spinner-border-sm me-2"
                  role="status"
                  aria-hidden="true"
                ></span>
                Send
              </button>
            </div>
          </form>
        </div>

        <!-- Scheduled ----------------------------------------------- -->
        <div v-show="activeTab === 'scheduled'">
          <div class="d-flex justify-content-between align-items-center mb-3">
            <h6 class="mb-0">Scheduled pushes</h6>
            <button
              type="button"
              class="btn btn-sm btn-primary"
              data-test="pushes-schedule-toggle"
              @click="newScheduleOpen = !newScheduleOpen"
            >
              {{ newScheduleOpen ? "Cancel" : "New schedule" }}
            </button>
          </div>

          <div v-if="newScheduleOpen" class="card mb-3">
            <div class="card-body">
              <form class="row g-3" @submit.prevent="createSchedule">
                <div class="col-12">
                  <label class="form-label">Domains *</label>
                  <div
                    v-if="domains.length === 0"
                    class="text-muted small"
                  >
                    No domains available.
                  </div>
                  <div
                    v-else
                    class="border rounded p-2"
                    style="max-height: 160px; overflow-y: auto;"
                  >
                    <div
                      v-for="d in domains"
                      :key="`s-domain-${d.id}`"
                      class="form-check"
                    >
                      <input
                        :id="`schedule-domain-${d.id}`"
                        type="checkbox"
                        class="form-check-input"
                        :checked="scheduleForm.domainIds.includes(d.id)"
                        @change="toggleScheduleDomain(d.id, ($event.target as HTMLInputElement).checked)"
                      />
                      <label class="form-check-label" :for="`schedule-domain-${d.id}`">
                        {{ d.domain }}
                      </label>
                    </div>
                  </div>
                </div>

                <div class="col-md-6">
                  <label class="form-label" for="pushes-schedule-category">Category *</label>
                  <select
                    id="pushes-schedule-category"
                    v-model.number="scheduleForm.category"
                    class="form-select"
                  >
                    <option
                      v-for="opt in PUSH_CATEGORY_OPTIONS"
                      :key="opt.value"
                      :value="opt.value"
                    >
                      {{ opt.label }}
                    </option>
                  </select>
                </div>

                <div class="col-md-6">
                  <label class="form-label" for="pushes-schedule-priority">Priority</label>
                  <select
                    id="pushes-schedule-priority"
                    v-model.number="scheduleForm.priority"
                    class="form-select"
                  >
                    <option
                      v-for="opt in PUSH_PRIORITY_OPTIONS"
                      :key="opt.value"
                      :value="opt.value"
                    >
                      {{ opt.label }}
                    </option>
                  </select>
                </div>

                <div class="col-12">
                  <label class="form-label" for="pushes-schedule-title">Title *</label>
                  <input
                    id="pushes-schedule-title"
                    v-model="scheduleForm.title"
                    type="text"
                    class="form-control"
                    maxlength="120"
                    required
                  />
                </div>

                <div class="col-12">
                  <label class="form-label" for="pushes-schedule-body">Body *</label>
                  <textarea
                    id="pushes-schedule-body"
                    v-model="scheduleForm.body"
                    class="form-control"
                    rows="2"
                    maxlength="2000"
                    required
                  ></textarea>
                </div>

                <div class="col-md-6">
                  <label class="form-label" for="pushes-schedule-type">Schedule type *</label>
                  <select
                    id="pushes-schedule-type"
                    v-model.number="scheduleForm.scheduleType"
                    class="form-select"
                  >
                    <option :value="PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT">One-shot</option>
                    <option :value="PushScheduleType.PUSH_SCHEDULE_TYPE_RECURRING">Recurring (cron)</option>
                  </select>
                </div>

                <div
                  v-if="scheduleForm.scheduleType === PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT"
                  class="col-md-6"
                >
                  <label class="form-label" for="pushes-schedule-runat">Run at *</label>
                  <input
                    id="pushes-schedule-runat"
                    v-model="scheduleForm.runAt"
                    type="datetime-local"
                    class="form-control"
                  />
                </div>

                <div
                  v-else
                  class="col-md-6"
                >
                  <label class="form-label" for="pushes-schedule-cron">Cron expression *</label>
                  <input
                    id="pushes-schedule-cron"
                    v-model="scheduleForm.cronExpr"
                    type="text"
                    class="form-control"
                    placeholder="0 9 * * *"
                  />
                </div>

                <div v-if="scheduleForm.error" class="col-12">
                  <div class="alert alert-danger mb-0">{{ scheduleForm.error }}</div>
                </div>

                <div class="col-12 d-flex justify-content-end">
                  <button
                    type="submit"
                    class="btn btn-primary"
                    :disabled="scheduleForm.loading"
                    data-test="pushes-schedule-create"
                  >
                    <span
                      v-if="scheduleForm.loading"
                      class="spinner-border spinner-border-sm me-2"
                      role="status"
                      aria-hidden="true"
                    ></span>
                    Create
                  </button>
                </div>
              </form>
            </div>
          </div>

          <div v-if="schedulePagination.error" class="alert alert-danger">
            {{ schedulePagination.error }}
          </div>

          <div
            v-if="schedulePagination.loading && schedulePagination.data.length === 0"
            class="text-center"
          >
            <div class="spinner-border" role="status">
              <span class="visually-hidden">Loading schedules...</span>
            </div>
          </div>

          <div
            v-else-if="schedulePagination.data.length > 0"
            class="table-responsive"
          >
            <table class="table table-striped align-middle">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Title</th>
                  <th>Type</th>
                  <th>When</th>
                  <th>Active</th>
                  <th>Next fire</th>
                  <th>Last fired</th>
                  <th>Actions</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="s in schedulePagination.data" :key="s.id">
                  <td>{{ s.id }}</td>
                  <td>{{ s.payload?.title ?? "-" }}</td>
                  <td>{{ scheduleTypeLabel(s.scheduleType) }}</td>
                  <td>
                    <code v-if="s.scheduleType === PushScheduleType.PUSH_SCHEDULE_TYPE_ONE_SHOT">
                      {{ formatTimestamp(s.runAt) }}
                    </code>
                    <code v-else>{{ s.cronExpr || "-" }}</code>
                  </td>
                  <td>
                    <span
                      class="badge"
                      :class="s.isActive ? 'bg-success' : 'bg-secondary'"
                    >
                      {{ s.isActive ? "Active" : "Paused" }}
                    </span>
                  </td>
                  <td>{{ formatTimestamp(s.nextFireAt) }}</td>
                  <td>{{ formatTimestamp(s.lastFiredAt) }}</td>
                  <td>
                    <button
                      type="button"
                      class="btn btn-sm btn-outline-primary me-2"
                      :disabled="schedulePagination.loading"
                      @click="toggleScheduleActive(s)"
                    >
                      {{ s.isActive ? "Pause" : "Activate" }}
                    </button>
                    <button
                      type="button"
                      class="btn btn-sm btn-outline-danger"
                      :disabled="schedulePagination.loading"
                      @click="deleteSchedule(s.id)"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>

            <div
              v-if="schedulePagination.hasMore"
              class="d-flex justify-content-center mt-3"
            >
              <button
                type="button"
                class="btn btn-sm btn-outline-secondary"
                :disabled="schedulePagination.loading"
                @click="loadMoreSchedules"
              >
                Load more
              </button>
            </div>
          </div>

          <div
            v-else-if="!schedulePagination.loading"
            class="text-center text-muted"
          >
            No schedules yet.
          </div>
        </div>

        <!-- Metrics ------------------------------------------------- -->
        <div v-show="activeTab === 'metrics'">
          <div class="d-flex justify-content-between align-items-center mb-3">
            <h6 class="mb-0">Aggregate metrics</h6>
            <button
              type="button"
              class="btn btn-sm btn-outline-primary"
              :disabled="aggregateLoading || recentPushesLoading"
              @click="refreshMetrics"
            >
              Refresh
            </button>
          </div>

          <div v-if="aggregateError" class="alert alert-danger">
            {{ aggregateError }}
          </div>

          <div v-if="aggregateLoading && !aggregate" class="text-center">
            <div class="spinner-border" role="status">
              <span class="visually-hidden">Loading aggregate metrics...</span>
            </div>
          </div>

          <div v-else-if="aggregate" class="row g-3 mb-4">
            <div class="col-md-4 col-sm-6">
              <div class="card h-100">
                <div class="card-body">
                  <small class="text-muted text-uppercase">Messages sent</small>
                  <div class="h4 mb-0">{{ formatCount(aggregate.messagesSentTotal) }}</div>
                </div>
              </div>
            </div>
            <div class="col-md-4 col-sm-6">
              <div class="card h-100">
                <div class="card-body">
                  <small class="text-muted text-uppercase">Messages delivered</small>
                  <div class="h4 mb-0">{{ formatCount(aggregate.messagesDeliveredTotal) }}</div>
                </div>
              </div>
            </div>
            <div class="col-md-4 col-sm-6">
              <div class="card h-100">
                <div class="card-body">
                  <small class="text-muted text-uppercase">Delivery rate</small>
                  <div class="h4 mb-0">{{ formatPercent(aggregate.deliveryRateTotal) }}</div>
                </div>
              </div>
            </div>
            <div class="col-md-4 col-sm-6">
              <div class="card h-100">
                <div class="card-body">
                  <small class="text-muted text-uppercase">Active schedules</small>
                  <div class="h4 mb-0">{{ formatCount(aggregate.activeSchedules) }}</div>
                </div>
              </div>
            </div>
            <div class="col-md-4 col-sm-6">
              <div class="card h-100">
                <div class="card-body">
                  <small class="text-muted text-uppercase">Scheduled fires (24h)</small>
                  <div class="h4 mb-0">{{ formatCount(aggregate.scheduledFires24h) }}</div>
                </div>
              </div>
            </div>
            <div class="col-md-4 col-sm-6">
              <div class="card h-100">
                <div class="card-body">
                  <small class="text-muted text-uppercase">Audience size</small>
                  <div class="h4 mb-0">{{ formatCount(aggregate.audienceSize) }}</div>
                </div>
              </div>
            </div>
          </div>

          <h6 class="mb-2">Recent pushes</h6>

          <div v-if="recentPushesError" class="alert alert-danger">
            {{ recentPushesError }}
          </div>

          <div
            v-if="recentPushesLoading && recentPushes.length === 0"
            class="text-center"
          >
            <div class="spinner-border" role="status">
              <span class="visually-hidden">Loading recent pushes...</span>
            </div>
          </div>

          <div v-else-if="recentPushes.length > 0" class="table-responsive">
            <table class="table table-striped align-middle">
              <thead>
                <tr>
                  <th>ID</th>
                  <th>Title</th>
                  <th>Category</th>
                  <th>Priority</th>
                  <th>Source</th>
                  <th>Created</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                <template v-for="p in recentPushes" :key="p.id">
                  <tr
                    :class="{ 'table-active': expandedPushId === p.id }"
                    style="cursor: pointer"
                    @click="togglePushExpansion(p)"
                  >
                    <td>{{ p.id }}</td>
                    <td>{{ p.title }}</td>
                    <td>{{ categoryLabel(p.category) }}</td>
                    <td>{{ priorityLabel(p.priority) }}</td>
                    <td>{{ p.source || "-" }}</td>
                    <td>{{ formatTimestamp(p.createdAt) }}</td>
                    <td>
                      <span aria-hidden="true">
                        {{ expandedPushId === p.id ? "▾" : "▸" }}
                      </span>
                    </td>
                  </tr>
                  <tr v-if="expandedPushId === p.id">
                    <td colspan="7" class="bg-light">
                      <div v-if="campaignMetricsLoading[p.id]" class="text-center py-2">
                        <div class="spinner-border spinner-border-sm" role="status">
                          <span class="visually-hidden">Loading metrics...</span>
                        </div>
                      </div>
                      <div
                        v-else-if="campaignMetricsById[p.id]"
                        class="row g-2"
                      >
                        <div class="col-md-3 col-sm-6">
                          <small class="text-muted text-uppercase">Targets</small>
                          <div>{{ formatCount(campaignMetricsById[p.id]!.targetsTotal) }}</div>
                        </div>
                        <div class="col-md-3 col-sm-6">
                          <small class="text-muted text-uppercase">Delivered</small>
                          <div>{{ formatCount(campaignMetricsById[p.id]!.delivered) }}</div>
                        </div>
                        <div class="col-md-3 col-sm-6">
                          <small class="text-muted text-uppercase">Not delivered</small>
                          <div>{{ formatCount(campaignMetricsById[p.id]!.notDelivered) }}</div>
                        </div>
                        <div class="col-md-3 col-sm-6">
                          <small class="text-muted text-uppercase">Delivery rate</small>
                          <div>{{ formatPercent(campaignMetricsById[p.id]!.deliveryRate) }}</div>
                        </div>
                        <div class="col-md-3 col-sm-6">
                          <small class="text-muted text-uppercase">Latency p50</small>
                          <div>{{ formatCount(campaignMetricsById[p.id]!.latencyP50Ms) }} ms</div>
                        </div>
                        <div class="col-md-3 col-sm-6">
                          <small class="text-muted text-uppercase">Latency p95</small>
                          <div>{{ formatCount(campaignMetricsById[p.id]!.latencyP95Ms) }} ms</div>
                        </div>
                        <div
                          v-if="campaignMetricsById[p.id]!.failuresByReason.length > 0"
                          class="col-12 mt-2"
                        >
                          <small class="text-muted text-uppercase d-block mb-1">
                            Failures by reason
                          </small>
                          <ul class="list-unstyled mb-0">
                            <li
                              v-for="f in campaignMetricsById[p.id]!.failuresByReason"
                              :key="f.reason"
                            >
                              <code>{{ failureReasonLabel(f.reason) }}</code>
                              <span class="ms-2">{{ formatCount(f.count) }}</span>
                            </li>
                          </ul>
                        </div>
                      </div>
                      <div v-else class="text-muted small">No metrics available.</div>
                    </td>
                  </tr>
                </template>
              </tbody>
            </table>
          </div>

          <div
            v-else-if="!recentPushesLoading"
            class="text-center text-muted"
          >
            No pushes yet.
          </div>
        </div>

        <!-- Feature flag -------------------------------------------- -->
        <div v-show="activeTab === 'flag'">
          <h6 class="mb-3">My account</h6>

          <div v-if="currentUserId === null" class="text-muted" data-test="pushes-flag-loading">
            {{ flagPlaceholder }}
          </div>

          <div v-else>
            <div v-if="flagState.error" class="alert alert-danger">
              {{ flagState.error }}
            </div>

            <div
              v-if="!flagState.initialLoaded && flagState.loading"
              class="text-muted"
            >
              {{ flagPlaceholder }}
            </div>

            <div
              v-else
              class="form-check form-switch d-flex align-items-center gap-2"
            >
              <input
                id="pushes-flag-toggle"
                type="checkbox"
                class="form-check-input"
                role="switch"
                :checked="flagState.enabled"
                :disabled="flagState.toggling"
                data-test="pushes-flag-toggle"
                @change="onFlagChange"
              />
              <label class="form-check-label" for="pushes-flag-toggle">
                Enable push notifications for my account
              </label>
              <span
                v-if="flagState.toggling"
                class="spinner-border spinner-border-sm ms-2"
                role="status"
                aria-hidden="true"
              ></span>
            </div>

            <small class="text-muted d-block mt-2">
              When disabled, the create endpoints return 403 and the
              compose / scheduled forms cannot send.
            </small>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
