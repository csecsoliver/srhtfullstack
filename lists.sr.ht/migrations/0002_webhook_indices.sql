-- +brant Up
-- +brant no transaction
CREATE INDEX CONCURRENTLY gql_list_wh_sub_list_id_idx ON gql_list_wh_sub USING btree(list_id);
CREATE INDEX CONCURRENTLY gql_list_wh_sub_user_id_idx ON gql_list_wh_sub USING btree(user_id);
CREATE INDEX CONCURRENTLY gql_user_wh_sub_user_id_idx ON gql_user_wh_sub USING btree(user_id);

-- +brant Down
DROP INDEX gql_list_wh_sub_list_id_idx;
DROP INDEX gql_list_wh_sub_user_id_idx;
DROP INDEX gql_user_wh_sub_user_id_idx;
