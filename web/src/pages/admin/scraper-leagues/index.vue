<template>
  <div class="scraper-leagues-page">
    <h1>Scraper Leagues</h1>

    <div class="filters">
      <input v-model="searchQuery" placeholder="Search leagues" @input="onSearch" />
      <button @click="openCreateModal">Add new</button>
    </div>

    <table v-if="!store.loading && store.items.length > 0">
      <thead>
        <tr>
          <th>Name</th>
          <th>Source</th>
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
          <td>{{ item.source_league_id }}</td>
          <td>{{ item.country }}</td>
          <td>{{ item.sport }}</td>
          <td>
            <label class="status-toggle-placeholder">
              <input type="checkbox" :checked="item.enabled" @change="onToggle(item, ($event.target as HTMLInputElement).checked)" />
              <span>{{ item.enabled ? 'Yes' : 'No' }}</span>
            </label>
          </td>
          <td>
            <button @click="openEditModal(item)">Edit</button>
            <button @click="confirmDelete(item)">Delete</button>
          </td>
        </tr>
      </tbody>
    </table>

    <p v-if="!store.loading && store.items.length === 0">No leagues configured yet.</p>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useScraperLeaguesStore, type ScraperLeague } from '../../../store/pinia/scraperLeaguesStore'

// NOTE on i18n: the brief asks for vue-i18n translations ($t('scraperLeagues.title') etc.).
// vue-i18n is not yet a dependency of this project (it is not in package.json or
// package-lock.json), and Task 6 is the planned landing point for both the
// dependency and the locale files. Until that lands this page uses plain English
// string literals (matching the convention in every other page in this project,
// e.g. pages/domains.vue). When the i18n layer is added in Task 6 the literal
// here will be replaced with $t('scraperLeagues.title') and friends.
//
// Tasks 4 and 5 will introduce StatusToggle, ConfirmDelete, and CreateOrEdit
// components under @/components/scraper-league/. Until those land, this page
// uses inline placeholders (plain checkboxes/buttons) so the table renders and
// the basic actions (toggle, edit, delete) still hit the store. When the
// components land they will replace the inline markup; the event handlers
// below are the same shape the components will emit.
const store = useScraperLeaguesStore()
const searchQuery = ref('')

onMounted(() => store.fetch())

function onSearch() {
  store.fetch({ q: searchQuery.value, page: 1 })
}

function openCreateModal() {
  // Wired up in Task 4 (CreateOrEdit component).
}

function openEditModal(_item: ScraperLeague) {
  // Wired up in Task 4 (CreateOrEdit component).
}

async function onToggle(item: ScraperLeague, enabled: boolean) {
  await store.update(item.id, { enabled })
}

function confirmDelete(_item: ScraperLeague) {
  // Wired up in Task 5 (ConfirmDelete component).
}
</script>
