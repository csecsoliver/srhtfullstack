package api

import (
	"fmt"
	"regexp"

	"github.com/goccy/go-yaml"
	"github.com/google/uuid"
)

var taskRE = regexp.MustCompile("^[a-z0-9_-]+$")

type Trigger struct {
	Action    string `yaml:"action" json:"action"`
	Condition string `yaml:"condition" json:"condition"`

	// Email fields
	To        *string `yaml:"to" json:"to,omitempty"`
	Cc        *string `yaml:"cc" json:"cc,omitempty"`
	InReplyTo *string `yaml:"in_reply_to" json:"in_reply_to,omitempty"`

	// Webhook fields
	Url *string `yaml:"url" json:"url"`
}

type Manifest struct {
	Arch         *string           `yaml:"arch,omitempty" json:"arch,omitempty"`
	Artifacts    []string          `yaml:"artifacts,omitempty" json:"artifacts,omitempty"`
	Environment  map[string]any    `yaml:"environment,omitempty" json:"environment,omitempty"`
	Image        string            `yaml:"image" json:"image"`
	Packages     []string          `yaml:"packages,omitempty" json:"packages,omitempty"`
	Repositories map[string]string `yaml:"repositories,omitempty" json:"repositories,omitempty"`
	Secrets      []string          `yaml:"secrets,omitempty" json:"secrets,omitempty"`
	Shell        bool              `yaml:"shell,omitempty" json:"shell,omitempty"`
	Sources      []string          `yaml:"sources,omitempty" json:"sources,omitempty"`
	Tasks        []Task            `yaml:"tasks" json:"tasks"`
	Triggers     []Trigger         `yaml:"triggers,omitempty" json:"triggers,omitempty"`
	OAuth        string            `yaml:"oauth,omitempty" json:"oauth,omitempty"`
}

func LoadManifest(in string) (*Manifest, error) {
	var manifest Manifest
	err := yaml.Unmarshal([]byte(in), &manifest)
	if err != nil {
		return nil, err
	}

	if manifest.Image == "" {
		return nil, fmt.Errorf("image is required")
	}

	for _, sec := range manifest.Secrets {
		_, err := uuid.Parse(sec)
		if err != nil && (len(sec) <= 3 || len(sec) >= 512) {
			return nil, err
		}
	}

	artset := make(map[string]any)
	for _, art := range manifest.Artifacts {
		if _, ok := artset[art]; ok {
			return nil, fmt.Errorf("duplicate artifact %s", art)
		}
		artset[art] = nil
	}

	if len(manifest.Tasks) == 0 && !manifest.Shell {
		return nil, fmt.Errorf("list of tasks is required")
	}

	taskset := make(map[string]any)
	for _, task := range manifest.Tasks {
		if _, ok := taskset[task.Name]; ok {
			return nil, fmt.Errorf("duplicate task %s", task.Name)
		}
		taskset[task.Name] = nil
	}

	for _, trigger := range manifest.Triggers {
		switch trigger.Action {
		case "email":
			if trigger.To == nil && trigger.Cc == nil {
				return nil, fmt.Errorf("email trigger requires 'to' or 'cc'")
			}
		case "webhook":
			if trigger.Url == nil {
				return nil, fmt.Errorf("webhook trigger requires 'url'")
			}
		default:
			return nil, fmt.Errorf("unknown trigger type '%s'", trigger.Action)
		}
	}

	return &manifest, nil
}
