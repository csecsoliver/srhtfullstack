CREATE TYPE auth_method AS ENUM (
	'OAUTH_LEGACY',
	'OAUTH2',
	'COOKIE',
	'INTERNAL',
	'WEBHOOK'
);

CREATE TYPE webhook_event AS ENUM (
	'JOB_CREATED',
	'JOB_UPDATED'
);

CREATE TYPE visibility AS ENUM (
	'PUBLIC',
	'UNLISTED',
	'PRIVATE'
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
	email character varying(256) NOT NULL,
	user_type user_type NOT NULL,
	url character varying(256),
	location character varying(256),
	bio character varying(4096),
	suspension_notice character varying(4096)
);

CREATE TABLE secret (
	id serial PRIMARY KEY,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	uuid uuid NOT NULL,
	name character varying(512),
	from_user_id integer REFERENCES "user"(id) ON DELETE SET NULL,
	-- Key secrets:
	secret_type character varying NOT NULL,
	secret bytea NOT NULL,
	-- File secrets:
	path character varying(512),
	mode integer,
	CONSTRAINT secret_user_id_uuid_unique UNIQUE (user_id, uuid)
);

CREATE INDEX ix_user_username ON "user" USING btree (username);

CREATE TABLE job_group (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	note character varying(4096)
);

CREATE TABLE job (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	manifest character varying(16384) NOT NULL,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	job_group_id integer REFERENCES job_group(id) ON DELETE SET NULL,
	note character varying(4096),
	tags character varying,
	runner character varying,
	status character varying NOT NULL,
	secrets boolean DEFAULT true NOT NULL,
	image character varying(128),
	visibility visibility NOT NULL
);

CREATE INDEX ix_job_owner_id ON job USING btree (owner_id);

CREATE INDEX ix_job_job_group_id ON job USING btree (job_group_id);

CREATE TABLE artifact (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	job_id integer NOT NULL REFERENCES job(id) ON DELETE CASCADE,
	name character varying NOT NULL,
	path character varying NOT NULL,
	url character varying NOT NULL,
	size integer NOT NULL
);

CREATE TABLE task (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	name character varying(256) NOT NULL,
	status character varying NOT NULL,
	job_id integer NOT NULL REFERENCES job(id) ON DELETE CASCADE
);

CREATE INDEX ix_task_job_id ON task USING btree (job_id);

CREATE TABLE trigger (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	details character varying(4096) NOT NULL,
	condition character varying NOT NULL,
	trigger_type character varying NOT NULL,
	job_id integer REFERENCES job(id) ON DELETE CASCADE,
	job_group_id integer REFERENCES job_group(id) ON DELETE CASCADE
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
		CHECK ((auth_method = ANY (ARRAY['OAUTH2'::auth_method, 'INTERNAL'::public.auth_method]))),
	CONSTRAINT gql_user_wh_sub_check
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (token_hash IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_check1
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (expires IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_check2
		CHECK (((auth_method = 'INTERNAL'::auth_method) = (node_id IS NOT NULL))),
	CONSTRAINT gql_user_wh_sub_events_check
		CHECK ((array_length(events, 1) > 0))
);

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
