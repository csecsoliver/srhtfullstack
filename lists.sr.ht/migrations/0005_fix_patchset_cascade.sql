-- +brant Up
ALTER TABLE email
	DROP CONSTRAINT email_patchset_id_fkey,
	ADD CONSTRAINT email_patchset_id_fkey
	FOREIGN KEY (patchset_id)
	REFERENCES patchset(id) ON DELETE SET NULL;

-- +brant Down
ALTER TABLE email
	DROP CONSTRAINT email_patchset_id_fkey,
	ADD CONSTRAINT email_patchset_id_fkey
	FOREIGN KEY (patchset_id)
	REFERENCES patchset(id) ON DELETE CASCADE;
