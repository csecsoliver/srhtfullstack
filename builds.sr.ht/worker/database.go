package worker

import (
	"database/sql"
	"time"

	"git.sr.ht/~sircmpwn/builds.sr.ht/api"
	"github.com/google/uuid"
)

type Job struct {
	db *sql.DB

	Id         int
	Created    time.Time
	Updated    time.Time
	Manifest   api.Manifest
	OwnerId    int
	JobGroupId *int
	Note       *string
	Status     string
	Runner     *string
	Tags       *string
	Secrets    bool
	Image      string

	Username string
}

type Secret struct {
	Id         int
	UserId     int
	Created    time.Time
	Updated    time.Time
	Uuid       string
	Name       *string
	SecretType string
	Secret     []byte
	Path       *string
	Mode       *int
}

type JobGroup struct {
	db *sql.DB

	Id       int
	OwnerId  int
	Created  time.Time
	Updated  time.Time
	Note     *string
	Complete bool

	Jobs []*Job
}

type Trigger struct {
	Id        int
	Created   time.Time
	Updated   time.Time
	Action    string
	Condition string
	Details   string
}

func GetJob(db *sql.DB, id int) (*Job, error) {
	row := db.QueryRow(`
		SELECT
			"job"."id", "job"."created", "job"."updated", "job"."manifest",
			"job"."owner_id", "job"."job_group_id", "job"."note",
			"job"."status", "job"."runner", "job"."tags", "job"."secrets",
			"job"."image", "user".username
		FROM "job"
		JOIN "user" ON "job"."owner_id" = "user"."id"
		WHERE "job"."id" = $1;
	`, id)
	var (
		job      Job
		manifest string
	)
	job.db = db
	if err := row.Scan(
		&job.Id, &job.Created, &job.Updated, &manifest, &job.OwnerId,
		&job.JobGroupId, &job.Note, &job.Status, &job.Runner, &job.Tags,
		&job.Secrets, &job.Image, &job.Username); err != nil {

		return nil, err
	}
	parsed, err := api.LoadManifest(manifest)
	if err != nil {
		return nil, err
	}
	job.Manifest = *parsed
	return &job, nil
}

func GetSecret(db *sql.DB, sec string, ownerId int) (*Secret, error) {
	_, err := uuid.Parse(sec)
	if err != nil {
		return GetSecretByName(db, sec, ownerId)
	}
	return GetSecretById(db, sec, ownerId)
}

func GetSecretById(db *sql.DB, uuid string, ownerId int) (*Secret, error) {
	row := db.QueryRow(`
		SELECT
			"id", "user_id", "created", "updated", "uuid",
			"name", "secret_type", "secret", "path", "mode"
		FROM secret
    WHERE uuid = $1 AND user_id = $2;
	`, uuid, ownerId)
	var secret Secret
	if err := row.Scan(
		&secret.Id, &secret.UserId, &secret.Created, &secret.Updated,
		&secret.Uuid, &secret.Name, &secret.SecretType, &secret.Secret,
		&secret.Path, &secret.Mode); err != nil {
		return nil, err
	}
	return &secret, nil
}

func GetSecretByName(db *sql.DB, uuid string, ownerId int) (*Secret, error) {
	row := db.QueryRow(`
		SELECT
			"id", "user_id", "created", "updated", "uuid",
			"name", "secret_type", "secret", "path", "mode"
		FROM "secret" WHERE "name" = $1 AND "user_id" = $2;
	`, uuid, ownerId)
	var secret Secret
	if err := row.Scan(
		&secret.Id, &secret.UserId, &secret.Created, &secret.Updated,
		&secret.Uuid, &secret.Name, &secret.SecretType, &secret.Secret,
		&secret.Path, &secret.Mode); err != nil {
		return nil, err
	}
	return &secret, nil
}

