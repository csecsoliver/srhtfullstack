-- +brant Up

-- +brant statementbegin
CREATE OR REPLACE FUNCTION column_to_assoc_table() RETURNS VOID AS $$
DECLARE
  event record;
BEGIN
    CREATE TABLE event_project_association (
        event_id integer NOT NULL REFERENCES event(id) ON DELETE CASCADE,
        project_id integer NOT NULL REFERENCES project(id) ON DELETE CASCADE
    );
    FOR event IN SELECT id, project_id FROM event
        LOOP
            INSERT INTO event_project_association(event_id, project_id) VALUES(event.id, event.project_id);
        END LOOP;
    ALTER TABLE event DROP COLUMN project_id;
END
$$ LANGUAGE plpgsql;
-- +brant statementend

SELECT column_to_assoc_table();

-- +brant Down

-- +brant statementbegin
CREATE OR REPLACE FUNCTION assoc_table_to_column() RETURNS VOID AS $$
DECLARE
  assoc record;
BEGIN
    ALTER TABLE event ADD COLUMN project_id integer REFERENCES project(id) ON DELETE CASCADE;
    FOR assoc IN SELECT event_id, project_id FROM event_project_association
        LOOP
            UPDATE event SET project_id = assoc.project_id WHERE id = assoc.event_id;
        END LOOP;
    ALTER TABLE event ALTER COLUMN project_id SET NOT NULL;
    DROP TABLE event_project_association;
END
$$ LANGUAGE plpgsql;
-- +brant statementend

SELECT assoc_table_to_column();
