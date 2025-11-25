package util

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"text/template"
	"time"

	"github.com/Infisical/infisical-agent-injector/pkg/templates"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

type EnvironmentVariable struct {
	Name  string
	Value string
}

func CreateSensitiveEnvironmentVariable(k8s *kubernetes.Clientset, pod *corev1.Pod, envVars []EnvironmentVariable) ([]corev1.EnvVar, error) {

	secretName := fmt.Sprintf("infisical-agent-injector-secret-%s-%s-%d", pod.Namespace, pod.Name, time.Now().Unix())

	envVarsMap := make(map[string]string)
	for _, envVar := range envVars {
		envVarsMap[envVar.Name] = envVar.Value
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: pod.Namespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Pod",
					Name:       pod.Name,
					UID:        pod.UID,
				},
			},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: envVarsMap,
	}

	// Create or update the secret

	// try to get the secret

	_, err := k8s.CoreV1().Secrets(pod.Namespace).Get(context.TODO(), secretName, metav1.GetOptions{})

	// create the secret if it doesn't exist
	if err != nil && errors.IsNotFound(err) {
		_, err := k8s.CoreV1().Secrets(pod.Namespace).Create(context.TODO(), secret, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return nil, fmt.Errorf("failed to create secret: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	environmentVariables := []corev1.EnvVar{}

	for _, envVar := range envVars {

		environmentVariables = append(environmentVariables, corev1.EnvVar{
			Name: envVar.Name,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
					Key: envVar.Name,
				},
			},
		})
	}

	return environmentVariables, nil
}

func BuildAgentConfigFromConfigMap(k8s *kubernetes.Clientset, pod *corev1.Pod, configMap *ConfigMap, exitAfterAuth bool, isWindowsPod bool, injectMode string, cachingEnabled bool, podAnnotations map[string]string) (*AgentConfig, []corev1.EnvVar, error) {

	if configMap == nil {
		return nil, nil, fmt.Errorf("config map is required")
	}

	if injectMode == InjectModeInit && configMap.Infisical.RevokeCredentialsOnShutdown {
		return nil, nil, fmt.Errorf("revoke credentials on shutdown is not supported when inject mode is 'init'")
	}

	var envVars []EnvironmentVariable = []EnvironmentVariable{}

	delimiter := "/"
	if isWindowsPod {
		delimiter = "\\"
	}

	mountPath := LinuxContainerWorkDirVolumeMountPath
	if isWindowsPod {
		mountPath = WindowsContainerWorkDirVolumeMountPath
	}

	if configMap.Infisical.Auth.Type == KubernetesAuthType {

		identityID, ok := configMap.Infisical.Auth.Config["identity-id"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("identity-id is required for kubernetes auth")
		}

		envVars = append(envVars, EnvironmentVariable{
			Name:  "INFISICAL_MACHINE_IDENTITY_ID",
			Value: identityID,
		})

	} else if configMap.Infisical.Auth.Type == LdapAuthType {

		identityID, ok := configMap.Infisical.Auth.Config["identity-id"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("identity-id is required for ldap auth")
		}

		username, ok := configMap.Infisical.Auth.Config["username"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("username is required for ldap auth")
		}

		password, ok := configMap.Infisical.Auth.Config["password"].(string)
		if !ok {
			return nil, nil, fmt.Errorf("password is required for ldap auth")
		}

		envVars = append(
			envVars,
			EnvironmentVariable{
				Name:  "INFISICAL_MACHINE_IDENTITY_ID",
				Value: identityID,
			}, EnvironmentVariable{
				Name:  "INFISICAL_LDAP_USERNAME",
				Value: username,
			}, EnvironmentVariable{
				Name:  "INFISICAL_LDAP_PASSWORD",
				Value: password,
			},
		)

	} else {
		return nil, nil, fmt.Errorf("unsupported auth type: %s", configMap.Infisical.Auth.Type)
	}

	revokeCredentialsOnShutdown := configMap.Infisical.RevokeCredentialsOnShutdown || podAnnotations[AnnotationRevokeCredentialsOnShutdown] == "true"

	// confgirue retry config from annotations or configmap
	retryCfg := &RetryConfig{}

	if podAnnotations[AnnotationAgentClientMaxRetries] != "" {
		annotatedMaxRetries, err := strconv.Atoi(podAnnotations[AnnotationAgentClientMaxRetries])
		if err != nil {
			return nil, nil, fmt.Errorf("failed to parse max retries: %w", err)
		}
		retryCfg.MaxRetries = annotatedMaxRetries
	}

	if podAnnotations[AnnotationAgentClientBaseDelay] != "" {
		retryCfg.BaseDelay = podAnnotations[AnnotationAgentClientBaseDelay]
	}

	if podAnnotations[AnnotationAgentClientMaxDelay] != "" {
		retryCfg.MaxDelay = podAnnotations[AnnotationAgentClientMaxDelay]
	}

	if configMap.Infisical.RetryConfig != nil {

		if configMap.Infisical.RetryConfig.BaseDelay != "" {
			retryCfg.BaseDelay = configMap.Infisical.RetryConfig.BaseDelay
		}
		if configMap.Infisical.RetryConfig.MaxDelay != "" {
			retryCfg.MaxDelay = configMap.Infisical.RetryConfig.MaxDelay
		}
		if configMap.Infisical.RetryConfig.MaxRetries != 0 {
			retryCfg.MaxRetries = configMap.Infisical.RetryConfig.MaxRetries
		}
	}

	agentConfig := &AgentConfig{
		Auth: AuthConfig{
			Type: configMap.Infisical.Auth.Type,
		},
		Infisical: InfisicalConfig{
			Address:                     configMap.Infisical.Address,
			ExitAfterAuth:               exitAfterAuth,
			RevokeCredentialsOnShutdown: revokeCredentialsOnShutdown && !exitAfterAuth, // if set in configmap or annotation. only enable if sidecar container,
			RetryConfig:                 retryCfg,
		},
		Templates: configMap.Templates,
		// we manage the sink files for the user so they won't need to configure this.
		// also makes it easier in terms of volume management.
		Sinks: []Sink{
			{
				Type: "file",
				Config: SinkDetails{
					Path: fmt.Sprintf("%s%sidentity-access-token", mountPath, delimiter),
				},
			},
		},
	}

	if cachingEnabled {
		// allow the user to configure cache themselves if they they dont want the default behavior of using the annotation.
		if configMap.Cache.Persistent != nil {
			agentConfig.Cache = CacheConfig{
				Persistent: &PersistentCacheConfig{
					Type:                    configMap.Cache.Persistent.Type,
					ServiceAccountTokenPath: configMap.Cache.Persistent.ServiceAccountTokenPath,
					Path:                    configMap.Cache.Persistent.Path,
				},
			}
		} else {
			defaultServiceAccountTokenPath := LinuxKubernetesServiceAccountTokenPath
			if isWindowsPod {
				defaultServiceAccountTokenPath = WindowsKubernetesServiceAccountTokenPath
			}

			agentConfig.Cache = CacheConfig{
				Persistent: &PersistentCacheConfig{
					Type:                    "kubernetes",
					ServiceAccountTokenPath: defaultServiceAccountTokenPath,
					Path:                    fmt.Sprintf("%s%scache", mountPath, delimiter),
				},
			}
		}
	}

	environmentVariables, err := CreateSensitiveEnvironmentVariable(k8s, pod, envVars)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create sensitive environment variables: %w", err)
	}

	return agentConfig, environmentVariables, nil
}

