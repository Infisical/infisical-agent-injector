package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	"github.com/Infisical/infisical-agent-injector/pkg/util/path"
	jsonpatch "github.com/evanphx/json-patch"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/kubernetes"
)

type Agent struct {
	k8s                       *kubernetes.Clientset
	pod                       *corev1.Pod
	configMap                 *util.ConfigMap
	serviceAccountTokenVolume *ServiceAccountTokenVolume
	injectMode                string
	cachingEnabled            bool
	agentImage                string
	isWindows                 bool
}

func NewAgent(k8s *kubernetes.Clientset, pod *corev1.Pod, configMap *util.ConfigMap) (*Agent, error) {

	if configMap == nil {
		return nil, fmt.Errorf("config map is required")
	}

	injectMode := pod.Annotations[util.InjectModeAnnotation]
	if injectMode == "" {
		injectMode = util.InjectModeInit
	}

	cachingEnabled := pod.Annotations[util.AnnotationCachingEnabled] == "true"

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

	agentImage := util.LinuxContainerImage
	isWindows := false

	if util.IsWindowsPod(pod) {
		agentImage = util.WindowsContainerImage
		isWindows = true
	}

	return &Agent{
		pod:                       pod,
		k8s:                       k8s,
		configMap:                 configMap,
		serviceAccountTokenVolume: serviceAccountTokenVolume,
		injectMode:                injectMode,
		cachingEnabled:            cachingEnabled,
		agentImage:                agentImage,
		isWindows:                 isWindows,
	}, nil
}

