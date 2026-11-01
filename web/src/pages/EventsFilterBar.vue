<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from "vue";
import type { EventsPageFilters } from "../store/services/models/apiModels";

const props = defineProps<{ modelValue: EventsPageFilters }>();
const emit = defineEmits<{ (e: "update:modelValue", value: EventsPageFilters): void }>();

// Canonical slugs of internal/scraper/scores365/feed.go (sportMap) and
// internal/scraper/catalog/discovery.go. The API filters by exact match.
const SPORTS = [
  { value: "football", label: "Fútbol" },
  { value: "basketball", label: "Baloncesto" },
  { value: "tennis", label: "Tenis" },
  { value: "ice-hockey", label: "Hockey hielo" },
  { value: "handball", label: "Balonmano" },
  { value: "american-football", label: "Fútbol americano" },
  { value: "baseball", label: "Béisbol" },
  { value: "volleyball", label: "Voleibol" },
  { value: "rugby", label: "Rugby" },
] as const;

const STATUSES = [
  { value: "", label: "Todos" },
  { value: "inprogress", label: "En vivo" },
  { value: "notstarted", label: "No iniciados" },
  { value: "finished", label: "Finalizados" },
] as const;

const leagueInput = ref(props.modelValue.league ?? "");
const teamInput = ref(props.modelValue.team ?? "");
const debounceTimers: Record<"league" | "team", number | null> = { league: null, team: null };

const isAsc = computed(() => props.modelValue.direction !== "desc");
const activeFilterCount = computed(() => {
  const { sport, status, league, team } = props.modelValue;
  return [sport, status, league, team].filter((value) => !!value).length;
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
          <option v-for="sport in SPORTS" :key="sport.value" :value="sport.value">
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
          autocomplete="off"
          @input="debouncedText('league', leagueInput)"
        />
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
