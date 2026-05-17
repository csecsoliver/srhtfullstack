-- +brant Up
ALTER TABLE sshkey
	RENAME COLUMN key TO raw;

-- +brant Down
ALTER TABLE sshkey
	RENAME COLUMN raw TO key;
