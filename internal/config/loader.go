package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Load reads the first accessible YAML file from paths and unmarshals it into dst.
// Returns an error if none of the paths can be read.
func Load(dst any, paths ...string) error {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, dst); err != nil {
			return fmt.Errorf("parse yaml %s: %w", p, err)
		}
		return nil
	}
	return fmt.Errorf("no config file found in: %v", paths)
}
