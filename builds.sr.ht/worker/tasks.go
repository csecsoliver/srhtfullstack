package worker

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"git.sr.ht/~sircmpwn/core-go/auth"
	"git.sr.ht/~sircmpwn/core-go/objects"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/creack/pty"
	goredis "github.com/go-redis/redis/v8"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	buildsRun = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "buildsrht_image_runs_total",
		Help: "The total number of builds run per arch and image",
	}, []string{"image", "arch"})
	settleTime = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "buildsrht_vm_settle_duration_seconds",
		Help:    "Duration taken by a VM to settle in seconds",
		Buckets: []float64{1, 2, 3, 5, 10, 30, 60, 90, 120, 300},
	}, []string{"image", "arch"})
)

var hutConfigTemplate = template.Must(template.New("hutConfig").Parse(`instance {{ printf "%q" .Name }} {
	access-token {{ printf "%q" .Token }}
	{{- range $service, $origin := .Services }}
	{{ $service }} {
		origin {{ printf "%q" $origin }}
	}
	{{- end }}
}
`))

func (ctx *JobContext) Boot(r *goredis.Client) func() {
	port, err := r.Incr(ctx.Context, "builds.sr.ht.ssh-port").Result()
	if err == nil && port < 22000 {
		port = 22100
		err = r.Set(ctx.Context, "builds.sr.ht.ssh-port", port, 0).Err()
	} else if err == nil && port >= 23000 {
		port = 22000
		err = r.Set(ctx.Context, "builds.sr.ht.ssh-port", port, 0).Err()
	}
	if err != nil {
		panic(errors.Wrap(err, "assign port"))
	}

	arch := "default"
	if ctx.Manifest.Arch != nil {
		arch = *ctx.Manifest.Arch
	}
	ctx.Port = uint16(port)
	ctx.Log.Printf("Booting image %s (%s) on port %d",
		ctx.Manifest.Image, arch, port)

	boot := ctx.Control(ctx.Context,
		ctx.Manifest.Image, "boot", arch, fmt.Sprintf("%d", ctx.Port))
	boot.Env = append(os.Environ(), fmt.Sprintf("BUILD_JOB_ID=%d", ctx.Job.Id))
	boot.Stdout = ctx.LogFile
	boot.Stderr = ctx.LogFile
	if err := boot.Run(); err != nil {
		panic(errors.Wrap(err, "boot"))
	}

	buildsRun.WithLabelValues(ctx.Manifest.Image, arch).Inc()

	return func() {
		ctx.Log.Printf("Tearing down build VM")
		cleanup := ctx.Control(context.TODO(), ctx.Manifest.Image, "cleanup",
			fmt.Sprintf("%d", ctx.Port))
		if err := cleanup.Run(); err != nil {
			fmt.Printf("Failed to destroy build VM: %v\n", err)
		}
	}
}

func (ctx *JobContext) Settle() error {
	ctx.Log.Println("Waiting for guest to settle")

	arch := "default"
	if ctx.Manifest.Arch != nil {
		arch = *ctx.Manifest.Arch
	}
	settleTimer := prometheus.NewTimer(settleTime.WithLabelValues(ctx.Manifest.Image, arch))
	defer settleTimer.ObserveDuration()

	timeout, cancel := context.WithTimeout(ctx.Context, 120*time.Second)
	defer cancel()
	done := make(chan error, 1)
	attempt := 0
	go func() {
		for {
			attempt++
			check := ctx.SSH("echo", "'hello world'")
			pipe, _ := check.StdoutPipe()
			if err := check.Start(); err != nil {
				done <- err
				return
			}
			stdout, _ := io.ReadAll(pipe)
			if err := check.Wait(); err == nil {
				if string(stdout) == "hello world\n" {
					ctx.Settled = true
					done <- nil
					return
				} else {
					done <- fmt.Errorf("unexpected sanity check output: %s",
						string(stdout))
					return
				}
			}

			select {
			case <-timeout.Done():
				done <- fmt.Errorf("Settle timed out after %d attempts",
					attempt)
				return
			case <-time.After(1 * time.Second):
				// Loop
			}
		}
	}()
	return <-done
}

func (ctx *JobContext) SendTasks() error {
	ctx.Log.Println("Sending tasks")
	taskdir := path.Join(ctx.ImageConfig.Homedir, ".tasks")
	if err := ctx.SSH("mkdir", "-p", taskdir).Run(); err != nil {
		return err
	}
	for _, task := range ctx.Manifest.Tasks {
		taskpath := path.Join(taskdir, task.Name)
		script := ctx.ImageConfig.Preamble + task.Script + "\n"
		if err := ctx.Tee(taskpath, []byte(script)); err != nil {
			return err
		}
		if err := ctx.SSH("chmod", "755", taskpath).Run(); err != nil {
			return err
		}
	}
	return nil
}

