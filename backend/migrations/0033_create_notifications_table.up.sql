-- Phase 21 — minimal notification inbox. type/reference_type/
-- reference_id are generic (not hardcoded to stock adjustments) so a
-- future notification type could reuse this same table, but Phase 21
-- itself only ever writes rows with type = 'STOCK_ADJUSTMENT_REMINDER'
-- and reference_type = 'STOCK_ADJUSTMENT'.
--
-- message_id is the originating RabbitMQ message's own message_id and
-- is the idempotency guard for Phase 21's reminder consumer: a reminder
-- fans out to every eligible approver as its own row sharing the same
-- message_id, so UQ_notifications_message_user is per (message,
-- recipient) — a redelivered message safely re-attempts every
-- recipient's insert, each rejected individually as an already-
-- processed duplicate. NOT NULL because every current writer (the one
-- consumer that exists) always has a real message to attribute a row
-- to; a future notification type created outside any queued message
-- would need its own migration to widen this, not a speculative NULL
-- allowance added here (SQL Server's own NULL-uniqueness semantics in a
-- composite unique index make "sometimes NULL" a real footgun, not a
-- free convenience).
CREATE TABLE notifications (
    id             BIGINT IDENTITY(1,1) NOT NULL,
    type           VARCHAR(50)   NOT NULL,
    user_id        BIGINT        NOT NULL,
    reference_type VARCHAR(50)   NULL,
    reference_id   BIGINT        NULL,
    title          NVARCHAR(200) NOT NULL,
    message        NVARCHAR(500) NOT NULL,
    is_read        BIT           NOT NULL CONSTRAINT DF_notifications_is_read DEFAULT (0),
    message_id     VARCHAR(36)   NOT NULL,
    created_at     DATETIME2     NOT NULL CONSTRAINT DF_notifications_created_at DEFAULT (SYSUTCDATETIME()),
    read_at        DATETIME2     NULL,
    CONSTRAINT PK_notifications PRIMARY KEY (id),
    CONSTRAINT FK_notifications_user FOREIGN KEY (user_id) REFERENCES users(id),
    CONSTRAINT UQ_notifications_message_user UNIQUE (message_id, user_id)
);

CREATE INDEX IX_notifications_user_id_is_read ON notifications(user_id, is_read);
