<template>
  <div class="scraper-leagues-page">
    <h1>Scraper Leagues</h1>

    <div class="banner banner-info" data-testid="scores365-banner" role="status">
      <strong>365scores</strong> is the default source for all newly discovered leagues.
      To keep scraping a league via FotMob instead, set
      <code>override_source=fotmob</code> in the edit form below.
    </div>

    <div class="filters">
      <input v-model="searchQuery" placeholder="Search leagues" @input="onSearch" />
      <button class="add-new" @click="openCreateModal">Add new</button>
    </div>

    <table v-if="!store.loading && store.items.length > 0">
      <thead>
        <tr>
          <th>Name</th>
          <th>Source</th>
          <th>Source override</th>
          <th>Source League ID</th>
          <th>Country</th>
          <th>Sport</th>
          <th>Enabled</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="item in store.items" :key="item.id">
          <td>{{ item.name }}</td>
          <td>{{ item.source }}</td>
          <td>{{ item.override_source ?? '—' }}</td>
          <td>{{ item.source_league_id }}</td>
          <td>{{ item.country }}</td>
          <td>{{ item.sport }}</td>
          <td>
            <StatusToggle
              :modelValue="item.enabled"
              @update:modelValue="(v: boolean) => onToggle(item, v)"
            />
          </td>
          <td>
            <button class="edit" @click="openEditModal(item)">Edit</button>
            <button class="delete" @click="confirmDelete(item)">Delete</button>
          </td>
        </tr>
      </tbody>
    </table>

    <p v-if="!store.loading && store.items.length === 0">No leagues configured yet.</p>

    <CreateOrEdit
      v-if="modalOpen"
      :item="editTarget"
      @close="closeModal"
      @saved="closeModal"
    />

    <ConfirmDelete
      v-if="deleteTarget"
      :item="deleteTarget"
      @cancel="deleteTarget = null"
      @confirm="onConfirmDelete"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useScraperLeaguesStore, type ScraperLeague } from '../../../store/pinia/scraperLeaguesStore'
import StatusToggle from '../../../components/scraper-league/StatusToggle.vue'
import ConfirmDelete from '../../../components/scraper-league/ConfirmDelete.vue'
import CreateOrEdit from './CreateOrEdit.vue'

const store = useScraperLeaguesStore()
const searchQuery = ref('')
const modalOpen = ref(false)
const editTarget = ref<ScraperLeague | null>(null)
const deleteTarget = ref<ScraperLeague | null>(null)

onMounted(() => store.fetch())

function onSearch() {
  store.fetch({ q: searchQuery.value, page: 1 })
}

function openCreateModal() {
  editTarget.value = null
  modalOpen.value = true
}

function openEditModal(item: ScraperLeague) {
  editTarget.value = item
  modalOpen.value = true
}

function closeModal() {
  modalOpen.value = false
  editTarget.value = null
}

async function onToggle(item: ScraperLeague, enabled: boolean) {
  await store.update(item.id, { enabled })
}

function confirmDelete(item: ScraperLeague) {
  deleteTarget.value = item
}

async function onConfirmDelete() {
  const target = deleteTarget.value
  deleteTarget.value = null
  if (target) {
    await store.remove(target.id)
  }
}
</script>

<style scoped>
.banner {
  padding: 12px 16px;
  border-radius: 6px;
  margin-bottom: 16px;
  border-left: 4px solid;
}
.banner-info {
  background: #e7f3ff;
  border-color: #1976d2;
  color: #0d47a1;
}
</style>
