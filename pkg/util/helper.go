package util

import "encoding/json"

func BuildAgentConfigFromConfigMap(configMap *ConfigMap, injectMode string) *AgentConfig {

	agentConfig := &AgentConfig{
		Infisical: InfisicalConfig{
			Address:       configMap.Infisical.Address,
			ExitAfterAuth: injectMode == "init", // Currently only init is supported, but we can expand to sidecar later
		},
		Templates: configMap.Templates,
		Auth: AuthConfig{
			Type: configMap.Infisical.Auth.Type,
			Config: map[string]interface{}{
				// hacky. but we need to get around the file-based identity ID storage somehow
				"identity-id": "/home/infisical/config/identity-id",
			},
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

	return agentConfig
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
