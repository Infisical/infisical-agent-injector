package util

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	InjectAnnotation                      = "org.infisical.com/inject"
	InjectModeAnnotation                  = "org.infisical.com/inject-mode"
	AnnotationAgentConfigMap              = "org.infisical.com/agent-config-map"
	AnnotationAgentStatus                 = "org.infisical.com/agent-status"
	AnnotationCachingEnabled              = "org.infisical.com/agent-cache-enabled"
	AnnotationRevokeCredentialsOnShutdown = "org.infisical.com/agent-revoke-on-shutdown"

	AnnotationAgentClientMaxRetries = "org.infisical.com/agent-client-max-retries"
	AnnotationAgentClientBaseDelay  = "org.infisical.com/agent-client-base-delay"
	AnnotationAgentClientMaxDelay   = "org.infisical.com/agent-client-max-delay"

	AnnotationLimitsCPU       = "org.infisical.com/agent-limits-cpu"
	AnnotationLimitsMemory    = "org.infisical.com/agent-limits-memory"
	AnnotationLimitsEphemeral = "org.infisical.com/agent-limits-ephemeral"

	AnnotationRequestsCPU       = "org.infisical.com/agent-requests-cpu"
	AnnotationRequestsMemory    = "org.infisical.com/agent-requests-memory"
	AnnotationRequestsEphemeral = "org.infisical.com/agent-requests-ephemeral"
)

var KubeSystemNamespaces = []string{
	metav1.NamespaceSystem,
	metav1.NamespacePublic,
}

const (
	DefaultDestinationPath                 = "/shared/infisical-secrets"
	AccessTokenSinkFileDestinationFileName = "/identity-access-token"
)

const (
	InjectModeInit        = "init"
	InjectModeSidecar     = "sidecar"
	InjectModeSidecarInit = "sidecar-init"
)

const (
	KubernetesAuthType = "kubernetes"
	LdapAuthType       = "ldap-auth"
)

const (
	InitContainerName     = "infisical-agent-init"
	SidecarContainerName  = "infisical-agent"
	LinuxContainerImage   = "infisical/cli:0.43.32"               // todo(daniel): we might want to make this configurable in the future
	WindowsContainerImage = "infisical/cli:0.43.32-windows-amd64" // note(daniel): currently only windows amd64 is supported. we throw if the user is trying to use a different architecture on windows.

	ContainerAgentConfigVolumeName = "infisical-agent-config"

	ContainerWorkDirMountName              = "infisical-work-dir"
	LinuxContainerWorkDirVolumeMountPath   = "/home/.infisical-workdir"
	WindowsContainerWorkDirVolumeMountPath = "C:\\.infisical-workdir"

	LinuxKubernetesServiceAccountTokenPath   = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	WindowsKubernetesServiceAccountTokenPath = "C:\\var\\run\\secrets\\kubernetes.io\\serviceaccount\\token"
)

var (
	PatchOperationAdd    = json.RawMessage(`"add"`)
	PatchOperationRemove = json.RawMessage(`"remove"`)
)
