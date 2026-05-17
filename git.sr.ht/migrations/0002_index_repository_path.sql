-- +brant Up
ALTER TABLE repository
	ALTER COLUMN path SET NOT NULL,
	ADD CONSTRAINT repository_path_key UNIQUE (path);

-- +brant Down
ALTER TABLE repository
	DROP CONSTRAINT repository_path_key,
	ALTER COLUMN path DROP NOT NULL;
