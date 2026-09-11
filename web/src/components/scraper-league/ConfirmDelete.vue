<template>
  <div class="modal-backdrop" @click.self="$emit('cancel')">
    <div class="modal">
      <h2>Delete league</h2>
      <p>Are you sure you want to delete '{{ item.name }}'?</p>
      <div class="actions">
        <button @click="$emit('cancel')">Cancel</button>
        <button class="danger" @click="$emit('confirm')">Delete</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// NOTE on @/ alias: the brief's snippet uses `@/store/...` imports. The alias
// only exists in vite.config.ts (runtime); there is no matching `paths` mapping
// in tsconfig.app.json, and `eslint.config.js` does not register the alias with
// import/resolver. The rest of the project uses relative imports (e.g.
// pages/admin/scraper-leagues/CreateOrEdit.vue:39,
// pages/admin/scraper-leagues/index.vue). Switched to a relative import — the
// file passes `tsc --noEmit` and `yarn lint` cleanly this way.
import type { ScraperLeague } from '../../../store/pinia/scraperLeaguesStore'

// NOTE on i18n: the brief asks for vue-i18n translations
// ($t('scraperLeagues.confirmDeleteTitle'),
// $t('scraperLeagues.confirmDeleteMsg', { name: item.name }),
// $t('common.cancel'), $t('common.delete')). vue-i18n is not yet a dependency of
// this project (it is not in package.json or package-lock.json), and Task 6 is
// the planned landing point for both the dependency and the locale files.
// Until that lands this modal uses plain English string literals (matching the
// convention used in the sibling CreateOrEdit.vue modal and in every other page
// in this project). When the i18n layer is added in Task 6 these literals will
// be replaced with the $t('scraperLeagues.*') and $t('common.*') calls shown in
// the original brief snippet.
defineProps<{ item: ScraperLeague }>()
defineEmits<{ (e: 'confirm'): void; (e: 'cancel'): void }>()
</script>
