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
	stdAvailabilityZoneId = "az-id-1"
)

func TestRetrieveMetadataFromEC2MetadataService(t *testing.T) {
	testCases := []struct {
		name             string
		isAvailable      bool
		isPartial        bool
		identityDocument imds.InstanceIdentityDocument
		err              error
	}{
		{
			name:        "success: normal",
			isAvailable: true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           stdRegionName,
				AvailabilityZone: stdAvailabilityZone,
			},
			err: nil,
		},
		{
			name:        "fail: GetInstanceIdentityDocument returned error",
			isAvailable: true,
			identityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           stdRegionName,
				AvailabilityZone: stdAvailabilityZone,
			},
			err: fmt.Errorf(""),
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
			err: nil,
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
			err: nil,
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
			err: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			mockEC2Metadata := mocks.NewMockEC2Metadata(mockCtrl)

			if tc.isAvailable {
				mockEC2Metadata.EXPECT().GetInstanceIdentityDocument(context.TODO(), &imds.GetInstanceIdentityDocumentInput{}).Return(&imds.GetInstanceIdentityDocumentOutput{InstanceIdentityDocument: tc.identityDocument}, tc.err)
			}
			if tc.isAvailable && tc.err == nil && !tc.isPartial {
				mockEC2Metadata.EXPECT().GetMetadata(context.TODO(), &imds.GetMetadataInput{Path: "placement/availability-zone-id"}).Return(&imds.GetMetadataOutput{Content: io.NopCloser(strings.NewReader(stdAvailabilityZoneId))}, nil)
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

				if m.GetAvailabilityZoneId() != stdAvailabilityZoneId {
					t.Fatalf("GetAvailabilityZoneId() failed: expected %v, got %v", stdAvailabilityZoneId, m.GetAvailabilityZoneId())
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

func TestRetrieveAvailabilityZoneIdFromEC2MetadataService(t *testing.T) {
	testCases := []struct {
		name         string
		output       *imds.GetMetadataOutput
		err          error
		expectedAZId string
	}{
		{
			name:         "success: AZ ID returned",
			output:       &imds.GetMetadataOutput{Content: io.NopCloser(strings.NewReader("use1-az1\n"))},
			expectedAZId: "use1-az1",
		},
		{
			name:         "not fatal: GetMetadata returned error",
			err:          fmt.Errorf("not found"),
			expectedAZId: "",
		},
		{
			name:         "not fatal: GetMetadata returned no content",
			output:       &imds.GetMetadataOutput{},
			expectedAZId: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			mockEC2Metadata := mocks.NewMockEC2Metadata(mockCtrl)

			mockEC2Metadata.EXPECT().GetInstanceIdentityDocument(context.TODO(), &imds.GetInstanceIdentityDocumentInput{}).Return(&imds.GetInstanceIdentityDocumentOutput{InstanceIdentityDocument: imds.InstanceIdentityDocument{
				InstanceID:       stdInstanceID,
				Region:           stdRegionName,
				AvailabilityZone: stdAvailabilityZone,
			}}, nil)
			mockEC2Metadata.EXPECT().GetMetadata(context.TODO(), &imds.GetMetadataInput{Path: "placement/availability-zone-id"}).Return(tc.output, tc.err)

			m, err := ec2MetadataProvider{ec2MetadataService: mockEC2Metadata}.getMetadata()
			if err != nil {
				t.Fatalf("getMetadata() failed: expected no error, got %v", err)
			}
			if m.GetAvailabilityZone() != stdAvailabilityZone {
				t.Fatalf("GetAvailabilityZone() failed: expected %v, got %v", stdAvailabilityZone, m.GetAvailabilityZone())
			}
			if m.GetAvailabilityZoneId() != tc.expectedAZId {
				t.Fatalf("GetAvailabilityZoneId() failed: expected %q, got %q", tc.expectedAZId, m.GetAvailabilityZoneId())
			}
		})
	}
}
