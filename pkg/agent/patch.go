package agent

import (
	"encoding/json"
	"strings"

	"github.com/Infisical/infisical-agent-injector/pkg/util"
	jsonpatch "github.com/evanphx/json-patch"
	corev1 "k8s.io/api/core/v1"
)

func toRawJSON(v interface{}) json.RawMessage {
	b, _ := json.Marshal(v)
	return json.RawMessage(b)
}

func AddOp(path string, value interface{}) jsonpatch.Operation {
	pathRaw, valueRaw := toRawJSON(path), toRawJSON(value)

	return map[string]*json.RawMessage{
		"op":    &util.PatchOperationAdd,
		"path":  &pathRaw,
		"value": &valueRaw,
	}
}

func RemoveOp(path string) jsonpatch.Operation {
	pathRaw := toRawJSON(path)
	return map[string]*json.RawMessage{
		"op":   &util.PatchOperationRemove,
		"path": &pathRaw,
	}
}

// addResourcesWithPath is a generic function that handles adding both volumes and volume mounts
func addResourcesWithPath[T any](existing []T, newItems []T, basePath string) jsonpatch.Patch {
	var patches jsonpatch.Patch

	if len(existing) == 0 && len(newItems) > 0 {
		// if there are no existing items, add them all at once as an array
		patches = append(patches, AddOp(basePath, newItems))
		return patches
	}

	// add each item individually to the end of the array
	for _, item := range newItems {
		patches = append(patches, AddOp(basePath+"/-", item))
	}

	return patches
}

func addVolumes(target, volumes []corev1.Volume, base string) jsonpatch.Patch {
	return addResourcesWithPath(target, volumes, base)
}

func addVolumeMounts(target, mounts []corev1.VolumeMount, base string) jsonpatch.Patch {
	return addResourcesWithPath(target, mounts, base)
}
func removeContainers(path string) jsonpatch.Patch {
	return []jsonpatch.Operation{RemoveOp(path)}
}

func addContainers(target, containers []corev1.Container, base string) jsonpatch.Patch {
	if len(target) == 0 && len(containers) > 0 {
		return []jsonpatch.Operation{AddOp(base, containers)}
	}

	var patches jsonpatch.Patch
	for _, container := range containers {
		patches = append(patches, AddOp(base+"/-", container))
	}
	return patches
}

func updatePodAnnotations(target, annotations map[string]string) jsonpatch.Patch {
	var result jsonpatch.Patch
	if len(target) == 0 {
		return []jsonpatch.Operation{AddOp("/metadata/annotations", annotations)}
	}

	for key, value := range annotations {
		escapedKey := strings.NewReplacer("~", "~0", "/", "~1").Replace(key)

		result = append(result, AddOp("/metadata/annotations/"+escapedKey, value))
	}

	return result
}
