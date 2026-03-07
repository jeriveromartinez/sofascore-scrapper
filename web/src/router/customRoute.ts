import type { RouteRecordRaw } from "vue-router";

export type CustomRouteRecordRaw = RouteRecordRaw & {
  icon?: string;
};
