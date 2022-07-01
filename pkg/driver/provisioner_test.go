package driver

import (
	"context"
	"reflect"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/driver/mocks"
)

func TestProvisioner_GetCloud_NoRoleArnGivesOriginalObjectBack(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)

	actualCloud, _, _ := getCloud(mockCloud, map[string]string{})
	if actualCloud != mockCloud {
		t.Fatalf("Expected cloud object to be %v but was %v", mockCloud, actualCloud)
	}

	mockCtl.Finish()
}

func TestProvisioner_GetCloud_IncorrectRoleArnGivesError(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)

	_, _, err := getCloud(mockCloud, map[string]string{
		RoleArn: "foo",
	})
	if err == nil {
		t.Fatalf("Expected error but none was returned")
	}
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Expected 'Unauthenticated' error but found %v", err)
	}

	mockCtl.Finish()
}

func TestProvisioner_GetMountOptions_NoRoleArnGivesStandardOptions(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)
	ctx := context.Background()
	expectedOptions := []string{"tls", "iam"}

	options, _ := getMountOptions(ctx, mockCloud, fileSystemId, "")

	if !reflect.DeepEqual(options, expectedOptions) {
		t.Fatalf("Expected returned options to be %v but was %v", expectedOptions, options)
	}
}

func TestProvisioner_GetMountOptions_RoleArnAddsMountTargetIp(t *testing.T) {
	mockCtl := gomock.NewController(t)
	mockCloud := mocks.NewMockCloud(mockCtl)
	fakeMountTarget := cloud.MountTarget{
		AZName:        "foo",
		AZId:          "",
		MountTargetId: "",
		IPAddress:     "8.8.8.8",
	}
	ctx := context.Background()
	mockCloud.EXPECT().DescribeMountTargets(ctx, fileSystemId, "").Return(&fakeMountTarget, nil)

	expectedOptions := []string{"tls", "iam", MountTargetIp + "=" + fakeMountTarget.IPAddress}

	options, _ := getMountOptions(ctx, mockCloud, fileSystemId, "roleArn")

	if !reflect.DeepEqual(options, expectedOptions) {
		t.Fatalf("Expected returned options to be %v but was %v", expectedOptions, options)
	}
}
