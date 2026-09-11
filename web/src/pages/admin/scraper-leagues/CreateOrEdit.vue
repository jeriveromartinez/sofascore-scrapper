<template>
  <div class="modal-backdrop" @click.self="$emit('close')">
    <div class="modal">
      <h2>{{ item ? 'Edit league' : 'Add new league' }}</h2>

      <label>Search</label>
      <input v-model="search" class="search" placeholder="Search leagues..." />

      <ul v-if="suggestions.length > 0" class="suggestions">
        <li v-for="s in suggestions" :key="s.source_league_id" class="suggestion" @click="select(s)">
          {{ s.name }} ({{ s.country }}) - {{ s.source_league_id }}
        </li>
      </ul>

      <label>Name</label>
      <input v-model="form.name" />

      <label>Country</label>
      <input v-model="form.country" maxlength="8" />

      <label>Sport</label>
      <input v-model="form.sport" />

      <label>
        <input type="checkbox" v-model="form.enabled" />
        Enabled
      </label>

      <div class="actions">
        <button @click="$emit('close')">Cancel</button>
        <button class="submit" @click="onSave">Save</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, onMounted } from 'vue'
import { useScraperLeaguesStore, type ScraperLeague } from '../../../store/pinia/scraperLeaguesStore'

// NOTE on @/ alias: the brief's snippet uses `@/store/...` imports. The alias
// only exists in vite.config.ts (runtime); there is no matching `paths` mapping
// in tsconfig.app.json, and `eslint.config.js` does not register the alias with
// import/resolver. The rest of the project uses relative imports (e.g.
// pages/scraper-leagues/index.vue:49). Switched to relative imports — the
// file passes `tsc --noEmit` and `yarn lint` cleanly this way.
const props = defineProps<{ item: ScraperLeague | null }>()
const emit = defineEmits<{ (e: 'close'): void; (e: 'saved'): void }>()

const store = useScraperLeaguesStore()
const search = ref('')
const suggestions = ref<Array<{ source_league_id: string; name: string; country: string; sport: string }>>([])

const form = reactive({
  source: 'fotmob',
  source_league_id: '',
  name: '',
  country: '',
  sport: 'football',
  enabled: true,
})

let searchTimer: number | null = null

onMounted(() => {
  if (props.item) {
    Object.assign(form, props.item)
  }
})

watch(search, (q) => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = window.setTimeout(async () => {
    if (q.length < 2) {
      suggestions.value = []
      return
    }
    suggestions.value = await store.searchLeagues(q)
  }, 250)
})

function select(s: { source_league_id: string; name: string; country: string; sport: string }) {
  form.source_league_id = s.source_league_id
  form.name = s.name
  form.country = s.country
  form.sport = s.sport
  suggestions.value = []
}

async function onSave() {
  if (props.item) {
    await store.update(props.item.id, {
      name: form.name,
      country: form.country,
      sport: form.sport,
      enabled: form.enabled,
    })
  } else {
    await store.create({ ...form })
  }
  emit('saved')
}
</script>
