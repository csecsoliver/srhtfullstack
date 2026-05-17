-- +brant Up
CREATE TYPE ticket_status AS ENUM (
	'REPORTED',
	'CONFIRMED',
	'IN_PROGRESS',
	'PENDING',
	'RESOLVED'
);

CREATE TYPE ticket_resolution AS ENUM (
	'UNRESOLVED',
	'FIXED',
	'IMPLEMENTED',
	'WONT_FIX',
	'BY_DESIGN',
	'INVALID',
	'DUPLICATE',
	'NOT_OUR_BUG',
	'CLOSED'
);

CREATE TYPE authenticity AS ENUM (
	'AUTHENTIC',
	'UNAUTHENTICATED',
	'TAMPERED'
);

ALTER TABLE ticket
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN status TYPE ticket_status USING (CASE
		WHEN status = 0 THEN 'REPORTED'
		WHEN status = 1 THEN 'CONFIRMED'
		WHEN status = 2 THEN 'IN_PROGRESS'
		WHEN status = 4 THEN 'PENDING'
		WHEN status = 8 THEN 'RESOLVED'
	END)::ticket_status,
	ALTER COLUMN status SET DEFAULT 'REPORTED',
	ALTER COLUMN resolution DROP DEFAULT,
	ALTER COLUMN resolution TYPE ticket_resolution USING (CASE
		WHEN resolution = 0   THEN 'UNRESOLVED'
		WHEN resolution = 1   THEN 'FIXED'
		WHEN resolution = 2   THEN 'IMPLEMENTED'
		WHEN resolution = 4   THEN 'WONT_FIX'
		WHEN resolution = 8   THEN 'BY_DESIGN'
		WHEN resolution = 16  THEN 'INVALID'
		WHEN resolution = 32  THEN 'DUPLICATE'
		WHEN resolution = 64  THEN 'NOT_OUR_BUG'
		WHEN resolution = 128 THEN 'CLOSED'
	END)::ticket_resolution,
	ALTER COLUMN resolution SET DEFAULT 'UNRESOLVED',
	ALTER COLUMN authenticity DROP DEFAULT,
	ALTER COLUMN authenticity TYPE authenticity USING (CASE
		WHEN authenticity = 0 THEN 'AUTHENTIC'
		WHEN authenticity = 1 THEN 'UNAUTHENTICATED'
		WHEN authenticity = 2 THEN 'TAMPERED'
	END)::authenticity,
	ALTER COLUMN authenticity SET DEFAULT 'AUTHENTIC';

ALTER TABLE ticket_comment
	ALTER COLUMN authenticity DROP DEFAULT,
	ALTER COLUMN authenticity TYPE authenticity USING (CASE
		WHEN authenticity = 0 THEN 'AUTHENTIC'
		WHEN authenticity = 1 THEN 'UNAUTHENTICATED'
		WHEN authenticity = 2 THEN 'TAMPERED'
	END)::authenticity,
	ALTER COLUMN authenticity SET DEFAULT 'AUTHENTIC';

ALTER TABLE event
	ALTER COLUMN old_status TYPE ticket_status USING (CASE
		WHEN old_status = 0 THEN 'REPORTED'
		WHEN old_status = 1 THEN 'CONFIRMED'
		WHEN old_status = 2 THEN 'IN_PROGRESS'
		WHEN old_status = 4 THEN 'PENDING'
		WHEN old_status = 8 THEN 'RESOLVED'
	END)::ticket_status,
	ALTER COLUMN new_status TYPE ticket_status USING (CASE
		WHEN new_status = 0 THEN 'REPORTED'
		WHEN new_status = 1 THEN 'CONFIRMED'
		WHEN new_status = 2 THEN 'IN_PROGRESS'
		WHEN new_status = 4 THEN 'PENDING'
		WHEN new_status = 8 THEN 'RESOLVED'
	END)::ticket_status,
	ALTER COLUMN old_resolution TYPE ticket_resolution USING (CASE
		WHEN old_resolution = 0   THEN 'UNRESOLVED'
		WHEN old_resolution = 1   THEN 'FIXED'
		WHEN old_resolution = 2   THEN 'IMPLEMENTED'
		WHEN old_resolution = 4   THEN 'WONT_FIX'
		WHEN old_resolution = 8   THEN 'BY_DESIGN'
		WHEN old_resolution = 16  THEN 'INVALID'
		WHEN old_resolution = 32  THEN 'DUPLICATE'
		WHEN old_resolution = 64  THEN 'NOT_OUR_BUG'
		WHEN old_resolution = 128 THEN 'CLOSED'
	END)::ticket_resolution,
	ALTER COLUMN new_resolution TYPE ticket_resolution USING (CASE
		WHEN new_resolution = 0   THEN 'UNRESOLVED'
		WHEN new_resolution = 1   THEN 'FIXED'
		WHEN new_resolution = 2   THEN 'IMPLEMENTED'
		WHEN new_resolution = 4   THEN 'WONT_FIX'
		WHEN new_resolution = 8   THEN 'BY_DESIGN'
		WHEN new_resolution = 16  THEN 'INVALID'
		WHEN new_resolution = 32  THEN 'DUPLICATE'
		WHEN new_resolution = 64  THEN 'NOT_OUR_BUG'
		WHEN new_resolution = 128 THEN 'CLOSED'
	END)::ticket_resolution;

