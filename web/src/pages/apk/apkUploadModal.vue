<script setup lang="ts">
import { onBeforeUnmount, reactive, watch } from "vue";
import { apkApiService } from "../../store/services";

const props = withDefaults(defineProps<{ autoCloseModal?: boolean }>(), {
  autoCloseModal: true,
});

const emit = defineEmits<{
  uploaded: [version: string];
}>();

const upload = reactive({
  file: null as File | null,
  version: "",
  description: "",
  loading: false,
  progress: 0,
  error: "",
});

const modal = reactive({
  open: false,
});

function resetUploadForm(): void {
  upload.file = null;
  upload.version = "";
  upload.description = "";
  upload.error = "";
  upload.progress = 0;
}

function openUploadModal(): void {
  upload.error = "";
  modal.open = true;
}

function closeUploadModal(): void {
  modal.open = false;
  resetUploadForm();
}

function autoCloseModal(): void {
  if (props.autoCloseModal) closeUploadModal();
}

function onFileChange(event: Event): void {
  const target = event.target as HTMLInputElement;
  upload.file = target.files?.[0] ?? null;
}

async function submitUpload(): Promise<void> {
  if (!upload.file) {
    upload.error = "Selecciona un archivo APK";
    return;
  }

  upload.loading = true;
  upload.progress = 0;
  upload.error = "";

  try {
    const response = await apkApiService.uploadApk(
      upload.file,
      upload.version || undefined,
      upload.description || undefined,
      (percent) => {
        upload.progress = percent;
      },
    );
    closeUploadModal();
    emit("uploaded", response.version);
  } catch (error) {
    upload.error = error instanceof Error ? error.message : "No se pudo subir el APK";
  } finally {
    upload.loading = false;
  }
}

watch(
  () => modal.open,
  (isOpen) => {
    document.body.classList.toggle("modal-open", isOpen);
    document.body.style.overflow = isOpen ? "hidden" : "";
  },
);

onBeforeUnmount(() => {
  document.body.classList.remove("modal-open");
  document.body.style.overflow = "";
});
</script>

<template>
  <button class="btn btn-outline-primary btn-sm" @click="openUploadModal">
    Cargar APK
  </button>

  <div
    v-if="modal.open"
    class="modal fade show"
    tabindex="-1"
    style="display: block"
    aria-modal="true"
    role="dialog"
    @click.self="autoCloseModal"
  >
    <div class="modal-dialog modal-dialog-centered">
      <div class="modal-content">
        <div class="modal-header">
          <h5 class="modal-title">Subir APK</h5>
          <button
            type="button"
            class="btn-close"
            aria-label="Close"
            @click="closeUploadModal"
          ></button>
        </div>
        <div class="modal-body">
          <form
            id="upload-apk-form"
            class="row g-3"
            @submit.prevent="submitUpload"
          >
            <div class="col-12">
              <label class="form-label">Archivo APK *</label>
              <input
                class="form-control"
                type="file"
                accept=".apk"
                @change="onFileChange"
                required
              />
            </div>
            <div class="col-12">
              <label class="form-label">Version (x.y.z)</label>
              <input
                v-model="upload.version"
                class="form-control"
                type="text"
                placeholder="1.2.3"
              />
            </div>
            <div class="col-12">
              <label class="form-label">Descripcion</label>
              <textarea
                v-model="upload.description"
                class="form-control"
                rows="3"
              ></textarea>
            </div>
            <div v-if="upload.loading" class="col-12">
              <div class="progress">
                <div
                  class="progress-bar progress-bar-striped progress-bar-animated"
                  role="progressbar"
                  :style="{ width: upload.progress + '%' }"
                  :aria-valuenow="upload.progress"
                  aria-valuemin="0"
                  aria-valuemax="100"
                ></div>
              </div>
            </div>
            <div v-if="upload.error" class="col-12">
              <div class="alert alert-danger mb-0">{{ upload.error }}</div>
            </div>
          </form>
        </div>
        <div class="modal-footer">
          <button
            type="button"
            class="btn btn-label-secondary"
            :disabled="upload.loading"
            @click="closeUploadModal"
          >
            Cancelar
          </button>
          <button
            type="submit"
            class="btn btn-primary"
            form="upload-apk-form"
            :disabled="upload.loading"
          >
            {{ upload.loading ? `Subiendo... ${upload.progress}%` : "Subir" }}
          </button>
        </div>
      </div>
    </div>
  </div>
  <div v-if="modal.open" class="modal-backdrop fade show"></div>
</template>
