package util

import (
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	InjectAnnotation         = "org.infisical.com/inject"
	InjectModeAnnotation     = "org.infisical.com/inject-mode"
	AnnotationAgentConfigMap = "org.infisical.com/agent-config-map"
	AnnotationAgentStatus    = "org.infisical.com/agent-status"
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
	LinuxContainerImage   = "infisical/cli:0.43.22"               // todo(daniel): we might want to make this configurable in the future
	WindowsContainerImage = "infisical/cli:0.43.22-windows-amd64" // note(daniel): currently only windows amd64 is supported. we throw if the user is trying to use a different architecture on windows.

	InitContainerVolumeMountName    = "infisical-init"
	SidecarContainerVolumeMountName = "infisical-sidecar"
	ContainerAgentConfigVolumeName  = "infisical-agent-config"

	ContainerWorkDirVolumeName             = "infisical-work-dir"
	LinuxContainerWorkDirVolumeMountPath   = "/home/infisical"
	WindowsContainerWorkDirVolumeMountPath = "C:\\infisical"

	// Should probably be removed:
	LinuxInitContainerVolumeMountPath    = "/home/infisical"
	LinuxSidecarContainerVolumeMountPath = "/home/infisical"

	WindowsInitContainerVolumeMountPath    = "C:\\infisical"
	WindowsSidecarContainerVolumeMountPath = "C:\\infisical"

	LinuxContainerAgentConfigVolumeMountPath   = "/home/infisical/config"
	WindowsContainerAgentConfigVolumeMountPath = "C:\\infisical\\config"
)

var (
	PatchOperationAdd    = json.RawMessage(`"add"`)
	PatchOperationRemove = json.RawMessage(`"remove"`)
)
