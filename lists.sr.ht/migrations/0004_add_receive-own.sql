-- +brant Up
ALTER TABLE "user" ADD COLUMN copy_self boolean DEFAULT false;
UPDATE "user" SET copy_self=FALSE;
ALTER TABLE "user" ALTER COLUMN copy_self SET NOT NULL;

-- +brant Down
ALTER TABLE "user" DROP COLUMN copy_self;
