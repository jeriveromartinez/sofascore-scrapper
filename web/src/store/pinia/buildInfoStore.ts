import { defineStore } from "pinia";
import { ref } from "vue";
import { BuildInfoService } from "../services/BuildInfoService";

export const useBuildInfoStore = defineStore("buildInfo", () => {
  const version = ref("");
  const commit = ref("");
  const loaded = ref(false);

  const service = new BuildInfoService();

  async function load(): Promise<void> {
    if (loaded.value) return;
    try {
      const info = await service.getBuildInfo();
      version.value = info.version;
      commit.value = info.commit;
      loaded.value = true;
    } catch {
      // Silent: the version chip is decorative; missing is acceptable.
    }
  }

  return { version, commit, loaded, load };
});
