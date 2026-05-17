-- +brant Up
ALTER TABLE "email"
RENAME COLUMN envelope TO raw_message;

-- +brant Down
ALTER TABLE "email"
RENAME COLUMN raw_message TO envelope;
