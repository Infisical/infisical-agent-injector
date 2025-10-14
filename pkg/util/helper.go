package util

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

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

func BuildAgentConfigFromConfigMap(configMap *ConfigMap, injectMode string, isWindowsPod bool) (*AgentConfig, error) {

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
					Path: fmt.Sprintf("%s%sidentity-access-token", mountPath, delimiter),
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

func GetFilePathCreationScriptLinux(configMap ConfigMap) ([]string, error) {
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
			fmt.Sprintf("cat > %s/identity-id << 'EOF'", LinuxContainerAgentConfigVolumeMountPath),
			authConfig.IdentityID,
			"EOF",
			fmt.Sprintf("chmod 600 %s/identity-id", LinuxContainerAgentConfigVolumeMountPath),
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
			fmt.Sprintf("cat > %s/username << 'EOF'", LinuxContainerAgentConfigVolumeMountPath),
			authConfig.Username,
			"EOF",
			fmt.Sprintf("chmod 600 %s/username", LinuxContainerAgentConfigVolumeMountPath),

			fmt.Sprintf("cat > %s/password << 'EOF'", LinuxContainerAgentConfigVolumeMountPath),
			authConfig.Password,
			"EOF",
			fmt.Sprintf("chmod 600 %s/password", LinuxContainerAgentConfigVolumeMountPath),

			fmt.Sprintf("cat > %s/identity-id << 'EOF'", LinuxContainerAgentConfigVolumeMountPath),
			authConfig.IdentityID,
			"EOF",
			fmt.Sprintf("chmod 600 %s/identity-id", LinuxContainerAgentConfigVolumeMountPath),
		}, nil
	}

	return []string{}, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
}

func GetFilePathCreationScriptWindows(configMap ConfigMap) ([]string, error) {
	escapeForPowerShell := func(s string) string {
		s = strings.ReplaceAll(s, "'", "''") // single quotes need doubling
		s = strings.ReplaceAll(s, "`", "``") // backticks need doubling
		return s
	}

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

		// escape any backticks or quotes in the content
		escapedIdentityID := escapeForPowerShell(authConfig.IdentityID)

		return []string{
			// write identity ID and set permissions:
			fmt.Sprintf("'%s' | Out-File -FilePath '%s\\identity-id' -Encoding UTF8 -NoNewline", escapedIdentityID, WindowsContainerAgentConfigVolumeMountPath),
			fmt.Sprintf("icacls '%s\\identity-id' /grant Everyone:F | Out-Null", WindowsContainerAgentConfigVolumeMountPath),
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

		escapedUsername := escapeForPowerShell(authConfig.Username)
		escapedPassword := escapeForPowerShell(authConfig.Password)
		escapedIdentityID := escapeForPowerShell(authConfig.IdentityID)

		return []string{
			// write username and set permissions:
			fmt.Sprintf("'%s' | Out-File -FilePath '%s\\username' -Encoding UTF8 -NoNewline", escapedUsername, WindowsContainerAgentConfigVolumeMountPath),
			fmt.Sprintf("icacls '%s\\username' /grant Everyone:F | Out-Null", WindowsContainerAgentConfigVolumeMountPath),
			"",
			// write password and set permissions:
			fmt.Sprintf("'%s' | Out-File -FilePath '%s\\password' -Encoding UTF8 -NoNewline", escapedPassword, WindowsContainerAgentConfigVolumeMountPath),
			fmt.Sprintf("icacls '%s\\password' /grant Everyone:F | Out-Null", WindowsContainerAgentConfigVolumeMountPath),
			"",
			// write identity ID and set permissions:
			fmt.Sprintf("'%s' | Out-File -FilePath '%s\\identity-id' -Encoding UTF8 -NoNewline", escapedIdentityID, WindowsContainerAgentConfigVolumeMountPath),
			fmt.Sprintf("icacls '%s\\identity-id' /grant Everyone:F | Out-Null", WindowsContainerAgentConfigVolumeMountPath),
		}, nil
	}

	return []string{}, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
}

func BuildAgentScript(configMap ConfigMap, injectMode string, isWindowsPod bool) (string, error) {
	parsedAgentConfig, err := BuildAgentConfigFromConfigMap(&configMap, injectMode, isWindowsPod)
	if err != nil {
		return "", fmt.Errorf("failed to build agent config: %w", err)
	}

	agentConfigYaml, err := yaml.Marshal(parsedAgentConfig)
	if err != nil {
		return "", fmt.Errorf("failed to marshal yaml agent config: %w", err)
	}

	if isWindowsPod {
		return buildWindowsAgentScript(configMap, injectMode, string(agentConfigYaml))
	}
	return buildLinuxAgentScript(configMap, injectMode, string(agentConfigYaml))
}

