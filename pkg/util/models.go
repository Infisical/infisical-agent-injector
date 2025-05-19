package util

type InfisicalConfig struct {
	Address       string `yaml:"address"`
	ExitAfterAuth bool   `yaml:"exit-after-auth"`
}

type Template struct {
	SourcePath            string `yaml:"source-path"`
	Base64TemplateContent string `yaml:"base64-template-content"`
	DestinationPath       string `yaml:"destination-path"`
	TemplateContent       string `yaml:"template-content"`

	Config struct { // Configurations for the template
		PollingInterval string `yaml:"polling-interval"` // How often to poll for changes in the secret
		Execute         struct {
			Command string `yaml:"command"` // Command to execute once the template has been rendered
			Timeout int64  `yaml:"timeout"` // Timeout for the command
		} `yaml:"execute"` // Command to execute once the template has been rendered
	} `yaml:"config"`
}

type AuthConfig struct {
	Type   string      `yaml:"type"`
	Config interface{} `yaml:"config"`
}

type Sink struct {
	Type   string      `yaml:"type"`
	Config SinkDetails `yaml:"config"`
}

type SinkDetails struct {
	Path string `yaml:"path"`
}

type AgentConfig struct {
	Infisical InfisicalConfig `yaml:"infisical"`
	Auth      AuthConfig      `yaml:"auth"`
	Sinks     []Sink          `yaml:"sinks"`
	Templates []Template      `yaml:"templates"`
}

type ConfigMap struct {
	Infisical struct {
		Address string `yaml:"address"`
		Auth    struct {
			Type   string `yaml:"type"` // Only kubernetes is supported for now
			Config struct {
				IdentityID string `yaml:"identity-id"`
			} `yaml:"config"`
		} `yaml:"auth"`
	} `yaml:"infisical"`
	Templates []Template `yaml:"templates"`
}
