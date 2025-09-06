
# CODING.md — Score Orchestrator (controllers)

Shared coding rules for Score Orchestrator / Resolver / Runtime controllers. We operate **docs‑first** (the spec is the source of truth), enforce **single‑writer**, keep **platform‑agnostic vocabulary**, and prefer **event‑driven** designs.

---

## 0. Principles

* **Docs‑first**: Code must follow `README`, `ADR`, `docs/spec/*`, `lifecycle`, `rbac`. Propose changes by updating docs first.
* **Single writer**: Only the Orchestrator writes `Workload.status`. Others must not.
* **Agnostic wording**: User‑facing `Workload.status` must avoid runtime‑specific nouns.
* **Idempotent reconcile**: Reconciliation is safe to run repeatedly; side effects are minimized.
* **OwnerReferences**: All internal resources created for a `Workload` must set `OwnerReference=Workload`.

---

## 1. Go / repo conventions

* **Context**: Do not use `context.TODO()`. Always thread the caller’s `ctx`.
* **Errors**: Prefer `errors.Is/As` and `fmt.Errorf("...: %w", err)`.
* **Static analysis**: `go vet` and `golangci-lint` (e.g., `unused`, `errcheck`, `gocritic`). `make lint` must pass.
* **Naming**: Short, singular package names (e.g., `endpoint`, `conditions`, `status`, `reconcile`).
* **Function size**: Keep the Reconciler thin; split intent into helpers like `ensureFinalizer`, `applyPolicy`, `upsertBindings`, `aggregateBindings`, `upsertPlan`, `updateStatusAndEndpoint`.
* **Log keys**: Use consistent keys: `ns`, `name`, `workload`, `bindingKey`, `planName`.

---

## 2. controller‑runtime conventions

* **Watches & Indexers**: Register field indexers *before* manager start (`ResourceBinding.byWorkload`, `WorkloadPlan.byWorkload`). Use `Watches(..., EnqueueRequestsFromMapFunc(byIndex))`.
* **Cache**: Read via the cached client by default. Use `APIReader` only when strict consistency is required.
* **Concurrency**: Start with `MaxConcurrentReconciles=1` (tune later).
* **Requeue**: Requeue on external waits (Resolver/Runtime) and on `IsConflict`.
* **Periodic resync**: Keep long (e.g., 1h) to reduce noise.

---

## 3. Mutation policy (Patch‑first)

* **Finalizer add/remove**: `Patch(MergeFrom(before))`. On 409 conflicts, requeue.
* **Status updates**: Use `Status().Patch`. Batch multiple condition and endpoint updates into a **single patch**.
* **Owned resource upsert**: `Create` or `Patch` (avoid `Apply` for now). Always set OwnerRef.
* **Never write `ResourceBinding.status`**: Only the Resolver updates it.
* **No `WorkloadPlan.status`**: Do not add or write `.status` on `WorkloadPlan`.

---

## 4. Conditions & vocabulary

* **Types**: `Ready`, `BindingsReady`, `RuntimeReady`, `InputsValid`.
* **Reasons** (fixed): `Succeeded`, `SpecInvalid`, `PolicyViolation`, `BindingPending`, `BindingFailed`, `ProjectionError`, `RuntimeSelecting`, `RuntimeProvisioning`, `RuntimeDegraded`, `QuotaExceeded`, `PermissionDenied`, `NetworkUnavailable`.
* **Message**: Neutral, single sentence, no runtime‑specific nouns (e.g., "All required bindings are available.").
* **Readiness rule**: `Ready = InputsValid ∧ BindingsReady ∧ RuntimeReady`. Centralize logic in the `conditions` package.

---

## 5. Endpoint rules

* **Single value**: `Workload.status.endpoint` holds at most one canonical endpoint.
* **Priority (MVP)**: PlatformPolicy template → (future) Runtime report → (future) Service‑derived → default.
* **Normalization**: Prefer https, omit default ports 80/443, prefer FQDN, support IPv6 with brackets, no trailing slash.

---

## 6. RBAC & visibility

* **Orchestrator**: read `workloads`; write `workloads/status`; CRUD `resourcebindings` and `workloadplans`; read `resourcebindings/status`; read `platformpolicies`; create/patch `events`.
* **Resolver**: may write `resourcebindings/status` (others minimal).
* **Runtime**: writes details to its own internal objects; must not write `Workload.status`.

---

## 7. Logging & events

* **Structured logs**: Emit at key transitions (BindingsReady change, Plan creation, Endpoint reflection, Policy evaluation).
* **Events**: Use `Normal`/`Warning`. Centralize reason strings as constants (e.g., `PlanCreated`, `BindingsReady`, `EndpointReflected`).

---

## 8. Testing (envtest) conventions

* **Event‑driven**: Do **not** call `Reconcile()` directly; let Watches drive it.
* **Manager/namespace per test**: Use a dedicated Namespace and Manager per test; restrict cache to that namespace; always `WaitForCacheSync`.
* **Observation viewpoint**: Read using `mgr.GetClient()` (cached client) in assertions.
* **Deletion order**: Foreground delete resources/namespace → wait for NotFound → then stop the manager.
* **Eventually defaults**: e.g., 10s timeout / 100ms interval. Register indexers before manager start.
* **Unit vs envtest**: Keep `aggregate/derive` pure and cover with table‑driven unit tests; reserve envtest for minimal E2E paths.

---

## 9. Package layout

```
/internal/
  controller/        # Reconciler, Setup, mappers
  reconcile/         # ensureFinalizer, binding/plan upsert (side effects)
  status/            # condition aggregation (pure)
  endpoint/          # endpoint derivation & normalization (pure)
  conditions/        # vocabulary & SetCondition helpers
  meta/              # constants for finalizer, index keys, labels
```

---

## 10. Change process

1. For spec changes, **update ADR/docs first**.
2. Keep PRs small (**one controller per PR** when possible).
3. **Definition of Done**: tests (envtest/unit) green; RBAC aligned; key logs/events present; link back to docs.

