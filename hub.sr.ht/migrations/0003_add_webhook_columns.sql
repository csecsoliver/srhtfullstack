-- +brant Up
ALTER TABLE source_repo
ADD COLUMN webhook_id INTEGER,
ADD COLUMN webhook_version INTEGER;

ALTER TABLE tracker
ADD COLUMN webhook_id INTEGER,
ADD COLUMN webhook_version INTEGER;

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

CREATE TABLE tracker_ticket_webhook (
	id serial PRIMARY KEY,
	-- Local tracker ID
	tracker_id integer NOT NULL REFERENCES tracker(id),
	-- Remote ticket ID
	ticket_id integer NOT NULL,
	-- Remote webhook ID
	webhook_id integer NOT NULL,
	webhook_version integer NOT NULL
);

-- +brant Down
ALTER TABLE source_repo
DROP COLUMN webhook_id,
DROP COLUMN webhook_version;

ALTER TABLE tracker
DROP COLUMN webhook_id,
DROP COLUMN webhook_version;

DROP TABLE user_webhooks;
DROP TABLE tracker_ticket_webhook;
