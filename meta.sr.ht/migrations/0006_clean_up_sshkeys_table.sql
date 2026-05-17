-- +brant Up
ALTER TABLE sshkey DROP COLUMN b64_key;
ALTER TABLE sshkey DROP COLUMN key_type;

-- +brant Down
ALTER TABLE sshkey ADD COLUMN b64_key character varying(4096);
ALTER TABLE sshkey ADD COLUMN key_type character varying(256);
