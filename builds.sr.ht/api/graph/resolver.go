package graph

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/client"
	"git.sr.ht/~sircmpwn/core-go/config"
	"github.com/99designs/gqlgen/graphql"

	"git.sr.ht/~sircmpwn/builds.sr.ht/api"
	"git.sr.ht/~sircmpwn/builds.sr.ht/api/errors"
	"git.sr.ht/~sircmpwn/builds.sr.ht/api/graph/model"
)

type Resolver struct{}

func FetchLogs(ctx context.Context, runner string, jobID int, taskName string) (*model.Log, error) {
	conf := config.ForContext(ctx)
	origin := config.GetAPI(conf, "builds.sr.ht", true)

	var (
		externalURL string
		internalURL string
	)
	if taskName == "" {
		externalURL = fmt.Sprintf("%s/query/log/%d/log", origin, jobID)
		internalURL = fmt.Sprintf("http://%s/logs/%d/log", runner, jobID)
	} else {
		externalURL = fmt.Sprintf("%s/query/log/%d/%s/log", origin, jobID, taskName)
		internalURL = fmt.Sprintf("http://%s/logs/%d/%s/log", runner, jobID, taskName)
	}
	log := &model.Log{FullURL: externalURL}

	// If the user hasn't requested the log body, stop here
	if graphql.GetFieldContext(ctx) != nil {
		found := false
		for _, field := range graphql.CollectFieldsCtx(ctx, nil) {
			if field.Name == "last128KiB" {
				found = true
				break
			}
		}
		if !found {
			return log, nil
		}
	}

	// TODO: It might be possible/desirable to set up an API with the
	// runners we can use to fetch logs in bulk, perhaps gzipped, and set
	// up a loader for it.
	req, err := http.NewRequestWithContext(ctx, "GET", internalURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Range", "bytes=-131072") // Last 128 KiB
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusPartialContent:
		// OK
		break
	case http.StatusNotFound:
		return nil, nil
	default:
		return nil, fmt.Errorf("unexpected response from build runner: %s", resp.Status)
	}
	limit := io.LimitReader(resp.Body, 131072)
	b, err := io.ReadAll(limit)
	if err != nil {
		return nil, err
	}
	log.Last128KiB = string(b)

	return log, nil
}

// Starts a job group. Does not authenticate the user.
func StartJobGroupUnsafe(ctx context.Context, tx *sql.Tx, id, ownerID int) error {
	var manifests []struct {
		ID       int
		Manifest *api.Manifest
	}

	rows, err := tx.QueryContext(ctx, `
		UPDATE job SET status = 'queued'
		WHERE
			job_group_id = $1 AND
			owner_id = $2
		RETURNING id, manifest;
	`, id, ownerID)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			id       int
			manifest string
		)
		if err := rows.Scan(&id, &manifest); err != nil {
			return err
		}

		man, err := api.LoadManifest(manifest)
		if err != nil {
			// Invalid manifests shouldn't make it to the database
			panic(err)
		}

		manifests = append(manifests, struct {
			ID       int
			Manifest *api.Manifest
		}{
			ID:       id,
			Manifest: man,
		})
	}

	if err := rows.Err(); err != nil {
		return err
	}

	err = tx.Commit()
	if err != nil {
		return err
	}

	for _, job := range manifests {
		if err := SubmitJob(ctx, job.ID, job.Manifest); err != nil {
			return fmt.Errorf("failed to submit some jobs: %e", err)
		}
	}

	return nil
}

func checkPaymentRequirement(ctx context.Context) error {
	conf := config.ForContext(ctx)
	user := auth.ForContext(ctx)

	allowFree, _ := conf.Get("builds.sr.ht", "allow-free")
	if allowFree == "yes" {
		return nil
	}

	query := client.GraphQLQuery{
		Query: `query { me { receivesPaidServices } }`,
	}
	var resp struct {
		Me struct {
			ReceivesPaidServices bool `json:"receivesPaidServices"`
		} `json:"me"`
	}
	if err := client.Do(ctx, user.Username, "meta.sr.ht", query, &resp); err != nil {
		return err
	}

	if !resp.Me.ReceivesPaidServices {
		return errors.ErrPaymentRequired
	}

	return nil
}
