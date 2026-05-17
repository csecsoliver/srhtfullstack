-- +brant Up
ALTER TABLE invoice
ADD COLUMN reverse_vat boolean NOT NULL DEFAULT 'f';

ALTER TABLE invoice
ALTER COLUMN reverse_vat DROP DEFAULT;

-- +brant Down
ALTER TABLE invoice
DROP COLUMN reverse_vat;
