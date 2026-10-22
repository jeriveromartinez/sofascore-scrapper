import { readdirSync } from "node:fs";
import { extname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { PurgeCSS } from "purgecss";

const webRoot = fileURLToPath(new URL("..", import.meta.url));

// The vendored stylesheets that make up the layout CSS chunk. First-party CSS
// under src/assets/css is hand written and is never purged.
const purgedStylesheets = [
  "src/assets/vendor/css/core.css",
  "src/assets/vendor/fonts/iconify-icons.css",
];

const contentExtensions = new Set([".vue", ".ts"]);

// The vendor scripts that run in the browser. bootstrap.js is not imported
// anywhere, so the Bootstrap JS class names it drives are dead weight.
const runtimeScripts = [
  "src/assets/vendor/js/helpers.js",
  "src/assets/vendor/js/menu.js",
];

// Classes that never appear literally in a template but are added to the DOM
// at runtime, so scanning the sources for class names cannot find them.
const safelist = [
  // helpers.js swaps the icon of the password visibility toggle
  "bx-hide",
  "bx-show",
  // helpers.js toggles the layout state classes on <html>
  "layout-footer-fixed",
  "layout-menu-100vh",
  "layout-menu-collapsed",
  "layout-menu-expanded",
  "layout-menu-fixed",
  "layout-menu-fixed-offcanvas",
  "layout-menu-hover",
  "layout-menu-offcanvas",
  "layout-navbar-fixed",
  "layout-no-transition",
  "layout-transitioning",
  "light-style",
  // menu.js expands and collapses the navigation tree
  "menu",
  "menu-inner",
  "menu-item",
  "menu-item-animating",
  "menu-item-closing",
  "menu-link",
  "menu-no-animation",
  "menu-sub",
  "menu-toggle",
  "menu-vertical",
  "open",
  "active",
  "disabled",
  "overflow-auto",
  // helpers.js shows the sidebar and its overlay
  "show",
  // useModal locks the body while a dialog is open
  "modal-open",
];

// Attribute selectors whose value is only known at runtime.
const dynamicAttributes = ["data-bs-theme", "data-skin", "data-semidark-menu"];

function collectContentFiles(): string[] {
  const files: string[] = [];

  const walk = (dir: string) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      const full = join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (contentExtensions.has(extname(entry.name))) files.push(full);
    }
  };

  walk(join(webRoot, "src"));
  files.push(join(webRoot, "index.html"));
  for (const script of runtimeScripts) files.push(join(webRoot, script));

  return files;
}

function toRepoRelativePath(id: string): string {
  const withoutQuery = id.split("?")[0];
  return relative(webRoot, resolve(withoutQuery)).split(sep).join("/");
}

export function purgeVendoredCss() {
  let contentFiles: string[] | undefined;

  return {
    name: "purge-vendored-css",
    // Build only: the dev server keeps the full stylesheet so a class added
    // to a template shows up without restarting the server.
    apply: "build" as const,
    enforce: "pre" as const,
    async transform(code: string, id: string) {
      const file = toRepoRelativePath(id);
      if (!purgedStylesheets.includes(file)) return null;

      contentFiles ??= collectContentFiles();

      const [result] = await new PurgeCSS().purge({
        content: contentFiles,
        css: [{ raw: code }],
        safelist,
        dynamicAttributes,
        // Unused CSS variables and keyframes are kept: the theme variables
        // are consumed by inline styles and by scripts at runtime.
        variables: false,
        keyframes: false,
      });

      const before = Buffer.byteLength(code, "utf-8");
      const after = Buffer.byteLength(result.css, "utf-8");
      const saved = (((before - after) / before) * 100).toFixed(1);
      console.log(
        `purged ${file}: ${before} -> ${after} bytes (-${saved}%)`
      );

      return { code: result.css, map: null };
    },
  };
}
