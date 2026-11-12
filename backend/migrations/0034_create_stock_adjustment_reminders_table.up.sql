-- Phase 21 — one row per stock adjustment that has ever had a reminder
-- published for it. Purely additive (does not touch stock_adjustments
-- itself or any migration 0001-0033) and deliberately a separate table
-- rather than columns on stock_adjustments: this is reminder-delivery
-- state, not adjustment data, and keeping it separate means the
-- reminder scanner's own claim writes never contend with the
-- adjustment's own row lock during a concurrent Confirm.
--
-- The scanner's "claim" (see internal/stockadjustment's
-- ClaimAndListOverdueForReminder) is: INSERT if no row exists yet,
-- else UPDATE only when last_reminded_at is old enough to be outside
-- the cooldown window — one row per adjustment_id, enforced by the
-- PRIMARY KEY itself acting as the uniqueness guard (no separate UNIQUE
-- constraint needed).
CREATE TABLE stock_adjustment_reminders (
    stock_adjustment_id BIGINT    NOT NULL,
    last_reminded_at    DATETIME2 NOT NULL,
    reminder_count      INT       NOT NULL CONSTRAINT DF_stock_adjustment_reminders_count DEFAULT (0),
    CONSTRAINT PK_stock_adjustment_reminders PRIMARY KEY (stock_adjustment_id),
    CONSTRAINT FK_stock_adjustment_reminders_adjustment
        FOREIGN KEY (stock_adjustment_id) REFERENCES stock_adjustments(id) ON DELETE CASCADE
);
