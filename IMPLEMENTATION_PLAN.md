---
goal: "Implement lightspeed-agentic-alerts-adapter: AlertManager-to-Proposal bridge"
date_created: 2026-05-21
date_updated: 2026-05-21
go_version: "1.26"
status: "Planned"
git_strategy: "Stacked Branches / Git Worktrees"
---

## Overview

The lightspeed-agentic-alerts-adapter is a stateless, single-purpose Go binary that polls the in-cluster AlertManager API for firing alerts and creates `Proposal` CRs (`agentic.openshift.io/v1alpha1`) to trigger automated analysis, remediation, and verification workflows in the Lightspeed Agentic system. It runs as a single-replica Deployment in the `openshift-lightspeed` namespace, uses deterministic naming for race-condition prevention, and implements deduplication via initial-delay and cooldown-window checks against existing Proposals. The design is create-only and stateless — correctness derives from diffing AlertManager state against Kubernetes API state on every poll cycle. This plan is derived from `.ai/spec/initial-design.md`.

### Impact Dashboard

| Phase | Go Package / Target | Files Modified / Created | Task Stacking / Parallelism | Status |
|-------|--------------------|--------------------------|-----------------------------|--------|
| Phase 1 | Project root | `go.mod`, `Makefile`, directory structure | Independent | Planned |
| Phase 2 | `internal/alertmanager` | `types.go`, `types_test.go` | Independent (after Phase 1) | Planned |
| Phase 3 | `internal/proposal` | `naming.go`, `naming_test.go` | Independent (parallel with Phase 2) | Planned |
| Phase 4 | `internal/alertmanager` | `client.go`, `client_test.go` | Blocked by Phase 2 | Planned |
| Phase 5 | `internal/proposal` | `builder.go`, `builder_test.go` | Blocked by Phase 2, 3 | Planned |
| Phase 6 | `internal/poller` | `poller.go`, `poller_test.go` | Blocked by Phase 4, 5 | Planned |
| Phase 7 | `cmd` | `main.go` | Blocked by Phase 6 | Planned |
| Phase 8 | Project root | `Dockerfile`, `Makefile` (finalize) | Blocked by Phase 7 | Planned |
| Phase 9 | `deploy/` | `serviceaccount.yaml`, `clusterrole.yaml`, `clusterrolebinding.yaml`, `deployment.yaml` | Independent | Planned |

---

## Technical Constraints & Boundary Guardrails

### Scope Boundaries

**In-scope**:
- Poll AlertManager `GET /api/v2/alerts` for firing alerts at a configurable interval
- Create `Proposal` CRs for firing alerts that pass deduplication checks
- Deduplicate via initial-delay (alert must fire for N minutes) and cooldown-window (no re-proposal within N time of terminal Proposal)
- Deterministic Proposal naming from alert fingerprint for race-condition prevention
- Map alert data into Proposal spec fields using a Go `text/template`
- Health (`/healthz`) and readiness (`/readyz`) HTTP probes
- Structured logging
- Kubernetes RBAC manifests (ClusterRole, ClusterRoleBinding, ServiceAccount)
- Deployment manifest (single-replica)

**Out-of-scope**:
- AlertManager-aware filtering (silencing, inhibition, grouping)
- Custom fingerprinting and alert grouping
- Configurable parameters via CRD or ConfigMap (constants only in v1)
- Per-alert-group configuration
- Alert filtering by labels or severity
- Prometheus metrics
- Adapter-specific analysis output schema
- Workflow selection based on alert labels
- Multi-replica support / leader election

### Go Architectural Constraints
- **Concurrency**: Single goroutine poll loop with `time.Ticker`. Health probe server runs in a separate goroutine. Context propagation via `context.Context` for cancellation on SIGTERM/SIGINT.
- **Interfaces**: `AlertFetcher` interface for AlertManager client, `ProposalClient` interface wrapping `sigs.k8s.io/controller-runtime/pkg/client.Client` for Proposal CRUD. Enables unit testing with fakes.
- **Dependencies** (exact import paths):
  - `github.com/openshift/lightspeed-agentic-operator/api` — Typed Proposal CRD Go types
  - `sigs.k8s.io/controller-runtime/pkg/client` — Typed Kubernetes client for Proposal CRUD
  - `k8s.io/client-go` — In-cluster config, ServiceAccount auth
