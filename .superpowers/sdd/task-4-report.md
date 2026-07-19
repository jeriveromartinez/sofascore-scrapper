# Task 4 Report: Reduce Authentication CSS Payload

## Summary

Reduced initial CSS payload for login/register pages from ~2 MB (405.5 kB gzip) to ~11 kB (2.15 kB gzip) by extracting a minimal `auth-critical.css` and removing the heavy `core.css` and `iconify-icons.css` imports.

## Changes Made

### Created: `web/src/assets/css/auth-critical.css`
- CSS custom properties (both light and dark themes)
- Base reset, typography, body styles
- Container, card, form (controls, checks, input groups), button styles
- Utility classes needed by auth templates
- Auth layout (authentication-wrapper decorative elements)
- App brand / demo styles
- Password visibility inline SVG styling class

### Modified: `web/src/pages/login.vue`
- Removed `core.css`, `iconify-icons.css`, `perfect-scrollbar.css` imports
- Added `auth-critical.css` import
- Replaced `<i class="icon-base bx">` password toggle with inline SVG (v-if/v-else)

### Modified: `web/src/pages/register.vue`
- Same changes as login.vue

### Created: `web/scripts/check-bundle-budget.mjs`
- Parses `dist/index.html` to find entry CSS files
- Gzips combined CSS and validates < 50 KiB budget
- Exits non-zero on budget violation

### Modified: `web/package.json`
- Added `"bundle-budget": "node scripts/check-bundle-budget.mjs"` script

## Build Results

```
entry CSS files: /assets/index-rzNTtEL3.css
auth CSS raw:    11.17 KiB
auth CSS gzip:   2.15 KiB
budget:          50.00 KiB
Budget check passed!
```

- Layout CSS (core.css + iconify-icons.css) remains at 1,999.59 kB (405.52 kB gzip) but is code-split — only loaded for authenticated pages via `layout.vue`
- `npm run bundle-budget` passes

## Verification

- `npx vite build` — succeeds
- `node scripts/check-bundle-budget.mjs` — passes (2.15 KiB < 50 KiB)
- Remaining CSS imports per auth page: `auth-critical.css`, `demo.css`, `page-auth.css`, `inputs.css`
