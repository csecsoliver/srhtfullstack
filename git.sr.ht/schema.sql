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

CREATE TYPE auth_method AS ENUM (
	'OAUTH_LEGACY',
	'OAUTH2',
	'COOKIE',
	'INTERNAL',
	'WEBHOOK'
);

CREATE TYPE clone_status AS ENUM (
	'NONE',
	'IN_PROGRESS',
	'COMPLETE',
	'ERROR'
);

CREATE TYPE visibility AS ENUM (
	'PUBLIC',
	'PRIVATE',
	'UNLISTED'
);

CREATE TYPE webhook_event AS ENUM (
	'REPO_CREATED',
	'REPO_UPDATE',
	'REPO_DELETED',
	'GIT_PRE_RECEIVE',
	'GIT_POST_RECEIVE'
);

CREATE TYPE user_type AS ENUM (
	'PENDING',
	'USER',
	'ADMIN',
	'SUSPENDED'
);

CREATE TYPE access_mode AS ENUM (
	'RO',
	'RW'
);

CREATE TYPE owner_repo_name AS (
	owner text,
	repo_name text
);

CREATE TYPE owner_id_repo_name AS (
	owner_id integer,
	repo_name text
);

CREATE TABLE "user" (
	id serial PRIMARY KEY,
	username character varying(256) UNIQUE,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	email character varying(256) NOT NULL UNIQUE,
	user_type user_type NOT NULL,
	url character varying(256),
	location character varying(256),
	bio character varying(4096),
	suspension_notice character varying(4096)
);

CREATE INDEX ix_user_username ON "user" USING btree (username);

CREATE TABLE repository (
	id serial PRIMARY KEY,
	rid uuid UNIQUE NOT NULL DEFAULT gen_uuidv7(),
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	name character varying(256) NOT NULL,
	description character varying(1024),
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	path character varying(1024) NOT NULL UNIQUE,
	visibility visibility NOT NULL,
	readme character varying,
	clone_status clone_status NOT NULL,
	clone_error character varying,
	CONSTRAINT repository_check
		CHECK (((clone_status = 'ERROR'::clone_status) <> (clone_error IS NULL))),
	CONSTRAINT uq_repo_owner_id_name UNIQUE (owner_id, name)
);

CREATE TABLE access (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	repo_id integer NOT NULL REFERENCES repository(id) ON DELETE CASCADE,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	mode access_mode NOT NULL,
	CONSTRAINT uq_access_user_id_repo_id UNIQUE (user_id, repo_id)
);

CREATE TABLE artifacts (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	repo_id integer NOT NULL REFERENCES repository(id) ON DELETE CASCADE,
	commit character varying NOT NULL,
	filename character varying NOT NULL,
	checksum character varying NOT NULL,
	size integer NOT NULL,
	CONSTRAINT repo_artifact_filename_unique UNIQUE (repo_id, filename)
);

CREATE TABLE redirect (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	name character varying(256) NOT NULL,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	path character varying(1024),
	new_repo_id integer NOT NULL REFERENCES repository(id) ON DELETE CASCADE
);

-- GraphQL webhooks
CREATE TABLE gql_user_wh_sub (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	events webhook_event[] NOT NULL,
	url character varying NOT NULL,
	query character varying NOT NULL,
	auth_method auth_method NOT NULL,
	token_hash character varying(128),
	grants character varying,
	client_id uuid,
	expires timestamp without time zone,
	node_id character varying,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	CONSTRAINT gql_user_wh_sub_auth_method_check
		CHECK ((auth_method = ANY (ARRAY['OAUTH2'::auth_method, 'INTERNAL'::auth_method]))),
	CONSTRAINT gql_user_wh_sub_check
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (token_hash IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_check1
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (expires IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_check2
		CHECK (((auth_method = 'INTERNAL'::auth_method) = (node_id IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_events_check
		CHECK ((array_length(events, 1) > 0))
);
CREATE INDEX gql_user_wh_sub_user_id_idx ON gql_user_wh_sub USING btree(user_id);

CREATE INDEX gql_user_wh_sub_token_hash_idx
	ON gql_user_wh_sub
	USING btree (token_hash);

CREATE TABLE gql_user_wh_delivery (
	id serial PRIMARY KEY,
	uuid uuid NOT NULL,
	date timestamp without time zone NOT NULL,
	event webhook_event NOT NULL,
	subscription_id integer NOT NULL
		REFERENCES gql_user_wh_sub(id) ON DELETE CASCADE,
	request_body character varying NOT NULL,
	response_body character varying,
	response_headers character varying,
	response_status integer
);

CREATE TABLE gql_git_wh_sub (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	events webhook_event[] NOT NULL,
	url character varying NOT NULL,
	query character varying NOT NULL,
	sync boolean NOT NULL,
	auth_method auth_method NOT NULL,
	token_hash character varying(128),
	grants character varying,
	client_id uuid,
	expires timestamp without time zone,
	node_id character varying,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	repo_id integer NOT NULL REFERENCES repository(id) ON DELETE CASCADE,
	CONSTRAINT gql_git_wh_sub_auth_method_check
		CHECK ((auth_method = ANY (ARRAY['OAUTH2'::auth_method, 'INTERNAL'::auth_method]))),
	CONSTRAINT gql_git_wh_sub_check
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (token_hash IS NOT NULL))),
	CONSTRAINT gql_git_wh_sub_check1
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (expires IS NOT NULL))),
	CONSTRAINT gql_git_wh_sub_check2
		CHECK (((auth_method = 'INTERNAL'::auth_method) = (node_id IS NOT NULL))),
	CONSTRAINT gql_git_wh_sub_events_check
		CHECK ((array_length(events, 1) > 0))
);
CREATE INDEX gql_git_wh_sub_repo_id_idx ON gql_git_wh_sub USING btree(repo_id);
CREATE INDEX gql_git_wh_sub_user_id_idx ON gql_git_wh_sub USING btree(user_id);

CREATE INDEX gql_git_wh_sub_token_hash_idx
	ON gql_git_wh_sub
	USING btree (token_hash);

CREATE TABLE gql_git_wh_delivery (
	id serial PRIMARY KEY,
	uuid uuid NOT NULL,
	date timestamp without time zone NOT NULL,
	event webhook_event NOT NULL,
	subscription_id integer NOT NULL
		REFERENCES gql_git_wh_sub(id) ON DELETE CASCADE,
	request_body character varying NOT NULL,
	response_body character varying,
	response_headers character varying,
	response_status integer
);
