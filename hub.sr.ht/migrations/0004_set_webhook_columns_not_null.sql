-- +brant Up
ALTER TABLE tracker
ALTER COLUMN webhook_id SET NOT NULL,
ALTER COLUMN webhook_version SET NOT NULL;

-- +brant Down
ALTER TABLE tracker
ALTER COLUMN webhook_id DROP NOT NULL,
ALTER COLUMN webhook_version DROP NOT NULL;
