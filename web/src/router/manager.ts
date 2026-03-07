import type { CustomRouteRecordRaw } from "./customRoute";

export const managerRoutes: CustomRouteRecordRaw[] = [
  {
    name: "Devices",
    path: "devices",
    component: () => import("../pages/devices.vue"),
    icon: "bx-tv",
  },
  {
    name: "Playback",
    path: "playback",
    component: () => import("../pages/playback.vue"),
    icon: "bx-poll",
  },
  {
    name: "Stats",
    path: "stats",
    component: () => import("../pages/stats.vue"),
    icon: "bx-pie-chart",
  },
];
