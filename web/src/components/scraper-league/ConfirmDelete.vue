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
// pages/admin/scraper-leagues/CreateOrEdit.vue,
// pages/admin/scraper-leagues/index.vue). Switched to a relative import — the
// file passes `tsc --noEmit` and `yarn lint` cleanly this way.
import type { ScraperLeague } from '../../store/pinia/scraperLeaguesStore'

defineProps<{ item: ScraperLeague }>()
defineEmits<{ (e: 'confirm'): void; (e: 'cancel'): void }>()
</script>
