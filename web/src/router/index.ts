import { createRouter, createWebHashHistory } from "vue-router";
import { routes } from "./routes";
import { KEY_USER_LOGIN } from "../constants";

const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes,
});

router.beforeEach((to, _) => {
  const userLogin =
    sessionStorage.getItem(KEY_USER_LOGIN) ??
    localStorage.getItem(KEY_USER_LOGIN) ??
    "{}";
  let data: { token?: string } = {};

  try {
    data = JSON.parse(userLogin) as { token?: string };
  } catch {
    data = {};
  }

  if (!data?.token && to.name !== "Login" && to.name !== "Register")
    return { name: "Login" };

  return true;
});

export { router };