- **Error handling**: Return errors up the call stack. Log at the poll-loop level. Never panic on recoverable errors. Skip individual alerts on per-alert failures.
- **Testing**: Standard `testing` package with table-driven tests. No external test framework.

### Interactive Planning Gate

> **MANDATORY**: The executing agent MUST halt and request explicit human sign-off
> on this plan before writing any production code. No code generation is permitted
> until the human approves this plan document.

---

## Implementation Phases

### Phase 1: Project Scaffolding

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (sections: Project Structure, Dependencies, Configuration) |
| **Status** | `Active` |
| **Goal** | Initialize the Go module, directory layout, and build tooling so subsequent phases can compile |
| **Package** | Project root |
| **Dependencies** | None |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 1.1: Initialize Go module**

  **Target Location**: `go.mod`

  **Red Phase (Test Specification)**:
  No test file — this is project initialization. RED state: `go build ./...` fails because `go.mod` does not exist.

  **Execution Command**:
  ```bash
  go mod init github.com/rioloc/lightspeed-agentic-alerts-adapter
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - `go.mod` exists with module path `github.com/rioloc/lightspeed-agentic-alerts-adapter`
  - Go version directive set to `1.26`

  **Advancement Policy**:
  - **Auto-Advance**: If `go.mod` is valid, proceed.
  - **Halt**: If module path or Go version is wrong.

---

- [ ] **Task 1.2: Create directory structure**

  **Target Location**: `cmd/`, `internal/alertmanager/`, `internal/proposal/`, `internal/poller/`

  **Red Phase (Test Specification)**:
  No test file — structural scaffolding. RED state: directories do not exist.

  **Execution Command**:
  ```bash
  mkdir -p cmd internal/alertmanager internal/proposal internal/poller
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - All four directories exist
  - Each directory under `internal/` contains a placeholder `.go` file with just `package <name>` to make the package valid for `go build`

  **Advancement Policy**:
  - **Auto-Advance**: If `go build ./...` succeeds with the placeholder files, proceed.
  - **Halt**: If directory structure doesn't match spec.

---

