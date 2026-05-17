package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"os"
	"os/signal"
	"runtime"

	"git.sr.ht/~sircmpwn/core-go/config"
	"git.sr.ht/~sircmpwn/core-go/crypto"
	goredis "github.com/go-redis/redis/v8"
	"github.com/vaughan0/go-ini"

	celery "github.com/gocelery/gocelery"
	_ "github.com/lib/pq"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"git.sr.ht/~sircmpwn/builds.sr.ht/worker"
)

var (
	cfg     ini.File
	workers int

	build_workers = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "buildsrht_build_workers",
		Help: "The number of build workers configured",
	})
)

func main() {
	flag.IntVar(&workers, "workers", runtime.NumCPU(),
		"configure number of workers")
	flag.Parse()

	build_workers.Set(float64(workers))

	cfg = config.LoadConfig()
	crypto.InitCrypto(cfg)

	pgcs := conf("builds.sr.ht", "connection-string")
	db, err := sql.Open("postgres", pgcs)
	if err != nil {
		panic(err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to open a database connection: %v", err)
	}

	clusterRedis := conf("builds.sr.ht", "redis")
	broker := celery.NewRedisCeleryBroker(clusterRedis)   //nolint:staticcheck
	backend := celery.NewRedisCeleryBackend(clusterRedis) //nolint:staticcheck

	client, err := celery.NewCeleryClient(broker, backend, workers)
	if err != nil {
		panic(err)
	}
	redisHost, ok := cfg.Get("sr.ht", "redis-host")
	if !ok {
		redisHost = "redis://localhost:6379"
	}
	ropts, err := goredis.ParseURL(redisHost)
	if err != nil {
		panic(err)
	}
	localRedis := goredis.NewClient(ropts)
	if _, err := localRedis.Ping(context.Background()).Result(); err != nil {
		panic(err)
	}

	jobs := worker.NewJobs()
	ctx := &worker.WorkerContext{
		Db:    db,
		Redis: localRedis,
		Conf:  cfg,
		Jobs:  jobs,
	}
	client.Register("buildsrht.runner.run_build_manifest", ctx.RunBuild)

	log.Printf("Starting %d workers...", workers)
	go worker.HttpServer(cfg, jobs)
	client.StartWorker()
	log.Println("Waiting for tasks.")

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	<-c

	log.Println("Waiting for workers to terminate...")
	client.StopWorker()
	log.Println("All workers terminated, ready for shutdown")

	<-c
	log.Println("Exiting.")
}

func conf(section string, key string) string {
	value, ok := cfg.Get(section, key)
	if !ok {
		log.Fatalf("Expected config option [%s]%s", section, key)
	}
	return value
}
