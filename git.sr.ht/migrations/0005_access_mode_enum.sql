-- +brant Up
CREATE TYPE access_mode AS ENUM (
	'RO',
	'RW'
);
ALTER TABLE "access" ADD COLUMN mode2 access_mode;
UPDATE "access" SET mode2 = UPPER(mode)::access_mode;
ALTER TABLE "access" DROP COLUMN mode;
ALTER TABLE "access" RENAME COLUMN mode2 TO mode;
ALTER TABLE "access" ALTER COLUMN mode SET NOT NULL;

-- +brant Down
ALTER TABLE "access" ADD COLUMN mode2 character varying;
UPDATE "access" SET mode2 = LOWER(mode::character varying);
ALTER TABLE "access" DROP COLUMN mode;
ALTER TABLE "access" RENAME COLUMN mode2 TO mode;
ALTER TABLE "access" ALTER COLUMN mode SET NOT NULL;
DROP TYPE access_mode;
