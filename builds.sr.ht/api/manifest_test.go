package api

import (
	"encoding/json"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
)

func TestGood(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- foo: |
    echo "foo"
- bar: |
    echo "bar"
`

	manifest, err := LoadManifest(input)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(manifest.Tasks))
	assert.Equal(t, "foo", manifest.Tasks[0].Name)
	assert.Equal(t, "bar", manifest.Tasks[1].Name)
}

func TestMarshal(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- foo: |
    echo "foo"
- bar: |
    echo "bar"
`

	manifest, err := LoadManifest(input)
	assert.Nil(t, err)
	out, err := yaml.Marshal(manifest)
	assert.Nil(t, err)
	assert.Equal(t, input, string(out))
}

func TestMerryGoRound(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- foo: |
    echo "foo"
- bar: |
    echo "bar"
`

	manifest, err := LoadManifest(input)
	assert.Nil(t, err)
	out, err := json.Marshal(manifest)
	assert.Nil(t, err)
	err = json.Unmarshal(out, manifest)
	assert.Nil(t, err)
	out, err = yaml.Marshal(manifest)
	assert.Nil(t, err)
	assert.Equal(t, input, string(out))
}

func TestLongName(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- foobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobarfoobar: |
    echo "foo"
- bar: |
    echo "bar"
`

	_, err := LoadManifest(input)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "must be <= 128 characters")
}

func TestInvalidName(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- Foo: |
    echo "foo"
- bar: |
    echo "bar"
`

	_, err := LoadManifest(input)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "invalid task name")
}

func TestDuplicateName(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- foo: |
    echo "foo"
- foo: |
    echo "bar"
`

	_, err := LoadManifest(input)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "duplicate task")
}

func TestMultipleNames(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- foo: "echo 'foo'"
  foobar: "echo 'foobar'"
- bar: |
    echo "bar"
`

	_, err := LoadManifest(input)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "exactly once")
}

func TestNotMap(t *testing.T) {
	input := `image: alpine/3.22
tasks:
- echo "foo"
- echo "bar"
`

	_, err := LoadManifest(input)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "string was used where mapping is expected")
}

func TestNotList(t *testing.T) {
	input := `image: alpine/3.22
tasks:
  foo: |
    echo "foo"
  bar: |
    echo "bar"
`

	_, err := LoadManifest(input)
	assert.NotNil(t, err)
	assert.Contains(t, err.Error(), "mapping was used where sequence is expected")
}
