# Demo Data Generator — Resume Notes (session paused 2026-11-17)

**Branch:** `fix/reports-orders-shipping-main` (also carries the earlier Reports/Orders/Shipping fixes and the AddStockTx deadlock-retry hardening — see git log). Not merged into `main`. Live NARM/WMS was not touched during any of this Phase 5/7 work.

## What's actually done (Phase 5 — design)

- `docs/proposals/wms-demo-data-generator/00-design.md` — full design: demo data goes into the **existing EWTA tenant** (id=2), reusing all 39 products and EWTA-WH (id=7), isolated by `DEMO-EWTA-20261118` reference-code prefixes rather than a separate tenant. Two open decisions were resolved with you: add one new warehouse (`DEMO-EWTA-WH2`) for Inter-Warehouse Transfer, and add 3-4 new demo users (EWTA currently has only one user, East-Demo/ADMIN).
- Fresh **post-migration** backup of live NARM/WMS taken and verified: `WMS_postmigration_0122_20261117_1.bak` (the earlier backup was pre-migration, schema 0078 — superseded).
- Clean rehearsal/dev database restored from that backup: `WMS_demo_rehearsal` (schema 0122, byte-identical to live EWTA state, no test-fixture noise). **This is the correct target for all demodata development/testing** — not the older `WMS_migration_rehearsal`, which is dirtied by earlier regression-test runs.
- Audited EWTA's actual existing master data directly against that clean copy:

  | Table | Count |
  |---|---|
  | products | 39 |
  | warehouses | 1 (EWTA-WH, id=7) |
  | customers | 3 |
  | suppliers | 8 |
  | product_categories | 36 |
  | brands | 15 |
  | zones | 8 |
  | locations | 72 |
  | customer_delivery_locations | 11 |
  | users (EWTA membership) | 1 (East-Demo, ADMIN) |
  | orders / receipts / inventory / warehouse_layout_versions | 0 each |

## Phase 7 implementation progress (partial)

`backend/cmd/demodata/` — a new Go command mirroring `cmd/migrate`'s shape (`preview`/`run` subcommands, `--confirm-target=<db>` safety guard). Files I wrote and verified compiling as a package (before the incident below): `main.go`, `refgen.go`, `manifest.go`, `stages.go`, `wiring.go`, `util.go`, plus placeholder stubs for `stage_inbound.go`, `stage_inventoryops.go`, `stage_outbound.go`, `stage_transfer.go`.

**`stage_masterdata.go` is NOT in a trusted state — see incident below. Do not run `go build ./cmd/demodata/...` until this is resolved**, since that file is included in the package.

### Critical finding carried over from research: `qc_required=false` on all 39 EWTA products

All 39 existing EWTA products have `qc_required = false`. The Receiving → QC → Putaway demo flow has nothing eligible for QC to inspect unless this changes.

**Preferred strategy (your decision, recorded here): create 3-5 new `DEMO-EWTA-PROD-###`-tagged products with `qc_required=true`, used only for the receiving/QC/putaway demo flow.** The 39 real EWTA products stay completely untouched — same pattern as the second warehouse and the demo users (pure additions, never modifying existing master data). Do not flip `qc_required` on existing products.

## Incident: unexplained file modifications in `cmd/demodata/`

While implementing `stage_masterdata.go`, the file's content changed twice without my own Edit/Write calls producing that content — each time to different, fully-formed, plausible Go code I did not author. The second occurrence also left behind a file I never created (`helpers.go`), and the shared task list showed a task (#12, "Master-data additions stage") marked completed that I had not marked.

**Assessment:** the second version and `helpers.go` are mutually consistent (functions call each other correctly, e.g. `generateDemoPassword()`/`writeDemoCredentials`), which rules out random corruption. Combined with the task-list change, the most likely explanation is **a second Claude Code session or agent working on this same project concurrently** (this environment shares task state and, apparently, file-write timing across sessions in the same project), not a security attack — though I did not have a way to fully confirm this from inside this session, and flagged it to you rather than assume.