func buildLinuxAgentScript(configMap ConfigMap, injectMode string, agentConfigYaml string) (string, error) {
	filePathCreationScript, err := GetFilePathCreationScriptLinux(configMap)
	if err != nil {
		return "", fmt.Errorf("failed to get file path creation script: %w", err)
	}

	script := []string{
		"#!/bin/sh",
		"set -ex",
		"",
		fmt.Sprintf("cat > %s/agent-config.yaml << 'EOF'", LinuxContainerAgentConfigVolumeMountPath), // writes config file to volume
		agentConfigYaml,
		"EOF",
		"",
		fmt.Sprintf("mkdir -p %s", LinuxContainerAgentConfigVolumeMountPath),
	}

	script = append(script, filePathCreationScript...)

	script = append(script, []string{
		"",
		"echo \"Starting infisical agent...\"",
	}...)

	if injectMode == InjectModeInit {
		script = append(script, []string{
			fmt.Sprintf("timeout 180s infisical agent --config %s/agent-config.yaml || { echo \"Agent failed with exit code $?\"; exit 1; }", LinuxContainerAgentConfigVolumeMountPath),
		}...)
	} else {
		script = append(script, []string{
			fmt.Sprintf("infisical agent --config %s/agent-config.yaml", LinuxContainerAgentConfigVolumeMountPath),
		}...)
	}

	return strings.Join(script, "\n"), nil
}

func buildWindowsAgentScript(configMap ConfigMap, injectMode string, agentConfigYaml string) (string, error) {
	filePathCreationScript, err := GetFilePathCreationScriptWindows(configMap)
	if err != nil {
		return "", fmt.Errorf("failed to get file path creation script: %w", err)
	}

	configPath := WindowsContainerAgentConfigVolumeMountPath

	// powerShell here-string for multiline content - need to escape any @ symbols in yaml or we get parsing errors :-(
	escapedYaml := strings.ReplaceAll(agentConfigYaml, "@", "``@")

	script := []string{
		"$ErrorActionPreference = 'Stop'",
		"",
		"$configContent = @\"", // writes config file to volume
		escapedYaml,
		"\"@",
		fmt.Sprintf("New-Item -ItemType Directory -Force -Path '%s' | Out-Null", configPath),
		fmt.Sprintf("$configContent | Out-File -FilePath '%s\\agent-config.yaml' -Encoding UTF8 -NoNewline", configPath),
		"",
	}

	script = append(script, filePathCreationScript...)

	script = append(script, []string{
		"",
		"Write-Host 'Starting infisical agent...'",
		"",
	}...)

	if injectMode == InjectModeInit {
		script = append(script, []string{
			"$timeoutSeconds = 180",
			fmt.Sprintf("$process = Start-Process -FilePath 'infisical.exe' -ArgumentList 'agent','--config','%s\\agent-config.yaml' -NoNewWindow -PassThru -Wait:$false", configPath),
			"$finished = $process.WaitForExit($timeoutSeconds * 1000)",
			"if (-not $finished) {",
			"    $process.Kill()",
			"    Remove-Variable process",
			"    Write-Error \"Agent timed out after $timeoutSeconds seconds\"",
			"    exit 1",
			"}",
			// give the process a moment to fully terminate or the exit code might not be set and cause an error even if the agent exited successfully
			"Start-Sleep -Milliseconds 1000",
			"$exitCode = $process.ExitCode",
			"Remove-Variable process",
			"if ($null -ne $exitCode -and $exitCode -ne 0) {",
			"    Write-Error \"Agent failed with exit code $exitCode\"",
			"    exit $exitCode",
			"}",
		}...)
	} else {
		script = append(script, []string{
			fmt.Sprintf("$process = Start-Process -FilePath 'infisical.exe' -ArgumentList 'agent','--config','%s\\agent-config.yaml' -NoNewWindow -PassThru -Wait", configPath),
			"if ($process.ExitCode -ne 0) {",
			"    Write-Error \"Agent failed with exit code $($process.ExitCode)\"",
			"    exit $process.ExitCode",
			"}",
		}...)
	}

	return strings.Join(script, "\r\n"), nil
}
