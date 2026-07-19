import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { gzipSync } from "node:zlib";

const root = resolve(import.meta.dirname, "..");
const distDir = resolve(root, "dist");
const assetsDir = resolve(distDir, "assets");
const indexPath = resolve(distDir, "index.html");

let html;
try {
  html = readFileSync(indexPath, "utf-8");
} catch {
  console.error("ERROR: dist/index.html not found. Run build first.");
  process.exit(1);
}

const cssLinks = [...html.matchAll(/href="([^"]+\.css)"/g)].map((m) => m[1]);

let combined = "";

if (cssLinks.length > 0) {
  for (const href of cssLinks) {
    const filename = href.replace(/^\/assets\//, "");
    const cssPath = resolve(assetsDir, filename);
    try {
      combined += readFileSync(cssPath, "utf-8");
    } catch {
      console.error(`WARNING: CSS file not found: ${cssPath}`);
    }
  }
} else {
  const files = readdirSync(assetsDir).filter((f) => f.endsWith(".css"));
  for (const file of files) {
    combined += readFileSync(resolve(assetsDir, file), "utf-8");
  }
}

const rawBytes = Buffer.byteLength(combined, "utf-8");
const gzipBytes = gzipSync(combined).length;
const maxAuthCSSGzip = 50 * 1024;

console.log(`entry CSS files: ${cssLinks.join(", ") || "(all css)"}`);
console.log(`auth CSS raw:    ${(rawBytes / 1024).toFixed(2)} KiB`);
console.log(`auth CSS gzip:   ${(gzipBytes / 1024).toFixed(2)} KiB`);
console.log(`budget:          ${(maxAuthCSSGzip / 1024).toFixed(2)} KiB`);

if (gzipBytes > maxAuthCSSGzip) {
  console.error(
    `\nERROR: auth CSS gzip ${gzipBytes} bytes exceeds ${maxAuthCSSGzip} bytes budget`
  );
  process.exit(1);
}

console.log("\nBudget check passed!");
process.exit(0);
