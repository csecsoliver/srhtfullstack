-- +brant Up
CREATE TYPE owner_id_project_name AS (
	owner_id integer,
	project_name text
);

-- +brant Down
DROP TYPE owner_id_project_name;
