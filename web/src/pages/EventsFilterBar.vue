<script setup lang="ts">
import { computed, ref, watch } from "vue";
import type { EventsPageFilters } from "../store/services/models/apiModels";

const props = defineProps<{ modelValue: EventsPageFilters }>();
const emit = defineEmits<{ (e: "update:modelValue", value: EventsPageFilters): void }>();

// Categories surfaced here are limited to the non-football
// sports the multi-sport scraper (TheSportsDB, PR #131) seeds
// in `internal/seeder/defaults.go`. Football events live on the
// FotMob source and are excluded so the dropdown reads as a
// switch between scrapers, not a free-form sport selector.
//
// Adding a new TheSportsDB sport requires two changes:
//   1. Seed a `scraper_leagues` row in `initialNonFootballLeagues`
//      with the canonical lowercase sport string.
//   2. Append that sport to this list.
const SPORTS = ["basketball", "american-football", "baseball", "ice-hockey"] as const;
const STATUSES = [
  { value: "", label: "Todos" },
  { value: "inprogress", label: "En vivo" },
  { value: "notstarted", label: "No iniciados" },
  { value: "finished", label: "Finalizados" },
] as const;

const leagueInput = ref(props.modelValue.league ?? "");
const teamInput = ref(props.modelValue.team ?? "");
const debounceTimers: Record<"league" | "team", number | null> = { league: null, team: null };

function commit(patch: Partial<EventsPageFilters>): void {
  emit("update:modelValue", { ...props.modelValue, ...patch });
}

function toggleDir(): void {
  commit({ direction: props.modelValue.direction === "asc" ? "desc" : "asc" });
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

watch(leagueInput, (v) => debouncedText("league", v));
watch(teamInput, (v) => debouncedText("team", v));

const isAsc = computed(() => props.modelValue.direction !== "desc");
</script>

<template>
  <div class="row g-2 align-items-center mb-3">
    <div class="col-auto">
      <div class="btn-group" role="group" aria-label="Sort direction">
        <button
          type="button"
          class="btn btn-sm btn-outline-secondary"
          :class="{ active: isAsc }"
          @click="toggleDir"
        >
          ASC ⇅
        </button>
        <button
          type="button"
          class="btn btn-sm btn-outline-secondary"
          :class="{ active: !isAsc }"
          @click="toggleDir"
        >
          DESC ⇅
        </button>
      </div>
    </div>

    <div class="col-auto">
      <select
        class="form-select form-select-sm"
        :value="modelValue.sport ?? ''"
        @change="commit({ sport: ($event.target as HTMLSelectElement).value })"
      >
        <option value="">Todos los deportes</option>
        <option v-for="s in SPORTS" :key="s" :value="s">{{ s }}</option>
      </select>
    </div>

    <div class="col-auto">
      <select
        class="form-select form-select-sm"
        :value="modelValue.status ?? ''"
        @change="commit({ status: ($event.target as HTMLSelectElement).value })"
      >
        <option v-for="opt in STATUSES" :key="opt.value" :value="opt.value">
          {{ opt.label }}
        </option>
      </select>
    </div>

    <div class="col-auto">
      <input
        v-model="leagueInput"
        type="text"
        name="league"
        class="form-control form-control-sm"
        placeholder="Liga (mín 2 letras)"
      />
    </div>

    <div class="col-auto">
      <input
        v-model="teamInput"
        type="text"
        name="team"
        class="form-control form-control-sm"
        placeholder="Equipo (mín 2 letras)"
      />
    </div>
  </div>
</template>
