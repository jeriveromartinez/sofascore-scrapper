import type { RouteRecordRaw } from "vue-router";
import { adminRoutes } from "./admin";
import { managerRoutes } from "./manager";

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
    redirect: { name: "Events" },
    children: [...adminRoutes, ...managerRoutes],
  },
];
