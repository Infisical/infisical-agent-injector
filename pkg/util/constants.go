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
	InjectModeInit = "init"
)

const (
	KubernetesAuthType = "kubernetes"
	LdapAuthType       = "ldap-auth"
)

const (
	InitContainerName            = "infisical-agent-init"
	InitContainerImage           = "infisical/cli:0.41.96" // todo(daniel): we might want to make this configurable in the future
	InitContainerVolumeMountName = "infisical-init"
	InitContainerVolumeMountPath = "/home/infisical"

	InitContainerAgentConfigVolumeName      = "infisical-agent-config"
	InitContainerAgentConfigVolumeMountPath = "/home/infisical/config"
)

var (
	PatchOperationAdd    = json.RawMessage(`"add"`)
	PatchOperationRemove = json.RawMessage(`"remove"`)
)
