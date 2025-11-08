package util

type InfisicalConfig struct {
	Address                     string `yaml:"address"`
	ExitAfterAuth               bool   `yaml:"exit-after-auth"`
	RevokeCredentialsOnShutdown bool   `yaml:"revoke-credentials-on-shutdown"`
}

type Template struct {
	SourcePath            string `yaml:"source-path"`
	Base64TemplateContent string `yaml:"base64-template-content"`
	DestinationPath       string `yaml:"destination-path"`
	TemplateContent       string `yaml:"template-content"`

	Config struct { // Configurations for the template
		PollingInterval string `yaml:"polling-interval"` // How often to poll for changes in the secret
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

type KubernetesAuthConfig struct {
	IdentityID string `yaml:"identity-id"`
}

type LdapAuthConfig struct {
	Username   string `yaml:"username"`
	Password   string `yaml:"password"`
	IdentityID string `yaml:"identity-id"`
}

type ConfigMap struct {
	Infisical struct {
		Address                     string `yaml:"address"`
		RevokeCredentialsOnShutdown bool   `yaml:"revoke-credentials-on-shutdown"`
		Auth                        struct {
			Type   string                 `yaml:"type"` // Only kubernetes and ldap-auth is supported for now
			Config map[string]interface{} `yaml:"config"`
		} `yaml:"auth"`
	} `yaml:"infisical"`
	Templates []Template `yaml:"templates"`
}

type StartupScriptTemplateData struct {
	ConfigPath      string
	AgentConfigYaml string
	ExitAfterAuth   bool
	TimeoutSeconds  int
	Auth            StartupScriptAuth
}

type StartupScriptAuth struct {
	Type       string
	IdentityID string
	Username   string
	Password   string
}
