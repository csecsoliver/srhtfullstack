-- +brant Up
ALTER TABLE email
ADD COLUMN patch_trailers text[] NOT NULL DEFAULT '{}';

CREATE INDEX email_patch_trailer_key ON email USING GIN (patch_trailers);

-- +brant Down
DROP INDEX email_patch_trailer_key;

ALTER TABLE email
DROP COLUMN patch_trailers;
