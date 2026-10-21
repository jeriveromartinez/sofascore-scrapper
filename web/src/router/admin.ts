import type { CustomRouteRecordRaw } from "./customRoute";

export const adminRoutes: CustomRouteRecordRaw[] = [
  {
    name: "Events",
    path: "events",
    component: () => import("../pages/events.vue"),
    icon: "bx-calendar-alt",
  },
  {
    name: "ApkAdmin",
    path: "apk-admin",
    component: () => import("../pages/apk/apkAdmin.vue"),
    icon: "bx-joystick",
  },
  {
    name: "Pushes",
    path: "pushes",
    component: () => import("../pages/pushes.vue"),
    icon: "bx-bell",
  },
  {
    name: "About",
    path: "about",
    component: () => import("../pages/About.vue"),
    icon: "bx-info-circle",
  },
];

export default adminRoutes;
