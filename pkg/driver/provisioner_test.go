package driver

import (
	"testing"

	"github.com/golang/mock/gomock"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestProvisioner_GetCloud_NoRoleArnGivesOriginalObjectBack(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)

	actualCloud, _, _, err := getCloud(map[string]string{}, mockCloud, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if actualCloud != mockCloud {
		t.Fatalf("Expected cloud object to be %v but was %v", mockCloud, actualCloud)
	}

	mockCtl.Finish()
}

func TestProvisioner_GetCloud_WithRoleArnGivesNewObject(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)

	actualCloud, _, _, err := getCloud(map[string]string{
		RoleArn: "arn:aws:iam::1234567890:role/EFSCrossAccountRole",
	}, mockCloud, false)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if actualCloud == mockCloud {
		t.Fatalf("Unexpected cloud object: %v", actualCloud)
	}

	mockCtl.Finish()
}
