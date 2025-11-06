package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/Infisical/infisical-agent-injector/pkg/templates"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

func IsWindowsPod(pod *corev1.Pod) bool {
	// check the OS field (supported in k8s 1.25+)
	if pod.Spec.OS != nil && pod.Spec.OS.Name == "windows" {
		return true
	}

	// check node selector for windows (supported in k8s 1.6+)
	if pod.Spec.NodeSelector != nil {
		if os, exists := pod.Spec.NodeSelector["kubernetes.io/os"]; exists && os == "windows" {
			return true
		}
		// also check legacy label (removed in k8s 1.18 and later)
		if os, exists := pod.Spec.NodeSelector["beta.kubernetes.io/os"]; exists && os == "windows" {
			return true
		}
	}

	// last resort: check node affinity for Windows requirements, shouldn't fail as this has been around for a while (beta since 1.6, available in all newer versions)
	// checks the pod specification for which node the pod can run on.
	// example:
	/*
		affinity:
		nodeAffinity:
			requiredDuringSchedulingIgnoredDuringExecution:
				nodeSelectorTerms:
				- matchExpressions:
					- key: kubernetes.io/os
						operator: In
						values:
						- windows
	*/
	if pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil && pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution != nil {
		for _, nodeSelectorTerm := range pod.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {

			for _, expression := range nodeSelectorTerm.MatchExpressions {

				if (expression.Key == "kubernetes.io/os" || expression.Key == "beta.kubernetes.io/os") && expression.Operator == corev1.NodeSelectorOpIn {

					for _, value := range expression.Values {

						if value == "windows" {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

func BuildAgentConfigFromConfigMap(configMap *ConfigMap, exitAfterAuth bool, isWindowsPod bool) (*AgentConfig, error) {

	mountPath := LinuxContainerAgentConfigVolumeMountPath
	if isWindowsPod {
		mountPath = WindowsContainerAgentConfigVolumeMountPath
	}

	var authConfig map[string]interface{} = map[string]interface{}{}

	delimiter := "/"
	if isWindowsPod {
		delimiter = "\\"
	}

	if configMap.Infisical.Auth.Type == KubernetesAuthType {
		authConfig = map[string]interface{}{
			// hacky. but we need to get around the file-based identity ID storage somehow
			"identity-id": fmt.Sprintf("%s%sidentity-id", mountPath, delimiter),
		}
	} else if configMap.Infisical.Auth.Type == LdapAuthType {
		authConfig = map[string]interface{}{
			"identity-id": fmt.Sprintf("%s%sidentity-id", mountPath, delimiter),
			"username":    fmt.Sprintf("%s%susername", mountPath, delimiter),
			"password":    fmt.Sprintf("%s%spassword", mountPath, delimiter),
		}
	} else {
		return nil, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
	}

	agentConfig := &AgentConfig{
		Infisical: InfisicalConfig{
			Address:       configMap.Infisical.Address,
			ExitAfterAuth: exitAfterAuth,
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
					Path: fmt.Sprintf("%s%sidentity-access-token", mountPath, delimiter),
				},
			},
		},
	}

	return agentConfig, nil
}

func getAgentAuthDataFromConfigMap(configMap ConfigMap, isWindowsPod bool) (StartupScriptAuth, error) {
	escapeForPowerShell := func(s string) string {
		s = strings.ReplaceAll(s, "'", "''") // single quotes need doubling
		s = strings.ReplaceAll(s, "`", "``") // backticks need doubling
		return s
	}

	data := StartupScriptAuth{}

	// Extract and escape auth config based on type
	if configMap.Infisical.Auth.Type == KubernetesAuthType {
		if identityID, ok := configMap.Infisical.Auth.Config["identity-id"].(string); ok {
			data.IdentityID = identityID
		}
		if data.IdentityID == "" {
			return StartupScriptAuth{}, fmt.Errorf("identity-id is required for kubernetes auth")
		}

		data.Type = KubernetesAuthType
		if isWindowsPod {
			data.IdentityID = escapeForPowerShell(data.IdentityID)
		}
	} else if configMap.Infisical.Auth.Type == LdapAuthType {
		if identityID, ok := configMap.Infisical.Auth.Config["identity-id"].(string); ok {
			data.IdentityID = identityID
		}
		if username, ok := configMap.Infisical.Auth.Config["username"].(string); ok {
			data.Username = username
		}
		if password, ok := configMap.Infisical.Auth.Config["password"].(string); ok {
			data.Password = password
		}

		if data.IdentityID == "" || data.Username == "" || data.Password == "" {
			return StartupScriptAuth{}, fmt.Errorf("identity-id, username, and password are required for ldap auth")
		}

		data.Type = LdapAuthType
		if isWindowsPod {
			data.IdentityID = escapeForPowerShell(data.IdentityID)
			data.Username = escapeForPowerShell(data.Username)
			data.Password = escapeForPowerShell(data.Password)
		}
	}

	if data.Type == "" {
		return StartupScriptAuth{}, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
	}

	return data, nil
}

func BuildAgentScript(configMap ConfigMap, exitAfterAuth bool, isWindowsPod bool) (string, error) {

	parsedAgentConfig, err := BuildAgentConfigFromConfigMap(&configMap, exitAfterAuth, isWindowsPod)
	if err != nil {
		return "", fmt.Errorf("failed to build agent config: %w", err)
	}

	agentConfigYaml, err := yaml.Marshal(parsedAgentConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal yaml agent config: %w", err)
	}

	authData, err := getAgentAuthDataFromConfigMap(configMap, isWindowsPod)
	if err != nil {
		return "", fmt.Errorf("failed to get auth data: %w", err)
	}

	if isWindowsPod {
		return buildWindowsAgentScript(exitAfterAuth, string(agentConfigYaml), authData)
	}
	return buildLinuxAgentScript(exitAfterAuth, string(agentConfigYaml), authData)
}

func buildLinuxAgentScript(exitAfterAuth bool, agentConfigYaml string, authData StartupScriptAuth) (string, error) {
	data := StartupScriptTemplateData{
		ConfigPath:      LinuxContainerAgentConfigVolumeMountPath,
		AgentConfigYaml: agentConfigYaml,
		ExitAfterAuth:   exitAfterAuth,
		TimeoutSeconds:  180,
		Auth:            authData,
	}

	tmpl, err := template.ParseFS(templates.TemplatesFS, "linux-container-startup.sh.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func buildWindowsAgentScript(exitAfterAuth bool, agentConfigYaml string, authData StartupScriptAuth) (string, error) {
	// escape @ symbols for PowerShell here-strings
	escapedYaml := strings.ReplaceAll(agentConfigYaml, "@", "``@")

	data := StartupScriptTemplateData{
		ConfigPath:      WindowsContainerAgentConfigVolumeMountPath,
		AgentConfigYaml: escapedYaml,
		ExitAfterAuth:   exitAfterAuth,
		TimeoutSeconds:  180,
		Auth:            authData,
	}

	tmpl, err := template.ParseFS(templates.TemplatesFS, "windows-container-startup.ps1.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

func ValidateInjectMode(injectMode string) error {
	if injectMode != InjectModeSidecarInit && injectMode != InjectModeInit && injectMode != InjectModeSidecar {
		return fmt.Errorf("inject mode %s not supported. please use %s, %s, or %s", injectMode, InjectModeInit, InjectModeSidecar, InjectModeSidecarInit)
	}
	return nil
}
