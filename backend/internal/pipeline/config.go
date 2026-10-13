package pipeline

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Image          string  `yaml:"image"`
	TimeoutSeconds Seconds `yaml:"timeoutSeconds"`
	Steps          []Step  `yaml:"steps"`
}

type Step struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

type Seconds int

func (s *Seconds) UnmarshalYAML(node *yaml.Node) error {
	if node.ShortTag() != "!!int" {
		return fmt.Errorf("timeoutSeconds must be an integer")
	}

	return node.Decode((*int)(s))
}
