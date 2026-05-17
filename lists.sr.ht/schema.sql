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

CREATE TYPE list_webhook_event AS ENUM (
	'LIST_UPDATED',
	'LIST_DELETED',
	'EMAIL_RECEIVED',
	'PATCHSET_RECEIVED'
);

CREATE TYPE visibility AS ENUM (
	'PUBLIC',
	'UNLISTED',
	'PRIVATE'
);

CREATE TYPE webhook_event AS ENUM (
	'LIST_CREATED',
	'LIST_UPDATED',
	'LIST_DELETED',
	'EMAIL_RECEIVED',
	'PATCHSET_RECEIVED'
);

CREATE TYPE user_type AS ENUM (
	'PENDING',
	'USER',
	'ADMIN',
	'SUSPENDED'
);

CREATE TABLE "user" (
	id serial PRIMARY KEY,
	username character varying(256) UNIQUE,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	session character varying(128),
	email character varying(256) UNIQUE NOT NULL,
	user_type user_type NOT NULL,
	url character varying(256),
	location character varying(256),
	bio character varying(4096),
	suspension_notice character varying(4096),
	copy_self boolean DEFAULT false NOT NULL
);

CREATE TABLE list (
	id serial PRIMARY KEY,
	rid uuid UNIQUE NOT NULL DEFAULT gen_uuidv7(),
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	name character varying(128) NOT NULL,
	description character varying(2048),
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	default_access integer DEFAULT 7 NOT NULL,
	mirror_id integer REFERENCES list(id) ON DELETE SET NULL,
	permit_mimetypes character varying DEFAULT 'text/*,application/pgp-signature,application/pgp-keys'::character varying NOT NULL,
	reject_mimetypes character varying DEFAULT 'text/html'::character varying NOT NULL,
	import_in_progress boolean DEFAULT false NOT NULL,
	visibility visibility NOT NULL,
	last_activity timestamp without time zone,
	UNIQUE (owner_id, name)
);

CREATE TABLE access (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	email character varying,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	list_id integer NOT NULL REFERENCES list(id) ON DELETE CASCADE,
	permissions integer DEFAULT 7 NOT NULL,
	UNIQUE (list_id, email),
	UNIQUE (list_id, user_id)
);

CREATE TABLE email (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,

	subject character varying(2048) NOT NULL,
	message_id character varying(2048) NOT NULL,
	message_date timestamp without time zone,
	raw_message bytea NOT NULL,
	headers json NOT NULL,
	body character varying NOT NULL,

	list_id integer NOT NULL REFERENCES list(id) ON DELETE CASCADE,
	parent_id integer REFERENCES email(id) ON DELETE SET NULL,
	thread_id integer REFERENCES email(id) ON DELETE SET NULL,
	sender_id integer REFERENCES "user"(id) ON DELETE SET NULL,

	is_patch boolean NOT NULL,
	is_request_pull boolean NOT NULL,
	nreplies integer DEFAULT 0,
	nparticipants integer DEFAULT 1,
	in_reply_to character varying(2048),

	patchset_id integer,
	patch_index integer,
	patch_count integer,
	patch_version integer,
	patch_prefix character varying,
	patch_subject character varying,
	superseded_by_id integer REFERENCES email(id) ON DELETE SET NULL,
	patch_trailers text[] NOT NULL DEFAULT '{}',

	UNIQUE (list_id, message_id)
);

CREATE INDEX email_patch_trailer_key ON email USING GIN (patch_trailers);

-- TODO: Remove me
CREATE TABLE mirror (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	configure_attempts integer DEFAULT 0 NOT NULL,
	configured boolean DEFAULT false NOT NULL,
	mailer_sender character varying,
	list_subscribe character varying,
	list_unsubscribe character varying,
	list_post character varying
);

CREATE TABLE patchset (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	subject character varying(2048) NOT NULL,
	prefix character varying,
	version integer NOT NULL,
	status character varying DEFAULT 'proposed'::character varying NOT NULL,
	list_id integer NOT NULL REFERENCES list(id) ON DELETE CASCADE,
	cover_letter_id integer REFERENCES email(id) ON DELETE SET NULL,
	superseded_by_id integer REFERENCES patchset(id) ON DELETE SET NULL,
	supersedes_id integer REFERENCES patchset(id) ON DELETE SET NULL,
	submitter character varying,
	message_id character varying,
	reply_to character varying
);

