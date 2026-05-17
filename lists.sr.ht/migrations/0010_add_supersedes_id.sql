-- +brant Up

ALTER TABLE patchset
ADD COLUMN supersedes_id integer REFERENCES patchset(id) ON DELETE SET NULL;

-- +brant statementbegin
CREATE OR REPLACE FUNCTION backfill_supersedes_id() RETURNS VOID AS $$
DECLARE
  patch record;
BEGIN
    FOR patch IN SELECT id, superseded_by_id FROM patchset WHERE superseded_by_id IS NOT NULL
        LOOP
	    UPDATE patchset SET supersedes_id=patch.id where id=patch.superseded_by_id;
        END LOOP;
END
$$ LANGUAGE plpgsql;
-- +brant statementend

SELECT backfill_supersedes_id();

-- +brant Down
ALTER TABLE patchset
DROP COLUMN supersedes_id;
