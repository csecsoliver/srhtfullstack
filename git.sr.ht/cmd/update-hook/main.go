package main

import (
	"log"
	"os"

	updatehook "git.sr.ht/~sircmpwn/git.sr.ht/update-hook"
)

var (
	logger *log.Logger
)

func main() {
	logf, err := os.OpenFile("/var/log/git.sr.ht/git.sr.ht-update-hook.log",
		os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("Warning: unable to open log file: %v "+
			"(using stderr instead)", err)
		logger = log.New(os.Stderr, os.Args[0]+" ", log.LstdFlags)
	} else {
		logger = log.New(logf, os.Args[0]+" ", log.LstdFlags)
	}

	log.SetFlags(0)
	logger.Printf("%v", os.Args)

	ctx := updatehook.NewContext(logger)

	// The update hook is run on the update and post-update git hooks, and
	// also runs a third stage directly. The first two stages are
	// performance critical and take place while the user is blocked at
	// their terminal. The third stage is done in the background.
	switch os.Args[0] {
	case "hooks/pre-receive":
		ctx.ReceiveHook("GIT_PRE_RECEIVE")
	case "hooks/update":
		ctx.Update()
	case "hooks/post-update":
		ctx.PostUpdate()
	case "hooks/post-receive":
		ctx.ReceiveHook("GIT_POST_RECEIVE")
	default:
		log.Fatalf("Unknown git hook %s", os.Args[0])
	}
}
