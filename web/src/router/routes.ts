import type { RouteRecordRaw } from "vue-router";

export const routes: RouteRecordRaw[] = [
  {
    name: "Login",
    path: "/login",
    component: () => import("../pages/login.vue"),
  },
  {
    name: "Register",
    path: "/register",
    component: () => import("../pages/register.vue"),
  },
  {
    name: "Dashboard",
    path: "/",
    component: () => import("../components/layout.vue"),
  },
];
