CREATE TYPE auth_method AS ENUM (
	'OAUTH_LEGACY',
	'OAUTH2',
	'COOKIE',
	'INTERNAL',
	'WEBHOOK'
);

CREATE TYPE visibility AS ENUM (
	'PUBLIC',
	'PRIVATE',
	'UNLISTED'
);

CREATE TYPE webhook_event AS ENUM (
	'PASTE_CREATED',
	'PASTE_UPDATED',
	'PASTE_DELETED'
);

CREATE TYPE user_type AS ENUM (
	'PENDING',
	'USER',
	'ADMIN',
	'SUSPENDED'
);

CREATE TABLE "user" (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	username character varying(256) NOT NULL UNIQUE,
	email character varying(256) NOT NULL UNIQUE,
	user_type user_type NOT NULL,
	url character varying(256),
	location character varying(256),
	bio character varying(4096),
	suspension_notice character varying(4096)
);

CREATE INDEX ix_user_username ON "user" USING btree (username);

CREATE TABLE blob (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	sha character varying(40) NOT NULL,
	contents character varying NOT NULL
);

ALTER TABLE ONLY public.blob ADD CONSTRAINT sha_unique UNIQUE (sha);

CREATE INDEX ix_blob_sha ON blob USING btree (sha);

CREATE TABLE paste (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	sha character varying(40),
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	visibility visibility NOT NULL
);

CREATE INDEX ix_paste_sha ON paste USING btree (sha);

CREATE TABLE paste_file (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	filename character varying(1024),
	blob_id integer NOT NULL REFERENCES blob(id),
	paste_id integer NOT NULL REFERENCES paste(id) ON DELETE CASCADE
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

CREATE INDEX gql_user_wh_sub_token_hash_idx ON gql_user_wh_sub USING btree (token_hash);

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