func GetJobGroup(db *sql.DB, jobGroupId int) (*JobGroup, error) {
	row := db.QueryRow(`
		SELECT
			jg.id, jg.created, jg.updated, jg.owner_id, jg.note,
			sum(CASE WHEN j.status not in ('pending', 'queued')
				THEN 1 ELSE 0 END) = count(j) as complete
		FROM job_group jg
		JOIN job j ON j.job_group_id = jg.id
		WHERE jg.id = $1
		GROUP BY jg.id;
	`, jobGroupId)
	var jg JobGroup
	jg.db = db
	if err := row.Scan(&jg.Id, &jg.Created, &jg.Updated, &jg.OwnerId, &jg.Note,
		&jg.Complete); err != nil {

		return nil, err
	}
	return &jg, nil
}

func (job *Job) SetRunner(runner string) error {
	_, err := job.db.Exec(`UPDATE "job" SET "runner" = $2 WHERE "id" = $1`,
		job.Id, runner)
	if err == nil {
		_runner := runner
		job.Runner = &_runner
	}
	return err
}

func (job *Job) SetStatus(status string) error {
	_, err := job.db.Exec(`UPDATE "job" SET "status" = $2 WHERE "id" = $1`,
		job.Id, status)
	if err == nil {
		job.Status = status
		_, err = job.db.Exec(`UPDATE "job" SET "updated" = $2 WHERE "id" = $1`,
			job.Id, time.Now().UTC())
	}
	return err
}

func (job *Job) SetTaskStatus(name, status string) error {
	_, err := job.db.Exec(`
		UPDATE "task"
		SET "status" = $3
		WHERE "job_id" = $1 AND "name" = $2
	`, job.Id, name, status)
	return err
}

func (job *Job) GetTaskStatus(name string) (string, error) {
	row := job.db.QueryRow(`
		SELECT "status" FROM "task" WHERE "job_id" = $1 AND "name" = $2
	`, job.Id, name)
	var status string
	if err := row.Scan(&status); err != nil {
		return "", err
	}
	return status, nil
}

func (job *Job) InsertArtifact(path string, name string,
	url string, size int64) error {

	_, err := job.db.Exec(`
		INSERT INTO
		"artifact" (created, job_id, path, name, url, size)
		VALUES (NOW() AT TIME ZONE 'UTC',
			$1, $2, $3, $4, $5)
	`, job.Id, path, name, url, size)
	return err
}

func (jg *JobGroup) GetTriggers() ([]*Trigger, error) {
	rows, err := jg.db.Query(`
		SELECT
			id, created, updated, details, condition, trigger_type
		FROM trigger
		WHERE job_group_id = $1;
	`, jg.Id)
	if err != nil {
		return nil, err
	}
	var triggers []*Trigger
	for rows.Next() {
		trigger := &Trigger{}
		if err := rows.Scan(&trigger.Id, &trigger.Created, &trigger.Updated,
			&trigger.Details, &trigger.Condition, &trigger.Action); err != nil {

			return nil, err
		}
		triggers = append(triggers, trigger)
	}
	return triggers, nil
}

func (jg *JobGroup) GetJobs() error {
	rows, err := jg.db.Query(`
		SELECT
			"job"."id", "job"."created", "job"."updated", "job"."manifest",
			"job"."owner_id", "job"."job_group_id", "job"."note",
			"job"."status", "job"."runner", "job"."tags", "job"."secrets",
			"job".image, "user".username
		FROM "job"
		JOIN "user" ON "job"."owner_id" = "user"."id"
		WHERE "job"."job_group_id" = $1;
	`, jg.Id)
	if err != nil {
		return err
	}
	var jobs []*Job
	for rows.Next() {
		var manifest string
		job := &Job{}
		if err := rows.Scan(
			&job.Id, &job.Created, &job.Updated, &manifest, &job.OwnerId,
			&job.JobGroupId, &job.Note, &job.Status, &job.Runner, &job.Tags,
			&job.Secrets, &job.Image, &job.Username); err != nil {

			return err
		}
		parsed, err := api.LoadManifest(manifest)
		if err != nil {
			return err
		}
		job.Manifest = *parsed
		jobs = append(jobs, job)
	}
	jg.Jobs = jobs
	return nil
}
