<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useStyleStore } from "../store/pinia/styleStore";

const styleStore = useStyleStore();
const showingDropdown = ref(false);

const changeTheme = (theme?: string) => {
  showingDropdown.value = false;
  if (theme) styleStore.setTheme(theme);
  document.documentElement.setAttribute("data-bs-theme", styleStore.theme);
};

onMounted(() => changeTheme());
</script>

<template>
  <li class="nav-item dropdown me-2">
    <a
      class="nav-link dropdown-toggle hide-arrow"
      :class="{ show: showingDropdown }"
      id="nav-theme"
      href="javascript:void(0)"
      @click="showingDropdown = !showingDropdown"
      data-bs-toggle="dropdown"
      aria-label="Toggle theme (dark)"
      aria-expanded="false"
    >
      <i class="bx-moon icon-base bx icon-md theme-icon-active"></i>
      <span class="d-none ms-2" id="nav-theme-text">Toggle theme</span>
    </a>
    <ul
      class="dropdown-menu dropdown-menu-end"
      :class="{ show: showingDropdown }"
      aria-labelledby="nav-theme-text"
    >
      <li>
        <button
          type="button"
          class="dropdown-item align-items-center"
          :class="{ active: styleStore.theme === 'light' }"
          data-bs-theme-value="light"
          aria-pressed="false"
          @click="changeTheme('light')"
        >
          <span
            ><i class="icon-base bx bx-sun icon-md me-3" data-icon="sun"></i
            >Light</span
          >
        </button>
      </li>
      <li>
        <button
          type="button"
          class="dropdown-item align-items-center"
          :class="{ active: styleStore.theme === 'dark' }"
          data-bs-theme-value="dark"
          aria-pressed="true"
          @click="changeTheme('dark')"
        >
          <span
            ><i class="icon-base bx bx-moon icon-md me-3" data-icon="moon"></i
            >Dark</span
          >
        </button>
      </li>
    </ul>
  </li>
</template>
