-- +brant Up
-- +brant no transaction
CREATE INDEX CONCURRENTLY gql_tracker_wh_sub_tracker_id_idx ON gql_tracker_wh_sub USING btree(tracker_id);
CREATE INDEX CONCURRENTLY gql_tracker_wh_sub_user_id_idx ON gql_tracker_wh_sub USING btree(user_id);
CREATE INDEX CONCURRENTLY gql_user_wh_sub_user_id_idx ON gql_user_wh_sub USING btree(user_id);
CREATE INDEX CONCURRENTLY gql_ticket_wh_sub_user_id_idx ON gql_ticket_wh_sub USING btree (user_id);
CREATE INDEX CONCURRENTLY gql_ticket_wh_sub_ticket_id_idx ON gql_ticket_wh_sub USING btree (ticket_id);
CREATE INDEX CONCURRENTLY gql_ticket_wh_sub_tracker_id_idx ON gql_ticket_wh_sub USING btree (tracker_id);

-- +brant Down
DROP INDEX gql_tracker_wh_sub_tracker_id_idx;
DROP INDEX gql_tracker_wh_sub_user_id_idx;
DROP INDEX gql_user_wh_sub_user_id_idx;
DROP INDEX gql_ticket_wh_sub_user_id_idx;
DROP INDEX gql_ticket_wh_sub_ticket_id_idx;
DROP INDEX gql_ticket_wh_sub_tracker_id_idx;
