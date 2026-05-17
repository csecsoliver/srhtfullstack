-- +brant Up
DROP TABLE list_webhook_delivery;
DROP TABLE list_webhook_subscription;
DROP TABLE user_webhook_delivery;
DROP TABLE user_webhook_subscription;
DROP TABLE oauthtoken;

ALTER TABLE "user"
DROP COLUMN oauth_token,
DROP COLUMN oauth_token_expires,
DROP COLUMN oauth_token_scopes;

-- +brant Down
CREATE TABLE oauthtoken (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	updated timestamp without time zone NOT NULL,
	expires timestamp without time zone NOT NULL,
	token_hash character varying(128) NOT NULL,
	token_partial character varying(8) NOT NULL,
	scopes character varying(512) NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE
);

CREATE TABLE list_webhook_subscription (
	id serial PRIMARY KEY,
	created timestamp without time zone NOT NULL,
	url character varying(2048) NOT NULL,
	events character varying NOT NULL,
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	token_id integer REFERENCES oauthtoken(id) ON DELETE CASCADE,
	list_id integer REFERENCES list(id) ON DELETE CASCADE
);

CREATE TABLE list_webhook_delivery (
	id serial PRIMARY KEY,
	uuid uuid NOT NULL,
	created timestamp without time zone NOT NULL,
	event character varying(256) NOT NULL,
	url character varying(2048) NOT NULL,
	payload character varying(65536) NOT NULL,
	payload_headers character varying(16384) NOT NULL,
	response character varying(65536),
	response_status integer NOT NULL,
	response_headers character varying(16384),
	subscription_id integer NOT NULL
		REFERENCES list_webhook_subscription(id) ON DELETE CASCADE
);

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
	payload character varying(65536) NOT NULL,
	payload_headers character varying(16384) NOT NULL,
	response character varying(65536),
	response_status integer NOT NULL,
	response_headers character varying(16384),
	subscription_id integer NOT NULL REFERENCES user_webhook_subscription(id) ON DELETE CASCADE
);

ALTER TABLE "user"
ADD COLUMN oauth_token character varying(256),
ADD COLUMN oauth_token_expires timestamp without time zone,
ADD COLUMN oauth_token_scopes character varying;
