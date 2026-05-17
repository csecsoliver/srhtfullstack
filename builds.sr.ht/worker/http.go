package worker

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path"
	"strings"

	"git.sr.ht/~sircmpwn/builds.sr.ht/api"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vaughan0/go-ini"
)

type JobInfo struct {
	Deadline int64        `json:"deadline"`
	Manifest api.Manifest `json:"manifest"`
	Note     *string      `json:"note"`
	OwnerId  int          `json:"owner_id"`
	Port     uint16       `json:"port"`
	Status   string       `json:"status"`
	Task     int          `json:"task"`
	Tasks    int          `json:"tasks"`
	Username string       `json:"username"`
}

func HttpServer(cfg ini.File, jobs *Jobs) {
	requireConf := func(section, key string) string {
		value, ok := cfg.Get(section, key)
		if !ok {
			log.Fatalf("Expected config option [%s]%s", section, key)
		}
		return value
	}

	logDir := requireConf("builds.sr.ht::worker", "buildlogs")
	http.HandleFunc("/job/", func(w http.ResponseWriter, r *http.Request) {
		var (
			jobId int
			op    string
		)
		_, err := fmt.Sscanf(r.URL.Path, "/job/%d/%s", &jobId, &op)
		if err != nil {
			w.WriteHeader(404)
			w.Write([]byte("404 not found"))
			return
		}
		switch op {
		case "info":
			if r.Method != "GET" {
				w.WriteHeader(405)
				w.Write([]byte("405 method not allowed"))
				return
			}
			if job, ok := jobs.Get(jobId); ok {
				w.WriteHeader(200)
				bytes, _ := json.Marshal(JobInfo{
					Deadline: job.Deadline.Unix(),
					Manifest: job.Job.Manifest,
					Note:     job.Job.Note,
					OwnerId:  job.Job.OwnerId,
					Port:     job.Port,
					Status:   job.Job.Status,
					Task:     job.Task,
					Tasks:    job.NTasks,
					Username: job.Job.Username,
				})
				w.Write(bytes)
			} else {
				w.WriteHeader(404)
				w.Write([]byte("404 not found"))
			}
		case "cancel":
			fallthrough
		case "terminate":
			if r.Method != "POST" {
				w.WriteHeader(405)
				w.Write([]byte("405 method not allowed"))
				return
			}
			ok := jobs.With(jobId, func(jc *JobContext) {
				jc.Cancel()
				if op == "cancel" {
					jc.Job.SetStatus("cancelled")
				}
			})
			jobs.DeleteJobContext(jobId)
			if !ok {
				w.WriteHeader(404)
				w.Write([]byte("404 not found"))
				return
			}
			w.WriteHeader(200)
			w.Write([]byte("cancelled"))
		case "claim":
			if r.Method != "POST" {
				w.WriteHeader(405)
				w.Write([]byte("405 method not allowed"))
				return
			}
			ok := jobs.With(jobId, func(jc *JobContext) { jc.Claimed = true })
			if ok {
				w.WriteHeader(200)
				w.Write([]byte("claimed"))
			} else {
				w.WriteHeader(404)
				w.Write([]byte("404 not found"))
			}
		default:
			w.WriteHeader(404)
			w.Write([]byte("404 not found"))
		}
	})
	http.HandleFunc("/logs/", func(w http.ResponseWriter, r *http.Request) {
		file := path.Join(logDir, strings.TrimPrefix(r.URL.Path, "/logs"))
		http.ServeFile(w, r, file)
	})
	http.Handle("/metrics", promhttp.Handler())
	bindAddress, ok := cfg.Get("builds.sr.ht::worker", "bind-address")
	if !ok {
		bindAddress = "0.0.0.0:8080"
	}
	http.ListenAndServe(bindAddress, nil)
}
