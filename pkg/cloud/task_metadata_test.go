package cloud

import (
	"encoding/json"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud/mocks"
	"os"
	"testing"
)

var (
	clusterId = "default"
	taskId    = "158d1c8083dd49d6b527399fd6414f5c"
	region    = "us-west-2"
	az        = fmt.Sprintf(`%sa`, region)
	taskArn   = fmt.Sprintf(`arn:aws:ecs:us-west-2:111122223333:task/%s/%s`, clusterId, taskId)
)

func TestGetTaskMetadataService(t *testing.T) {
	tests := []struct {
		name           string
		returnResponse interface{}
		err            error
		wantErr        bool
	}{
		{
			"success: normal",
			TMDSV4Response{
				Cluster:          clusterId,
				TaskARN:          taskArn,
				AvailabilityZone: az,
			},
			nil,
			false,
		},
		{
			"fail: GetTMDSV4Response returned error",
			TMDSV4Response{
				Cluster:          clusterId,
				TaskARN:          taskArn,
				AvailabilityZone: az,
			},
			fmt.Errorf(""),
			true,
		},
		{
			"fail: GetTMDSV4Response returned json unmarshal error",
			"ANY STRING",
			nil,
			true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockTaskMetadata := mocks.NewMockTaskMetadataService(mockCtrl)
			jsonData, _ := json.Marshal(tc.returnResponse)
			mockTaskMetadata.EXPECT().GetTMDSV4Response().Return(jsonData, tc.err)

			m, err := getTaskMetadata(mockTaskMetadata)

			if !tc.wantErr {
				if err != nil {
					t.Fatalf("getTaskMetadata failed: expected no error, got %v", err)
				}

				if m.GetInstanceID() != taskId {
					t.Fatalf("GetInstanceID() failed: expeted %v, got %v", taskId, m.GetInstanceID())
				}

				if m.GetRegion() != region {
					t.Fatalf("GetRegion() failed: expeted %v, got %v", region, m.GetRegion())
				}

				if m.GetAvailabilityZone() != az {
					t.Fatalf("GetAvailabilityZone() failed: expeted %v, got %v", az, m.GetAvailabilityZone())
				}
			} else {
				if err == nil {
					t.Fatalf("getTaskMetadata() failed: expected error")
				}
			}
		})
	}
}

func TestGetTMDSV4ResponseNonECSEnvironment(t *testing.T) {
	// with current test we don't need it, but serve as double down on existent of this env
	os.Unsetenv(taskMetadataV4EnvName)
	taskMetadataService := taskMetadata{}
	_, err := taskMetadataService.GetTMDSV4Response()
	if err == nil {
		t.Fatalf("GetTMDSV4Response failed: expected error in non ECS env")
	}
}
