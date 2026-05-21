---
name: generate-implementation-plan
description: Incrementally generates or updates IMPLEMENTATION_PLAN.md from design specs in .ai/spec/. Produces a strict MADR/MSDD-style plan with TDD enforcement, atomized micro-tasks, and agent orchestration guardrails for Go projects. Tracks spec changes via SHA256 manifest.
---

<!-- =======================================================================================
SKILL SUMMARY (TL;DR)
- Core Function: Incremental parser translating `.ai/spec/` into `IMPLEMENTATION_PLAN.md`.
- Target Ecosystem: Go (Golang) — enforces table-driven tests, `go test` layout, and pkg boundaries.
- State Management: Uses a SHA256 manifest to track spec deltas, avoiding full plan rewrites.
- Context Optimization: Slices phases into micro-tasks fitting a single LLM context window.
- Rigid TDD Enforcement: Mandatory Red-Execution-Green flow; accepts Go compilation errors as RED.
  Auto-advances on PASS, halts on FAIL.
- Human-in-the-Loop: Strict interactive planning gate blocking code generation until manual approval.
======================================================================================== -->

# Generate Implementation Plan — Go Project Skill

You are a **strategic technical coordinator** bridging human intent and autonomous agent execution. Your role is to read software specifications from `.ai/spec/` and produce (or incrementally update) a rigidly structured `IMPLEMENTATION_PLAN.md` that serves as the single source of truth for implementation work.

The generated plan must be machine-parseable by executing agents, human-reviewable by engineers, and resilient to incremental spec evolution.

---

## Phase A: Discover and Diff Spec Files (Incremental Processing)

This skill operates **incrementally**. You MUST follow this diffing protocol on every invocation.

### A.1 — Inventory

1. List all `.md` files in `.ai/spec/` (recursively).
2. Compute the SHA256 hash of each spec file: `sha256sum .ai/spec/*.md`.
3. Read the existing `IMPLEMENTATION_PLAN.md` if it exists. Locate the `## Spec Manifest` section at the bottom — it contains a table of previously processed spec files, their hashes, and statuses.

### A.2 — Classify changes

- **New files**: present on disk but absent from the manifest.
- **Changed files**: present in both but hash differs.
- **Unchanged files**: hash matches — **skip these entirely**, do not re-read them.
- **Removed files**: listed in the manifest but no longer on disk.

### A.3 — Short-circuit

If there are zero new, changed, or removed files, report:
> "Implementation plan is up to date — no spec changes detected."

And **stop**. Do not rewrite the plan.

---

## Phase B: Read Only the Delta

- Read the **full content** of each new and changed spec file.
- Do NOT re-read unchanged spec files.
- If specs are ambiguous, lack detail on testing frameworks (standard `testing` package, `testify`, mock generators), or contain contradictory requirements, **halt and ask the human user for clarification** before generating the plan.

---

## Phase C: Generate or Update the Plan

### First run (no existing IMPLEMENTATION_PLAN.md)

Generate the full plan from all spec files following the strict structure defined below.

### Incremental update (IMPLEMENTATION_PLAN.md exists)

1. Read the existing plan in full.
2. **Changed specs**: identify phases annotated with `source:` pointing to the changed file. Update those phases to reflect the new spec content — add, modify, or remove steps. Preserve phase numbering stability where possible.
3. **New specs**: generate new phases and insert them in dependency order. Renumber subsequent phases if necessary.
4. **Removed specs**: do **NOT** delete the corresponding phases. Instead:
   - Set `status: "Deprecated"` on each affected phase.
   - Add a deprecation notice: `> DEPRECATED: The source spec (.ai/spec/<filename>.md) has been removed. Review whether this phase is still needed or should be manually removed.`
   - Keep all phase content intact for traceability.
   - Update the manifest entry to show `REMOVED` status.
5. Re-validate all phase dependencies and ordering after merging.
6. Update the Overview & Impact Dashboard to reflect current state.
7. When updating existing phases, preserve any user-added notes or modifications (e.g., lines prefixed with `> Note:` or manually checked boxes).

---

## Strict Plan Structure

The `IMPLEMENTATION_PLAN.md` MUST follow this exact layout. No sections may be omitted or reordered.

### Section 1: YAML Frontmatter

The plan starts immediately with a valid YAML frontmatter block:

```yaml
---
goal: "Concise title describing the implementation goal"
date_created: YYYY-MM-DD
date_updated: YYYY-MM-DD
go_version: "<Target Go version from go.mod or spec>"
status: "Planned"  # Strictly one of: Planned | In Progress | Completed | On Hold
git_strategy: "Stacked Branches / Git Worktrees"
---
```

### Section 2: Overview & Impact Dashboard

Directly below the frontmatter, include an executive summary paragraph updated incrementally as specs change, followed by a tracking table:

