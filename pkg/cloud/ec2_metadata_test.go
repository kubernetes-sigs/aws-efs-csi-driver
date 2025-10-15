package cloud

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/golang/mock/gomock"

	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud/mocks"
)

var (
	stdInstanceID         = "instance-1"
	stdRegionName         = "instance-1"
	stdAvailabilityZone   = "az-1"
	stdAvailabilityZoneID = "use1-az1"
)

func TestRetrieveMetadataFromEC2MetadataService(t *testing.T) {
	testCases := []struct {
		name               string
		isAvailable        bool
		isPartial          bool
		identityDocument   imds.InstanceIdentityDocument
		availabilityZoneID string
		err                error
	}{
		{
			name:        "success: normal",
			isAvailable: true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           stdRegionName,
				AvailabilityZone: stdAvailabilityZone,
			},
			availabilityZoneID: stdAvailabilityZoneID,
			err:                nil,
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned error",
			isAvailable: true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           stdRegionName,
				AvailabilityZone: stdAvailabilityZone,
			},
			availabilityZoneID: stdAvailabilityZoneID,
			err:                fmt.Errorf(""),
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned empty instance",
			isAvailable: true,
			isPartial:   true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       "",
				Region:           stdRegionName,
				AvailabilityZone: stdAvailabilityZone,
			},
			availabilityZoneID: stdAvailabilityZoneID,
			err:                nil,
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned empty region",
			isAvailable: true,
			isPartial:   true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           "",
				AvailabilityZone: stdAvailabilityZone,
			},
			availabilityZoneID: stdAvailabilityZoneID,
			err:                nil,
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned empty az",
			isAvailable: true,
			isPartial:   true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           stdRegionName,
				AvailabilityZone: "",
			},
			availabilityZoneID: stdAvailabilityZoneID,
			err:                nil,
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned empty az id",
			isAvailable: true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           stdRegionName,
				AvailabilityZone: stdAvailabilityZone,
			},
			availabilityZoneID: "",
			err:                nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockEC2Metadata := mocks.NewMockEC2Metadata(mockCtrl)

			if tc.isAvailable {
				mockEC2Metadata.EXPECT().GetInstanceIdentityDocument(context.TODO(), &imds.GetInstanceIdentityDocumentInput{}).Return(&imds.GetInstanceIdentityDocumentOutput{InstanceIdentityDocument: tc.identityDocument}, tc.err)

				if tc.err == nil &&
					tc.identityDocument.InstanceID != "" &&
					tc.identityDocument.Region != "" &&
					tc.identityDocument.AvailabilityZone != "" {
					mockEC2Metadata.EXPECT().GetMetadata(context.TODO(), &imds.GetMetadataInput{
						Path: "placement/availability-zone-id",
					}).Return(&imds.GetMetadataOutput{Content: io.NopCloser(strings.NewReader(tc.availabilityZoneID))}, nil)
				}
			}

			ec2Mp := ec2MetadataProvider{ec2MetadataService: mockEC2Metadata}
			m, err := ec2Mp.getMetadata()

			if tc.isAvailable && tc.err == nil && !tc.isPartial {
				if err != nil {
					t.Fatalf("getEC2Metadata() failed: expected no error, got %v", err)
				}

				if m.GetInstanceID() != tc.identityDocument.InstanceID {
					t.Fatalf("GetInstanceID() failed: expected %v, got %v", tc.identityDocument.InstanceID, m.GetInstanceID())
				}

				if m.GetRegion() != tc.identityDocument.Region {
					t.Fatalf("GetRegion() failed: expected %v, got %v", tc.identityDocument.Region, m.GetRegion())
				}

				if m.GetAvailabilityZone() != tc.identityDocument.AvailabilityZone {
					t.Fatalf("GetAvailabilityZone() failed: expected %v, got %v", tc.identityDocument.AvailabilityZone, m.GetAvailabilityZone())
				}

				if m.GetAvailabilityZoneID() != tc.availabilityZoneID {
					t.Fatalf("GetAvailabilityZoneID() failed: expected %v, got %v", tc.availabilityZoneID, m.GetAvailabilityZoneID())
				}
			} else {
				if err == nil {
					t.Fatal("getEC2Metadata() failed: expected error when GetInstanceIdentityDocument returns partial data, got nothing")
				}
			}

			mockCtrl.Finish()
		})
	}
}
