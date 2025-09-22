package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	jsonpatch "github.com/evanphx/json-patch"
	corev1 "k8s.io/api/core/v1"
)

type Agent struct {
	pod                       *corev1.Pod
	configMap                 *util.ConfigMap
	serviceAccountTokenVolume *ServiceAccountTokenVolume
	injectMode                string
}

func NewAgent(pod *corev1.Pod, configMap *util.ConfigMap) (*Agent, error) {

	injectMode := pod.Annotations[util.InjectModeAnnotation]
	if injectMode == "" {
		injectMode = util.InjectModeInit
	}

	serviceAccountTokenVolume, err := getServiceAccountTokenVolume(pod)
	if err != nil {
		return nil, err
	}

	if configMap.Infisical.Address == "" {
		configMap.Infisical.Address = "https://app.infisical.com"
	}

	if len(configMap.Templates) == 0 {
		return nil, fmt.Errorf("no templates found in config map")
	}
	templateCount := len(configMap.Templates)
	for i, template := range configMap.Templates {
		if template.DestinationPath == "" {
			if templateCount > 1 {
				configMap.Templates[i].DestinationPath = fmt.Sprintf("%s-%d", util.DefaultDestinationPath, i+1)
			} else {
				configMap.Templates[i].DestinationPath = util.DefaultDestinationPath
			}
		}
	}

	return &Agent{
		pod:                       pod,
		configMap:                 configMap,
		serviceAccountTokenVolume: serviceAccountTokenVolume,
		injectMode:                injectMode,
	}, nil
}

func (a *Agent) ContainerVolumeMounts() []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{}

	templateCount := len(a.configMap.Templates)

	for i, template := range a.configMap.Templates {

		destinationPath := strings.Join(strings.Split(template.DestinationPath, "/")[:len(strings.Split(template.DestinationPath, "/"))-1], "/")

		var name string
		if templateCount > 1 {
			name = fmt.Sprintf("infisical-secrets-%d", i+1)
		} else {
			name = "infisical-secrets"
		}

		// check if the volume mounts already have a path that matches the destination path
		alreadyExists := false
		for _, volumeMount := range volumeMounts {
			if volumeMount.MountPath == destinationPath {
				alreadyExists = true
				break
			}
		}

		if alreadyExists {
			log.Printf("volume mount %s already exists at %s. skipping creation.", name, destinationPath)
			continue
		}

		log.Printf("adding volume mount %s to %s", name, destinationPath)

		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      name,
			MountPath: destinationPath,
			ReadOnly:  false,
		})
	}
	return volumeMounts
}

