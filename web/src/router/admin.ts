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
    component: () => import("../pages/apkAdmin.vue"),
    icon: "bx-joystick",
  },
];

export default adminRoutes;
