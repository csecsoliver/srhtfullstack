-- +brant Up
ALTER TABLE "user"
ADD COLUMN pronouns text;

-- +brant Down
ALTER TABLE "user"
DROP COLUMN pronouns;