- [ ] **Task 1.3: Makefile with build, test, and vet targets**

  **Target Location**: `Makefile`

  **Red Phase (Test Specification)**:
  No test file. RED state: `make build` fails because `Makefile` does not exist.

  **Execution Command**:
  ```bash
  make build && make test && make vet
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - `make build` compiles `cmd/main.go` (once it exists; initially a no-op or placeholder)
  - `make test` runs `go test ./...`
  - `make vet` runs `go vet ./...`
  - All three targets succeed

  **Advancement Policy**:
  - **Auto-Advance**: If all make targets pass, proceed.
  - **Halt**: If targets fail.

---

### Phase 2: AlertManager Response Types

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (sections: Alert-to-Proposal Cardinality, AlertManager Authentication, Poll Loop) |
| **Status** | `Active` |
| **Goal** | Define Go types that model the AlertManager v2 API alert response |
| **Package** | `internal/alertmanager` |
| **Dependencies** | Phase 1 |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 2.1: Define Alert and AlertStatus types**

  **Target Location**: `internal/alertmanager/types.go`, `internal/alertmanager/types_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/alertmanager/types_test.go`.
  - Test function: `TestAlert_JSONUnmarshal`
  - Expected behavior: Unmarshal a sample AlertManager v2 JSON response into `Alert` struct. Verify fields: `Labels` (map), `Annotations` (map), `StartsAt` (time.Time), `Fingerprint` (string), `Status.State` (string). Note: In Go, compilation failure due to missing types/functions is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/alertmanager -run ^TestAlert_JSONUnmarshal$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - `Alert` struct has fields: `Labels map[string]string`, `Annotations map[string]string`, `StartsAt time.Time`, `EndsAt time.Time`, `Fingerprint string`, `Status AlertStatus`
  - `AlertStatus` struct has field: `State string`
  - JSON tags match AlertManager v2 API field names (`labels`, `annotations`, `startsAt`, `endsAt`, `fingerprint`, `status`)
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails, an architectural ambiguity arises, or the implementation deviates from the spec, immediately halt and prompt the human for review.

---

- [ ] **Task 2.2: Helper methods on Alert**

  **Target Location**: `internal/alertmanager/types.go`, `internal/alertmanager/types_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/alertmanager/types_test.go`.
  - Test function: `TestAlert_Accessors`
  - Table-driven test covering:
    - `AlertName()` returns `labels["alertname"]`
    - `Severity()` returns `labels["severity"]` (empty string if missing)
    - `Namespace()` returns `labels["namespace"]` (empty string if missing)
    - `Summary()` returns `annotations["summary"]`
    - `Description()` returns `annotations["description"]`
  - Note: In Go, compilation failure due to missing methods is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/alertmanager -run ^TestAlert_Accessors$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - All accessor methods return correct values, including empty string for missing keys
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the implementation deviates from the spec.

---

### Phase 3: Proposal Naming

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (section: Race Condition Prevention) |
| **Status** | `Active` |
| **Goal** | Implement deterministic Proposal name generation and DNS-safe sanitization |
| **Package** | `internal/proposal` |
| **Dependencies** | Phase 1 |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 3.1: Deterministic ProposalName function**

  **Target Location**: `internal/proposal/naming.go`, `internal/proposal/naming_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/proposal/naming_test.go`.
  - Test function: `TestProposalName`
  - Table-driven test covering:
    - Normal case: `alertname=KubePodCrashLooping`, `namespace=production`, `fingerprint=a1b2c3d4e5f6` → `kubepodcrashlooping-production-a1b2c3d4`
    - No namespace: `alertname=EtcdHighFsyncDurations`, `namespace=""`, `fingerprint=f9e8d7c6b5a4` → `etcdhighfsyncdurations--f9e8d7c6`
    - Short fingerprint (< 8 chars): use full fingerprint
  - Note: In Go, compilation failure due to missing functions is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/proposal -run ^TestProposalName$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Function signature: `func ProposalName(alertname, namespace, fingerprint string) string`
  - Output format: `{alertname}-{namespace}-{fingerprint[:8]}` (all lowercased)
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or naming format doesn't match the spec.

---

- [ ] **Task 3.2: DNS subdomain sanitization**

  **Target Location**: `internal/proposal/naming.go`, `internal/proposal/naming_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/proposal/naming_test.go`.
  - Test function: `TestSanitizeDNSSubdomain`
  - Table-driven test covering:
    - Uppercase → lowercase
    - Non-alphanumeric characters (except `-` and `.`) → replaced with `-`
    - Leading/trailing dashes stripped
    - Name exceeding 253 characters → truncated to 253
    - Empty string → returns empty string
  - Note: In Go, compilation failure due to missing functions is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/proposal -run ^TestSanitizeDNSSubdomain$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Function conforms to RFC 1123 DNS subdomain rules
  - `ProposalName` (from Task 3.1) uses this sanitization internally
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or sanitization rules don't match RFC 1123.

---

### Phase 4: AlertManager Client

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (sections: AlertManager Authentication, Why Polling, Error Handling) |
| **Status** | `Active` |
| **Goal** | Implement an HTTP client that fetches firing alerts from the AlertManager v2 API with ServiceAccount auth and TLS |
| **Package** | `internal/alertmanager` |
| **Dependencies** | Phase 2 |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 4.1: Define AlertFetcher interface and Client struct**

  **Target Location**: `internal/alertmanager/client.go`, `internal/alertmanager/client_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/alertmanager/client_test.go`.
  - Test function: `TestNewClient`
  - Expected behavior: `NewClient(baseURL, httpClient)` returns a non-nil `*Client` that satisfies the `AlertFetcher` interface. Note: In Go, compilation failure due to missing types/functions is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/alertmanager -run ^TestNewClient$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - `AlertFetcher` interface: `FetchFiringAlerts(ctx context.Context) ([]Alert, error)`
  - `Client` struct implements `AlertFetcher`
  - `NewClient` constructor accepts base URL and `*http.Client`
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the interface doesn't match the spec.

---

- [ ] **Task 4.2: FetchFiringAlerts implementation**

  **Target Location**: `internal/alertmanager/client.go`, `internal/alertmanager/client_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/alertmanager/client_test.go`.
  - Test function: `TestFetchFiringAlerts`
  - Table-driven test using `httptest.Server` to mock AlertManager responses:
    - Success: valid JSON array of alerts → returns parsed `[]Alert`
    - Empty array: `[]` → returns empty slice, no error
    - HTTP error (500): returns error
    - Invalid JSON: returns error
    - Context cancelled: returns context error
  - Note: In Go, compilation failure due to missing methods is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/alertmanager -run ^TestFetchFiringAlerts$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Hits `GET <baseURL>/api/v2/alerts?filter=active%3Dtrue` (or equivalent query param for firing alerts)
  - Sends `Authorization: Bearer <token>` header (token provided via constructor or read from SA mount)
  - Parses JSON response into `[]Alert`
  - Returns descriptive errors for HTTP failures and JSON parse failures
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the implementation deviates from the spec.

---

### Phase 5: Proposal Builder

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (sections: Alert to Proposal Mapping, Labels and Annotations) |
| **Status** | `Active` |
| **Goal** | Implement the mapping from an AlertManager Alert to a Proposal CR, including template rendering, label/annotation construction, and namespace resolution |
| **Package** | `internal/proposal` |
| **Dependencies** | Phase 2, Phase 3 |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 5.1: Request template rendering**

  **Target Location**: `internal/proposal/builder.go`, `internal/proposal/builder_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/proposal/builder_test.go`.
  - Test function: `TestRenderRequest`
  - Table-driven test covering:
    - Full alert data: all fields populated → rendered template includes AlertName, Severity, Namespace, Summary, Description, and all Labels
    - Missing optional fields: no namespace, no summary → template renders with empty values, no template execution error
    - Special characters in annotations: HTML/template-safe rendering
  - Note: In Go, compilation failure due to missing functions is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/proposal -run ^TestRenderRequest$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Template matches the spec's `requestTemplate` format
  - Uses `text/template` (not `html/template`)
  - Template input struct populated from Alert accessor methods
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the template doesn't match the spec.

---

- [ ] **Task 5.2: BuildProposal function**

  **Target Location**: `internal/proposal/builder.go`, `internal/proposal/builder_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/proposal/builder_test.go`.
  - Test function: `TestBuildProposal`
  - Table-driven test covering:
    - Namespaced alert: Proposal created in alert's namespace, `targetNamespaces` set
    - Cluster-scoped alert (no namespace label): Proposal created in `openshift-lightspeed`, `targetNamespaces` empty
    - Proposal name matches deterministic format from `ProposalName()`
    - `spec.analysis.agent`, `spec.execution.agent`, `spec.verification.agent` all set to `"default"`
    - `spec.analysisOutput.mode` set to `"Default"`
  - Note: In Go, compilation failure due to missing functions is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/proposal -run ^TestBuildProposal$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Function signature: `func BuildProposal(alert alertmanager.Alert, defaultNamespace, agentName string) (*agentic.Proposal, error)`
  - Proposal `metadata.name` set via `ProposalName()`
  - Proposal `metadata.namespace` set to alert namespace or `defaultNamespace` fallback
  - `spec.request` set via template rendering
  - All three agent steps set to `agentName`
  - `spec.analysisOutput.mode` set to `Default`
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the mapping doesn't match the spec.

---

- [ ] **Task 5.3: Labels and annotations construction**

  **Target Location**: `internal/proposal/builder.go`, `internal/proposal/builder_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/proposal/builder_test.go`.
  - Test function: `TestBuildProposal_LabelsAndAnnotations`
  - Table-driven test verifying the Proposal returned by `BuildProposal` has:
    - Label `agentic.openshift.io/source` = `"alertmanager"`
    - Label `agentic.openshift.io/alert-fingerprint` = fingerprint[:8]
    - Label `agentic.openshift.io/alert-name` = alertname (lowercased)
    - Label `agentic.openshift.io/alert-severity` = severity
    - Annotation `agentic.openshift.io/alert-starts-at` = RFC3339 timestamp
    - Annotation `agentic.openshift.io/alert-summary` = summary (truncated if long)
  - Note: In Go, compilation failure due to missing labels/annotations in the Proposal is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/proposal -run ^TestBuildProposal_LabelsAndAnnotations$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - All labels and annotations match the spec exactly
  - Annotation values are properly formatted (RFC3339 for timestamp, truncation for summary)
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or labels/annotations don't match the spec.

---

### Phase 6: Poll Loop

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (sections: Poll Loop, Deduplication, Race Condition Prevention, Error Handling, Alert Resolution Behavior) |
| **Status** | `Active` |
| **Goal** | Implement the core poll loop that fetches alerts, diffs against existing Proposals, and creates new Proposals subject to deduplication rules |
| **Package** | `internal/poller` |
| **Dependencies** | Phase 4, Phase 5 |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 6.1: Define Poller struct and dependencies**

  **Target Location**: `internal/poller/poller.go`, `internal/poller/poller_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/poller/poller_test.go`.
  - Test function: `TestNewPoller`
  - Expected behavior: `NewPoller(alertFetcher, k8sClient, config)` returns a valid `*Poller`. Accepts interface dependencies for testability. Config includes `InitialDelay`, `CooldownWindow`, `DefaultNamespace`, `DefaultAgent`. Note: In Go, compilation failure due to missing types is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/poller -run ^TestNewPoller$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - `Poller` struct holds `AlertFetcher` interface, `client.Client` (controller-runtime), and config values
  - `Config` struct defined with `InitialDelay`, `CooldownWindow`, `DefaultNamespace`, `DefaultAgent` fields
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the struct doesn't support the required dependencies.

---

- [ ] **Task 6.2: InitialDelay filtering**

  **Target Location**: `internal/poller/poller.go`, `internal/poller/poller_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/poller/poller_test.go`.
  - Test function: `TestPoller_InitialDelayFilter`
  - Table-driven test covering:
    - Alert `startsAt` is 10 minutes ago, `InitialDelay` is 5 minutes → alert passes filter
    - Alert `startsAt` is 2 minutes ago, `InitialDelay` is 5 minutes → alert is filtered out
    - Alert `startsAt` is exactly 5 minutes ago → alert passes filter (boundary)
  - Note: In Go, compilation failure due to missing methods is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/poller -run ^TestPoller_InitialDelayFilter$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Filter uses `time.Since(alert.StartsAt) >= config.InitialDelay`
  - No in-memory state needed — uses alert's `startsAt` field directly
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the filtering logic doesn't match the spec.

---

- [ ] **Task 6.3: Active Proposal deduplication**

  **Target Location**: `internal/poller/poller.go`, `internal/poller/poller_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/poller/poller_test.go`.
  - Test function: `TestPoller_ActiveProposalDedup`
  - Table-driven test covering:
    - Alert has an existing active Proposal (same fingerprint, non-terminal phase) → skip
    - Alert has no existing Proposal → proceed to create
    - Alert has an existing Proposal in terminal phase (Completed/Failed/Escalated/Denied) → not blocked by this check (cooldown check is separate)
  - Uses a fake `client.Client` with pre-populated Proposal objects.
  - Note: In Go, compilation failure due to missing methods is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/poller -run ^TestPoller_ActiveProposalDedup$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Lists Proposals with label selector `agentic.openshift.io/alert-fingerprint=<fingerprint[:8]>`
  - Correctly identifies active vs terminal Proposals
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the dedup logic doesn't match the spec.

---

- [ ] **Task 6.4: Cooldown window check**

  **Target Location**: `internal/poller/poller.go`, `internal/poller/poller_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/poller/poller_test.go`.
  - Test function: `TestPoller_CooldownWindow`
  - Table-driven test covering:
    - Terminal Proposal's condition timestamp is 30 minutes ago, `CooldownWindow` is 1 hour → skip (within cooldown)
    - Terminal Proposal's condition timestamp is 2 hours ago, `CooldownWindow` is 1 hour → proceed (cooldown expired)
    - No terminal Proposal exists → proceed
  - Note: In Go, compilation failure due to missing methods is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/poller -run ^TestPoller_CooldownWindow$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Uses terminal Proposal's condition timestamps (not creation time)
  - `CooldownWindow` comparison is correct
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the cooldown logic doesn't match the spec.

---

- [ ] **Task 6.5: Proposal creation with 409 Conflict handling**

  **Target Location**: `internal/poller/poller.go`, `internal/poller/poller_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/poller/poller_test.go`.
  - Test function: `TestPoller_CreateProposal`
  - Table-driven test covering:
    - Successful creation: `client.Create` returns nil → success
    - AlreadyExists (409): `client.Create` returns `apierrors.IsAlreadyExists` error → treated as success (no error returned)
    - Other error: `client.Create` returns a non-409 error → error logged, alert skipped
  - Note: In Go, compilation failure due to missing methods is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/poller -run ^TestPoller_CreateProposal$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - Uses `apierrors.IsAlreadyExists(err)` to detect 409
  - 409 is treated as success (not returned as error)
  - Other errors are returned for logging
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or 409 handling doesn't match the spec.

---

- [ ] **Task 6.6: Full poll cycle integration**

  **Target Location**: `internal/poller/poller.go`, `internal/poller/poller_test.go`

  **Red Phase (Test Specification)**:
  Write test first at `internal/poller/poller_test.go`.
  - Test function: `TestPoller_PollCycle`
  - Integration-style test using fakes for both `AlertFetcher` and `client.Client`:
    - 3 firing alerts: one too recent (initial delay), one with active Proposal, one eligible → only 1 Proposal created
    - AlertManager returns error → poll cycle skipped, no Proposals created, no panic
    - Single alert with invalid data (missing alertname) → skipped, others still processed
  - Note: In Go, compilation failure due to missing methods is an acceptable and expected RED state.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go test -v ./internal/poller -run ^TestPoller_PollCycle$
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Test output: `PASS`
  - `PollOnce(ctx)` method orchestrates: fetch → filter → dedup → create
  - Each step's errors are handled per the spec's error handling section
  - Individual alert failures don't block other alerts
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the test passes and meets all exit criteria, check the box and proceed to the next task.
  - **Halt**: If the test fails or the poll cycle behavior doesn't match the spec.

---

### Phase 7: Entrypoint and Health Probes

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (sections: Health Probes, Configuration, Deployment) |
| **Status** | `Active` |
| **Goal** | Wire up the main binary with signal handling, health/readiness probes, and the poll loop |
| **Package** | `cmd` |
| **Dependencies** | Phase 6 |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 7.1: Health and readiness probe server**

  **Target Location**: `cmd/main.go`

  **Red Phase (Test Specification)**:
  No separate test file — this is the main package. RED state: `go build ./cmd` fails because `main.go` does not exist or is incomplete. Manually verify probes respond correctly.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go build -o /dev/null ./cmd
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - `go build ./cmd` succeeds
  - HTTP server on port 8081 serves `/healthz` (always 200) and `/readyz` (200 if last poll succeeded)
  - Readiness state is updated by the poll loop
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the binary compiles and probe endpoints are wired, proceed.
  - **Halt**: If compilation fails or probe logic is missing.

---

- [ ] **Task 7.2: Signal handling and graceful shutdown**

  **Target Location**: `cmd/main.go`

  **Red Phase (Test Specification)**:
  No separate test file. RED state: binary doesn't handle SIGTERM/SIGINT gracefully.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go build -o /dev/null ./cmd
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - `context.WithCancel` or `signal.NotifyContext` used for SIGTERM/SIGINT
  - Context cancellation propagates to poll loop and HTTP server
  - Binary exits cleanly on signal
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the binary compiles with signal handling, proceed.
  - **Halt**: If signal handling is missing or context propagation is broken.

---

- [ ] **Task 7.3: In-cluster client setup and main loop**

  **Target Location**: `cmd/main.go`

  **Red Phase (Test Specification)**:
  No separate test file. RED state: binary doesn't initialize clients or start the poll loop.

  **Execution Command**:
  ```bash
  # Ensure the package path starts with './'
  go build -o /dev/null ./cmd
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Uses `rest.InClusterConfig()` for Kubernetes client setup
  - Creates `controller-runtime` client with Proposal scheme registered
  - Creates AlertManager HTTP client with ServiceAccount token and cluster CA
  - Initializes `Poller` with configured constants (`PollInterval`, `InitialDelay`, `CooldownWindow`, `DefaultNamespace`, `DefaultAgent`)
  - Runs `time.Ticker`-based loop calling `poller.PollOnce(ctx)` every `PollInterval`
  - All constants match the spec's configuration table
  - Code compiles with zero warnings under `go vet ./...`

  **Advancement Policy**:
  - **Auto-Advance**: If the binary compiles and all wiring is correct, proceed.
  - **Halt**: If compilation fails or constants don't match the spec.

