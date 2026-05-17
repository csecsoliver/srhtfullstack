-- +brant Up
ALTER TABLE sshkey
	ADD COLUMN key character varying(4096),
	ADD COLUMN key_type character varying(256),
	ADD COLUMN fingerprint_sha256 character varying(256),
	ADD CONSTRAINT sshkey_fingerprint_sha256_key UNIQUE (fingerprint_sha256);


-- +brant Down
ALTER TABLE sshkey
	DROP CONSTRAINT sshkey_fingerprint_sha256_key,
	DROP COLUMN key,
	DROP COLUMN key_type,
	DROP COLUMN fingerprint_sha256;
