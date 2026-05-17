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

CREATE TYPE webhook_event AS ENUM (
	'PROFILE_UPDATE',
	'PGP_KEY_ADDED',
	'PGP_KEY_REMOVED',
	'SSH_KEY_ADDED',
	'SSH_KEY_REMOVED'
);

CREATE TYPE user_type AS ENUM (
	'PENDING',
	'USER',
	'ADMIN',
	'SUSPENDED'
);

CREATE TYPE subscription_status AS ENUM (
        'PENDING',
        'SETTLEMENT',
        'ACTIVE',
        'INACTIVE'
);

CREATE TYPE payment_status AS ENUM (
	-- User does not pay for their account
	'UNPAID',
	-- User is paid and their payment is current
	'CURRENT',
	-- User's payment has lapsed
	'DELINQUENT',
	-- User's paid services are subsidized
	'SUBSIDIZED',
	-- User receives paid services for free
	'FREE'
);

CREATE TYPE payment_outcome AS ENUM (
	'PENDING',
	'CANCELLED',
	'PROCESSING',
	'FAILED',
	'SUCCEEDED'
);

CREATE TYPE payment_interval AS ENUM (
	'MONTHLY',
	'ANNUALLY'
);

CREATE TYPE payment_currency AS ENUM (
	'USD',
	'EUR'
);

CREATE TABLE "user" (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,

	user_type user_type NOT NULL,
	username character varying(256) NOT NULL UNIQUE,
	password character varying(256) NOT NULL,

	email character varying(256) NOT NULL UNIQUE,
	pgp_key_id integer,

	url character varying(256),
	location character varying(256),
	bio character varying(4096),
	avatar text,
	pronouns text,

	welcome_emails integer DEFAULT 0 NOT NULL,
	suspension_notice character varying(4096),

	payment_status payment_status DEFAULT 'UNPAID' NOT NULL,
	payment_due timestamp without time zone,
	next_invoice_no integer NOT NULL DEFAULT 1,
	default_payment_method_id integer,
	billing_address_id integer
);

-- XXX BEGIN Temporary table while transitioning billing system from US => NL
CREATE TABLE user_payment_processor (
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	payment_processor_id text NOT NULL,
	currency payment_currency NOT NULL,
	UNIQUE (user_id, currency)
);
-- XXX END

-- Product available for purchase
CREATE TABLE product (
	id serial PRIMARY KEY,
	name text NOT NULL,
	retired boolean NOT NULL DEFAULT 'f',
	subsidized boolean NOT NULL DEFAULT 'f'
);

-- Price point for a product
CREATE TABLE product_price (
	id serial PRIMARY KEY,
	-- Applicable product ID
	product_id integer NOT NULL REFERENCES product(id),
	-- Applicable currency
	currency payment_currency NOT NULL,
	-- Price in the currency's smallest denomination (e.g. cents USD)
	amount integer NOT NULL,
	UNIQUE (product_id, currency)
);

-- User billing details
CREATE TABLE subscription (
	id serial PRIMARY KEY,

	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	status subscription_status NOT NULL DEFAULT 'ACTIVE',

	product_id integer NOT NULL REFERENCES product(id),
	currency payment_currency NOT NULL,
	interval payment_interval NOT NULL,
	autorenew boolean NOT NULL,

	-- Details about the most recent payment attempt
	payment_intent text,
	payment_outcome payment_outcome NOT NULL,
	payment_error text
);

CREATE TABLE billing_address (
	id serial PRIMARY KEY,
	user_id integer REFERENCES "user"(id) ON DELETE SET NULL,
	country text NOT NULL,
	full_name text,
	business_name text,
	address_1 text,
	address_2 text,
	city text,
	region text,
	postcode text,
	vat text
);

ALTER TABLE "user"
	ADD CONSTRAINT user_billing_address_id_fkey
	FOREIGN KEY (billing_address_id) REFERENCES billing_address(id) ON DELETE SET NULL;

