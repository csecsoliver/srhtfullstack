-- +brant Up
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
	user_id integer REFERENCES "user"(id) ON DELETE CASCADE,
	token_hash character varying(128) NOT NULL,
	token_partial character varying(8) NOT NULL,
	scopes character varying(512) NOT NULL
);

ALTER TABLE "user"
ADD COLUMN oauth_token character varying(256),
ADD COLUMN oauth_token_expires timestamp without time zone,
ADD COLUMN oauth_token_scopes character varying DEFAULT 'profile'::character varying;
