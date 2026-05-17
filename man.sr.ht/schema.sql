CREATE TYPE user_type AS ENUM (
	'PENDING',
	'USER',
	'ADMIN',
	'SUSPENDED'
);

CREATE TABLE "user" (
	id serial PRIMARY KEY,
	username character varying(256) NOT NULL UNIQUE,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	oauth_token character varying(256),
	oauth_token_expires timestamp without time zone,
	oauth_token_scopes character varying,
	email character varying(256) NOT NULL UNIQUE,
	user_type user_type NOT NULL,
	url character varying(256),
	location character varying(256),
	bio character varying(4096),
	suspension_notice character varying(4096)
);

CREATE INDEX ix_user_username ON "user" USING btree (username);

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

CREATE TYPE visibility AS ENUM (
	'PUBLIC',
	'PRIVATE',
	'UNLISTED'
);

CREATE TABLE wiki (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	name character varying(256) NOT NULL,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	visibility visibility NOT NULL,
	-- git.sr.ht repository name:
	repo_name text NOT NULL,
	-- git repository reference:
	repo_ref text NOT NULL
);

CREATE TABLE root_wiki (
	id integer NOT NULL PRIMARY KEY REFERENCES wiki(id)
);
