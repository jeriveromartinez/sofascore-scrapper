import { createRouter, createWebHashHistory } from "vue-router";
import { routes } from "./routes";
import { readAuthStorage } from "../store/authStorage";

const router = createRouter({
  history: createWebHashHistory(import.meta.env.BASE_URL),
  routes,
});

router.beforeEach((to, _) => {
  const { user } = readAuthStorage();

  if (!user?.token && to.name !== "Login" && to.name !== "Register")
    return { name: "Login" };

  return true;
});

export { router };