func BuildAgentScript(k8s *kubernetes.Clientset, pod *corev1.Pod, configMap *ConfigMap, exitAfterAuth bool, isWindowsPod bool, injectMode string, cachingEnabled bool, podAnnotations map[string]string) (string, []corev1.EnvVar, error) {

	parsedAgentConfig, envVars, err := BuildAgentConfigFromConfigMap(k8s, pod, configMap, exitAfterAuth, isWindowsPod, injectMode, cachingEnabled, podAnnotations)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build agent config: %w", err)
	}

	agentConfigYaml, err := yaml.Marshal(parsedAgentConfig)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal yaml agent config: %w", err)
	}

	base64AgentConfigYaml := base64.StdEncoding.EncodeToString(agentConfigYaml)

	envVars = append(envVars, corev1.EnvVar{
		Name:  "INFISICAL_AGENT_CONFIG_BASE64",
		Value: base64AgentConfigYaml,
	})

	if isWindowsPod {
		windowsScript, err := buildWindowsAgentScript(exitAfterAuth)
		if err != nil {
			return "", nil, fmt.Errorf("failed to build windows agent script: %w", err)
		}
		return windowsScript, envVars, nil
	}
	linuxScript, err := buildLinuxAgentScript(exitAfterAuth)
	if err != nil {
		return "", nil, fmt.Errorf("failed to build linux agent script: %w", err)
	}
	return linuxScript, envVars, nil
}

func buildLinuxAgentScript(exitAfterAuth bool) (string, error) {
	data := StartupScriptTemplateData{
		ExitAfterAuth:  exitAfterAuth,
		TimeoutSeconds: 180,
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

func buildWindowsAgentScript(exitAfterAuth bool) (string, error) {
	data := StartupScriptTemplateData{
		ExitAfterAuth:  exitAfterAuth,
		TimeoutSeconds: 180,
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
