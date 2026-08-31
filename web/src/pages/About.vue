<script lang="ts" setup>
import { onMounted } from "vue";
import { useBuildInfoStore } from "../store/pinia/buildInfoStore";

const buildInfo = useBuildInfoStore();
onMounted(() => {
  void buildInfo.load();
});
</script>

<template>
  <div class="container-xxl flex-grow-1 container-p-y">
    <h4 class="py-3 mb-4">About</h4>
    <div class="card">
      <div class="card-body">
        <h5 class="card-title">Build</h5>
        <p class="card-text">
          <template v-if="buildInfo.loaded">
            Version: <code>v{{ buildInfo.version }}</code><br />
            Commit: <code>{{ buildInfo.commit }}</code>
          </template>
          <template v-else>
            <span class="text-body-secondary">Loading build info…</span>
          </template>
        </p>
      </div>
    </div>
  </div>
</template>
