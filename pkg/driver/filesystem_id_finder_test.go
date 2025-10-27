package driver

import (
	"context"
	"strings"
	"testing"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFindFileSystemId(t *testing.T) {
	testCases := []struct {
		name          string
		volumeParams  map[string]string
		mockSetup     func() kubernetes.Interface
		expectedFsId  string
		expectedError bool
		errorContains string
	}{
		{
			name: "Success: Given fileSystemId parameter",
			volumeParams: map[string]string{
				"fileSystemId": "fs-12345678",
			},
			expectedFsId:  "fs-12345678",
			expectedError: false,
		},
		{
			name: "Success: ConfigMap reference",
			volumeParams: map[string]string{
				FileSystemIdConfigRef: "default/efs-config/fileSystemId",
			},
			mockSetup: func() kubernetes.Interface {
				return createMockClientWithConfigMap("default", "efs-config", map[string]string{
					"fileSystemId": "fs-from-configmap",
				})
			},
			expectedFsId:  "fs-from-configmap",
			expectedError: false,
		},
		{
			name: "Success: Secret reference",
			volumeParams: map[string]string{
				FileSystemIdSecretRef: "kube-system/efs-secret/fsId",
			},
			mockSetup: func() kubernetes.Interface {
				return createMockClientWithSecret("kube-system", "efs-secret", map[string][]byte{
					"fsId": []byte("secret"),
				})
			},
			expectedFsId:  "secret",
			expectedError: false,
		},
		{
			name: "Fail: Multiple sources - fileSystemId and ConfigRef",
			volumeParams: map[string]string{
				"fileSystemId":        "fs-direct",
				FileSystemIdConfigRef: "default/efs-config/fileSystemId",
			},
			expectedError: true,
			errorContains: "only one of fileSystemId, fileSystemIdConfigRef, or fileSystemIdSecretRef can be specified",
		},
		{
			name: "Fail: Multiple sources - fileSystemId and SecretRef",
			volumeParams: map[string]string{
				"fileSystemId":        "fs-direct",
				FileSystemIdSecretRef: "default/efs-secret/fsId",
			},
			expectedError: true,
			errorContains: "only one of fileSystemId, fileSystemIdConfigRef, or fileSystemIdSecretRef can be specified",
		},
		{
			name: "Fail: Multiple sources - ConfigRef and SecretRef",
			volumeParams: map[string]string{
				FileSystemIdConfigRef: "default/efs-config/fileSystemId",
				FileSystemIdSecretRef: "default/efs-secret/fsId",
			},
			expectedError: true,
			errorContains: "only one of fileSystemId, fileSystemIdConfigRef, or fileSystemIdSecretRef can be specified",
		},
		{
			name: "Success: Empty fileSystemId with valid ConfigRef",
			volumeParams: map[string]string{
				"fileSystemId":        "",
				FileSystemIdConfigRef: "default/efs-config/fileSystemId",
			},
			mockSetup: func() kubernetes.Interface {
				cm := &v1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{Name: "efs-config", Namespace: "default"},
					Data:       map[string]string{"fileSystemId": "fs-from-configmap"},
				}
				return fake.NewSimpleClientset(cm)
			},
			expectedFsId:  "fs-from-configmap",
			expectedError: false,
		},
		{
			name: "Success: Empty fileSystemId with valid SecretRef",
			volumeParams: map[string]string{
				"fileSystemId":        "   ",
				FileSystemIdSecretRef: "default/efs-secret/fsId",
			},
			mockSetup: func() kubernetes.Interface {
				secret := &v1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "efs-secret", Namespace: "default"},
					Data:       map[string][]byte{"fsId": []byte("fs-from-secret")},
				}
				return fake.NewSimpleClientset(secret)
			},
			expectedFsId:  "fs-from-secret",
			expectedError: false,
		},
		{
			name: "Fail: All empty values",
			volumeParams: map[string]string{
				"fileSystemId":        "   ",
				FileSystemIdConfigRef: " ",
				FileSystemIdSecretRef: " ",
			},
			expectedError: true,
			errorContains: "one of fileSystemId, fileSystemIdConfigRef, or fileSystemIdSecretRef must be specified",
		},
		{
			name: "Fail: ConfigMap not found",
			volumeParams: map[string]string{
				FileSystemIdConfigRef: "default/nonexistent/fileSystemId",
			},
			mockSetup: func() kubernetes.Interface {
				return fake.NewSimpleClientset()
			},
			expectedError: true,
			errorContains: "failed to get",
		},
		{
			name: "Fail: Secret not found",
			volumeParams: map[string]string{
				FileSystemIdSecretRef: "default/nonexistent/fsId",
			},
			mockSetup: func() kubernetes.Interface {
				return fake.NewSimpleClientset()
			},
			expectedError: true,
			errorContains: "failed to get",
		},
		{
			name: "Fail: Invalid configmap reference format",
			volumeParams: map[string]string{
				FileSystemIdConfigRef: "invalid-format",
			},
			expectedError: true,
			errorContains: "invalid",
		},
		{
			name: "Fail: Invalid secret reference format",
			volumeParams: map[string]string{
				FileSystemIdSecretRef: "invalid/format",
			},
			expectedError: true,
			errorContains: "invalid",
		},
		{
			name: "Fail: No filesystem ID specified",
			volumeParams: map[string]string{
				"provisioningMode": "efs-ap",
			},
			expectedError: true,
			errorContains: "one of fileSystemId, fileSystemIdConfigRef, or fileSystemIdSecretRef must be specified",
		},
		{
			name: "Fail: ConfigMap with key not found",
			volumeParams: map[string]string{
				FileSystemIdConfigRef: "default/efs-config/test-key",
			},
			mockSetup: func() kubernetes.Interface {
				return createMockClientWithConfigMap("default", "efs-config", map[string]string{
					"key": "value",
				})
			},
			expectedError: true,
			errorContains: "key test-key not found or empty in ConfigMap",
		},
		{
			name: "Fail: Secret with key not found",
			volumeParams: map[string]string{
				FileSystemIdSecretRef: "default/efs-secret/missing-key",
			},
			mockSetup: func() kubernetes.Interface {
				return createMockClientWithSecret("default", "efs-secret", map[string][]byte{
					"otherKey": []byte("some-value"),
				})
			},
			expectedError: true,
			errorContains: "key missing-key not found or empty in Secret",
		},
		{
			name: "Fail: Secret with empty value",
			volumeParams: map[string]string{
				FileSystemIdSecretRef: "default/efs-secret/fsId",
			},
			mockSetup: func() kubernetes.Interface {
				return createMockClientWithSecret("default", "efs-secret", map[string][]byte{
					"fsId": []byte("   "),
				})
			},
			expectedError: true,
			errorContains: "key fsId",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockSetup != nil {
				originalClient := cloud.DefaultKubernetesAPIClient
				defer func() { cloud.DefaultKubernetesAPIClient = originalClient }()

				mockClient := tc.mockSetup()
				cloud.DefaultKubernetesAPIClient = func() (kubernetes.Interface, error) {
					return mockClient, nil
				}
			}

			result, err := findFileSystemId(context.TODO(), tc.volumeParams)

			if tc.expectedError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tc.errorContains != "" && !strings.Contains(err.Error(), tc.errorContains) {
					t.Errorf("Expected error to contain '%s', got: %v", tc.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if result != tc.expectedFsId {
					t.Errorf("Expected filesystem ID '%s', got '%s'", tc.expectedFsId, result)
				}
			}
		})
	}
}

// Helper function to create mock client with configmap
func createMockClientWithConfigMap(namespace, name string, data map[string]string) kubernetes.Interface {
	configMap := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
	return fake.NewSimpleClientset(configMap)
}

// Helper to create mock client with Secret
func createMockClientWithSecret(namespace, name string, data map[string][]byte) kubernetes.Interface {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
	return fake.NewSimpleClientset(secret)
}
