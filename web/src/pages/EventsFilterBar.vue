<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { toast } from "vue3-toastify";
import { eventsApiService } from "../store/services";
import type { EventsPageFilters } from "../store/services/models/apiModels";

const props = defineProps<{ modelValue: EventsPageFilters }>();
const emit = defineEmits<{ (e: "update:modelValue", value: EventsPageFilters): void }>();

// Labels for the canonical slugs of internal/scraper/scores365/feed.go
// (sportMap). The category list itself comes from the database: the
// sport of an event is copied from the league it was scraped for and the
// catalog lets an operator type any category there, so the canonical
// slugs are only a subset of what the events table holds.
const SPORT_LABELS: Record<string, string> = {
  football: "Fútbol",
  basketball: "Baloncesto",
  tennis: "Tenis",
  "ice-hockey": "Hockey hielo",
  handball: "Balonmano",
  "american-football": "Fútbol americano",
  baseball: "Béisbol",
  volleyball: "Voleibol",
  rugby: "Rugby",
};

const CANONICAL_SPORTS = Object.keys(SPORT_LABELS);

const STATUSES = [
  { value: "", label: "Todos" },
  { value: "inprogress", label: "En vivo" },
  { value: "notstarted", label: "No iniciados" },
  { value: "finished", label: "Finalizados" },
] as const;

const sports = ref<string[]>([]);
const leagues = ref<string[]>([]);
const leagueInput = ref(props.modelValue.league ?? "");
const teamInput = ref(props.modelValue.team ?? "");
const debounceTimers: Record<"league" | "team", number | null> = { league: null, team: null };

const isAsc = computed(() => props.modelValue.direction !== "desc");
const activeFilterCount = computed(() => {
  const { sport, status, league, team } = props.modelValue;
  return [sport, status, league, team].filter((value) => !!value).length;
});

// The options are the categories the database holds, with the selected
// one always present so a filter kept in the URL never disappears.
const sportOptions = computed(() => {
  const values = new Set(sports.value.length ? sports.value : CANONICAL_SPORTS);
  const selected = props.modelValue.sport;
  if (selected) values.add(selected);
  return [...values]
    .sort(compareSports)
    .map((value) => ({ value, label: SPORT_LABELS[value] ?? value }));
});

// Canonical categories first, in the order of the scraper's vocabulary,
// then the ones the operator added, alphabetically.
function compareSports(a: string, b: string): number {
  const indexA = CANONICAL_SPORTS.indexOf(a);
  const indexB = CANONICAL_SPORTS.indexOf(b);
  if (indexA !== -1 && indexB !== -1) return indexA - indexB;
  if (indexA !== -1) return -1;
  if (indexB !== -1) return 1;
  return a.localeCompare(b);
}

onMounted(async () => {
  try {
    const categories = await eventsApiService.getSports();
    if (categories.length) sports.value = categories;
  } catch (err) {
    toast.error(
      err instanceof Error
        ? err.message
        : "No se pudieron cargar las categorías",
    );
  }
  try {
    const names = await eventsApiService.getLeagues();
    if (names.length) leagues.value = names;
  } catch (err) {
    toast.error(
      err instanceof Error
        ? err.message
        : "No se pudieron cargar las ligas",
    );
  }
});

function commit(patch: Partial<EventsPageFilters>): void {
  emit("update:modelValue", { ...props.modelValue, ...patch });
}

function setDirection(direction: "asc" | "desc"): void {
  commit({ direction });
}

function debouncedText(field: "league" | "team", value: string): void {
  if (debounceTimers[field] !== null) window.clearTimeout(debounceTimers[field]!);
  debounceTimers[field] = window.setTimeout(() => {
    if (value.length >= 2) {
      commit({ [field]: value } as Partial<EventsPageFilters>);
    } else if (value.length === 0) {
      commit({ [field]: "" } as Partial<EventsPageFilters>);
    }
    // 1 char: ignored (don't emit anything)
  }, 300);
}

function clearFilters(): void {
  for (const field of ["league", "team"] as const) {
    if (debounceTimers[field] !== null) {
      window.clearTimeout(debounceTimers[field]!);
      debounceTimers[field] = null;
    }
  }
  leagueInput.value = "";
  teamInput.value = "";
  commit({ sport: "", status: "", league: "", team: "" });
}

