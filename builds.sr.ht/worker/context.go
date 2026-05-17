package worker

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"git.sr.ht/~sircmpwn/builds.sr.ht/api"
	goredis "github.com/go-redis/redis/v8"
	"github.com/goccy/go-yaml"
	"github.com/google/shlex"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/vaughan0/go-ini"
)

var (
	buildsRunning = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "buildsrht_builds_running",
		Help: "The number of builds currently running",
	})
	buildDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "buildsrht_build_duration",
		Help:    "Duration of each build in seconds",
		Buckets: []float64{10, 30, 60, 90, 120, 300, 600, 900, 1800},
	}, []string{"status"})
)

type WorkerContext struct {
	Db    *sql.DB
	Redis *goredis.Client
	Conf  ini.File
	Jobs  *Jobs
}

type JobContext struct {
	Cancel      context.CancelFunc
	Claimed     bool
	Cfg         ini.File
	Conf        func(string, string) string
	Context     context.Context
	Db          *sql.DB
	Deadline    time.Time
	ImageConfig *ImageConfig
	Job         *Job
	Log         *log.Logger
	LogDir      string
	LogFile     *os.File
	Manifest    *api.Manifest
	Origin      string
	Port        uint16
	Settled     bool

	NTasks int
	Task   int

	ProcessedTriggers bool
}

func (wctx *WorkerContext) RunBuild(
	job_id int, _manifest string) error {

	var manifest api.Manifest
	if err := yaml.Unmarshal([]byte(_manifest), &manifest); err != nil {
		panic(err)
	}

	var (
		err error
		job *Job
		ctx *JobContext

		cleanup func()
	)

	requireConf := func(section, key string) string {
		value, ok := wctx.Conf.Get(section, key)
		if !ok {
			log.Fatalf("Expected config option [%s]%s", section, key)
		}
		return value
	}

	buildUser, ok := wctx.Conf.Get("builds.sr.ht", "ssh-user")
	if !ok {
		buildUser = "builds"
	}

	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		if job != nil {
			buildDuration.WithLabelValues(job.Status).Observe(v)
		} else {
			buildDuration.WithLabelValues("failed").Observe(v)
		}
	}))
	defer timer.ObserveDuration()
	buildsRunning.Inc()
	defer buildsRunning.Dec()

	job, err = GetJob(wctx.Db, job_id)
	if err != nil {
		panic(errors.Wrap(err, "GetJob"))
	}
	runner := requireConf("builds.sr.ht::worker", "name")
	if err = job.SetRunner(runner); err != nil {
		panic(errors.Wrap(err, "job.SetRunner"))
	}
	if err = job.SetStatus("running"); err != nil {
		panic(errors.Wrap(err, "job.SetStatus"))
	}

	if !job.Secrets {
		manifest.Secrets = []string{}
	}

	timeout_ := requireConf("builds.sr.ht::worker", "timeout")
	timeout, _ := time.ParseDuration(timeout_)
	goctx, cancel := context.WithTimeout(context.Background(), timeout)

	ctx = &JobContext{
		Cancel:   cancel,
		Cfg:      wctx.Conf,
		Conf:     requireConf,
		Context:  goctx,
		Db:       wctx.Db,
		Deadline: time.Now().UTC().Add(timeout),
		Job:      job,
		Manifest: &manifest,
		Origin:   requireConf("builds.sr.ht", "origin"),
	}

	logs := requireConf("builds.sr.ht::worker", "buildlogs")
	ctx.LogDir = path.Join(logs, strconv.Itoa(job_id))
	if err = os.MkdirAll(ctx.LogDir, 0755); err != nil {
		panic(errors.Wrap(err, "Make log directory"))
	}
	if ctx.LogFile, err = os.Create(path.Join(ctx.LogDir, "log")); err != nil {
		panic(errors.Wrap(err, "Make top-level log"))
	}
	ctx.Log = log.New(io.MultiWriter(ctx.LogFile, os.Stdout),
		"[#"+strconv.Itoa(job.Id)+"] ", log.LstdFlags)
	ctx.Log.Printf("Running build on %s", runner)

	defer func() {
		if err := recover(); err != nil {
			log.Printf("run_build panic: %v", err)
			if job != nil && ctx != nil {
				if ctx.Context.Err() == context.DeadlineExceeded {
					job.SetStatus("timeout")
				} else if ctx.Context.Err() == context.Canceled {
					job.SetStatus("cancelled")
				} else {
					job.SetStatus("failed")
				}
				ctx.ProcessTriggers()
				if ctx.Settled {
					ctx.Standby(buildUser)
				}
				if ctx.Log != nil {
					ctx.Log.Printf("Error: %v\n", err)
					ctx.LogFile.Close()
				}
			} else if job != nil {
				job.SetStatus("failed")
			}
		}
		if cleanup != nil {
			cleanup()
		}
	}()

	wctx.Jobs.SetJobContext(job_id, ctx)

	imagesPath, _ := wctx.Conf.Get("builds.sr.ht::worker", "images")
	ctx.ImageConfig = LoadImageConfig(imagesPath, manifest.Image)

	cleanup = ctx.Boot(wctx.Redis)

	if err = ctx.Settle(); err != nil {
		panic(err)
	}

	if manifest.Shell {
		ctx.Log.Println()
		ctx.Log.Println("\x1B[1m\x1B[96mShell access for this build was requested.\x1B[0m")
		ctx.Log.Println("To log in with SSH, use the following command:")
		ctx.Log.Println()
		ctx.Log.Printf("\tssh -t %s@%s connect %d", buildUser, runner, job_id)
		ctx.Log.Println()
	}

	tasks := []func() error{
		ctx.SendTasks,
		ctx.SendEnv,
		ctx.SendSecrets,
		ctx.SendHutConfig,
		ctx.ConfigureRepos,
		ctx.InstallPackages,
		ctx.CloneRepos,
		ctx.RunTasks,
		ctx.UploadArtifacts,
	}
	ctx.NTasks = len(tasks)
	for i, task := range tasks {
		ctx.Task = i
		if err = task(); err != nil {
			panic(err)
		}
	}
	ctx.Task = ctx.NTasks

	if manifest.Shell {
		<-goctx.Done()
	}

	wctx.Jobs.DeleteJobContext(job_id)

	cancel()
	job.SetStatus("success")
	ctx.ProcessTriggers()
	ctx.LogFile.Close()

	return nil
}