ALTER TABLE email
	ADD CONSTRAINT email_patchset_id_fkey
	FOREIGN KEY (patchset_id)
	REFERENCES patchset(id) ON DELETE SET NULL;

CREATE TABLE patchset_tool (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	patchset_id integer REFERENCES patchset(id) ON DELETE CASCADE,
	icon character varying DEFAULT 'pending'::character varying NOT NULL,
	details character varying NOT NULL,
	key character varying(128) NOT NULL
);

CREATE INDEX patchset_tool_key_idx ON patchset_tool USING btree (key);

CREATE TABLE subscription (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	email character varying(512),
	list_id integer NOT NULL REFERENCES list(id) ON DELETE CASCADE,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	CONSTRAINT subscription_email_xor_user_id
		CHECK ((((email IS NULL) OR (user_id IS NULL)) AND ((email IS NOT NULL) OR (user_id IS NOT NULL)))),
	UNIQUE (list_id, email),
	UNIQUE (list_id, user_id)
);

CREATE TABLE subscription_request (
	id serial PRIMARY KEY,
	email CHARACTER VARYING(512) NOT NULL,
	confirmation_hash CHARACTER VARYING(128) NOT NULL,
	list_id integer NOT NULL references "list"(id) ON DELETE CASCADE,
	CONSTRAINT sr_list_id_email_unique UNIQUE (list_id, email)
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
	CONSTRAINT gql_user_wh_sub_auth_method_check CHECK ((auth_method = ANY (ARRAY['OAUTH2'::auth_method, 'INTERNAL'::auth_method]))),
	CONSTRAINT gql_user_wh_sub_check CHECK (((auth_method = 'OAUTH2'::auth_method) = (token_hash IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_check1 CHECK (((auth_method = 'OAUTH2'::auth_method) = (expires IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_check2 CHECK (((auth_method = 'INTERNAL'::auth_method) = (node_id IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_events_check CHECK ((array_length(events, 1) > 0))
);

CREATE INDEX gql_user_wh_sub_user_id_idx ON gql_user_wh_sub USING btree (user_id);
CREATE INDEX gql_user_wh_sub_token_hash_idx ON gql_user_wh_sub USING btree (token_hash);

CREATE TABLE gql_user_wh_delivery (
	id serial PRIMARY KEY,
	uuid uuid NOT NULL,
	date timestamp without time zone NOT NULL,
	event webhook_event NOT NULL,
	subscription_id integer NOT NULL REFERENCES gql_user_wh_sub(id) ON DELETE CASCADE,
	request_body character varying NOT NULL,
	response_body character varying,
	response_headers character varying,
	response_status integer
);

CREATE TABLE gql_list_wh_sub (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	events list_webhook_event[] NOT NULL,
	url character varying NOT NULL,
	query character varying NOT NULL,
	auth_method auth_method NOT NULL,
	token_hash character varying(128),
	grants character varying,
	client_id uuid,
	expires timestamp without time zone,
	node_id character varying,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	list_id integer REFERENCES list(id) ON DELETE CASCADE,
	CONSTRAINT gql_list_wh_sub_auth_method_check
		CHECK ((auth_method = ANY (ARRAY['OAUTH2'::auth_method, 'INTERNAL'::auth_method]))),
	CONSTRAINT gql_list_wh_sub_check
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (token_hash IS NOT NULL))),
	CONSTRAINT gql_list_wh_sub_check1
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (expires IS NOT NULL))),
	CONSTRAINT gql_list_wh_sub_check2
		CHECK (((auth_method = 'INTERNAL'::auth_method) = (node_id IS NOT NULL))),
	CONSTRAINT gql_list_wh_sub_events_check
		CHECK ((array_length(events, 1) > 0))
);

CREATE INDEX gql_list_wh_sub_user_id_idx ON gql_list_wh_sub USING btree (user_id);
CREATE INDEX gql_list_wh_sub_list_id_idx ON gql_list_wh_sub USING btree (list_id);
CREATE INDEX gql_list_wh_sub_token_hash_idx ON gql_list_wh_sub USING btree (token_hash);

CREATE TABLE gql_list_wh_delivery (
	id serial PRIMARY KEY,
	uuid uuid NOT NULL,
	date timestamp without time zone NOT NULL,
	event list_webhook_event NOT NULL,
	subscription_id integer NOT NULL REFERENCES gql_list_wh_sub(id) ON DELETE CASCADE,
	request_body character varying NOT NULL,
	response_body character varying,
	response_headers character varying,
	response_status integer
);