---

### Phase 8: Dockerfile and Makefile Finalization

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (section: Deployment) |
| **Status** | `Active` |
| **Goal** | Create a multi-stage Dockerfile and finalize the Makefile with image build targets |
| **Package** | Project root |
| **Dependencies** | Phase 7 |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 8.1: Multi-stage Dockerfile**

  **Target Location**: `Dockerfile`

  **Red Phase (Test Specification)**:
  No Go test. RED state: `docker build .` fails because `Dockerfile` does not exist.

  **Execution Command**:
  ```bash
  # Verify Dockerfile syntax (does not require Docker daemon)
  go build -o /dev/null ./cmd
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Multi-stage build: Go builder stage + minimal runtime stage (e.g., `gcr.io/distroless/static` or `ubi-micro`)
  - Builder stage compiles `cmd/main.go` with `CGO_ENABLED=0`
  - Runtime stage copies binary and runs as non-root
  - Image name matches spec: `quay.io/openshift-lightspeed/lightspeed-agentic-alerts-adapter`

  **Advancement Policy**:
  - **Auto-Advance**: If the Dockerfile is syntactically valid and follows multi-stage pattern, proceed.
  - **Halt**: If the build would fail or doesn't match the spec's image configuration.

---

- [ ] **Task 8.2: Makefile finalization**

  **Target Location**: `Makefile`

  **Red Phase (Test Specification)**:
  No Go test. RED state: `make image-build` fails.

  **Execution Command**:
  ```bash
  make build && make test && make vet
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - `make build` compiles the binary
  - `make test` runs all tests
  - `make vet` runs `go vet`
  - `make image-build` builds the container image (target exists, may skip actual build in CI-less env)
  - `make lint` runs golangci-lint (if available)

  **Advancement Policy**:
  - **Auto-Advance**: If all make targets are defined and `build`/`test`/`vet` pass, proceed.
  - **Halt**: If targets are missing or fail.