CREATE TABLE payment_method (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	currency payment_currency NOT NULL,

	-- Expiration, if applicable to this payment method type
	expires timestamp without time zone,

	-- User-friendly name (e.g. "Visa ending in 1234")
	name text NOT NULL,

	-- Opaque processor-specific ID
	processor_id text UNIQUE NOT NULL
);

ALTER TABLE "user"
	ADD CONSTRAINT payment_method_user_id_fkey
	FOREIGN KEY (default_payment_method_id) REFERENCES payment_method(id) ON DELETE SET NULL;

CREATE TABLE invoice (
	id serial PRIMARY KEY,
	invoice_no integer NOT NULL,
	issued timestamp without time zone NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE SET NULL,
	billing_address_id integer REFERENCES billing_address(id),
	uuid uuid NOT NULL,

	product_id integer NOT NULL REFERENCES "product"(id),
	interval payment_interval NOT NULL,
	service_start timestamp without time zone NOT NULL,
	service_end timestamp without time zone NOT NULL,

	currency payment_currency NOT NULL,
	subtotal integer NOT NULL,
	-- Percentage, NULL if tax was not applicable for this invoice
	tax_rate double precision,
	tax_charged integer NOT NULL,
        reverse_vat boolean NOT NULL,
	total integer NOT NULL,

	-- Payment ID, opaque string meaningful to third-party payment processor
	--
	-- Is left NULL for payments which pre-date recording this information
	-- in the database
	payment_id text UNIQUE,

	UNIQUE (user_id, invoice_no)
);

-- State for operations requiring email confirmation
CREATE TABLE user_registration (
	issued timestamp without time zone NOT NULL DEFAULT (now() at time zone 'utc'),
	user_id integer NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
	token text NOT NULL
);

CREATE TABLE user_email_change (
	issued timestamp without time zone NOT NULL DEFAULT (now() at time zone 'utc'),
	user_id integer NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
	new_email text NOT NULL,
	token text NOT NULL
);

CREATE TABLE user_password_change (
	issued timestamp without time zone NOT NULL DEFAULT (now() at time zone 'utc'),
	user_id integer NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
	token text NOT NULL
);

-- etc
CREATE TABLE audit_log_entry (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	ip_address character varying(50) NOT NULL,
	event_type character varying(256) NOT NULL,
	details character varying(512)
);

CREATE TABLE pgpkey (
	id serial PRIMARY KEY,
	rid uuid NOT NULL UNIQUE DEFAULT gen_uuidv7(),
	created timestamp without time zone,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	key character varying(32768) NOT NULL,
	fingerprint bytea NOT NULL,
	expiration timestamp without time zone
);

ALTER TABLE pgpkey
	ADD CONSTRAINT ix_pgpkey_fingerprint UNIQUE (fingerprint);

ALTER TABLE "user"
	ADD CONSTRAINT user_pgp_key_id_fkey
	FOREIGN KEY (pgp_key_id) REFERENCES pgpkey(id) ON DELETE SET NULL;

CREATE TABLE sshkey (
	id serial PRIMARY KEY,
	rid uuid NOT NULL UNIQUE DEFAULT gen_uuidv7(),
	created timestamp without time zone,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	raw character varying(4096) NOT NULL,
	key character varying(4096) NOT NULL,
	key_type character varying(256) NOT NULL,
	fingerprint_sha256 character varying(512) NOT NULL,
	fingerprint character varying(512),
	comment character varying(256),
	last_used timestamp without time zone
);

ALTER TABLE sshkey
	ADD CONSTRAINT ix_sshkey_fingerprint UNIQUE (fingerprint),
	ADD CONSTRAINT sshkey_fingerprint_sha256_key UNIQUE (fingerprint_sha256);

CREATE TABLE user_auth_factor (
	id serial PRIMARY KEY,
	user_id integer NOT NULL UNIQUE REFERENCES "user"(id) ON DELETE CASCADE,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	factor_type character varying NOT NULL,
	secret bytea,
	extra json
);

CREATE TABLE user_notes (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	note character varying
);

CREATE TABLE reserved_usernames (
    username varchar NOT NULL
);

CREATE INDEX reserved_usernames_ix ON reserved_usernames(username);