-- +brant Down
ALTER TABLE ticket
	ALTER COLUMN status DROP DEFAULT,
	ALTER COLUMN status TYPE integer USING (CASE
		WHEN status = 'REPORTED'    THEN 0
		WHEN status = 'CONFIRMED'   THEN 1
		WHEN status = 'IN_PROGRESS' THEN 2
		WHEN status = 'PENDING'     THEN 4
		WHEN status = 'RESOLVED'    THEN 8
	END),
	ALTER COLUMN status SET DEFAULT 0,
	ALTER COLUMN resolution DROP DEFAULT,
	ALTER COLUMN resolution TYPE integer USING (CASE
		WHEN resolution = 'UNRESOLVED'  THEN 0
		WHEN resolution = 'FIXED'       THEN 1
		WHEN resolution = 'IMPLEMENTED' THEN 2
		WHEN resolution = 'WONT_FIX'    THEN 4
		WHEN resolution = 'BY_DESIGN'   THEN 8
		WHEN resolution = 'INVALID'     THEN 16
		WHEN resolution = 'DUPLICATE'   THEN 32
		WHEN resolution = 'NOT_OUR_BUG' THEN 64
		WHEN resolution = 'CLOSED'      THEN 128
	END),
	ALTER COLUMN resolution SET DEFAULT 0,
	ALTER COLUMN authenticity DROP DEFAULT,
	ALTER COLUMN authenticity TYPE integer USING (CASE
		WHEN authenticity = 'AUTHENTIC'       THEN 0
		WHEN authenticity = 'UNAUTHENTICATED' THEN 1
		WHEN authenticity = 'TAMPERED'        THEN 2
	END),
	ALTER COLUMN authenticity SET DEFAULT 0;

ALTER TABLE ticket_comment
	ALTER COLUMN authenticity DROP DEFAULT,
	ALTER COLUMN authenticity TYPE integer USING (CASE
		WHEN authenticity = 'AUTHENTIC'       THEN 0
		WHEN authenticity = 'UNAUTHENTICATED' THEN 1
		WHEN authenticity = 'TAMPERED'        THEN 2
	END),
	ALTER COLUMN authenticity SET DEFAULT 0;

ALTER TABLE event
	ALTER COLUMN old_status TYPE integer USING (CASE
		WHEN old_status = 'REPORTED'    THEN 0
		WHEN old_status = 'CONFIRMED'   THEN 1
		WHEN old_status = 'IN_PROGRESS' THEN 2
		WHEN old_status = 'PENDING'     THEN 4
		WHEN old_status = 'RESOLVED'    THEN 8
	END),
	ALTER COLUMN new_status TYPE integer USING (CASE
		WHEN new_status = 'REPORTED'    THEN 0
		WHEN new_status = 'CONFIRMED'   THEN 1
		WHEN new_status = 'IN_PROGRESS' THEN 2
		WHEN new_status = 'PENDING'     THEN 4
		WHEN new_status = 'RESOLVED'    THEN 8
	END),
	ALTER COLUMN old_resolution TYPE integer USING (CASE
		WHEN old_resolution = 'UNRESOLVED'  THEN 0
		WHEN old_resolution = 'FIXED'       THEN 1
		WHEN old_resolution = 'IMPLEMENTED' THEN 2
		WHEN old_resolution = 'WONT_FIX'    THEN 4
		WHEN old_resolution = 'BY_DESIGN'   THEN 8
		WHEN old_resolution = 'INVALID'     THEN 16
		WHEN old_resolution = 'DUPLICATE'   THEN 32
		WHEN old_resolution = 'NOT_OUR_BUG' THEN 64
		WHEN old_resolution = 'CLOSED'      THEN 128
	END),
	ALTER COLUMN new_resolution TYPE integer USING (CASE
		WHEN new_resolution = 'UNRESOLVED'  THEN 0
		WHEN new_resolution = 'FIXED'       THEN 1
		WHEN new_resolution = 'IMPLEMENTED' THEN 2
		WHEN new_resolution = 'WONT_FIX'    THEN 4
		WHEN new_resolution = 'BY_DESIGN'   THEN 8
		WHEN new_resolution = 'INVALID'     THEN 16
		WHEN new_resolution = 'DUPLICATE'   THEN 32
		WHEN new_resolution = 'NOT_OUR_BUG' THEN 64
		WHEN new_resolution = 'CLOSED'      THEN 128
	END);

DROP TYPE ticket_status;
DROP TYPE ticket_resolution;
DROP TYPE authenticity;
