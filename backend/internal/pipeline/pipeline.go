package pipeline

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

func Parse(r io.Reader) (Config, error) {
	var cfg Config

	decoder := yaml.NewDecoder(r)

	decoder.KnownFields(true)

	decodeErr := decoder.Decode(&cfg)

	if decodeErr != nil {
		return Config{}, fmt.Errorf("decode pipeline: %w", decodeErr)
	}

	var extra yaml.Node

	decodeExtraErr := decoder.Decode(&extra)

	if decodeExtraErr == nil {
		return Config{}, fmt.Errorf("pipeline must contain exactly one YAML document")
	}

	if !errors.Is(decodeExtraErr, io.EOF) {
		return Config{}, fmt.Errorf("decode trailing content: %w", decodeExtraErr)
	}

	validateErr := cfg.Validate()

	if validateErr != nil {
		return Config{}, fmt.Errorf("validate pipeline: %w", validateErr)
	}

	return cfg, nil
}

func (cfg Config) Validate() error {
	if strings.TrimSpace(cfg.Image) == "" {
		return fmt.Errorf("image is required")
	}

	if cfg.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeoutSeconds must be greater than zero")
	}

	if len(cfg.Steps) < 1 {
		return fmt.Errorf("steps must contain at least one step")
	}

	for i, step := range cfg.Steps {
		if strings.TrimSpace(step.Name) == "" {
			return fmt.Errorf("step %d: name is required", i+1)
		}

		if strings.TrimSpace(step.Command) == "" {
			return fmt.Errorf("step %d: command is required", i+1)
		}
	}

	return nil
}