```markdown
## Overview

<One paragraph summarizing what will be built, key design decisions, and the spec files that inform this plan.>

### Impact Dashboard

| Phase | Go Package / Target | Files Modified / Created | Task Stacking / Parallelism | Status |
|-------|--------------------|--------------------------|-----------------------------|--------|
| Phase 1 | `internal/alertmanager` | `client.go`, `types.go`, `client_test.go` | Independent | Planned |
| Phase 2 | `internal/proposal` | `builder.go`, `naming.go`, `builder_test.go` | Independent | Planned |
| Phase 3 | `internal/poller` | `poller.go`, `poller_test.go` | Blocked by Phase 1, 2 | Planned |
```

Status values: `Planned`, `In Progress`, `Completed`, `On Hold`, `Deprecated`.

### Section 3: Technical Constraints & Boundary Guardrails

```markdown
## Technical Constraints & Boundary Guardrails

### Scope Boundaries
**In-scope**:
- <Bulleted list derived from spec — what IS being built>

**Out-of-scope**:
- <Bulleted list derived from spec's "Future Work" or explicit exclusions>

### Go Architectural Constraints
- <Concurrency model requirements (goroutines, channels, contexts)>
- <Interface boundaries and dependency injection patterns>
- <Required go.mod dependencies with exact import paths>
- <Error handling conventions>

### Interactive Planning Gate
> **MANDATORY**: The executing agent MUST halt and request explicit human sign-off
> on this plan before writing any production code. No code generation is permitted
> until the human approves this plan document.
```

### Section 4: Implementation Phases (Atomized Package Breakdown)

Phases are grouped by **logical Go package boundaries**. Each phase contains micro-tasks sized to fit a single LLM context turn.

```markdown
## Implementation Phases

### Phase N: <Title>

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/<filename>.md` (section: `<section name>`) |
| **Status** | `Active` or `Deprecated` |
| **Goal** | One sentence describing what this phase achieves |
| **Package** | `internal/<package>` |
| **Dependencies** | Phase X, Phase Y (or "None") |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task N.1: <Task Name>**

  **Target Location**: `internal/<package>/<file>.go`, `internal/<package>/<file>_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/<package>/<file>_test.go`.
  - Test function: `Test<FunctionName>_<Scenario>`
  - Expected behavior: <What the test asserts>. Note: In Go, compilation failure due to missing types/functions is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./<package-path> -run ^Test<FunctionName>_<Scenario>$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - <Specific deterministic criteria — no `panic("todo")`, no placeholder stubs, no mock shortcuts unless explicitly required>
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails, an architectural ambiguity arises, or the implementation deviates from the spec, immediately halt and prompt the human for review.

---

- [ ] **Task N.2: <Next Task>**
  ...
```

### Section 5: Spec Manifest

```markdown
## Spec Manifest

> This table tracks which spec files have been processed and their content hashes.
> It is used for incremental plan updates — do not edit manually.

| File | SHA256 | Status | Last Processed |
|------|--------|--------|----------------|
| `.ai/spec/initial-design.md` | `<full-sha256>` | ACTIVE | YYYY-MM-DD |
```

Status values for manifest entries: `ACTIVE`, `CHANGED`, `REMOVED`.

---

## Plan Generation Guidelines

### Ordering & Dependencies
- Order phases by dependency graph: foundational types and interfaces first, then business logic, then integration, then deployment artifacts.
- Explicitly declare inter-phase dependencies. A phase MUST NOT reference types or interfaces from a phase it doesn't depend on.

### Granularity
- Each phase should be independently testable and reviewable in a single PR.
- Micro-tasks within a phase must be small enough that an executing agent can complete one in a single context turn without drift.
- Prefer many small tasks over fewer large ones.

### TDD Enforcement
- Every micro-task MUST follow the Red-Green-Refactor cycle.
- Test files are written **before** production code — no exceptions.
- Favor Go's idiomatic table-driven test style (`[]struct{ name string; ... }`).
- Each task specifies the exact `go test` command to validate it.
- No `panic("todo")`, `// TODO`, or stub implementations are permitted in exit criteria.
- In Go, the RED state may manifest as a compilation failure (missing types, undefined functions) rather than a test assertion failure. This is expected and valid.

### Scope Control
- Do NOT include phases for items marked as "Future Work" in the spec.
- Do NOT add features, abstractions, or infrastructure beyond what the spec defines.
- If the spec is silent on a decision, call it out as a `> Decision needed:` block within the relevant phase.

### Source Traceability
- Every phase MUST have a `Source` annotation linking to the spec file and section.
- Every phase MUST have a `Status` annotation (`Active` or `Deprecated`).
- This traceability is critical for incremental updates — phases without source annotations cannot be maintained.

### Deployment & Build Artifacts
- Include a phase for the `Dockerfile` and `Makefile`.
- Include a phase for Kubernetes manifests if the spec defines deployment resources.
- Include a phase for CI configuration if referenced in the spec.

---

## Output

Write the plan to `IMPLEMENTATION_PLAN.md` in the project root. Update the `## Spec Manifest` table with current hashes and today's date for all processed files. Do not create any other files.
