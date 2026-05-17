-- +brant Up
CREATE INDEX gql_git_wh_sub_repo_id_idx ON gql_git_wh_sub USING btree(repo_id);
CREATE INDEX gql_git_wh_sub_user_id_idx ON gql_git_wh_sub USING btree(user_id);
CREATE INDEX gql_user_wh_sub_user_id_idx ON gql_user_wh_sub USING btree(user_id);

-- +brant Down
DROP INDEX gql_git_wh_sub_repo_id_idx;
DROP INDEX gql_git_wh_sub_user_id_idx;
DROP INDEX gql_user_wh_sub_user_id_idx;
