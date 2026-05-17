package graph

import (
	"context"

	"git.sr.ht/~sircmpwn/builds.sr.ht/api"
	"git.sr.ht/~sircmpwn/core-go/config"

	"github.com/goccy/go-yaml"
	celery "github.com/gocelery/gocelery"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	buildsSubmitted = promauto.NewCounter(prometheus.CounterOpts{
		Name: "buildsrht_builds_submitted_total",
		Help: "Number of builds submitted",
	})
)

func SubmitJob(ctx context.Context, jobID int, manifest *api.Manifest) error {
	conf := config.ForContext(ctx)
	clusterRedis, _ := conf.Get("builds.sr.ht", "redis")
	broker := celery.NewRedisCeleryBroker(clusterRedis)   //nolint:staticcheck
	backend := celery.NewRedisCeleryBackend(clusterRedis) //nolint:staticcheck

	// XXX: Maybe we should keep this client instance around and stash it
	// somewhere on the context
	client, err := celery.NewCeleryClient(broker, backend, 1)
	if err != nil {
		panic(err)

	}

	m, err := yaml.Marshal(manifest)
	if err != nil {
		panic(err)
	}

	buildsSubmitted.Inc()
	// gocelery automatically converts []byte to string, so make this explicit
	_, err = client.Delay("buildsrht.runner.run_build_manifest", jobID, string(m))
	return err
}
