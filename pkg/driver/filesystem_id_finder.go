package driver

import (
	"context"
	"fmt"
	"strings"

	k8errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
)

type FileSystemIdRef struct {
	Namespace string
	Name      string
	Key       string
}

func findFileSystemId(ctx context.Context, volumeParams map[string]string) (string, error) {
	if err := validateFileSystemIdRefs(volumeParams); err != nil {
		klog.Errorf("Failed to validate filesystemId references: %v", err)
		return "", err
	}

	if value, ok := volumeParams[FsId]; ok {
		value = strings.TrimSpace(value)
		if value != "" {
			return value, nil
		}
	}

	if ref, ok := volumeParams[FileSystemIdConfigRef]; ok {
		return getFileSystemIdFromRef(ctx, FileSystemIdConfigRef, ref)
	}

	if ref, ok := volumeParams[FileSystemIdSecretRef]; ok {
		return getFileSystemIdFromRef(ctx, FileSystemIdSecretRef, ref)
	}
	return "", fmt.Errorf("fileSystemId not specified in volume parameters")
}

func validateFileSystemIdRefs(volumeParams map[string]string) error {
	refs := 0
	if value, ok := volumeParams[FsId]; ok && strings.TrimSpace(value) != "" {
		refs++
	}
	if value, ok := volumeParams[FileSystemIdConfigRef]; ok && strings.TrimSpace(value) != "" {
		refs++
	}
	if value, ok := volumeParams[FileSystemIdSecretRef]; ok && strings.TrimSpace(value) != "" {
		refs++
	}
	if refs > 1 {
		return fmt.Errorf("only one of fileSystemId, fileSystemIdConfigRef, or fileSystemIdSecretRef can be specified")
	}
	if refs == 0 {
		return fmt.Errorf("one of fileSystemId, fileSystemIdConfigRef, or fileSystemIdSecretRef must be specified")
	}
	return nil
}

func getFileSystemIdFromRef(ctx context.Context, resourceType, ref string) (string, error) {
	refPath, err := parseFileSystemIdRef(ref)
	if err != nil {
		return "", fmt.Errorf("invalid %s reference: %w", resourceType, err)
	}

	client, err := cloud.DefaultKubernetesAPIClient()
	if err != nil {
		return "", err
	}

	if resourceType == FileSystemIdConfigRef {
		configMap, err := client.CoreV1().ConfigMaps(refPath.Namespace).Get(ctx, refPath.Name, metav1.GetOptions{})
		if err != nil {
			return "", formatRBACPermissionErrors(err, resourceType, refPath.Namespace, refPath.Name)
		}
		fsId := strings.TrimSpace(configMap.Data[refPath.Key])
		if fsId == "" {
			return "", fmt.Errorf("key %s not found or empty in ConfigMap %s/%s", refPath.Key, refPath.Namespace, refPath.Name)
		}
		return fsId, nil
	}

	if resourceType == FileSystemIdSecretRef {
		secret, err := client.CoreV1().Secrets(refPath.Namespace).Get(ctx, refPath.Name, metav1.GetOptions{})
		if err != nil {
			return "", formatRBACPermissionErrors(err, resourceType, refPath.Namespace, refPath.Name)
		}
		fsId := strings.TrimSpace(string(secret.Data[refPath.Key]))
		if fsId == "" {
			return "", fmt.Errorf("key %s not found or empty in Secret %s/%s", refPath.Key, refPath.Namespace, refPath.Name)
		}
		return fsId, nil
	}

	return "", fmt.Errorf("unknown resource type %s", resourceType)
}

func parseFileSystemIdRef(ref string) (*FileSystemIdRef, error) {
	ref = strings.TrimSpace(ref)

	if ref == "" {
		return nil, fmt.Errorf("reference can't be empty")
	}

	parts := strings.Split(ref, "/")

	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid reference format, expected namespace/name/key, got: %s", ref)
	}

	namespace := strings.TrimSpace(parts[0])
	name := strings.TrimSpace(parts[1])
	key := strings.TrimSpace(parts[2])

	if namespace == "" || name == "" || key == "" {
		return nil, fmt.Errorf("namespace, name, and key must be non-empty in reference: %s", ref)
	}

	return &FileSystemIdRef{
		Namespace: namespace,
		Name:      name,
		Key:       key,
	}, nil
}

func formatRBACPermissionErrors(err error, resourceType, namespace, name string) error {
	if k8errors.IsForbidden(err) {
		return fmt.Errorf("%s '%s/%s' is forbidden: %w. "+
			"This feature requires RBAC permissions. "+
			"See documentation for details", resourceType, namespace, name, err)
	}
	return fmt.Errorf("failed to get %s %s/%s: %w", resourceType, namespace, name, err)
}
