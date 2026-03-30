# Copilot Instructions

## Project overview
This repository contains:
- A Go backend
- A Vue.js frontend
- Shared Protocol Buffers contracts for communication between frontend and backend

The protobuf definitions are the source of truth for cross-service and client-server contracts.

## General behavior
- Prefer the smallest possible change.
- Preserve existing behavior unless explicitly asked to change it.
- Do not refactor unrelated code.
- Do not scan the whole repository unless explicitly requested.
- For simple requests, work only on the selected code or current file.
- If a request would require touching multiple layers (proto, Go, Vue), explain that briefly and keep changes tightly scoped.

## Scope control
- Default to single-file changes.
- Only modify multiple files when the request clearly requires it.
- Do not introduce architecture changes for local refactors.
- Do not rename exported/public APIs unless explicitly requested.
- Do not create new abstractions unless duplication is clear and repeated.
- Do not add dependencies unless explicitly requested.

## Protobuf rules
- Treat `.proto` files as contract-defining files.
- Do not modify protobuf message names, field numbers, package names, or service names unless explicitly requested.
- Never reuse or reorder protobuf field numbers.
- When adding fields, preserve backward compatibility.
- Prefer additive protobuf changes over breaking changes.
- If a change would break compatibility between Go and Vue, warn briefly before editing.
- Keep generated code untouched unless the task explicitly involves regeneration workflow.
- Do not manually edit generated protobuf output files unless explicitly requested.

## Go backend rules
- Follow idiomatic Go.
- Prefer explicit, readable code over clever abstractions.
- Keep functions small, but do not split code unnecessarily.
- Preserve public interfaces unless explicitly asked to change them.
- Reuse existing types, helpers, and patterns before creating new ones.
- Do not introduce reflection, generics, or concurrency changes unless required.
- Respect existing error handling style.
- For refactors, prefer minimal diffs over broad cleanup.
- Do not change transport or serialization behavior unless explicitly requested.
- Do not change api urls/endpoints unless explicitly requested.

## Vue frontend rules
- Follow the existing Vue style already present in the repository.
- Prefer local, minimal component changes.
- Do not reorganize component structure unless requested.
- Preserve props, emits, and existing component contracts unless explicitly requested.
- Do not introduce new state management patterns for small fixes.
- Keep templates and script logic readable and simple.
- Do not change API payload assumptions unless the protobuf contract changed.

## Cross-layer contract rules
- Backend and frontend must remain aligned with protobuf contracts.
- If modifying DTO mapping or serialization logic, keep names and semantics consistent with the `.proto` definitions.
- Prefer changing mapping code over changing protobuf contracts when solving local issues.
- When a request affects both Go and Vue, make only the required contract-aligned changes.
- Do not invent fields that are not present in protobuf contracts.
- Do not silently change enum meanings, optionality, or repeated-field semantics.

## Refactoring policy
For simple refactors:
- change only the selected function, block, or file
- preserve behavior exactly
- prefer minimal diff
- avoid touching tests, docs, configs, generated files, or unrelated modules

Allowed simple refactors:
- remove duplication
- simplify conditionals
- extract small private helpers
- improve naming of local variables
- convert nested conditionals into early returns
- simplify mapping code without changing behavior

Avoid:
- broad rewrites
- style-only edits across many lines
- moving code across layers without need
- changing protobuf contracts for local cleanup
- updating backend and frontend together unless the task requires it

## Output style
When responding in chat:
- First state the plan in 3 bullets or fewer.
- Then make the smallest valid change.
- If more than one file is needed, say why in one sentence.
- Do not produce long explanations unless asked.

## Testing and validation
- Suggest only the most relevant validation steps.
- Do not add or rewrite tests unless requested.
- If a change impacts protobuf contracts, mention regeneration or sync steps only if necessary.
- For local refactors, avoid proposing project-wide validation.

## What to avoid by default
- No workspace-wide scans
- No multi-file cleanup
- No speculative improvements
- No comments added unless requested
- No dependency changes
- No generated file edits
- No breaking protobuf changes

## Preferred prompt interpretation
Interpret vague requests narrowly.

Examples:
- "refactor this" => refactor only the selected code with minimal diff
- "clean this up" => improve readability without changing behavior
- "simplify this mapper" => keep the protobuf contract exactly the same
- "fix frontend for this proto change" => update only affected Vue mapping/rendering code

## If the request is ambiguous
Assume the user wants:
- the smallest safe change
- no contract breakage
- no repository-wide refactor
- no extra files unless required
