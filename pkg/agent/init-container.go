package agent

import (
	"fmt"
	"strings"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

func getFilePathCreationScript(configMap util.ConfigMap) ([]string, error) {
	// We use heredoc instead of echo to avoid shell escaping issues and to make sure sensitive values aren't exposed in system logs / monitoring
	if configMap.Infisical.Auth.Type == util.KubernetesAuthType {
		authConfig := util.KubernetesAuthConfig{}

		if identityID, exists := configMap.Infisical.Auth.Config["identity-id"]; exists {
			if id, ok := identityID.(string); ok {
				authConfig.IdentityID = id
			}
		}

		if authConfig.IdentityID == "" {
			return []string{}, fmt.Errorf("identity-id is required for kubernetes auth")
		}

		return []string{
			fmt.Sprintf("cat > %s/identity-id << 'EOF'", util.InitContainerAgentConfigVolumeMountPath),
			authConfig.IdentityID,
			"EOF",
			fmt.Sprintf("chmod 600 %s/identity-id", util.InitContainerAgentConfigVolumeMountPath),
		}, nil
	}

	if configMap.Infisical.Auth.Type == util.LdapAuthType {
		authConfig := util.LdapAuthConfig{}

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
			fmt.Sprintf("cat > %s/username << 'EOF'", util.InitContainerAgentConfigVolumeMountPath),
			authConfig.Username,
			"EOF",
			fmt.Sprintf("chmod 600 %s/username", util.InitContainerAgentConfigVolumeMountPath),

			fmt.Sprintf("cat > %s/password << 'EOF'", util.InitContainerAgentConfigVolumeMountPath),
			authConfig.Password,
			"EOF",
			fmt.Sprintf("chmod 600 %s/password", util.InitContainerAgentConfigVolumeMountPath),

			fmt.Sprintf("cat > %s/identity-id << 'EOF'", util.InitContainerAgentConfigVolumeMountPath),
			authConfig.IdentityID,
			"EOF",
			fmt.Sprintf("chmod 600 %s/identity-id", util.InitContainerAgentConfigVolumeMountPath),
		}, nil
	}

	return []string{}, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
}

func (a *Agent) ContainerInitSidecar() (corev1.Container, error) {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      util.InitContainerVolumeMountName,
			MountPath: util.InitContainerVolumeMountPath,
			ReadOnly:  false,
		},
		{
			Name:      a.serviceAccountTokenVolume.Name,
			MountPath: a.serviceAccountTokenVolume.MountPath,
			ReadOnly:  true,
		},
	}
	volumeMounts = append(volumeMounts, a.ContainerVolumeMounts()...)

	volumeMounts = append(volumeMounts, corev1.VolumeMount{
		Name:      util.InitContainerAgentConfigVolumeName,
		MountPath: util.InitContainerAgentConfigVolumeMountPath,
		ReadOnly:  false, // Changed to false to allow writing
	})

	parsedAgentConfig, err := util.BuildAgentConfigFromConfigMap(a.configMap, a.injectMode)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to build agent config: %w", err)
	}

	agentConfigYaml, err := yaml.Marshal(parsedAgentConfig)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to marshal yaml agent config: %w", err)
	}

	filePathCreationScript, err := getFilePathCreationScript(*a.configMap)
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to get file path creation script: %w", err)
	}

	script := []string{
		"#!/bin/sh",
		"set -ex", // -x to trace command execution
		"",
		"# Write config file to volume",
		fmt.Sprintf("cat > %s/agent-config.yaml << 'EOF'", util.InitContainerAgentConfigVolumeMountPath),
		string(agentConfigYaml),
		"EOF",
		"",
		"# Write identity id to volume",
		fmt.Sprintf("mkdir -p %s", util.InitContainerAgentConfigVolumeMountPath),
	}

	script = append(script, filePathCreationScript...)

	script = append(script, []string{
		"",
		"# Run the agent",
		"echo \"Starting infisical agent...\"",
		fmt.Sprintf("timeout 180s infisical agent --config %s/agent-config.yaml || { echo \"Agent failed with exit code $?\"; exit 1; }", util.InitContainerAgentConfigVolumeMountPath),
	}...)

	resources, err := createResources()
	if err != nil {
		return corev1.Container{}, fmt.Errorf("failed to create resources: %w", err)
	}

	newContainer := corev1.Container{
		Name:      util.InitContainerName,
		Image:     util.InitContainerImage,
		Resources: resources,

		VolumeMounts: volumeMounts,
		Command:      []string{"/bin/sh", "-c"},
		Args:         []string{strings.Join(script, "\n")},
	}

	return newContainer, nil

}

func createResources() (corev1.ResourceRequirements, error) {
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
