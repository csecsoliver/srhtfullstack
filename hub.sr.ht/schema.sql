CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Note: PostgreSQL 18 includes native support for UUID v7
-- Replace this when we roll it out
CREATE FUNCTION gen_uuidv7() RETURNS uuid
    AS $$
        SELECT (
		lpad(to_hex(floor(extract(epoch FROM clock_timestamp()) * 1000)::bigint), 12, '0')
		|| '7'
		|| substring(encode(gen_random_bytes(2), 'hex') from 2)
		|| '8'
		|| substring(encode(gen_random_bytes(2), 'hex') from 2)
		|| encode(gen_random_bytes(6), 'hex')
	)::uuid;
    $$ LANGUAGE SQL;

CREATE TYPE visibility AS ENUM (
	'PUBLIC',
	'PRIVATE',
	'UNLISTED'
);

CREATE TYPE user_type AS ENUM (
	'PENDING',
	'USER',
	'ADMIN',
	'SUSPENDED'
);

CREATE TYPE owner_id_project_name AS (
	owner_id integer,
	project_name text
);

CREATE TYPE repo_type AS ENUM (
	'GIT',
	'HG'
);

CREATE TABLE "user" (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	username character varying(256),
	email character varying(256) NOT NULL,
	user_type user_type NOT NULL,
	url character varying(256),
	location character varying(256),
	bio character varying(4096),
	suspension_notice character varying(4096),
	oauth_token character varying(256),
	oauth_token_expires timestamp without time zone,
	oauth_token_scopes character varying
);

CREATE UNIQUE INDEX ix_user_username ON "user" USING btree (username);

CREATE TABLE user_webhooks (
	id serial PRIMARY KEY,
	user_id integer NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
	git_webhook_id integer,
	git_webhook_version integer,
	hg_webhook_id integer,
	hg_webhook_version integer,
	lists_webhook_id integer,
	lists_webhook_version integer,
	todo_webhook_id integer,
	todo_webhook_version integer
);

CREATE TABLE project (
	id serial PRIMARY KEY,
	rid uuid UNIQUE NOT NULL DEFAULT gen_uuidv7(),
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	name character varying(128) NOT NULL,
	description character varying(512) NOT NULL,
	website character varying,
	visibility visibility DEFAULT 'UNLISTED'::visibility NOT NULL,
	checklist_complete boolean DEFAULT false NOT NULL,
	summary_repo_id integer,
	tags character varying(16)[] DEFAULT '{}'::character varying[] NOT NULL,
	UNIQUE (owner_id, name)
);

CREATE TABLE features (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	project_id integer NOT NULL REFERENCES project(id) ON DELETE CASCADE,
	summary character varying NOT NULL
);

CREATE TABLE mailing_list (
	id serial PRIMARY KEY,
	remote_id integer NOT NULL,
	remote_rid text NOT NULL,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	project_id integer NOT NULL REFERENCES project(id) ON DELETE CASCADE,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	name character varying(128) NOT NULL,
	description character varying,
	visibility visibility DEFAULT 'UNLISTED'::visibility NOT NULL,
	-- Remote webhook ID
	webhook_id integer NOT NULL,
	webhook_version integer NOT NULL
);

CREATE TABLE source_repo (
	id serial PRIMARY KEY,
	remote_id integer NOT NULL,
	remote_rid text NOT NULL,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	project_id integer NOT NULL REFERENCES project(id) ON DELETE CASCADE,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	name character varying(128) NOT NULL,
	description character varying,
	repo_type repo_type NOT NULL,
	visibility visibility DEFAULT 'UNLISTED'::visibility NOT NULL,
	-- Remote webhook ID
	webhook_id integer,
	webhook_version integer,
	CONSTRAINT project_source_repo_unique UNIQUE (project_id, remote_id, repo_type)
);

ALTER TABLE project
	ADD CONSTRAINT project_summary_repo_id_fkey FOREIGN KEY (summary_repo_id) REFERENCES source_repo(id) ON DELETE CASCADE;

CREATE TABLE tracker (
	id serial PRIMARY KEY,
	remote_id integer NOT NULL,
	remote_rid text NOT NULL,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	project_id integer NOT NULL REFERENCES project(id) ON DELETE CASCADE,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	name character varying(128) NOT NULL,
	description character varying,
	visibility visibility DEFAULT 'UNLISTED'::visibility NOT NULL,
	-- Remote webhook ID
	webhook_id integer NOT NULL,
	webhook_version integer NOT NULL
);

CREATE TABLE event (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	event_type character varying NOT NULL,
	source_repo_id integer REFERENCES source_repo(id) ON DELETE CASCADE,
	mailing_list_id integer REFERENCES mailing_list(id) ON DELETE CASCADE,
	tracker_id integer REFERENCES tracker(id) ON DELETE CASCADE,
	external_source character varying,
	external_summary character varying,
	external_details character varying,
	external_summary_plain character varying,
	external_details_plain character varying,
	external_url character varying
);

CREATE TABLE event_project_association (
	event_id integer NOT NULL REFERENCES event(id) ON DELETE CASCADE,
	project_id integer NOT NULL REFERENCES project(id) ON DELETE CASCADE
);

CREATE TABLE redirect (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	name character varying(256) NOT NULL,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	new_project_id integer NOT NULL REFERENCES project(id) ON DELETE CASCADE
);
