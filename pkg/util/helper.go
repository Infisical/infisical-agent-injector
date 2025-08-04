package util

import (
	"encoding/json"
	"fmt"
)

func BuildAgentConfigFromConfigMap(configMap *ConfigMap, injectMode string) (*AgentConfig, error) {

	var authConfig map[string]interface{} = map[string]interface{}{}

	if configMap.Infisical.Auth.Type == KubernetesAuthType {
		authConfig = map[string]interface{}{
			// hacky. but we need to get around the file-based identity ID storage somehow
			"identity-id": "/home/infisical/config/identity-id",
		}
	} else if configMap.Infisical.Auth.Type == LdapAuthType {
		authConfig = map[string]interface{}{
			"identity-id": "/home/infisical/config/identity-id",
			"username":    "/home/infisical/config/username",
			"password":    "/home/infisical/config/password",
		}
	} else {
		return nil, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
	}

	agentConfig := &AgentConfig{
		Infisical: InfisicalConfig{
			Address:       configMap.Infisical.Address,
			ExitAfterAuth: injectMode == "init", // Currently only init is supported, but we can expand to sidecar later
		},
		Templates: configMap.Templates,
		Auth: AuthConfig{
			Type:   configMap.Infisical.Auth.Type,
			Config: authConfig,
		},
		Sinks: []Sink{
			{
				Type: "file",
				Config: SinkDetails{
					Path: "/home/infisical/config/identity-access-token",
				},
			},
		},
	}

	return agentConfig, nil
}

func PrettyPrintJSON(data []byte) string {
	var obj interface{}
	err := json.Unmarshal(data, &obj)
	if err != nil {
		// if we can't parse it as JSON, just return the original string
		return string(data)
	}

	prettyJSON, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		// if re-marshaling fails, return the original string
		return string(data)
	}

	return string(prettyJSON)
}