var shunsafe = regexp.MustCompile(`[^\w@%+=:,./-]`)

func shquote(v string) string {
	// Algorithm aped from shlex.py
	if v == "" {
		return "''"
	}
	if !shunsafe.MatchString(v) {
		return v
	}
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

func (ctx *JobContext) oauth2Token() *auth.BearerToken {
	if !ctx.Job.Secrets || ctx.Manifest.OAuth == "" {
		return nil
	}
	return &auth.BearerToken{
		Version:  auth.TokenVersion,
		Expires:  auth.ToTimestamp(ctx.Deadline),
		Grants:   ctx.Manifest.OAuth,
		ClientID: "",
		Username: ctx.Job.Username,
	}
}

func (ctx *JobContext) SendEnv() error {
	ctx.Log.Println("Sending build environment")
	envpath := path.Join(ctx.ImageConfig.Homedir, ".buildenv")
	env := `#!/bin/sh
complete-build() {
	echo 1 > ~/.complete-build
	exit 0
}
`
	if ctx.Manifest.Environment == nil {
		ctx.Manifest.Environment = make(map[string]any)
	}
	ctx.Manifest.Environment["CI_NAME"] = "sourcehut"
	ctx.Manifest.Environment["JOB_ID"] = ctx.Job.Id
	ctx.Manifest.Environment["JOB_URL"] = fmt.Sprintf(
		"%s/~%s/job/%d", ctx.Origin, ctx.Job.Username, ctx.Job.Id)

	if ot := ctx.oauth2Token(); ot != nil {
		ctx.Manifest.Environment["OAUTH2_TOKEN"] = ot.Encode()
	}

	for key, value := range ctx.Manifest.Environment {
		if value == nil {
			env += fmt.Sprintf("export %s=\"\"\n", key)
			continue
		}
		switch v := value.(type) {
		case bool:
			if v {
				env += fmt.Sprintf("export %s=true\n", key)
			} else {
				env += fmt.Sprintf("export %s=false\n", key)
			}
		case string:
			env += fmt.Sprintf("export %s=%s\n", key, shquote(v))
		case int:
			env += fmt.Sprintf("export %s=%d\n", key, v)
		case int64:
			env += fmt.Sprintf("export %s=%d\n", key, v)
		case uint64:
			env += fmt.Sprintf("export %s=%d\n", key, v)
		case float64:
			env += fmt.Sprintf("export %s=%g\n", key, v)
		case []any:
			env += key + "=("
			for i, _item := range v {
				switch item := _item.(type) {
				case string:
					env += fmt.Sprintf("\"%s\"", item)
				}
				if i != len(v)-1 {
					env += " "
				}
			}
			env += ")\n"
		default:
			panic(fmt.Errorf("unknown environment type %T", value))
		}
	}

	if err := ctx.Tee(envpath, []byte(env)); err != nil {
		return err
	}
	return ctx.SSH("chmod", "755", envpath).Run()
}

func (ctx *JobContext) SendSecrets() error {
	if len(ctx.Manifest.Secrets) == 0 {
		return nil
	}
	ctx.Log.Println("Sending secrets")
	sshKeys := 0
	for _, uuid := range ctx.Manifest.Secrets {
		ctx.Log.Printf("Resolving secret %s\n", uuid)
		secret, err := GetSecret(ctx.Db, uuid, ctx.Job.OwnerId)
		if err == sql.ErrNoRows {
			ctx.Log.Printf("Warning: secret %s not found\n", uuid)
			continue
		} else if err != nil {
			return errors.Wrap(err, "GetSecret")
		}
		switch secret.SecretType {
		case "ssh_key":
			sshdir := path.Join("/", "home", "build", ".ssh")
			keypath := path.Join(sshdir, uuid)
			if err := ctx.SSH("mkdir", "-p", sshdir).Run(); err != nil {
				return errors.Wrap(err, "mkdir -p ~/.ssh")
			}
			if err := ctx.Tee(keypath, secret.Secret); err != nil {
				return errors.Wrap(err, "tee")
			}
			if err := ctx.SSH("chmod", "600", keypath).Run(); err != nil {
				return errors.Wrap(err, "chmod")
			}
			if sshKeys == 0 {
				if err := ctx.SSH("ln", "-s",
					keypath, path.Join(sshdir, "id_rsa")).Run(); err != nil {

					return errors.Wrap(err, "ln -s id_rsa")
				}
			}
			sshKeys++
		case "pgp_key":
			gpg := ctx.SSH("gpg", "--import")
			pipe, err := gpg.StdinPipe()
			gpg.Stdout = ctx.LogFile
			gpg.Stderr = ctx.LogFile
			if err != nil {
				return errors.Wrap(err, "(gpg --import).StdinPipe")
			}
			if err := gpg.Start(); err != nil {
				return errors.Wrap(err, "(gpg --import).Start")
			}
			if _, err := pipe.Write(secret.Secret); err != nil {
				return errors.Wrap(err, "pipe.Write(secret)")
			}
			pipe.Close()
			if err := gpg.Wait(); err != nil {
				return errors.Wrap(err, "(gpg --import).Wait")
			}
		case "plaintext_file":
			if err := ctx.SSH("mkdir", "-p",
				path.Dir(*secret.Path)).Run(); err != nil {

				return errors.Wrap(err, "mkdir -p $(dirname)")
			}
			if err := ctx.Tee(*secret.Path, secret.Secret); err != nil {
				return errors.Wrap(err, "tee")
			}
			if err := ctx.SSH("chmod", fmt.Sprintf("%o", *secret.Mode),
				*secret.Path).Run(); err != nil {

				return errors.Wrap(err, "chmod")
			}
		default:
			return fmt.Errorf("unknown secret type %s", secret.SecretType)
		}
	}
	return nil
}

func (ctx *JobContext) SendHutConfig() error {
	ot := ctx.oauth2Token()
	if ot == nil {
		return nil
	}

	instanceName, ok := ctx.Cfg.Get("sr.ht", "site-name")
	if !ok || instanceName == "" {
		instanceName = "default"
	}
	data := struct {
		Name, Token string
		Services    map[string]string
	}{
		Name:     instanceName,
		Token:    ot.Encode(),
		Services: map[string]string{},
	}
	for name, section := range ctx.Cfg {
		if !strings.HasSuffix(name, ".sr.ht") || strings.Contains(name, "::") {
			continue
		}
		if name == "man.sr.ht" {
			// TODO remove this once hut supports it
			continue
		}
		origin, ok := section["api-origin"]
		if !ok {
			origin, ok = section["origin"]
		}
		if !ok {
			continue
		}
		data.Services[strings.TrimSuffix(name, ".sr.ht")] = origin
	}

	var buf bytes.Buffer
	if err := hutConfigTemplate.Execute(&buf, data); err != nil {
		return errors.Wrap(err, "hutConfigTemplate.Execute")
	}

	configPath := "/home/build/.config/hut/config"
	if err := ctx.SSH("mkdir", "-p", path.Dir(configPath)).Run(); err != nil {
		return errors.Wrap(err, "mkdir -p $(dirname)")
	}
	if err := ctx.Tee(configPath, buf.Bytes()); err != nil {
		return errors.Wrap(err, "tee")
	}
	if err := ctx.SSH("chmod", "0600", configPath).Run(); err != nil {
		return errors.Wrap(err, "chmod")
	}

	return nil
}

func (ctx *JobContext) ConfigureRepos() error {
	if len(ctx.Manifest.Repositories) == 0 {
		return nil
	}
	for name, source := range ctx.Manifest.Repositories {
		ctx.Log.Printf("Adding repository %s\n", name)
		ctrl := ctx.Control(ctx.Context, ctx.Manifest.Image, "add-repo",
			fmt.Sprintf("%d", ctx.Port), name, source)
		ctrl.Stdout = ctx.LogFile
		ctrl.Stderr = ctx.LogFile
		if err := ctrl.Run(); err != nil {
			return err
		}
	}
	return nil
}

func (ctx *JobContext) CloneGitRepo(srcurl, repo_name, ref string) error {
	git := ctx.SSH("GIT_SSH_COMMAND='ssh -o "+
		"UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no'",
		"git", "clone", srcurl, repo_name)
	git.Stdout = ctx.LogFile
	git.Stderr = ctx.LogFile
	if err := git.Run(); err != nil {
		ctx.Log.Println("Failed to clone git repository. " +
			"If this a private repository, make sure you've " +
			"added a suitable SSH key.")
		ctx.Log.Println("https://man.sr.ht/builds.sr.ht/private-repos.md")
		return errors.Wrap(err, "git clone")
	}
	if ref != "" {
		git := ctx.SSH("sh", "-euxc",
			fmt.Sprintf("'git -C %s checkout -q %s'", repo_name, ref))
		git.Stdout = ctx.LogFile
		git.Stderr = ctx.LogFile
		if err := git.Run(); err != nil {
			return errors.Wrap(err, "git checkout")
		}
	}
	git = ctx.SSH("GIT_SSH_COMMAND='ssh -o "+
		"UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no'",
		"sh", "-euxc",
		fmt.Sprintf("'git -C %s submodule update --init --recursive'", repo_name))
	git.Stdout = ctx.LogFile
	git.Stderr = ctx.LogFile
	if err := git.Run(); err != nil {
		return errors.Wrap(err, "git submodule update")
	}
	return nil
}

func (ctx *JobContext) CloneGit9Repo(srcurl, repo_name, ref string) error {
	if strings.Contains(srcurl, "@") {
		// Possible in theory, but we need to implement adding SSH key secrets
		// to factotum, and factotum in general, first. And rewrite the URL to
		// be git9 friendly.
		return errors.New("SSH cloning is not implemented for Plan 9")
	}
	git := ctx.SSH("git/clone", srcurl, repo_name)
	git.Stdout = ctx.LogFile
	git.Stderr = ctx.LogFile
	if err := git.Run(); err != nil {
		ctx.Log.Println("Failed to clone git repository.")
		return errors.Wrap(err, "git clone")
	}
	if ref != "" {
		git := ctx.SSH(fmt.Sprintf("cd %s; git/branch -n -b %s builds.sr.ht-working-branch",
			repo_name, ref))
		git.Stdout = ctx.LogFile
		git.Stderr = ctx.LogFile
		if err := git.Run(); err != nil {
			return errors.Wrap(err, "git checkout")
		}
	}
	return nil
}

func (ctx *JobContext) CloneRepos() error {
	if len(ctx.Manifest.Sources) == 0 {
		return nil
	}
	ctx.Log.Println("Cloning repositories")
	for _, srcurl := range ctx.Manifest.Sources {
		scm := "git"
		hash_bits := strings.Split(srcurl, "#")
		ref := ""
		if len(hash_bits) == 2 {
			srcurl = hash_bits[0]
			ref = hash_bits[1]
		}
		repo_name := path.Base(srcurl)
		directory_bits := strings.Split(srcurl, "::")
		if len(directory_bits) == 2 {
			// directory::... form
			srcurl = directory_bits[1]
			if directory_bits[0] != "" {
				repo_name = directory_bits[0]
			}
		}
		purl, err := url.Parse(srcurl)
		if err == nil {
			scheme_bits := strings.Split(purl.Scheme, "+")
			if len(scheme_bits) == 2 {
				// git+https://... form
				scm = scheme_bits[0]
				purl.Scheme = scheme_bits[1]
				srcurl = purl.String()
			}
		}
		switch scm {
		case "git":
			if len(directory_bits) == 1 {
				// we're using the repo name from the url, which may have .git
				repo_name = strings.TrimSuffix(repo_name, ".git")
			}
			switch ctx.ImageConfig.GitVariant {
			case "git":
				err = ctx.CloneGitRepo(srcurl, repo_name, ref)
			case "git9":
				err = ctx.CloneGit9Repo(srcurl, repo_name, ref)
			}
			if err != nil {
				return err
			}
		case "hg":
			hg := ctx.SSH("hg", "clone",
				"-e", "'ssh -o UserKnownHostsFile=/dev/null "+
					"-o StrictHostKeyChecking=no'", srcurl, repo_name)
			hg.Stdout = ctx.LogFile
			hg.Stderr = ctx.LogFile
			if err := hg.Run(); err != nil {
				ctx.Log.Println("Failed to clone mercurial repository. " +
					"If this a private repository, make sure you've " +
					"added a suitable SSH key.")
				ctx.Log.Println("https://man.sr.ht/builds.sr.ht/private-repos.md")
				return errors.Wrap(err, "hg clone")
			}
			if ref != "" {
				hg := ctx.SSH("sh", "-euxc",
					fmt.Sprintf("'hg --cwd %s update -y %s'", repo_name, ref))
				hg.Stdout = ctx.LogFile
				hg.Stderr = ctx.LogFile
				if err := hg.Run(); err != nil {
					return errors.Wrap(err, "hg update")
				}
			}
		default:
			return errors.New("Unknown scm: " + scm)
		}
	}
	return nil
}

func (ctx *JobContext) InstallPackages() error {
	if len(ctx.Manifest.Packages) == 0 {
		return nil
	}
	ctx.Log.Println("Installing packages")
	ctrl := ctx.Control(ctx.Context, ctx.Manifest.Image, "install",
		fmt.Sprintf("%d", ctx.Port), strings.Join(ctx.Manifest.Packages, " "))
	ctrl.Stdout = ctx.LogFile
	ctrl.Stderr = ctx.LogFile
	return ctrl.Run()
}

func (ctx *JobContext) RunTasks() error {
	for i, task := range ctx.Manifest.Tasks {
		var (
			err   error
			logfd *os.File
			ssh   *exec.Cmd
			tty   *os.File
		)

		ctx.Log.Printf("Running task %s\n", task.Name)
		err = ctx.Job.SetTaskStatus(task.Name, "running")
		if err != nil {
			goto fail
		}

		if err = os.Mkdir(path.Join(ctx.LogDir, task.Name), 0755); err != nil {
			goto fail
		}

		ssh = ctx.SSH(path.Join(".", ".tasks", task.Name))
		if logfd, err = os.Create(path.Join(ctx.LogDir, task.Name, "log")); err != nil {
			err = errors.Wrap(err, "Creating log file")
			goto fail
		}
		tty, err = pty.Start(ssh)
		if err != nil {
			err = errors.Wrap(err, "Allocating pty")
			goto fail
		}
		go io.Copy(logfd, tty)

		if err = ssh.Wait(); err != nil {
			err = errors.Wrap(err, "Running task on guest")
			goto fail
		}

		ctx.Job.SetTaskStatus(task.Name, "success")

		if ctx.isMarkedAsCompleted() {
			for i++; i < len(ctx.Manifest.Tasks); i++ {
				ctx.Job.SetTaskStatus(ctx.Manifest.Tasks[i].Name, "skipped")
			}
			break
		}

		continue
	fail:
		ctx.Job.SetTaskStatus(task.Name, "failed")
		return err
	}
	return nil
}

func (ctx *JobContext) isMarkedAsCompleted() bool {
	rc, cmd, err := ctx.Download(".complete-build")
	if err != nil {
		return false
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	cmd.Wait()
	return strings.TrimSpace(string(b)) == "1"
}

func (ctx *JobContext) UploadArtifacts() error {
	if len(ctx.Manifest.Artifacts) == 0 {
		return nil
	}
	if len(ctx.Manifest.Artifacts) > 8 {
		ctx.Log.Println("Error: no more than 8 artifacts " +
			"per build are accepted.")
		return nil
	}

	var (
		ok       bool
		upstream string
		bucket   string
		prefix   string
	)
	err := errors.New("Build artifacts were requested, but S3 " +
		"is not configured for this build runner.")
	if upstream, ok = ctx.Cfg.Get("objects", "s3-upstream"); !ok || upstream == "" {
		return err
	}
	if bucket, ok = ctx.Cfg.Get("builds.sr.ht::worker", "s3-bucket"); !ok || bucket == "" {
		return err
	}
	if prefix, ok = ctx.Cfg.Get("builds.sr.ht::worker", "s3-prefix"); !ok {
		return err
	}

	sc, err := objects.NewClient(ctx.Cfg)
	if err != nil {
		return err
	}

	random := make([]byte, 8) // Generated to prevent artifact enumeration
	if _, err := rand.Read(random); err != nil {
		return err
	}
	for _, src := range ctx.Manifest.Artifacts {
		ctx.Log.Printf("Uploading %s", src)
		s3key := path.Join(prefix, "~"+ctx.Job.Username,
			strconv.Itoa(ctx.Job.Id),
			hex.EncodeToString(random),
			filepath.Base(src))
		size, err := ctx.FileSize(shquote(src))
		if err != nil {
			ctx.Log.Printf("Error reading artifact file: %v", err)
			if strings.ContainsRune(src, '~') {
				ctx.Log.Printf("You probably need to remove ~/ from the artifact path.")
			}
			return err
		}
		if size > 1024*1024*1024 { // 1 GiB
			err = errors.New("Artifact exceeds maximum file size")
			ctx.Log.Printf("%v", err)
			return err
		}
		pipe, cmd, err := ctx.Download(shquote(src))
		if err != nil {
			ctx.Log.Printf("Error reading artifact file: %v", err)
			return err
		}

		_, err = sc.PutObject(context.TODO(), &s3.PutObjectInput{
			Bucket:      aws.String(bucket),
			Key:         aws.String(s3key),
			Body:        io.LimitReader(pipe, size),
			ContentType: aws.String("application/octet-stream"),
		})
		pipe.Close()
		if err != nil {
			return err
		}

		if err := cmd.Wait(); err != nil {
			return err
		}
		url := fmt.Sprintf("https://%s/%s/%s", upstream, bucket, s3key)
		err = ctx.Job.InsertArtifact(src, filepath.Base(src), url, size)
		if err != nil {
			return err
		}
		ctx.Log.Println(url)
	}
	return nil
}
