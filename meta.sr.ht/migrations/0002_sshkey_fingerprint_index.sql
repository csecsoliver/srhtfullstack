-- +brant Up
DROP INDEX sshkey_md5_idx;

-- +brant Down
CREATE INDEX sshkey_md5_idx ON sshkey USING btree (md5((key)::text));
