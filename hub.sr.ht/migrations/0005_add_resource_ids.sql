-- +brant Up
CREATE EXTENSION pgcrypto;

-- +brant StatementBegin
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
-- +brant StatementEnd

ALTER TABLE project
ADD COLUMN rid uuid NOT NULL UNIQUE DEFAULT gen_uuidv7();

ALTER TABLE mailing_list ADD COLUMN remote_rid text;
ALTER TABLE source_repo ADD COLUMN remote_rid text;
ALTER TABLE tracker ADD COLUMN remote_rid text;

-- +brant Down

ALTER TABLE tracker DROP COLUMN remote_rid;
ALTER TABLE source_repo DROP COLUMN remote_rid;
ALTER TABLE mailing_list DROP COLUMN remote_rid;

ALTER TABLE project DROP COLUMN rid;

DROP FUNCTION gen_uuidv7;
DROP EXTENSION pgcrypto;
