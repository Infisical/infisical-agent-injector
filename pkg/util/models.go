package util

type InfisicalConfig struct {
	Address                     string       `yaml:"address"`
	ExitAfterAuth               bool         `yaml:"exit-after-auth"`
	RevokeCredentialsOnShutdown bool         `yaml:"revoke-credentials-on-shutdown"`
	RetryConfig                 *RetryConfig `yaml:"retry-strategy,omitempty"`
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
	Type string `yaml:"type"`
}

type Sink struct {
	Type   string      `yaml:"type"`
	Config SinkDetails `yaml:"config"`
}

type SinkDetails struct {
	Path string `yaml:"path"`
}

type PersistentCacheConfig struct {
	Type                    string `yaml:"type"`
	ServiceAccountTokenPath string `yaml:"service-account-token-path"`
	Path                    string `yaml:"path"`
}

type CacheConfig struct {
	Persistent *PersistentCacheConfig `yaml:"persistent,omitempty"`
}

type RetryConfig struct {
	MaxRetries int    `yaml:"max-retries"`
	BaseDelay  string `yaml:"base-delay"`
	MaxDelay   string `yaml:"max-delay"`
}

type AgentConfig struct {
	Infisical InfisicalConfig `yaml:"infisical"`
	Sinks     []Sink          `yaml:"sinks"`
	Templates []Template      `yaml:"templates"`
	Auth      AuthConfig      `yaml:"auth"`
	Cache     CacheConfig     `yaml:"cache,omitempty"`
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
		RetryConfig *RetryConfig `yaml:"retry-strategy,omitempty"`
	} `yaml:"infisical"`
	Templates []Template  `yaml:"templates"`
	Cache     CacheConfig `yaml:"cache,omitempty"`
}

type StartupScriptTemplateData struct {
	ExitAfterAuth  bool
	TimeoutSeconds int
}

type StartupScriptAuth struct {
	Type       string
	IdentityID string
	Username   string
	Password   string
}