**Current file state:** `stage_masterdata.go` and `helpers.go` have been left exactly as found — not executed, not overwritten, not deleted, per your instruction. Evidence preserved outside the repo (this machine only, not committed):
`C:\Users\PC-Narm\AppData\Local\Temp\claude\C--vue-vue-wms\2e1e7eda-35ff-46a8-8dcc-190d38c2efb3\scratchpad\file-anomaly-evidence\` — contains timestamped copies of both files, a directory listing, and `git status`/`git diff` output from the moment of investigation.

**Before resuming demodata work:** check whether another Claude Code window/session is (or was) open against `C:\vue\vue-wms` on this same branch. If so, coordinate so only one session edits `cmd/demodata/` at a time — this was very likely two sessions racing on the same files, not a security incident.

**Confirmed (read-only check): the tool was actually executed by that other session, against the correct, safe target.** `WMS_demo_rehearsal` now has warehouses 1→2, zones 8→10, locations 72→76, users 1→6 — matching the master-data stage's plan (one new warehouse, two zones, four locations, four demo users). `WMS_migration_rehearsal` is untouched (0 demo rows). **Live NARM/WMS row counts are unchanged** (tenants=2, users=2, warehouses=1, products=39, tenant_users=2, zones=8) — re-verified directly, read-only, before this session paused. A real `docs/demo-data/EWTA-DEMO-20261118-credentials.txt` (4 lines, real generated passwords) and `EWTA-DEMO-20261118-manifest.json` now exist on disk from that run — **neither is committed, both contain run output from the untrusted code, and the credentials file must never be committed or printed.**

## Research already done — exact signatures for implementing the remaining stages

These came from three research passes this session. Capturing the key findings here so they aren't lost between sessions (the originating agents' transcripts are session-local and won't be available when resuming on another machine).

**Outbound + Transfer (order, picking, pickingbatch, packing, loading, shipping, transferreceipt) — fully captured, high confidence:**
- Full end-to-end call sequence PENDING→DISPATCHED for a sales order: `order.Create` → `order.Release` → `order.CreatePickingTask` → `picking.Confirm` (PICKED, this is where `ConsumeReservationTx` actually deducts stock) → `packing.Create` → `packing.CreateCarton` → `packing.PackItem` → `packing.CheckerConfirm` → `shipping.Create` (from the **packing job**, not from loading) → `loading.CreateTrip` → `loading.AssignShipment` → `loading.ScanCarton` → `loading.ConfirmTrip` (auto-calls `shipping.MarkLoaded`) → `shipping.Confirm(DISPATCH)` → optionally `shipping.Confirm(DELIVER)`.
- For `INTERNAL_TRANSFER`: same pipeline with `order_type=INTERNAL_TRANSFER` + `DestinationWarehouseID` set at Create, then after dispatch: `transferreceipt.Create(ShipmentID)` → `transferreceipt.ReceiveItem(...)` per line (this is what calls `inventory.AddStockTx` at the destination) → `transferreceipt.Confirm`.
- Short-pick scenario = a `picking.Confirm` line with `Action:"SHORT", ShortReason:"OUT_OF_STOCK"`.
- `shipping.Confirm` is confirmed (by reading the code, not just the Phase 33 checkpoint doc) to be pure status/timestamp transitions — no inventory call at all.

**Master data + Inbound (warehouse, user, tenant, role, warehouselayout, receiving, qualitycheck, putaway) — fully captured, high confidence:**
- `warehouse.Service.Create` and `zone.Service.Create` both silently default new rows to `tenant_id=1` (never thread a real TenantID through) — a real, pre-existing multi-tenancy gap in both packages, confirmed independently by direct source reading (not something this tool caused). Workaround: call `warehouseRepo.Create`/`zoneRepo.Create` directly with `TenantID` set explicitly. `location.Service.Create` does NOT have this bug — it's correctly tenant-scoped, safe to use normally.
- Real "add a user to a tenant with a role" mechanism: `tenant.MembershipService.CreateMembership(ctx, tenantID, CreateMembershipRequest{Mode: MembershipModeNew, RoleCode, Username, Password, FullName, Email}, actorUserID, actorType)` — starts the membership at `INVITED`, so a follow-up `ChangeStatus(ctx, tenantID, userID, ChangeMembershipStatusRequest{Status:"ACTIVE"}, actorUserID, actorType)` is required before the account is usable. `actorType` constants are `tenant.AuditActorPlatformAdmin`/`tenant.AuditActorTenantAdmin` (verified these two exist; a third constant referenced in the untrusted second file version, `AuditActorSystem`, was **not** independently verified — check before using it).
- `receiving.Create`→`Confirm` (Confirm puts stock on HOLD via `AddStockTx` internally) → `qualitycheck.Create` (items auto-populate from the receipt's confirmed-but-not-yet-inspected lines; **fails with `ErrNoEligibleLines` if the product's `qc_required` flag means nothing is eligible — ties directly into the finding above**) → `qualitycheck.Confirm` (PASS/REJECT call `ReleaseHoldTx`/`RejectHoldTx` internally) → `putaway.Sources(receiptID)` (discovers what's eligible) → `putaway.Create` → `putaway.Confirm` (this is the actual destination `AddStockTx`).
- `warehouselayout.Service.SaveLayout(ctx, tenantID, warehouseID, SaveLayoutRequest{Objects: [...]}, userID)` — `Objects` is the complete graph for a new version, not a diff.

**Inventory-ops (stockcount, stockadjustment, relocation, replenishment) — INCOMPLETE.** That research pass spent most of its effort on the `qc_required` discovery above and did not return a full signature cheat sheet before this session paused. **First step on resume: re-run that research** (constructors, request DTOs, and preconditions for `stockcount`/`stockadjustment`/`relocation`/`replenishment`) before writing those stage files.

## Exact steps to resume (at home or any later session)

1. `cd C:\vue\vue-wms\backend && git status` — confirm you're still on `fix/reports-orders-shipping-main` and nothing unexpected has changed.
2. Check whether another Claude Code session is open on this same project/branch before touching `cmd/demodata/` again — see the incident note above.
3. Read `cmd/demodata/stage_masterdata.go` and `helpers.go` fresh — decide whether to keep, discard, or merge ideas from whatever is currently there (the evidence copies at the path above capture the as-found state if useful for comparison).
4. Re-run research for stockcount/stockadjustment/relocation/replenishment signatures (incomplete, see above).
5. Decide and implement the `qc_required` fix: create 3-5 `DEMO-EWTA-PROD-###` products with `qc_required=true` (preferred, recorded above) as part of (or just before) the inbound stage.
6. Implement `stage_inbound.go`, `stage_inventoryops.go`, `stage_outbound.go`, `stage_transfer.go` using the signatures captured above.
7. `go build ./cmd/demodata/...` and `go run ./cmd/demodata preview --confirm-target=WMS_demo_rehearsal` against the clean rehearsal copy — never against live NARM/WMS at this stage.
8. Only after a clean rehearsal run, full acceptance-criteria check (see 00-design.md), and your explicit separate approval, consider live NARM/WMS.
