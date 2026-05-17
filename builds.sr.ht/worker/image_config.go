package worker

import (
	"io"
	"os"
	"path"

	"github.com/goccy/go-yaml"
)

type ImageConfig struct {
	LoginCmd   string `yaml:"logincmd"`
	GitVariant string `yaml:"git_variant"`
	Homedir    string `yaml:"homedir"`
	Preamble   string `yaml:"preamble"`
}

func LoadImageConfig(imagesPath, image string) *ImageConfig {
	// images, _ := cfg.Get("builds.sr.ht::worker", "images")
	iconf := &ImageConfig{
		LoginCmd:   "ssh",
		GitVariant: "git",
		Homedir:    "/home/build",
		Preamble: `#!/usr/bin/env bash
. ~/.buildenv
set -xe

acurl() (
	set +x
	curl --oauth2-bearer "$OAUTH2_TOKEN" "$@"
)
`,
	}
	f, err := os.Open(path.Join(imagesPath, image, "config.yml"))
	if err != nil {
		return iconf
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		panic(err)
	}
	err = yaml.Unmarshal(b, iconf)
	if err != nil {
		panic(err)
	}
	return iconf
}