func (ctx *JobContext) Standby(buildUser string) {
	select {
	case <-ctx.Context.Done():
		ctx.Log.Println("\x1B[1m\x1B[91mBuild cancelled.\x1B[0m")
		ctx.Log.Println("Terminating build environment.")
		return
	default:
		break
	}
	ctx.Log.Println("\x1B[1m\x1B[91mBuild failed.\x1B[0m")
	ctx.Log.Println("The build environment will be kept alive for 10 minutes.")
	ctx.Log.Println("To log in with SSH and examine it, use the following command:")
	ctx.Log.Println()
	ctx.Log.Printf("\tssh -t %s@%s connect %d", buildUser, *ctx.Job.Runner, ctx.Job.Id)
	ctx.Log.Println()
	ctx.Log.Println("After logging in, the deadline is increased to your remaining build time.")
	select {
	case <-time.After(10 * time.Minute):
		break
	case <-ctx.Context.Done():
		break
	}
	if ctx.Claimed {
		select {
		case <-time.After(time.Until(ctx.Deadline)):
			break
		case <-ctx.Context.Done():
			break
		}
	} else {
		ctx.Log.Println("Deadline elapsed. Terminating build environment.")
	}
}

func (ctx *JobContext) Control(
	context context.Context, args ...string) *exec.Cmd {

	controlString := ctx.Conf("builds.sr.ht::worker", "controlcmd")
	controlSplitted, err := shlex.Split(controlString)
	if err != nil {
		panic(errors.Wrap(err, "controlcmd"))
	}
	args = append(controlSplitted[1:], args...)

	return exec.CommandContext(context, controlSplitted[0], args...)
}

func (ctx *JobContext) SSH(args ...string) *exec.Cmd {
	switch ctx.ImageConfig.LoginCmd {
	case "drawterm":
		return exec.CommandContext(ctx.Context,
			"env", fmt.Sprintf("DIALSRV=%d", ctx.Port),
			"PASS=password", "drawterm",
			"-a", "none",
			"-u", "glenda",
			"-h", "127.0.0.1",
			"-Gc", strings.Join(args, " "))
	case "ssh":
		return exec.CommandContext(ctx.Context, "ssh",
			append([]string{"-q",
				"-p", fmt.Sprintf("%d", ctx.Port),
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", "StrictHostKeyChecking=no",
				"-o", "LogLevel=quiet",
				"build@localhost",
			}, args...)...)
	default:
		panic(errors.New("Unknown login command"))
	}
}

func (ctx *JobContext) Tee(path string, data []byte) error {
	tee := ctx.SSH("tee", path)
	pipe, err := tee.StdinPipe()
	if err != nil {
		return err
	}
	tee.Stderr = ctx.LogFile
	if err := tee.Start(); err != nil {
		return err
	}
	if _, err := pipe.Write(data); err != nil {
		return err
	}
	pipe.Close()
	return tee.Wait()
}

func (ctx *JobContext) FileSize(path string) (int64, error) {
	wc := ctx.SSH("wc", "-c", path)
	pipe, err := wc.StdoutPipe()
	if err != nil {
		return 0, err
	}
	defer pipe.Close()
	if err := wc.Start(); err != nil {
		return 0, err
	}
	stdout, err := io.ReadAll(io.LimitReader(pipe, 1024))
	if err != nil {
		return 0, err
	}
	if len(stdout) == 0 {
		return 0, errors.New("File not found")
	}
	parts := strings.Split(strings.Trim(string(stdout), " \t"), " ")
	if len(parts) != 2 {
		return 0, errors.New("Unexpected response from wc")
	}
	if err := wc.Wait(); err != nil {
		return 0, err
	}
	return strconv.ParseInt(parts[0], 10, 64)
}

func (ctx *JobContext) Download(path string) (io.ReadCloser, *exec.Cmd, error) {
	cat := ctx.SSH("cat", path)
	pipe, err := cat.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cat.Start(); err != nil {
		return nil, nil, err
	}
	return pipe, cat, nil
}
