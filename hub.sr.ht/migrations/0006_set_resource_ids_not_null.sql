-- +brant Up
ALTER TABLE mailing_list
ALTER COLUMN remote_rid SET NOT NULL;

ALTER TABLE source_repo
ALTER COLUMN remote_rid SET NOT NULL;

ALTER TABLE tracker
ALTER COLUMN remote_rid SET NOT NULL;

-- +brant Down
ALTER TABLE tracker
ALTER COLUMN remote_rid DROP NOT NULL;

ALTER TABLE source_repo
ALTER COLUMN remote_rid DROP NOT NULL;

ALTER TABLE mailing_list
ALTER COLUMN remote_rid DROP NOT NULL;