func (a *Agent) ContainerVolumeMounts(existingMounts []corev1.VolumeMount) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{}

	if !slices.ContainsFunc(existingMounts, func(mount corev1.VolumeMount) bool {
		mountPath := util.LinuxContainerWorkDirVolumeMountPath
		if a.isWindows {
			mountPath = util.WindowsContainerWorkDirVolumeMountPath
		}
		return mount.MountPath == mountPath && mount.Name == util.ContainerWorkDirMountName
	}) {

		mountPath := util.LinuxContainerWorkDirVolumeMountPath
		if a.isWindows {
			mountPath = util.WindowsContainerWorkDirVolumeMountPath
		}

		// we mount this on the users pod
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      util.ContainerWorkDirMountName,
			MountPath: mountPath,
		})
	}

	templateCount := len(a.configMap.Templates)

	for i, template := range a.configMap.Templates {

		destinationPath := path.Dir(template.DestinationPath, a.isWindows)

		var name string
		if templateCount > 1 {
			name = fmt.Sprintf("infisical-secrets-%d", i+1)
		} else {
			name = "infisical-secrets"
		}

		alreadyExists := false

		for _, volumeMount := range existingMounts {
			if volumeMount.MountPath == destinationPath {
				alreadyExists = true
				break
			}
		}

		if !alreadyExists {
			for _, volumeMount := range volumeMounts {
				if volumeMount.MountPath == destinationPath {
					alreadyExists = true
					break
				}
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

func (a *Agent) ValidateConfigMap() error {
	if err := util.ValidateInjectMode(a.injectMode); err != nil {
		return err
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

	delimiter := "/"
	examplePath := "/path/to/destination/secret-file"
	if a.isWindows {
		delimiter = "\\"
		examplePath = "C:\\path\\to\\destination\\secret-file"
	}

	for _, template := range a.configMap.Templates {

		if template.DestinationPath == "" {
			return fmt.Errorf("template destination path is required")
		}

		// validates drive letter paths and UNC paths
		if !path.IsAbs(template.DestinationPath, a.isWindows) {
			return fmt.Errorf("template destination path must be an absolute path (e.g. %s)", examplePath)
		}

		slashCount := strings.Count(template.DestinationPath, delimiter)
		if slashCount < 2 {
			return fmt.Errorf("template destination path must be a folder (e.g. %s)", examplePath)
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
			a.ContainerVolumeMounts(container.VolumeMounts),
			fmt.Sprintf("/spec/containers/%d/volumeMounts", i))...)
	}

	if err := util.ValidateInjectMode(a.injectMode); err != nil {
		return nil, err
	}

	var requiredVolumes []corev1.Volume

	requiredVolumes = append(requiredVolumes, corev1.Volume{
		Name: util.ContainerWorkDirMountName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMediumMemory,
			},
		},
	})

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

	podPatches = append(podPatches, addVolumes(
		a.pod.Spec.Volumes,
		requiredVolumes,
		"/spec/volumes")...)

	switch a.injectMode {
	case util.InjectModeInit:
		err := a.appendInitContainer(&podPatches)
		if err != nil {
			return nil, err
		}
	case util.InjectModeSidecar:
		err := a.appendSidecarContainer(&podPatches)
		if err != nil {
			return nil, err
		}
	case util.InjectModeSidecarInit:
		err := a.appendInitContainer(&podPatches)
		if err != nil {
			return nil, err
		}

		err = a.appendSidecarContainer(&podPatches)
		if err != nil {
			return nil, err
		}
	}

	podPatches = append(podPatches, updatePodAnnotations(
		a.pod.Annotations,
		map[string]string{util.AnnotationAgentStatus: "injected"})...)

	if len(podPatches) > 0 {
		return json.Marshal(podPatches)
	}
	return nil, nil

}

func (a *Agent) Lifecycle() corev1.Lifecycle {

	// No preStop needed - k8s will send SIGTERM automatically
	// k8s sends the sigterm to PID 1, so in the startup script we forward the signal to the agent process (see linux-container-startup.sh.tmpl)

	return corev1.Lifecycle{}
}

func (a *Agent) appendInitContainer(podPatches *jsonpatch.Patch) error {
	container, err := a.ContainerInitSidecar()
	if err != nil {
		return err
	}

	if len(a.pod.Spec.InitContainers) != 0 {
		*podPatches = append(*podPatches, removeContainers("/spec/initContainers")...)
	}

	containers := append([]corev1.Container{container}, a.pod.Spec.InitContainers...)

	*podPatches = append(*podPatches, addContainers(
		[]corev1.Container{},
		containers,
		"/spec/initContainers")...)

	for i, container := range containers {
		if container.Name == util.InitContainerName {
			continue
		}
		*podPatches = append(*podPatches, addVolumeMounts(
			container.VolumeMounts,
			a.ContainerVolumeMounts(container.VolumeMounts),
			fmt.Sprintf("/spec/initContainers/%d/volumeMounts", i))...)
	}
	return nil
}

func (a *Agent) appendSidecarContainer(podPatches *jsonpatch.Patch) error {
	container, err := a.ContainerSidecar()
	if err != nil {
		return err
	}
	*podPatches = append(*podPatches, addContainers(
		a.pod.Spec.Containers,
		[]corev1.Container{container},
		"/spec/containers")...)
	return nil
}

func (a *Agent) ResourceRequirements() corev1.ResourceRequirements {

	const (
		// CPU, Linux
		DefaultCPULimitLinux   = "500m"
		DefaultCPURequestLinux = "100m"

		// Memory, Linux
		DefaultMemoryLimitLinux   = "128Mi"
		DefaultMemoryRequestLinux = "64Mi"

		// CPU, Windows
		DefaultCPULimitWindows   = "500m"
		DefaultCPURequestWindows = "100m"

		// Memory, Windows
		DefaultMemoryLimitWindows   = "512Mi"
		DefaultMemoryRequestWindows = "256Mi"
	)

	limits := corev1.ResourceList{}
	requests := corev1.ResourceList{}

	var cpuLimit, cpuRequest resource.Quantity
	var memoryLimit, memoryRequest resource.Quantity

	// we don't set defaults for ephemeral storage as it first became generally available in k8s 1.25.
	// additionally we want to let the cluster dictacte the pods ephemeral storage limits if not explicitly provided by the user.
	var ephemeralLimit, ephemeralRequest resource.Quantity
	var empherealLimitSet, empherealRequestSet bool

	// default limits
	if a.isWindows {
		// windows pods need much more memory than linux pods (this resolves out of memory restarts)
		memoryLimit, _ = resource.ParseQuantity(DefaultMemoryLimitWindows)     // 4x more
		memoryRequest, _ = resource.ParseQuantity(DefaultMemoryRequestWindows) // 2x more

		cpuLimit, _ = resource.ParseQuantity(DefaultCPULimitWindows)
		cpuRequest, _ = resource.ParseQuantity(DefaultCPURequestWindows)
	} else {
		memoryLimit, _ = resource.ParseQuantity(DefaultMemoryLimitLinux)
		memoryRequest, _ = resource.ParseQuantity(DefaultMemoryRequestLinux)

		cpuLimit, _ = resource.ParseQuantity(DefaultCPULimitLinux)
		cpuRequest, _ = resource.ParseQuantity(DefaultCPURequestLinux)
	}

	// user-defined ephemeral storage limits
	if a.pod.Annotations[util.AnnotationLimitsEphemeral] != "" {
		ephemeralLimit, _ = resource.ParseQuantity(a.pod.Annotations[util.AnnotationLimitsEphemeral])
		empherealLimitSet = true
	}

	// user-defined ephemeral storage requests
	if a.pod.Annotations[util.AnnotationRequestsEphemeral] != "" {
		ephemeralRequest, _ = resource.ParseQuantity(a.pod.Annotations[util.AnnotationRequestsEphemeral])
		empherealRequestSet = true
	}

	// user-defined CPU limits
	if a.pod.Annotations[util.AnnotationLimitsCPU] != "" {
		cpuLimit, _ = resource.ParseQuantity(a.pod.Annotations[util.AnnotationLimitsCPU])
	}

	// user-defined CPU requests
	if a.pod.Annotations[util.AnnotationRequestsCPU] != "" {
		cpuRequest, _ = resource.ParseQuantity(a.pod.Annotations[util.AnnotationRequestsCPU])
	}

	// user-defined memory limits
	if a.pod.Annotations[util.AnnotationLimitsMemory] != "" {
		memoryLimit, _ = resource.ParseQuantity(a.pod.Annotations[util.AnnotationLimitsMemory])
	}

	// user-defined memory requests
	if a.pod.Annotations[util.AnnotationRequestsMemory] != "" {
		memoryRequest, _ = resource.ParseQuantity(a.pod.Annotations[util.AnnotationRequestsMemory])
	}

	limits[corev1.ResourceCPU] = cpuLimit
	limits[corev1.ResourceMemory] = memoryLimit
	if empherealLimitSet {
		limits[corev1.ResourceEphemeralStorage] = ephemeralLimit
	}

	requests[corev1.ResourceCPU] = cpuRequest
	requests[corev1.ResourceMemory] = memoryRequest
	if empherealRequestSet {
		requests[corev1.ResourceEphemeralStorage] = ephemeralRequest
	}

	// set the limits and requests on the resource requirements

	resources := corev1.ResourceRequirements{
		Limits:   limits,
		Requests: requests,
	}

	return resources
}
