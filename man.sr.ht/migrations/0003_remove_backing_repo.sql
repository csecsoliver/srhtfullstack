-- +brant Up
ALTER TABLE wiki
	ADD COLUMN repo_name text,
	ADD COLUMN repo_ref text;

UPDATE wiki SET repo_name =
	(SELECT name FROM backing_repo WHERE id = wiki.repo_id);
UPDATE wiki SET repo_ref =
	(SELECT ref FROM backing_repo WHERE id = wiki.repo_id);

ALTER TABLE wiki
	DROP COLUMN repo_id,
	ALTER COLUMN repo_name SET NOT NULL,
	ALTER COLUMN repo_ref SET NOT NULL;

DROP TABLE backing_repo;

-- +brant Down
-- NOTE: This migration is lossy
CREATE TABLE backing_repo (
	id serial PRIMARY KEY,
	new boolean NOT NULL,
	name character varying(256) NOT NULL,
	ref character varying(1024) NOT NULL,
	commit_sha character varying(256),
	commit_author character varying(256),
	commit_email character varying(256),
	commit_time character varying(256),
	commit_message character varying(1024),
	tree_sha character varying(256),
	resource_id integer
);

ALTER TABLE wiki
	ADD COLUMN repo_id integer NOT NULL REFERENCES backing_repo(id),
	DROP COLUMN repo_name,
	DROP COLUMN repo_ref;
