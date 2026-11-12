# WMS User Acceptance Test (UAT) Checklist

Status as of this document: **every case below is PENDING**. None of
these have been run against a live system — Docker Desktop was
unavailable for the entirety of Phase 26's offline preparation pass, so
"Actual result" and "Status" are placeholders for whoever runs the live
pass once SQL Server/RabbitMQ/the backend/the frontend are all up.

How to use this document during the live pass:

1. Work through each module top to bottom.
2. Fill in **Actual result** with what really happened.
3. Flip **Status** to `PASS` or `FAIL` (never leave it `PENDING` once
   tested).
4. If a case fails, note the exact error/response in **Notes** and file
   it as a defect before moving on — do not mark a later, dependent
   case PASS if an earlier precondition failed.
5. Default test account: `admin` / the password in `SEED_ADMIN_PASSWORD`
   (seeded by `wms-migrate seed`, see `backend/.env.example`). Tests
   marked "as WAREHOUSE_MANAGER" or another role need a second seeded
   user with that role — see AUTH-06.

Permission codes referenced below come from `backend/internal/rbac/catalog.go`
(the single source of truth — verify against it if this checklist and
the code ever disagree, and trust the code).

---

## 1. Authentication / RBAC

### AUTH-01: Successful login
- **Module:** Auth
- **Preconditions:** `wms-migrate seed` has run; admin user exists.
- **Steps:** `POST /api/v1/auth/login` with `{"username":"admin","password":"<seeded password>"}`.
- **Expected:** 200, `success:true`, response includes `access_token`, `expires_in`, and a `permissions` array containing every catalog code (ADMIN = `allPermissionCodes()`).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-02: Invalid credentials rejected
- **Module:** Auth
- **Steps:** `POST /api/v1/auth/login` with a wrong password.
- **Expected:** 401, `success:false`, no token issued, no user enumeration (same error whether username exists or not).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-03: `GET /auth/me` requires a valid token
- **Module:** Auth
- **Steps:** Call `GET /api/v1/auth/me` with no `Authorization` header, then with a garbage token, then with a valid one.
- **Expected:** 401 / 401 / 200 with the current user's profile respectively.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-04: Expired token rejected
- **Module:** Auth
- **Preconditions:** A token issued with a very short `JWT_EXPIRES_IN` (temporarily set, or wait out the normal expiry).
- **Steps:** Call any protected endpoint after expiry.
- **Expected:** 401 `invalid or expired token`.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-05: RBAC — missing permission returns 403, not 401
- **Module:** RBAC
- **Preconditions:** A logged-in user whose role lacks e.g. `users.delete` (any non-ADMIN role).
- **Steps:** Attempt `DELETE /api/v1/users/{id}` with that user's token.
- **Expected:** 403 `permission denied: users.delete` — token itself is valid (not 401).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-06: Role/permission change requires re-login to take effect
- **Module:** RBAC
- **Preconditions:** A logged-in user with a stale token; their role's grants are then changed via `PUT /roles/{id}/permissions`.
- **Steps:** Retry a call gated by the newly-granted (or newly-revoked) permission using the **same, still-valid** token.
- **Expected:** Old permission set still applies (permissions are embedded in the JWT at login, not re-queried per request — `internal/platform/middleware/permission.go`'s own documented tradeoff). Logging in again picks up the new grants.
- **Actual:**
- **Status:** PENDING
- **Notes:** This is expected behavior, not a bug — confirm it doesn't surprise the tester.

### AUTH-07: `order.*` permissions exist and are seeded correctly
- **Module:** RBAC (Phase 25 specific)
- **Preconditions:** `wms-migrate seed` has run against the current codebase (which includes the `order` catalog module).
- **Steps:** `GET /api/v1/permissions` (as ADMIN); inspect for `order.view`, `order.create`, `order.update`, `order.approve`.
- **Expected:** All four present, `module="order"`.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-08: ADMIN has full Order Management access
- **Module:** RBAC (Phase 25 specific)
- **Steps:** Log in as ADMIN (fresh token, after AUTH-07's seed). Call `GET /orders`, `POST /orders`, `PUT /orders/{id}`, `POST /orders/{id}/create-picking-task`, `POST /orders/{id}/cancel`.
- **Expected:** None return 403.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-09: WAREHOUSE_MANAGER has full Order Management access (via `allExcept("delete")`)
- **Module:** RBAC (Phase 25 specific)
- **Preconditions:** A user seeded/assigned role `WAREHOUSE_MANAGER`, fresh login after the `order` module was seeded.
- **Steps:** Same calls as AUTH-08.
- **Expected:** None return 403 (`order` has no `delete` action, so `allExcept("delete")` grants everything).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### AUTH-10: An unauthorized role gets 403 on Order Management
- **Module:** RBAC (Phase 25 specific)
- **Preconditions:** A user with role `PICKING` (which has no explicit `order.*` grant in `RoleDefaultGrants`).
- **Steps:** Call `POST /orders` with that user's token.
- **Expected:** 403 `permission denied: order.create`.
- **Actual:**
- **Status:** PENDING
- **Notes:** Confirms Order Management is scoped to admin/planning roles, not floor staff, as designed.

---

## 2. Warehouse / Zone / Location

### WH-01: Create warehouse, zone, location hierarchy
- **Module:** Warehouse Setup
- **Preconditions:** ADMIN or WAREHOUSE_MANAGER token.
- **Steps:** `POST /warehouses` (unique code) → `POST /zones` under it → `POST /locations` under the zone.
- **Expected:** Each 201, each persisted with correct parent FK, `warehouse_id`/`zone_id` visible on the child.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### WH-02: Duplicate warehouse code rejected
- **Module:** Warehouse Setup
- **Steps:** Create two warehouses with the same `code`.
- **Expected:** Second call fails cleanly (409/422, not a raw SQL error) — `UQ_warehouses_code`.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### WH-03: Duplicate zone code within the same warehouse rejected, but reused across warehouses
- **Module:** Warehouse Setup
- **Steps:** Create zone code `A` in warehouse 1, then again in warehouse 1 (should fail), then in warehouse 2 (should succeed).
- **Expected:** Matches `UQ_zones_warehouse_code (warehouse_id, code)` — scoped uniqueness, not global.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### WH-04: Location list/search/pagination
- **Module:** Warehouse Setup
- **Steps:** `GET /locations?zone_id=...` with more locations than one page.
- **Expected:** Correct `meta.total`/`total_pages`, results scoped to the zone.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### WH-05: Inactive warehouse/zone/location status enforced downstream
- **Module:** Warehouse Setup
- **Preconditions:** A location set to `INACTIVE`.
- **Steps:** Attempt to use that location as a Picking/Putaway/Replenishment destination.
- **Expected:** Rejected with a clear "location is not active" validation error, not a silent success.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 3. Product

### PROD-01: Create product with required fields
- **Module:** Product Master
- **Steps:** `POST /products` with sku, name, category, base_uom.
- **Expected:** 201, `UQ_products_sku` enforced on a duplicate SKU retry.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PROD-02: Barcode uniqueness is optional-but-enforced
- **Module:** Product Master
- **Steps:** Create two products with no barcode (should both succeed — partial unique index `WHERE barcode IS NOT NULL`), then two with the same barcode (second should fail).
- **Expected:** Matches `UQ_products_barcode` partial index behavior exactly.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PROD-03: Category/base_uom validation
- **Module:** Product Master
- **Steps:** `POST /products` with an invalid `category` or `base_uom` value.
- **Expected:** 400/422, not a raw CHECK-constraint SQL error.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PROD-04: min_stock/max_stock feed Inventory's LOW_STOCK and Replenishment's Recommend
- **Module:** Product Master
- **Preconditions:** A product with `min_stock` set, on-hand at/below that threshold.
- **Steps:** `GET /inventory` for that product; `GET /replenishment/recommendations`.
- **Expected:** Inventory shows `status=LOW_STOCK`; Replenishment recommends it (assuming a surplus location exists — see RPL-05).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PROD-05: Update / delete product
- **Module:** Product Master
- **Steps:** `PUT /products/{id}`, then `DELETE /products/{id}` on a product with no inventory/order history.
- **Expected:** Update persists; delete succeeds cleanly.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 4. Receiving

### RCV-01: Create receipt with expected lines
- **Module:** Receiving
- **Steps:** `POST /receipts` with warehouse, staging location, one or more items (product + expected_qty).
- **Expected:** 201, status PENDING, `receipt_no` generated (`RCV-######`-style).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RCV-02: Confirm receipt adds stock via Inventory Core
- **Module:** Receiving
- **Steps:** `POST /receipts/{id}/confirm` with a fresh `idempotency_key`, received_qty per line.
- **Expected:** `inventory.quantity` increases at the staging location by exactly received_qty; an `inventory_transactions` row of type RECEIVE is written; receipt status becomes COMPLETED/PARTIALLY_RECEIVED as appropriate.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RCV-03: Duplicate confirm (same idempotency_key) is a no-op, not a double-add
- **Module:** Receiving
- **Steps:** Replay RCV-02's exact request body (same `idempotency_key`).
- **Expected:** Same 200 result, `inventory.quantity` unchanged from the first call — no second ledger row.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RCV-04: Cancel a receipt before confirm
- **Module:** Receiving
- **Steps:** `POST /receipts/{id}/cancel` on a PENDING receipt.
- **Expected:** Status CANCELLED, `cancelled_at`/`cancelled_by`/`cancel_remark` populated, no inventory effect.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RCV-05: CSV export
- **Module:** Receiving
- **Steps:** `GET /receipts/export` with filters matching a known subset.
- **Expected:** CSV download, row count matches the filtered list count.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 5. Putaway

### PUT-01: Create putaway task from a completed receipt
- **Module:** Putaway
- **Preconditions:** A COMPLETED (or PARTIALLY_RECEIVED) receipt from RCV-02.
- **Steps:** `GET /putaway/sources/{receiptId}`, then `POST /putaway` moving staged stock to a real storage location.
- **Expected:** Task created PENDING; source items limited to what's actually staged/unputaway.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PUT-02: Confirm putaway moves stock, never adds/removes net quantity
- **Module:** Putaway
- **Steps:** `POST /putaway/{id}/confirm` with a fresh idempotency key.
- **Expected:** Staging location's balance decreases, destination location's balance increases by the same amount; ledger shows a MOVE (or matching PUTAWAY-labeled) pair; total on-hand for the product is unchanged.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PUT-03: Destination must be a storage-type location
- **Module:** Putaway
- **Steps:** Attempt to putaway into a `RECEIVING`-type zone location.
- **Expected:** Rejected with a validation error (per NewPutawayView.vue's own "storage bin locations only" subtitle).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PUT-04: Cancel before confirm releases nothing (no reservation exists pre-Phase-12-equivalent)
- **Module:** Putaway
- **Steps:** `POST /putaway/{id}/cancel` on a PENDING task.
- **Expected:** CANCELLED, audit columns populated, no inventory effect.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 6. Replenishment

### RPL-01: Create task reserves at the source, no movement yet
- **Module:** Replenishment
- **Preconditions:** Source location has available stock for the product/lot.
- **Steps:** `POST /replenishment` with product/from/to/lot/qty.
- **Expected:** 201 PENDING; source location's `reserved_quantity` increases by qty; `quantity` (on-hand) at source and destination both unchanged; no ledger row yet.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RPL-02: Insufficient available stock rejected atomically
- **Module:** Replenishment
- **Steps:** Request a quantity greater than the source's available (`quantity - reserved_quantity`).
- **Expected:** 422 `insufficient available stock to reserve this quantity`; no task row created; no reservation change.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RPL-03: Confirm moves stock and releases the reservation
- **Module:** Replenishment
- **Steps:** `POST /replenishment/{id}/confirm` with a fresh idempotency key.
- **Expected:** Source `quantity` and `reserved_quantity` both decrease by qty; destination `quantity` increases by qty; one MOVE-type ledger pair written; task COMPLETED.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RPL-04: Cancel/ReportIssue releases the reservation without moving stock
- **Module:** Replenishment
- **Steps:** Create a task, then Cancel it (separately, test ReportIssue on a second task with a required remark).
- **Expected:** Both release the source reservation; neither writes a ledger row; ReportIssue requires a non-empty remark (400 without one).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RPL-05: Recommendations are advisory only, never reserve
- **Module:** Replenishment
- **Steps:** `GET /replenishment/recommendations?warehouse_id=...` for a warehouse with a known low-stock/surplus pair.
- **Expected:** Recommendation returned with a `suggested_qty` capped by both destination room and source availability; calling this endpoint does not change any `reserved_quantity`.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 7. Inventory (Core)

### INV-01: Balance list/search/filter
- **Module:** Inventory
- **Steps:** `GET /inventory?search=...&status=LOW_STOCK` etc.
- **Expected:** Filters apply server-side; pagination correct.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### INV-02: Transaction history for a balance
- **Module:** Inventory
- **Preconditions:** A balance with at least one Receive + one Move/Pick against it.
- **Steps:** `GET /inventory/{id}/transactions`.
- **Expected:** Every mutation from other modules' own workflows appears here, in order, with correct `reference_type`/`reference_id` (e.g. `PICKING` / the pick_no).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### INV-03: `reserved_quantity` never exceeds `quantity`
- **Module:** Inventory
- **Steps:** Attempt to reserve more than available via any workflow (Picking/Replenishment create) against a tightly-stocked balance.
- **Expected:** Rejected before the row would ever violate `CK_inventory_reserved_within_quantity` — confirms the guarded atomic UPDATE, not just the DB constraint, is doing the real work.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### INV-04: CSV export
- **Module:** Inventory
- **Steps:** `GET /inventory/export`.
- **Expected:** CSV matches the current filtered balance list.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 8. Picking

### PICK-01: Create task reserves per line
- **Module:** Picking
- **Steps:** `POST /picking` with order_no, warehouse, one or more items (product/location/lot/ordered_qty).
- **Expected:** 201 PENDING; each line's location gets `reserved_quantity += ordered_qty`.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PICK-02: Confirm (PICKED) removes stock and consumes the reservation
- **Module:** Picking
- **Steps:** `POST /picking/{id}/confirm` action PICKED for a line, fresh idempotency key.
- **Expected:** Location `quantity` and `reserved_quantity` both decrease by the picked amount; PICK ledger row written; line status COMPLETED/PARTIALLY_PICKED.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PICK-03: Confirm (SHORT) releases the unpicked remainder, removes nothing
- **Module:** Picking
- **Steps:** Confirm a line with action SHORT and a required remark.
- **Expected:** `reserved_quantity` decreases by the unpicked remainder; `quantity` unchanged; no ledger row for the short amount; line status SHORT (terminal).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PICK-04: Header status derives correctly from line states
- **Module:** Picking
- **Steps:** A two-line task: pick one line fully, short the other.
- **Expected:** Header status becomes SHORT_PICK (not COMPLETED) once both lines are terminal.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PICK-05: Cancel releases whatever remains reserved
- **Module:** Picking
- **Steps:** `POST /picking/{id}/cancel` on a task with at least one still-open line.
- **Expected:** CANCELLED; remaining reservation released; already-COMPLETED/SHORT lines untouched (no double-release).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### PICK-06: Cancel rejected once task is COMPLETED/SHORT_PICK
- **Module:** Picking
- **Steps:** Attempt cancel on a fully COMPLETED task.
- **Expected:** 409 `task is not in a cancellable status`.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 9. Shipping

### SHIP-01: Create shipment from a completed picking task
- **Module:** Shipping
- **Preconditions:** A COMPLETED/SHORT_PICK picking task with no existing shipment.
- **Steps:** `GET /shipping/sources`, then `POST /shipping` for that picking task.
- **Expected:** 201 READY; lines copied verbatim from the picking task's own picked quantities (no client-supplied qty); one shipment per picking task enforced (`UQ_shipments_picking_task_id`).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### SHIP-02: Dispatch → Deliver progression, no inventory mutation at any step
- **Module:** Shipping
- **Steps:** `POST /shipping/{id}/confirm` DISPATCH, then DELIVER, each with a fresh idempotency key.
- **Expected:** Status progresses READY → DISPATCHED → DELIVERED; `dispatched_at`/`delivered_at` stamped; **zero** inventory/ledger writes at any point (Picking already removed the stock).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### SHIP-03: Report Issue
- **Module:** Shipping
- **Steps:** `POST /shipping/{id}/confirm` ISSUE on a READY or DISPATCHED shipment.
- **Expected:** Status ISSUE (terminal); rejected once already DELIVERED or already ISSUE.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### SHIP-04: Cancel only before dispatch
- **Module:** Shipping
- **Steps:** Cancel a READY shipment (should succeed); attempt cancel on a DISPATCHED one (should fail).
- **Expected:** First: CANCELLED with audit columns. Second: 409.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 10. Stock Adjustment

### ADJ-01: Create adjustment (DRAFT/PENDING), no inventory effect
- **Module:** Stock Adjustment
- **Steps:** `POST /stock-adjustments` with product/location/lot, adjust_type, adjust_qty, reason.
- **Expected:** 201; `before_qty` snapshot recorded (display only); inventory `quantity` unchanged regardless of status (DRAFT or PENDING).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ADJ-02: Update only while DRAFT
- **Module:** Stock Adjustment
- **Steps:** `PUT /stock-adjustments/{id}` while DRAFT (should succeed); attempt again after Confirm/APPROVE (should fail).
- **Expected:** First succeeds; second returns a "no longer editable" error.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ADJ-03: Confirm APPROVE mutates inventory exactly once
- **Module:** Stock Adjustment
- **Steps:** `POST /stock-adjustments/{id}/confirm` action APPROVE, fresh idempotency key, for both an INCREASE and a DECREASE case.
- **Expected:** INCREASE calls AddStockTx, DECREASE calls RemoveStockTx; ledger type ADJUST with the reason recorded; replaying the same idempotency key is a no-op.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ADJ-04: Confirm REJECT performs no inventory mutation
- **Module:** Stock Adjustment
- **Steps:** `POST /stock-adjustments/{id}/confirm` action REJECT.
- **Expected:** Status updated, zero inventory/ledger effect.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ADJ-05: Overdue/aging flag
- **Module:** Stock Adjustment
- **Preconditions:** A PENDING adjustment older than `STOCK_ADJUSTMENT_PENDING_WARNING_HOURS`.
- **Steps:** `GET /stock-adjustments?overdue=true` (or equivalent per API).
- **Expected:** `is_overdue=true` on that row; nothing auto-approved/rejected.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 11. Stock Count

### CNT-01: Create count, snapshot expected quantities
- **Module:** Stock Count
- **Steps:** `POST /stock-counts` for a warehouse/zone; `POST /stock-counts/{id}/items` adding lines.
- **Expected:** Each line snapshots its expected quantity from live Inventory at add-time.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CNT-02: Record count (variance computed, not applied)
- **Module:** Stock Count
- **Steps:** `PUT /stock-counts/{id}/items/{itemId}` with a counted qty different from expected.
- **Expected:** `variance_qty` computed and stored; inventory untouched.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CNT-03: Confirm applies variance directly via AddStock/RemoveStock
- **Module:** Stock Count
- **Steps:** `POST /stock-counts/{id}/confirm` with a fresh idempotency key.
- **Expected:** Inventory quantity moves to match the counted value via the correct Add/Remove primitive per line's variance sign; status CONFIRMED.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CNT-04: Stale-balance conflict detected
- **Module:** Stock Count
- **Preconditions:** A count line added, then the live balance changes via an unrelated transaction before Confirm.
- **Steps:** Attempt Confirm.
- **Expected:** Rejected — "inventory changed since this line was counted; recount required" — not silently applied against a stale snapshot.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CNT-05: Create Adjustments instead of direct Confirm — one-shot claim
- **Module:** Stock Count
- **Steps:** `POST /stock-counts/{id}/create-adjustments` on a count with variance.
- **Expected:** One `stock_adjustments` row generated per non-zero-variance line, each with `source_stock_count_id` set, status PENDING (needs its own separate approval); count claimed to ADJUSTMENT_CREATED; a second call to either `create-adjustments` or `confirm` on the same count returns "variance already applied" rather than generating a second set of documents.
- **Actual:**
- **Status:** PENDING
- **Notes:** Confirms Phase 16's `ClaimStatus` guard — same pattern this session reused for Order's `CreatePickingTask`.

---

## 12. Notifications

### NOTIF-01: List own notifications
- **Module:** Notifications
- **Steps:** `GET /notifications` as a user who holds `stock_adjustment.approve` (reminder recipients).
- **Expected:** Only that user's own rows, most recent first.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### NOTIF-02: Mark read is idempotent and ownership-scoped
- **Module:** Notifications
- **Steps:** `POST /notifications/{id}/read` on your own notification, then again (should be a no-op success), then on another user's notification id (should 404, not leak existence).
- **Expected:** As described.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### NOTIF-03: Reminder scanner produces exactly one notification per overdue adjustment per cooldown window
- **Module:** Notifications
- **Preconditions:** A PENDING stock adjustment overdue past the warning threshold; RabbitMQ up.
- **Steps:** Wait for/trigger the scan interval; inspect `wms.stock_adjustment.pending_reminder` queue and the resulting notification row.
- **Expected:** One notification created; the same adjustment does not generate a second one until its cooldown (same as the warning-hours value) elapses again.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 13. Reports / Export

### RPT-01: Request export returns 202 + job_id
- **Module:** Reports
- **Steps:** `POST /reports/inventory-transactions/export` with filters.
- **Expected:** 202 Accepted, `data.job_id` present, job row PENDING.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RPT-02: Worker processes the job asynchronously
- **Module:** Reports
- **Steps:** Poll `GET /reports/export-jobs/{id}` until COMPLETED.
- **Expected:** Status PROCESSING → COMPLETED; `row_count` matches the filtered transaction count.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RPT-03: Download is ownership-scoped
- **Module:** Reports
- **Steps:** `GET /reports/export-jobs/{id}/download` as the requester (should succeed), then as a different user (should 404).
- **Expected:** As described — matches `ResolveDownload`'s own `RequestedBy != userID → ErrNotFound` check.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### RPT-04: Download before COMPLETED is rejected
- **Module:** Reports
- **Steps:** Attempt download while job is still PENDING/PROCESSING.
- **Expected:** Clean "not ready" error, not a broken file.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 14. Dashboard

### DASH-01: Summary aggregates match underlying modules
- **Module:** Dashboard
- **Steps:** `GET /dashboard/summary`; independently query `GET /picking?status=PENDING` count, `GET /receiving` open count, low-stock inventory count.
- **Expected:** Dashboard's own counts match the independently-queried numbers.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### DASH-02: Dashboard is read-only
- **Module:** Dashboard
- **Steps:** Confirm (via code review, already done — see PRODUCTION_READINESS.md) and spot-check live that viewing the dashboard causes zero inventory/ledger writes.
- **Expected:** No side effects.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 15. Order Management (Phase 25)

### ORD-01: Create order — header + lines persisted, zero inventory effect
- **Module:** Order Management
- **Steps:** `POST /orders` with warehouse_id, customer, external_reference, remark, 2+ items (valid product_ids, positive ordered_qty).
- **Expected:** 201; `order_no` generated (`ORD-######`); status PENDING; `fulfillment_status` PENDING; all lines persisted with resolved sku/product_name; **no** `inventory_transactions` row written; **no** `reserved_quantity` change anywhere (order never calls Reserve).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ORD-02: Update while PENDING — full replace matches request exactly
- **Module:** Order Management
- **Steps:** `PUT /orders/{id}` on the ORD-01 order — change customer, remove one line, add a different one.
- **Expected:** 200; `GET /orders/{id}` afterward shows exactly the new header fields and exactly the new line set (old removed line gone, old line ids not reused).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ORD-03: Update rejected once no longer PENDING
- **Module:** Order Management
- **Preconditions:** An order that has already had `create-picking-task` called on it (see ORD-05).
- **Steps:** `PUT /orders/{id}` on it.
- **Expected:** 409 "order can no longer be edited".
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ORD-04: Duplicate external_reference rejected cleanly, no partial rows
- **Module:** Order Management
- **Steps:** Create order A with `external_reference: "ERP-TEST-1"`. Create order B with the same value.
- **Expected:** B fails with 409 `DUPLICATE_EXTERNAL_REFERENCE`; `GET /orders?search=ERP-TEST-1` shows exactly one order (A) — confirm B's header row was rolled back, not left as an orphaned partial insert, and confirm B has zero `order_items` rows.
- **Actual:**
- **Status:** PENDING
- **Notes:** This is the transaction-rollback proof for `CreateOrder`'s `isUniqueViolation` handling.

### ORD-05: create-picking-task success — reservation via Picking only
- **Module:** Order Management
- **Preconditions:** ORD-01's order, sufficient available stock at a chosen location for every line.
- **Steps:** `POST /orders/{id}/create-picking-task` with one `LocationAssignment` per order item (location_id + optional lot_no), priority, due_date.
- **Expected:** 200; `order.status = PICKING`; `order.picking_task_id` set; `order.pick_no` populated; exactly one row in `picking_tasks` for this order (`order_no` matches); each assigned location's `reserved_quantity` increases by the line's `ordered_qty` (Picking's own Phase 12 reservation — confirm via `GET /inventory`); **zero** `inventory_transactions` rows written by this call (reservation is not a ledger event); `order.fulfillment_status` becomes PICKING.
- **Actual:**
- **Status:** PENDING
- **Notes:** This is the critical cross-package atomicity test — see PRODUCTION_READINESS.md's Phase 25 section for the transaction design being verified.

### ORD-06: Duplicate create-picking-task returns 409, no second task/reservation
- **Module:** Order Management
- **Preconditions:** ORD-05 already succeeded for this order.
- **Steps:** Call `POST /orders/{id}/create-picking-task` again on the same order (same or different body).
- **Expected:** 409 `NOT_RELEASABLE`; `picking_tasks` still has exactly one row for this order; the order's `picking_task_id` is unchanged from ORD-05; no additional reservation anywhere.
- **Actual:**
- **Status:** PENDING
- **Notes:** Verifies `ClaimForPickingTask`'s guarded UPDATE (`WHERE status='PENDING'`) — order is already PICKING, so 0 rows affected.

### ORD-07: Insufficient stock — clean rollback, order stays retryable
- **Module:** Order Management
- **Preconditions:** A fresh PENDING order whose line(s) request far more quantity than any assigned location actually has available.
- **Steps:** `POST /orders/{id}/create-picking-task`.
- **Expected:** 422 (propagated from Picking's own `ErrValidation`/`insufficient available stock...`); `order.status` remains PENDING; `order.picking_task_id` remains null; **no** row in `picking_tasks` for this order; **no** reservation created at the assigned location; **no** ledger row; the order can be retried immediately with a corrected/smaller quantity or a different location.
- **Actual:**
- **Status:** PENDING
- **Notes:** This is the proof that `ClaimForPickingTask` + `picking.CreateTx` sharing one transaction actually rolls back together — the single most important Phase 25 test.

### ORD-08: Order detail reflects Picking fulfillment progress
- **Module:** Order Management
- **Preconditions:** ORD-05's order, with its linked picking task now partially or fully confirmed (PICKED) via `POST /picking/{id}/confirm`.
- **Steps:** `GET /orders/{id}`.
- **Expected:** `fulfillment_status` reflects the picking task's live status (PICKING while in progress, PICKED once COMPLETED/SHORT_PICK with no shipment yet — see `deriveFulfillmentStatus`); confirm the same picking Confirm call's own inventory deduction (RemoveStock via `ConsumeReservationTx`) is the **only** ledger effect — Order itself writes nothing.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ORD-09: Order detail reflects Shipping fulfillment progress
- **Module:** Order Management
- **Preconditions:** ORD-08's picking task COMPLETED; a shipment created from it via `POST /shipping` and progressed via Confirm (DISPATCH → DELIVER).
- **Steps:** `GET /orders/{id}` after each shipping state change; also check `GET /orders` list row for the same order.
- **Expected:** `fulfillment_status` progresses SHIPPING → DELIVERED matching the shipment's own status exactly (both the single-order `Get` path via `picking.Service.Get`/`shipping.Service.FindByPickingTaskID`, and the bulk `GET /orders` list path via the SQL join, must agree); confirm Shipping's own Confirm calls write **zero** additional inventory/ledger rows (Shipping never mutates inventory, per its own package doc).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ORD-10: Cancel a PENDING order
- **Module:** Order Management
- **Preconditions:** A fresh PENDING order (no picking task).
- **Steps:** `POST /orders/{id}/cancel` with an optional remark.
- **Expected:** 200; status CANCELLED; `cancelled_at`/`cancelled_by`/`cancel_remark` populated correctly (remark stored only if non-empty); zero inventory effect (nothing was ever reserved by Order).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ORD-11: Cancel rejected once a picking task exists
- **Module:** Order Management
- **Preconditions:** ORD-05's order (status PICKING).
- **Steps:** `POST /orders/{id}/cancel`.
- **Expected:** 409 `NOT_CANCELLABLE` — correct path is cancelling the linked picking task directly via `POST /picking/{id}/cancel`, not this endpoint.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### ORD-12: Concurrent create-picking-task — exactly one winner
- **Module:** Order Management
- **Preconditions:** A fresh PENDING order with sufficient stock.
- **Steps:** Fire two `POST /orders/{id}/create-picking-task` requests as close to simultaneously as practical (e.g. two terminal tabs, or a small concurrent-request script) with valid, identical bodies.
- **Expected:** Exactly one returns 200 with a real `picking_task_id`; the other returns 409 `NOT_RELEASABLE`; exactly one row in `picking_tasks` for this order; exactly one reservation, not two.
- **Actual:**
- **Status:** PENDING
- **Notes:** Strongly recommended per Phase 25's own design — this is precisely the scenario `ClaimForPickingTask`'s atomic guarded UPDATE exists to make impossible to get wrong, since the arithmetic-and-check happen in one SQL statement, not a Go read-then-write.

### ORD-13: Frontend — full browser flow
- **Module:** Order Management (frontend)
- **Steps:** Log in → "Orders" appears under Outbound in the sidebar → list loads → "+ New Order" creates one with 2 items → open it, Edit while PENDING → "Create Picking Task" panel: assign a location per line, submit → detail drawer now shows linked pick_no and updates fulfillment status → attempt Cancel (should be hidden/rejected since status is now PICKING).
- **Expected:** Every step matches the API behavior above; no console errors; permission-gated buttons (`order.create`/`order.update`/`order.approve`) show/hide correctly per logged-in role.
- **Actual:**
- **Status:** PENDING
- **Notes:**

---

## 16. Cancellation / Audit (cross-module)

These consolidate the per-module cancel cases above into one audit-trail
correctness pass — run after the individual module sections, spot-checking
the shared `cancelled_at`/`cancelled_by`/`cancel_remark` pattern (Phase 18)
across every workflow that has it.

### CANCEL-01: Audit columns populated identically across all six cancellable workflows
- **Module:** Cross-cutting
- **Preconditions:** One fresh cancellable record in each of: Receiving, Putaway, Picking, Shipping, Replenishment, Order.
- **Steps:** Cancel each with a distinct remark string; `GET` each afterward.
- **Expected:** Every one of the six shows the correct `cancelled_at` (matches call time), `cancelled_by` (the actual acting user id, resolved to a display name where the API exposes one), and `cancel_remark` (exact string) — no cross-contamination between records.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CANCEL-02: Empty/omitted remark leaves cancel_remark null, not an empty string
- **Module:** Cross-cutting
- **Steps:** Cancel a record with `remark` omitted entirely (or `""`) on each of the six workflows.
- **Expected:** `cancel_remark` is `null` in every response, not `""` — matches every `applyCancel`'s own `if remark != "" { ... }` guard.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CANCEL-03: Double-cancel is rejected everywhere, never silently accepted
- **Module:** Cross-cutting
- **Steps:** Cancel each of the six once (succeeds), then immediately cancel the same record again.
- **Expected:** Every second call returns 409 (each workflow's own "not in a cancellable status"/"not cancellable" error) — none silently re-applies or overwrites the original audit data.
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CANCEL-04: Cancellation never mutates inventory except where a reservation must be released
- **Module:** Cross-cutting
- **Steps:** For each of Picking/Replenishment/Order (which can hold a reservation before cancel) vs. Receiving/Putaway/Shipping (which cannot, per their own designs), cancel a record with an open reservation (where applicable) and inspect `inventory_transactions`.
- **Expected:** Picking/Replenishment cancel releases the reservation (`reserved_quantity` decreases) but writes **zero** ledger rows; Order/Receiving/Putaway/Shipping cancel has **zero** inventory effect of any kind (Order never reserved; the other three never do at all, per their own package docs).
- **Actual:**
- **Status:** PENDING
- **Notes:**

### CANCEL-05: Cancelling mid-workflow (unsafe state) is rejected, not silently corrupting state
- **Module:** Cross-cutting
- **Steps:** Attempt cancel on: a COMPLETED/SHORT_PICK Picking task; a DISPATCHED/DELIVERED Shipment; an Order whose picking task already exists; a COMPLETED Replenishment task.
- **Expected:** All four rejected with 409, each workflow's own state left completely unchanged (no partial cancellation, no orphaned audit columns).
- **Actual:**
- **Status:** PENDING
- **Notes:** This is the same case as ORD-11/PICK-06/SHIP-04 restated as a cross-module sanity pass — should already be PASS if those individual cases are.

---

## Summary tally (fill in after the live pass)

| Module | Total cases | PASS | FAIL | PENDING |
|---|---|---|---|---|
| Auth / RBAC | 10 | | | 10 |
| Warehouse / Zone / Location | 5 | | | 5 |
| Product | 5 | | | 5 |
| Receiving | 5 | | | 5 |
| Putaway | 4 | | | 4 |
| Replenishment | 5 | | | 5 |
| Inventory | 4 | | | 4 |
| Picking | 6 | | | 6 |
| Shipping | 4 | | | 4 |
| Stock Adjustment | 5 | | | 5 |
| Stock Count | 5 | | | 5 |
| Notifications | 3 | | | 3 |
| Reports / Export | 4 | | | 4 |
| Dashboard | 2 | | | 2 |
| Order Management | 13 | | | 13 |
| Cancellation / Audit | 5 | | | 5 |
| **Total** | **85** | **0** | **0** | **85** |
