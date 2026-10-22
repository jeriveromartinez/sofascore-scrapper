<script setup lang="ts">
import { reactive, ref, useId } from "vue";
import { ApkInfo } from "../../proto/api";
import { apkApiService } from "../../store/services";
import { useModal } from "../../composables/useModal";

const props = withDefaults(defineProps<{ autoCloseModal?: boolean }>(), {
  autoCloseModal: true,
});

const emit = defineEmits<{ updated: [id: number, url: string] }>();

const modal = reactive({
  open: false,
  error: "",
  loading: false,
  info: {} as ApkInfo,
  originalId: 0,
  originalUrl: "",
});

const titleId = useId();
const dialog = ref<HTMLElement | null>(null);

const closeModal = (): void => {
  modal.open = false;
  modal.error = "";
  modal.loading = false;
  modal.info = {} as ApkInfo;
};

const submitUrl = async (): Promise<void> => {
  if (!modal.info?.panelUrl) {
    modal.error = "La URL del panel no puede estar vacía";
    return;
  }

  modal.loading = true;

  try {
    await apkApiService.updateApkUrl(modal.originalId, modal.info.panelUrl);
    emit("updated", modal.originalId, modal.info.panelUrl);
    if (props.autoCloseModal) {
      closeModal();
    }
  } catch (error) {
    modal.error =
      error instanceof Error
        ? error.message
        : "No se pudo actualizar la URL del panel";
  } finally {
    modal.loading = false;
  }
};

const openModal = (info: ApkInfo): void => {
  modal.error = "";
  modal.originalId = info.id;
  modal.originalUrl = info.panelUrl;
  modal.info = { ...info };
  modal.open = true;
};

useModal({
  isOpen: () => modal.open,
  container: dialog,
  onClose: () => {
    if (!modal.loading) closeModal();
  },
});

defineExpose({ openModal });
</script>

<template>
  <div
    v-if="modal.open"
    ref="dialog"
    class="modal fade show"
    tabindex="-1"
    style="display: block"
    aria-modal="true"
    role="dialog"
    :aria-labelledby="titleId"
  >
    <div class="modal-dialog modal-dialog-centered">
      <div class="modal-content">
        <div class="modal-header">
          <h5 :id="titleId" class="modal-title">Editar URL del Panel</h5>
          <button
            type="button"
            class="btn-close"
            aria-label="Close"
            @click="closeModal"
          ></button>
        </div>
        <div class="modal-body">
          <form
            id="edit-apk-url-form"
            class="row g-3"
            @submit.prevent="submitUrl"
          >
            <div class="col-12">
              <label class="form-label">URL</label>
              <input
                class="form-control"
                type="text"
                placeholder="https://example.com"
                v-model="modal.info.panelUrl"
              />
            </div>
            <div v-if="modal.error" class="col-12">
              <div class="alert alert-danger mb-0">{{ modal.error }}</div>
            </div>
          </form>
        </div>
        <div class="modal-footer">
          <button
            type="button"
            class="btn btn-label-secondary"
            :disabled="modal.loading"
            @click="closeModal"
          >
            Cancelar
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            form="edit-apk-url-form"
            :disabled="modal.loading"
          >
            {{ modal.loading ? "Updating..." : "Update" }}
          </button>
        </div>
      </div>
    </div>
  </div>
  <div v-if="modal.open" class="modal-backdrop fade show"></div>
</template>
