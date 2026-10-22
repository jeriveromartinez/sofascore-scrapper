import { readFileSync, readdirSync } from "node:fs";
import { resolve } from "node:path";
import { gzipSync } from "node:zlib";

const root = resolve(import.meta.dirname, "..");
const distDir = resolve(root, "dist");
const assetsDir = resolve(distDir, "assets");
const indexPath = resolve(distDir, "index.html");

const maxAuthCSSGzip = 50 * 1024;
const maxLayoutCSSGzip = 40 * 1024;

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

const cssAssets = readdirSync(assetsDir)
  .filter((f) => f.endsWith(".css"))
  .sort();

const sizes = cssAssets.map((file) => {
  const content = readFileSync(resolve(assetsDir, file), "utf-8");
  return {
    file,
    raw: Buffer.byteLength(content, "utf-8"),
    gzip: gzipSync(content).length,
  };
});

// The layout chunk is emitted as `layout-<hash>.css`. Fall back to the
// largest CSS chunk so the gate keeps working if the chunk is renamed.
const layoutChunk =
  sizes.find((s) => s.file.startsWith("layout-")) ??
  sizes.reduce((largest, s) => (s.gzip > largest.gzip ? s : largest), sizes[0]);

console.log("CSS chunks:");
for (const chunk of sizes) {
  console.log(
    `  ${chunk.file.padEnd(28)} raw ${(chunk.raw / 1024).toFixed(2)} KiB   gzip ${(chunk.gzip / 1024).toFixed(2)} KiB`
  );
}

console.log(`\nentry CSS files: ${cssLinks.join(", ") || "(all css)"}`);
console.log(`auth CSS raw:    ${(rawBytes / 1024).toFixed(2)} KiB`);
console.log(`auth CSS gzip:   ${(gzipBytes / 1024).toFixed(2)} KiB`);
console.log(`auth CSS budget: ${(maxAuthCSSGzip / 1024).toFixed(2)} KiB`);

console.log(`\nlayout CSS chunk: ${layoutChunk.file}`);
console.log(`layout CSS raw:  ${(layoutChunk.raw / 1024).toFixed(2)} KiB`);
console.log(`layout CSS gzip: ${(layoutChunk.gzip / 1024).toFixed(2)} KiB`);
console.log(`layout CSS budget: ${(maxLayoutCSSGzip / 1024).toFixed(2)} KiB`);

let failed = false;

if (gzipBytes > maxAuthCSSGzip) {
  console.error(
    `\nERROR: auth CSS gzip ${gzipBytes} bytes exceeds ${maxAuthCSSGzip} bytes budget`
  );
  failed = true;
}

if (layoutChunk.gzip > maxLayoutCSSGzip) {
  console.error(
    `\nERROR: layout CSS gzip ${layoutChunk.gzip} bytes exceeds ${maxLayoutCSSGzip} bytes budget`
  );
  failed = true;
}

if (failed) process.exit(1);

console.log("\nBudget check passed!");
process.exit(0);
