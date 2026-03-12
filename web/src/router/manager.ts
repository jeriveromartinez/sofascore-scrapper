import type { CustomRouteRecordRaw } from "./customRoute";

export const managerRoutes: CustomRouteRecordRaw[] = [
  {
    name: "Devices",
    path: "devices",
    component: () => import("../pages/devices.vue"),
    icon: "bx-tv",
  },
  {
    name: "Tournaments",
    path: "tournaments",
    component: () => import("../pages/tournaments.vue"),
    icon: "bx-trophy",
  },
  {
    name: "Device Tournaments",
    path: "device-tournaments",
    component: () => import("../pages/device-tournaments.vue"),
    icon: "bx-link",
  },
  {
    name: "Global Config",
    path: "global-config",
    component: () => import("../pages/global-config.vue"),
    icon: "bx-cog",
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
