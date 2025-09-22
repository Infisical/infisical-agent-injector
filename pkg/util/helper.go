package util

import (
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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
			ExitAfterAuth: injectMode == InjectModeInit,
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

func GetFilePathCreationScript(configMap ConfigMap) ([]string, error) {
	// We use heredoc instead of echo to avoid shell escaping issues and to make sure sensitive values aren't exposed in system logs / monitoring
	if configMap.Infisical.Auth.Type == KubernetesAuthType {
		authConfig := KubernetesAuthConfig{}

		if identityID, exists := configMap.Infisical.Auth.Config["identity-id"]; exists {
			if id, ok := identityID.(string); ok {
				authConfig.IdentityID = id
			}
		}

		if authConfig.IdentityID == "" {
			return []string{}, fmt.Errorf("identity-id is required for kubernetes auth")
		}

		return []string{
			fmt.Sprintf("cat > %s/identity-id << 'EOF'", ContainerAgentConfigVolumeMountPath),
			authConfig.IdentityID,
			"EOF",
			fmt.Sprintf("chmod 600 %s/identity-id", ContainerAgentConfigVolumeMountPath),
		}, nil
	}

	if configMap.Infisical.Auth.Type == LdapAuthType {
		authConfig := LdapAuthConfig{}

		if identityID, exists := configMap.Infisical.Auth.Config["identity-id"]; exists {
			if id, ok := identityID.(string); ok {
				authConfig.IdentityID = id
			}
		}
		if username, exists := configMap.Infisical.Auth.Config["username"]; exists {
			if username, ok := username.(string); ok {
				authConfig.Username = username
			}
		}
		if password, exists := configMap.Infisical.Auth.Config["password"]; exists {
			if password, ok := password.(string); ok {
				authConfig.Password = password
			}
		}

		if authConfig.IdentityID == "" || authConfig.Username == "" || authConfig.Password == "" {
			return []string{}, fmt.Errorf("identity-id, username, and password are required for ldap auth")
		}

		return []string{
			fmt.Sprintf("cat > %s/username << 'EOF'", ContainerAgentConfigVolumeMountPath),
			authConfig.Username,
			"EOF",
			fmt.Sprintf("chmod 600 %s/username", ContainerAgentConfigVolumeMountPath),

			fmt.Sprintf("cat > %s/password << 'EOF'", ContainerAgentConfigVolumeMountPath),
			authConfig.Password,
			"EOF",
			fmt.Sprintf("chmod 600 %s/password", ContainerAgentConfigVolumeMountPath),

			fmt.Sprintf("cat > %s/identity-id << 'EOF'", ContainerAgentConfigVolumeMountPath),
			authConfig.IdentityID,
			"EOF",
			fmt.Sprintf("chmod 600 %s/identity-id", ContainerAgentConfigVolumeMountPath),
		}, nil
	}

	return []string{}, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
}

func CreateDefaultResources() (corev1.ResourceRequirements, error) {
	// currently has the same resources and limits as the infisical gateway
	// we might want to make this configurable in the future
	resources := corev1.ResourceRequirements{}

	// create the limits
	limits := corev1.ResourceList{}
	cpuLimit, err := resource.ParseQuantity("500m")
	if err != nil {
		return resources, fmt.Errorf("failed to parse CPU limit: %w", err)
	}
	memoryLimit, err := resource.ParseQuantity("128Mi")
	if err != nil {
		return resources, fmt.Errorf("failed to parse memory limit: %w", err)
	}
	limits[corev1.ResourceCPU] = cpuLimit
	limits[corev1.ResourceMemory] = memoryLimit

	// create the requests
	requests := corev1.ResourceList{}
	cpuRequest, err := resource.ParseQuantity("100m")
	if err != nil {
		return resources, fmt.Errorf("failed to parse CPU request: %w", err)
	}
	memoryRequest, err := resource.ParseQuantity("128Mi")
	if err != nil {
		return resources, fmt.Errorf("failed to parse memory request: %w", err)
	}
	requests[corev1.ResourceCPU] = cpuRequest
	requests[corev1.ResourceMemory] = memoryRequest

	// set the limits and requests on the resource requirements
	resources.Limits = limits
	resources.Requests = requests

	return resources, nil
}
