-- +brant Up
ALTER TABLE "user"
ADD COLUMN avatar text;

-- +brant Down
ALTER TABLE "user" DROP COLUMN avatar;