---

### Phase 9: Kubernetes Manifests

| Key | Value |
|-----|-------|
| **Source** | `.ai/spec/initial-design.md` (sections: Deployment, RBAC) |
| **Status** | `Active` |
| **Goal** | Create the Kubernetes resource manifests for deploying the adapter |
| **Package** | `deploy/` |
| **Dependencies** | None |

#### Context Management Note
> Each micro-task below is scoped to fit a single agent context turn. The executing
> agent should load only the files and types listed in "Target Location" for each task.
> Prior task outputs (committed code) serve as the sole context bridge between turns.

#### Tasks

- [ ] **Task 9.1: ServiceAccount**

  **Target Location**: `deploy/serviceaccount.yaml`

  **Red Phase (Test Specification)**:
  No Go test — YAML manifest. RED state: file does not exist.

  **Execution Command**:
  ```bash
  # Validate YAML syntax
  python3 -c "import yaml; yaml.safe_load(open('deploy/serviceaccount.yaml'))" 2>/dev/null || echo "YAML valid check skipped (no python3/yaml)"
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - ServiceAccount named `lightspeed-agentic-alerts-adapter` in namespace `openshift-lightspeed`

  **Advancement Policy**:
  - **Auto-Advance**: If YAML is valid and matches the spec, proceed.
  - **Halt**: If the manifest doesn't match the spec.

---

- [ ] **Task 9.2: ClusterRole and ClusterRoleBinding for AlertManager access**

  **Target Location**: `deploy/clusterrole-alertmanager.yaml`, `deploy/clusterrolebinding-alertmanager.yaml`

  **Red Phase (Test Specification)**:
  No Go test — YAML manifests. RED state: files do not exist.

  **Execution Command**:
  ```bash
  python3 -c "import yaml; yaml.safe_load(open('deploy/clusterrolebinding-alertmanager.yaml'))" 2>/dev/null || echo "YAML valid check skipped"
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - ClusterRoleBinding `lightspeed-agentic-alerts-adapter-alertmanager` binds role `monitoring-alertmanager-view` to the adapter's ServiceAccount
  - Matches the spec's RBAC section exactly

  **Advancement Policy**:
  - **Auto-Advance**: If YAML is valid and matches the spec, proceed.
  - **Halt**: If the manifest doesn't match the spec.

