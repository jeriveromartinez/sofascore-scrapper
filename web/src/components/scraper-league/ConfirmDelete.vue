<template>
  <div class="modal-backdrop" @click.self="$emit('cancel')">
    <div
      ref="dialog"
      class="modal modal-visible"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="titleId"
      tabindex="-1"
    >
      <h2 :id="titleId">Delete league</h2>
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
import { ref, useId } from 'vue'
import type { ScraperLeague } from '../../store/pinia/scraperLeaguesStore'
import { useModal } from '../../composables/useModal'

defineProps<{ item: ScraperLeague }>()
const emit = defineEmits<{ (e: 'confirm'): void; (e: 'cancel'): void }>()

const titleId = useId()
const dialog = ref<HTMLElement | null>(null)
useModal({ isOpen: () => true, container: dialog, onClose: () => emit('cancel') })
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
