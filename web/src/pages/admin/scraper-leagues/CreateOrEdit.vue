<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div
      ref="dialog"
      class="modal modal-visible"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      tabindex="-1"
    >
      <h2 :id="titleId">{{ item ? 'Edit league' : 'Add new league' }}</h2>

      <label>Search</label>
      <input v-model="search" class="search" placeholder="Search leagues..." />

      <ul v-if="suggestions.length > 0" class="suggestions">
        <li v-for="s in suggestions" :key="s.source_league_id" class="suggestion" @click="select(s)">
          {{ s.name }} ({{ s.country }}) - {{ s.source_league_id }}
        </li>
      </ul>
      <div v-if="searchError" class="alert alert-danger" role="alert">{{ searchError }}</div>

      <label>Name</label>
      <input v-model="form.name" />

      <label>Country</label>
      <input v-model="form.country" maxlength="8" />

      <label>Sport</label>
      <input v-model="form.sport" />

      <label for="override-source">Source override</label>
      <select id="override-source" v-model="form.override_source">
        <option :value="null">— use natural source —</option>
        <option :value="form.source">{{ form.source }}</option>
      </select>
      <small class="hint">
        Default: null (use natural source). An override may only name this league's own
        source, because a league id only means something inside the source that published it.
      </small>

      <label>
        <input type="checkbox" v-model="form.enabled" />
        Enabled
      </label>

      <div v-if="error" class="alert alert-danger" role="alert">{{ error }}</div>

      <div class="actions">
        <button @click="$emit('close')">Cancel</button>
        <button class="submit" :disabled="saving" @click="onSave">Save</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onBeforeUnmount, useId } from 'vue'
import { useScraperLeaguesStore, type ScraperLeague } from '../../../store/pinia/scraperLeaguesStore'
import { useModal } from '../../../composables/useModal'

// NOTE on @/ alias: the brief's snippet uses `@/store/...` imports. The alias
// only exists in vite.config.ts (runtime); there is no matching `paths` mapping
// in tsconfig.app.json, and `eslint.config.js` does not register the alias with
// import/resolver. The rest of the project uses relative imports (e.g.
// pages/admin/scraper-leagues/index.vue:76). Switched to relative imports — the
// file passes `vue-tsc -b` and `npm run lint` cleanly this way.
const props = defineProps<{ item: ScraperLeague | null }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()

const titleId = useId()
const dialog = ref<HTMLElement | null>(null)
useModal({ isOpen: () => true, container: dialog, onClose: () => emit('close') })

const store = useScraperLeaguesStore()
const search = ref('')
const suggestions = ref<Array<{ source: string; source_league_id: string; name: string; country: string; sport: string }>>([])
const error = ref('')
const searchError = ref('')
const saving = ref(false)

const form = reactive({
  source: 'fotmob',
  source_league_id: '',
  name: '',
  country: '',
  sport: 'football',
  enabled: true,
  override_source: null as string | null,
})

function applyItem(item: ScraperLeague | null): void {
  form.source = item?.source ?? 'fotmob'
  form.source_league_id = item?.source_league_id ?? ''
  form.name = item?.name ?? ''
  form.country = item?.country ?? ''
  form.sport = item?.sport ?? 'football'
  form.enabled = item?.enabled ?? true
  // A stored override that names another source is rejected by the backend,
  // so it is dropped instead of being shown and resubmitted.
  form.override_source = item?.override_source === form.source ? item.override_source : null
}

// The modal is reused between "add new" and "edit", so the form has to
// follow the prop instead of only reading it once on mount.
watch(() => props.item, applyItem, { immediate: true })

let searchTimer: number | null = null
let searchSeq = 0

watch(search, (q) => {
  if (searchTimer !== null) window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    void runSearch(q)
  }, 250)
})

async function runSearch(q: string): Promise<void> {
  if (q.length < 2) {
    suggestions.value = []
    searchError.value = ''
    return
  }
  // F3 (PR #123): the backend FotMob source returns `nil` when nothing
  // matches, and the JSON handler encodes that as `{"data": null}`. The
  // store hands the raw payload through, so this can be null. Guard the
  // assignment so the template's `suggestions.length` access doesn't
  // throw. The store is out of scope for this PR, so the defensive
  // normalization lives at the call site.
  const seq = ++searchSeq
  try {
    const result = await store.searchLeagues(q)
    // A newer search was started while this one was in flight: its result
    // is the one the user is waiting for.
    if (seq !== searchSeq) return
    searchError.value = ''
    suggestions.value = Array.isArray(result) ? result : []
  } catch (err) {
    if (seq !== searchSeq) return
    suggestions.value = []
    searchError.value =
      err instanceof Error && err.message ? err.message : 'No se pudo buscar ligas'
  }
}

onBeforeUnmount(() => {
  if (searchTimer !== null) {
    window.clearTimeout(searchTimer)
    searchTimer = null
  }
  searchSeq++
})

function select(s: { source: string; source_league_id: string; name: string; country: string; sport: string }) {
  // The suggestion carries the source that published it: a league id only
  // means something inside its own source, so the form must follow it.
  if (s.source) form.source = s.source
  form.source_league_id = s.source_league_id
  form.name = s.name
  form.country = s.country
  form.sport = s.sport
  suggestions.value = []
}

async function onSave() {
  error.value = ''
  saving.value = true
  // Safety net: the select only offers the row's own source, but a value
  // reaching the model from anywhere else must never be submitted as a
  // foreign override (the backend answers 400).
  const override = form.override_source === form.source ? form.override_source : null
  try {
    if (props.item) {
      await store.update(props.item.id, {
        name: form.name,
        country: form.country,
        sport: form.sport,
        enabled: form.enabled,
        override_source: override,
      })
    } else {
      await store.create({ ...form, override_source: override })
    }
    emit('saved')
  } catch (err) {
    error.value =
      err instanceof Error && err.message ? err.message : "No se pudo guardar la liga"
  } finally {
    saving.value = false
  }
}
</script>

<style scoped>
/* assets/vendor/css/core.css declares `.modal { display: none }` for the
 * Bootstrap modal lifecycle. We don't bootstrap Bootstrap JS, so we never add
 * the `.show` class that flips it back on — the modal content stays invisible.
 * `modal-visible` is our opt-in: scoped to this component so we never silently
 * override any real Bootstrap modal that might land on the same page later. */
.modal-visible {
  display: block !important;
}
</style>
