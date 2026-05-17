package updatehook

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"

	"git.sr.ht/~sircmpwn/core-go/client"
	coreconfig "git.sr.ht/~sircmpwn/core-go/config"
)

type UpdatedRefInput struct {
	Ref string `json:"ref"`
	Old string `json:"old"`
	New string `json:"new"`
}

type GitEventInput struct {
	RepositoryID int               `json:"repositoryID"`
	Event        string            `json:"event"`
	Updates      []UpdatedRefInput `json:"updates"`
}

func (h *HookContext) ReceiveHook(event string) {
	// TODO: We should consider moving our internal post-update logic to
	// post-receive instead. We can ditch the update hook as well if we go
	// this route, and drop the Redis usage.
	pushUuid, ok := os.LookupEnv("SRHT_PUSH")
	if !ok {
		h.logger.Fatal("Missing SRHT_PUSH in environment, configuration error?")
	}
	h.logger.Printf("Running %s for push %s", event, pushUuid)

	var pcontext PushContext
	contextJson, ctxOk := os.LookupEnv("SRHT_PUSH_CTX")
	if !ctxOk {
		h.logger.Fatal("Missing required variables in environment, " +
			"configuration error?")
	}
	if err := json.Unmarshal([]byte(contextJson), &pcontext); err != nil {
		h.logger.Fatalf("unmarshal SRHT_PUSH_CTX: %v", err)
	}

	loadOptions(h.logger, h.config)
	if _, ok := options["debug"]; ok {
		log.Printf("debug: %s", pushUuid)
	}

	var updates []UpdatedRefInput

	rd := bufio.NewReader(os.Stdin)
	for {
		line, prefix, err := rd.ReadLine()
		if err == io.EOF {
			break
		}
		if prefix {
			// Drop ref if it exceeds bufio buffer length
			break
		}
		items := strings.SplitN(string(line), " ", 3)
		if len(items) != 3 {
			panic("git invariant broken")
		}
		oldOid := items[0]
		newOid := items[1]
		refName := items[2]
		updates = append(updates, UpdatedRefInput{
			Ref: refName,
			Old: oldOid,
			New: newOid,
		})
	}

	ctx := coreconfig.Context(context.Background(), h.config, "git.sr.ht")
	err := client.Do(ctx, pcontext.User.Name, "git.sr.ht", client.GraphQLQuery{
		Query: `
		mutation SubmitGitHook($input: GitEventInput!) {
			deliverGitHook(input: $input)
		}`,
		Variables: map[string]interface{}{
			"input": &GitEventInput{
				RepositoryID: pcontext.Repo.Id,
				Event:        event,
				Updates:      updates,
			},
		},
	}, struct{}{})

	if err != nil {
		h.logger.Fatalf("Failed to execute %s hooks: %s",
			event, err.Error())
	}
}
