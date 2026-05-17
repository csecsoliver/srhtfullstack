-- +brant Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

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

ALTER TABLE repository
ADD COLUMN rid uuid NOT NULL UNIQUE DEFAULT gen_uuidv7();

-- +brant Down
ALTER TABLE repository DROP COLUMN rid;

DROP FUNCTION gen_uuidv7;
DROP EXTENSION pgcrypto;
