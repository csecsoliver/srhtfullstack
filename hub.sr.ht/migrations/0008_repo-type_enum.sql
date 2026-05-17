-- +brant Up
CREATE TYPE repo_type AS ENUM (
	'GIT',
	'HG'
);
ALTER TABLE "source_repo" ADD COLUMN repo_type2 repo_type;
UPDATE "source_repo" SET repo_type2 = UPPER(repo_type)::repo_type;
ALTER TABLE "source_repo" DROP COLUMN repo_type;
ALTER TABLE "source_repo" RENAME COLUMN repo_type2 TO repo_type;
ALTER TABLE "source_repo" ALTER COLUMN repo_type SET NOT NULL;

-- +brant Down
ALTER TABLE "source_repo" ADD COLUMN repo_type2 character varying;
UPDATE "source_repo" SET repo_type2 = LOWER(repo_type::character varying);
ALTER TABLE "source_repo" DROP COLUMN repo_type;
ALTER TABLE "source_repo" RENAME COLUMN repo_type2 TO repo_type;
ALTER TABLE "source_repo" ALTER COLUMN repo_type SET NOT NULL;
DROP TYPE repo_type;
