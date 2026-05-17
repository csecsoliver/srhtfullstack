-- +brant Up
CREATE TYPE visibility AS ENUM (
	'PUBLIC',
	'PRIVATE',
	'UNLISTED'
);

ALTER TABLE paste
	ALTER COLUMN visibility DROP DEFAULT,
	ALTER COLUMN visibility TYPE visibility USING upper(visibility)::visibility;

-- +brant Down
ALTER TABLE paste
	ALTER COLUMN visibility	TYPE varchar USING lower(visibility::varchar),
	ALTER COLUMN visibility SET DEFAULT 'unlisted'::character varying;

DROP TYPE visibility;
