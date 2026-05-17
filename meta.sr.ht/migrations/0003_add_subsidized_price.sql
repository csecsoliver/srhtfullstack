-- +brant Up
ALTER TABLE product
ADD COLUMN subsidized boolean NOT NULL DEFAULT 'f';

-- +brant Down
ALTER TABLE product
DROP COLUMN subsidized;
