package api

import (
	"encoding/json"
	"fmt"

	"github.com/goccy/go-yaml"
)

type Task struct {
	Name   string
	Script string
}

func (t *Task) Validate() error {
	if len(t.Name) > 128 {
		return fmt.Errorf("invalid task name '%s': must be <= 128 characters", t.Name)
	}
	if !taskRE.Match([]byte(t.Name)) {
		return fmt.Errorf("invalid task name '%s': must be all lowercase letters, "+
			"numbers, underscores, and dashes", t.Name)
	}
	return nil
}

func (t *Task) UnmarshalJSON(data []byte) error {
	var raw map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("task must be '<name>: <script>': %s", err.Error())
	}
	if len(raw) != 1 {
		return fmt.Errorf("task must be '<name>: <script>' exactly once")
	}
	for t.Name, t.Script = range raw {
		break
	}
	return t.Validate()
}

func (t *Task) UnmarshalYAML(data []byte) error {
	var raw map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("task must be '<name>: <script>': %s", err.Error())
	}
	if len(raw) != 1 {
		return fmt.Errorf("task must be '<name>: <script>' exactly once")
	}
	for t.Name, t.Script = range raw {
		break
	}
	return t.Validate()
}

func (t Task) MarshalJSON() ([]byte, error) {
	raw := map[string]string{
		t.Name: t.Script,
	}
	return json.Marshal(raw)
}

func (t Task) MarshalYAML() ([]byte, error) {
	raw := map[string]string{
		t.Name: t.Script,
	}
	return yaml.Marshal(raw)
}
