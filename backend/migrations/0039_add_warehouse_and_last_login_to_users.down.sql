ALTER TABLE users DROP COLUMN last_login_at;

DROP INDEX IX_users_warehouse_id ON users;

ALTER TABLE users DROP CONSTRAINT FK_users_warehouse;

ALTER TABLE users DROP COLUMN warehouse_id;
