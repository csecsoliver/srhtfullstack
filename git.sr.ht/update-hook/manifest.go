package updatehook

// TODO: Move this into builds.sr.ht

import (
	"bytes"

	"github.com/goccy/go-yaml"
)

type GitSubmitter struct {
	Enabled   *bool    `yaml:"enabled,omitempty"`
	AllowRefs []string `yaml:"allow-refs,omitempty"`
}

type Submitter struct {
	Git *GitSubmitter `yaml:"git.sr.ht,omitempty"`
}

type Manifest struct {
	Arch         *string                  `yaml:"arch,omitempty"`
	Artifacts    []string                 `yaml:"artifacts,omitempty"`
	Environment  map[string]interface{}   `yaml:"environment,omitempty"`
	Image        string                   `yaml:"image"`
	Packages     []string                 `yaml:"packages,omitempty"`
	Repositories map[string]string        `yaml:"repositories,omitempty"`
	Secrets      []string                 `yaml:"secrets,omitempty"`
	Shell        bool                     `yaml:"shell,omitempty"`
	Sources      []string                 `yaml:"sources,omitempty"`
	Submitter    *Submitter               `yaml:"submitter,omitempty"`
	Tasks        []map[string]string      `yaml:"tasks"`
	Triggers     []map[string]interface{} `yaml:"triggers,omitempty"`
	OAuth        string                   `yaml:"oauth,omitempty"`
}

func ManifestFromYAML(src string) (Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal([]byte(src), &m); err != nil {
		return m, err
	}
	// XXX: We could do validation here, but builds.sr.ht will also catch it
	// for us later so it's not especially important to
	return m, nil
}

func (manifest Manifest) ToYAML() (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf, yaml.UseLiteralStyleIfMultiline(true))
	err := enc.Encode(&manifest)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}