---

- [ ] **Task 9.3: ClusterRole and ClusterRoleBinding for Proposal management**

  **Target Location**: `deploy/clusterrole-proposals.yaml`, `deploy/clusterrolebinding-proposals.yaml`

  **Red Phase (Test Specification)**:
  No Go test — YAML manifests. RED state: files do not exist.

  **Execution Command**:
  ```bash
  python3 -c "import yaml; [yaml.safe_load(open(f)) for f in ['deploy/clusterrole-proposals.yaml', 'deploy/clusterrolebinding-proposals.yaml']]" 2>/dev/null || echo "YAML valid check skipped"
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - ClusterRole `lightspeed-agentic-alerts-adapter-proposals` with verbs `["create", "list", "get"]` on resource `proposals` in apiGroup `agentic.openshift.io`
  - ClusterRoleBinding binds this role to the adapter's ServiceAccount
  - Matches the spec's RBAC section exactly

  **Advancement Policy**:
  - **Auto-Advance**: If YAML is valid and matches the spec, proceed.
  - **Halt**: If the manifest doesn't match the spec.

---

- [ ] **Task 9.4: Deployment manifest**

  **Target Location**: `deploy/deployment.yaml`

  **Red Phase (Test Specification)**:
  No Go test — YAML manifest. RED state: file does not exist.

  **Execution Command**:
  ```bash
  python3 -c "import yaml; yaml.safe_load(open('deploy/deployment.yaml'))" 2>/dev/null || echo "YAML valid check skipped"
  ```

  **Green Phase (Exit Criteria / Definition of Done)**:
  - Deployment named `lightspeed-agentic-alerts-adapter` in namespace `openshift-lightspeed`
  - Single replica
  - ServiceAccount: `lightspeed-agentic-alerts-adapter`
  - Container image: `quay.io/openshift-lightspeed/lightspeed-agentic-alerts-adapter:latest`
  - Liveness probe: `httpGet /healthz :8081`
  - Readiness probe: `httpGet /readyz :8081`
  - Matches the spec's Deployment section exactly

  **Advancement Policy**:
  - **Auto-Advance**: If YAML is valid and matches the spec, proceed.
  - **Halt**: If the manifest doesn't match the spec.

---

## Spec Manifest

> This table tracks which spec files have been processed and their content hashes.
> It is used for incremental plan updates — do not edit manually.

| File | SHA256 | Status | Last Processed |
|------|--------|--------|----------------|
| `.ai/spec/initial-design.md` | `f801b3d22d97104486cea4cbe01fb010bbe5822f22237b5eee625593170b7a24` | ACTIVE | 2026-05-21 |