func (a *Agent) ContainerAgentConfigVolume() corev1.Volume {
	return corev1.Volume{
		Name: util.ContainerAgentConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func (a *Agent) ValidateConfigMap() error {

	// currnetly only init mode is supported
	if a.injectMode != util.InjectModeInit && a.injectMode != util.InjectModeSidecar {
		return fmt.Errorf("inject mode %s not supported. please use %s or %s", a.injectMode, util.InjectModeInit, util.InjectModeSidecar)
	}

	// check that the config map has a valid auth config
	if a.configMap.Infisical.Auth.Type == "" {
		return fmt.Errorf("auth type is required")
	}

	if a.configMap.Infisical.Auth.Type != util.KubernetesAuthType && a.configMap.Infisical.Auth.Type != util.LdapAuthType {
		return fmt.Errorf("auth type %s not supported. please use %s or %s", a.configMap.Infisical.Auth.Type, util.KubernetesAuthType, util.LdapAuthType)
	}

	// redundant if statement check, but will make sense when we add more authentication methods
	if a.configMap.Infisical.Auth.Type == util.KubernetesAuthType {

		if a.serviceAccountTokenVolume == nil {
			return fmt.Errorf("service account token volume is required")
		}

		if a.serviceAccountTokenVolume.Name == "" {
			return fmt.Errorf("service account token volume name is required")
		}

		if a.serviceAccountTokenVolume.MountPath == "" {
			return fmt.Errorf("service account token volume mount path is required")
		}

		if a.serviceAccountTokenVolume.TokenPath == "" {
			return fmt.Errorf("service account token volume token path is required")
		}
	}

	for _, template := range a.configMap.Templates {

		if template.DestinationPath == "" {
			return fmt.Errorf("template destination path is required")
		}

		// check if the path starts with a /, if it doesn't throw
		if !strings.HasPrefix(template.DestinationPath, "/") {
			return fmt.Errorf("template destination path must be absolute and start with a slash (e.g. /path/to/destination/secret-file)")
		}

		// ensure that the destination path is a folder
		slashCount := strings.Count(template.DestinationPath, "/")
		if slashCount < 2 {
			return fmt.Errorf("template destination path must be a folder (e.g. /path/to/destination/secret-file)")
		}
	}

	return nil

}

func (a *Agent) PatchPod() ([]byte, error) {
	var podPatches jsonpatch.Patch

	// 1. add volume mounts that will hold the secrets
	for i, container := range a.pod.Spec.Containers {
		podPatches = append(podPatches, addVolumeMounts(
			container.VolumeMounts,
			a.ContainerVolumeMounts(),
			fmt.Sprintf("/spec/containers/%d/volumeMounts", i))...)
	}

	requiredVolumeName := ""

	if a.injectMode == util.InjectModeInit {
		requiredVolumeName = util.InitContainerVolumeMountName
	} else if a.injectMode == util.InjectModeSidecar {
		requiredVolumeName = util.SidecarContainerVolumeMountName
	} else {
		return nil, fmt.Errorf("unknown inject mode: %s", a.injectMode)
	}

	// 2. add the user-provided agent config to a volume accessible from the init container
	requiredVolumes := []corev1.Volume{
		// Agent config volume
		a.ContainerAgentConfigVolume(),

		// Init volume
		{
			Name: requiredVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	templateCount := len(a.configMap.Templates)
	for i := range a.configMap.Templates {
		var name string
		if templateCount > 1 {
			name = fmt.Sprintf("infisical-secrets-%d", i+1)
		} else {
			name = "infisical-secrets"
		}
		requiredVolumes = append(requiredVolumes, corev1.Volume{
			Name: name,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}

	injectMode := a.injectMode

	podPatches = append(podPatches, addVolumes(
		a.pod.Spec.Volumes,
		requiredVolumes,
		"/spec/volumes")...)

	if injectMode == util.InjectModeInit {
		container, err := a.ContainerInitSidecar()
		if err != nil {
			return nil, err
		}

		if len(a.pod.Spec.InitContainers) != 0 {
			podPatches = append(podPatches, removeContainers("/spec/initContainers")...)
		}

		containers := append([]corev1.Container{container}, a.pod.Spec.InitContainers...)

		podPatches = append(podPatches, addContainers(
			[]corev1.Container{},
			containers,
			"/spec/initContainers")...)

		for i, container := range containers {
			if container.Name == util.InitContainerName {
				continue
			}
			podPatches = append(podPatches, addVolumeMounts(
				container.VolumeMounts,
				a.ContainerVolumeMounts(),
				fmt.Sprintf("/spec/initContainers/%d/volumeMounts", i))...)
		}
	} else if injectMode == util.InjectModeSidecar {
		container, err := a.ContainerSidecar()
		if err != nil {
			return nil, err
		}
		podPatches = append(podPatches, addContainers(
			a.pod.Spec.Containers,
			[]corev1.Container{container},
			"/spec/containers")...)
	}

	podPatches = append(podPatches, updatePodAnnotations(
		a.pod.Annotations,
		map[string]string{util.AnnotationAgentStatus: "injected"})...)

	if len(podPatches) > 0 {
		return json.Marshal(podPatches)
	}
	return nil, nil

}

func (a *Agent) createLifecycle() corev1.Lifecycle {
	// todo: add logic for cleaning up access token on pod shutdown
	return corev1.Lifecycle{}
}