onBeforeUnmount(() => {
  for (const field of ["league", "team"] as const) {
    if (debounceTimers[field] !== null) {
      window.clearTimeout(debounceTimers[field]!);
      debounceTimers[field] = null;
    }
  }
});
</script>

<template>
  <div class="filter-toolbar border rounded-3 bg-body-tertiary p-3 p-lg-4 mb-3">
    <div class="row g-3 align-items-end">
      <div class="col-12 col-sm-6 col-xl-auto">
        <span id="events-sort-label" class="form-label d-block">Orden</span>
        <div class="btn-group w-100" role="group" aria-labelledby="events-sort-label">
          <button
            type="button"
            class="btn btn-outline-secondary"
            :class="{ active: isAsc }"
            :aria-pressed="isAsc"
            aria-label="Orden ascendente"
            @click="setDirection('asc')"
          >
            <i class="icon-base bx bx-sort-up me-1" aria-hidden="true"></i>ASC
          </button>
          <button
            type="button"
            class="btn btn-outline-secondary"
            :class="{ active: !isAsc }"
            :aria-pressed="!isAsc"
            aria-label="Orden descendente"
            @click="setDirection('desc')"
          >
            <i class="icon-base bx bx-sort-down me-1" aria-hidden="true"></i>DESC
          </button>
        </div>
      </div>

      <div class="col-12 col-sm-6 col-xl-2">
        <label class="form-label" for="events-sport">Deporte</label>
        <select
          id="events-sport"
          name="sport"
          class="form-select"
          :value="modelValue.sport ?? ''"
          @change="commit({ sport: ($event.target as HTMLSelectElement).value })"
        >
          <option value="">Todos</option>
          <option v-for="sport in sportOptions" :key="sport.value" :value="sport.value">
            {{ sport.label }}
          </option>
        </select>
      </div>

      <div class="col-12 col-sm-6 col-xl-2">
        <label class="form-label" for="events-status">Estado</label>
        <select
          id="events-status"
          name="status"
          class="form-select"
          :value="modelValue.status ?? ''"
          @change="commit({ status: ($event.target as HTMLSelectElement).value })"
        >
          <option v-for="opt in STATUSES" :key="opt.value" :value="opt.value">
            {{ opt.label }}
          </option>
        </select>
      </div>

      <div class="col-12 col-sm-6 col-xl-2">
        <label class="form-label" for="events-league">Liga</label>
        <input
          id="events-league"
          v-model="leagueInput"
          type="text"
          name="league"
          class="form-control"
          placeholder="Liga (mín. 2)"
          list="events-league-options"
          autocomplete="off"
          @input="debouncedText('league', leagueInput)"
        />
        <datalist id="events-league-options">
          <option v-for="league in leagues" :key="league" :value="league"></option>
        </datalist>
      </div>

      <div class="col-12 col-sm-6 col-xl-2">
        <label class="form-label" for="events-team">Equipo</label>
        <input
          id="events-team"
          v-model="teamInput"
          type="text"
          name="team"
          class="form-control"
          placeholder="Equipo (mín. 2)"
          autocomplete="off"
          @input="debouncedText('team', teamInput)"
        />
      </div>

      <div class="col-12 col-xl-auto ms-xl-auto">
        <div class="d-flex align-items-center gap-2">
          <span
            v-if="activeFilterCount > 0"
            class="badge bg-label-primary d-inline-flex align-items-center gap-1"
            data-testid="active-filters"
          >
            <i class="icon-base bx bx-filter" aria-hidden="true"></i>
            {{ activeFilterCount }} {{ activeFilterCount === 1 ? "filtro" : "filtros" }}
          </span>
          <button
            type="button"
            class="btn btn-outline-secondary flex-grow-1"
            data-testid="clear-filters"
            :disabled="activeFilterCount === 0"
            @click="clearFilters"
          >
            <i class="icon-base bx bx-reset me-1" aria-hidden="true"></i>Limpiar
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* Sneat's placeholder color measures 1.9:1 on this surface. */
.filter-toolbar .form-control::placeholder {
  color: #5a6470;
  opacity: 1;
}

[data-bs-theme="dark"] .filter-toolbar .form-control::placeholder {
  color: #c2c3d1;
}
</style>
