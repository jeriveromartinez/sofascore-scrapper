import { defineStore } from "pinia";
import { KEY_THEME } from "../../constants";

export const useStyleStore = defineStore("style", {
  state: () => ({
    theme: localStorage.getItem(KEY_THEME) ?? "dark",
  }),
  actions: {
    setTheme(theme: string) {
      this.theme = theme;
      localStorage.setItem(KEY_THEME, theme);
    },
  },
});
