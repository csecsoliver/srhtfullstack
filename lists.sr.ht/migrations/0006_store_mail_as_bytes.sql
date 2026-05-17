-- +brant Up
ALTER TABLE "email"
ALTER COLUMN envelope TYPE bytea USING convert_to(envelope, 'utf-8');

-- +brant Down
ALTER TABLE "email"
ALTER COLUMN envelope TYPE character varying USING convert_from(envelope, 'utf-8');
