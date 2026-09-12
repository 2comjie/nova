package diffgen

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type config struct {
	SourceRoot string   `yaml:"source_root"`
	Packages   []string `yaml:"packages"`
	Proto      struct {
		Dir   string `yaml:"dir"`
		GoOut string `yaml:"go_out"`
	} `yaml:"proto"`

	dir string
}

func Generate(configPath string) error {
	path, err := filepath.Abs(configPath)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	var config config
	decoder := yaml.NewDecoder(file)
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	if config.SourceRoot == "" || len(config.Packages) == 0 || config.Proto.Dir == "" || config.Proto.GoOut == "" {
		return errors.New("diffgen: 配置需要 source_root、packages、proto.dir 和 proto.go_out")
	}
	config.dir = filepath.Dir(path)
	for _, value := range []*string{&config.SourceRoot, &config.Proto.Dir, &config.Proto.GoOut} {
		if !filepath.IsAbs(*value) {
			*value = filepath.Join(config.dir, *value)
		}
	}
	return config.generate()
}