-- OAuth 2.0
CREATE TABLE oauth2_client (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	owner_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	client_uuid uuid NOT NULL,
	client_secret_hash character varying(128) NOT NULL,
	client_secret_partial character varying(8) NOT NULL,
	redirect_url character varying,
	client_name character varying(256) NOT NULL,
	client_description character varying,
	client_url character varying,
	revoked boolean DEFAULT false NOT NULL
);

CREATE TABLE oauth2_grant (
	id serial PRIMARY KEY,
	issued timestamp without time zone NOT NULL,
	expires timestamp without time zone NOT NULL,
	comment character varying,
	token_hash character varying(128) NOT NULL,
	user_id integer NOT NULL REFERENCES "user"(id) ON DELETE CASCADE,
	client_id integer REFERENCES oauth2_client(id) ON DELETE CASCADE,
	grants character varying,
	refresh_token_hash character varying(128)
);

-- GraphQL webhooks
CREATE TABLE gql_profile_wh_sub (
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
	CONSTRAINT gql_profile_wh_sub_auth_method_check
		CHECK ((auth_method = ANY(ARRAY['OAUTH2'::auth_method, 'INTERNAL'::auth_method]))),
	CONSTRAINT gql_profile_wh_sub_check
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (token_hash IS NOT NULL))),
	CONSTRAINT gql_profile_wh_sub_check1
		CHECK (((auth_method = 'OAUTH2'::auth_method) = (expires IS NOT NULL))),
	CONSTRAINT gql_profile_wh_sub_check2
		CHECK (((auth_method = 'INTERNAL'::auth_method) = (node_id IS NOT NULL))),
	CONSTRAINT gql_profile_wh_sub_events_check
		CHECK ((array_length(events, 1) > 0))
);

CREATE INDEX gql_profile_wh_sub_token_hash_idx ON gql_profile_wh_sub USING btree (token_hash);

CREATE TABLE gql_profile_wh_delivery (
	id serial PRIMARY KEY,
	uuid uuid NOT NULL,
	date timestamp without time zone NOT NULL,
	event webhook_event NOT NULL,
	subscription_id integer NOT NULL
		REFERENCES gql_profile_wh_sub(id) ON DELETE CASCADE,
	request_body character varying NOT NULL,
	response_body character varying,
	response_headers character varying,
	response_status integer
);

-- Legacy OAuth (TODO: Remove these)
CREATE TABLE oauthclient (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	client_name character varying(256) NOT NULL,
	client_id character varying(16) NOT NULL,
	client_secret_hash character varying(128) NOT NULL,
	client_secret_partial character varying(8) NOT NULL,
	redirect_uri character varying(256),
	preauthorized boolean DEFAULT false NOT NULL
);

CREATE TABLE oauthtoken (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	expires timestamp without time zone NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	client_id integer REFERENCES oauthclient(id) ON DELETE CASCADE,
	token_hash character varying(128) NOT NULL,
	token_partial character varying(8) NOT NULL,
	scopes character varying(512) NOT NULL,
	comment character varying(128)
);

-- Legacy webhooks (TODO: Remove these)
CREATE TABLE user_webhook_subscription (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	url character varying(2048) NOT NULL,
	events character varying NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	token_id integer REFERENCES oauthtoken(id) ON DELETE CASCADE
);

CREATE TABLE user_webhook_delivery (
	id serial PRIMARY KEY,
	uuid uuid NOT NULL,
	created timestamp without time zone NOT NULL,
	event character varying(256) NOT NULL,
	url character varying(2048) NOT NULL,
	payload character varying(16384) NOT NULL,
	payload_headers character varying(16384) NOT NULL,
	response character varying(16384),
	response_status integer NOT NULL,
	response_headers character varying(16384),
	subscription_id integer REFERENCES user_webhook_subscription(id) ON DELETE CASCADE
);

CREATE TABLE webhook_subscription (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	url character varying(2048) NOT NULL,
	events character varying NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	client_id integer REFERENCES oauthclient(id) ON DELETE CASCADE
);
