-- +brant Up
ALTER TABLE sshkey
	ALTER COLUMN fingerprint DROP NOT NULL;


-- +brant Down
ALTER TABLE sshkey
	ALTER COLUMN fingerprint SET NOT NULL;
