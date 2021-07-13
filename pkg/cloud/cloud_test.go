package cloud

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/efs"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud/mocks"
)

type errtyp struct {
	code    string
	message string
}

func TestCreateAccessPoint(t *testing.T) {
	var (
		arn                  = "arn:aws:elasticfilesystem:us-east-1:1234567890:access-point/fsap-abcd1234xyz987"
		accessPointId        = "fsap-abcd1234xyz987"
		fsId                 = "fs-abcd1234"
		uid            int64 = 1001
		gid            int64 = 1001
		directoryPerms       = "0777"
		directoryPath        = "/test"
		volName              = "volName"
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockCtl)
				c := &cloud{
					efs: mockEfs,
				}

				tags := make(map[string]string)
				tags["cluster"] = "efs"

				req := &AccessPointOptions{
					FileSystemId:   fsId,
					Uid:            uid,
					Gid:            gid,
					DirectoryPerms: directoryPerms,
					DirectoryPath:  directoryPath,
					Tags:           tags,
				}

				output := &efs.CreateAccessPointOutput{
					AccessPointArn: aws.String(arn),
					AccessPointId:  aws.String(accessPointId),
					ClientToken:    aws.String("test"),
					FileSystemId:   aws.String(fsId),
					PosixUser: &efs.PosixUser{
						Gid: aws.Int64(gid),
						Uid: aws.Int64(uid),
					},
					RootDirectory: &efs.RootDirectory{
						CreationInfo: &efs.CreationInfo{
							OwnerGid:    aws.Int64(gid),
							OwnerUid:    aws.Int64(uid),
							Permissions: aws.String(directoryPerms),
						},
						Path: aws.String(directoryPath),
					},
				}

				ctx := context.Background()
				mockEfs.EXPECT().CreateAccessPointWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.CreateAccessPoint(ctx, volName, req)

				if err != nil {
					t.Fatalf("CreateAccessPointFailed is failed: %v", err)
				}

				if res == nil {
					t.Fatal("Result is nil")
				}

				if accessPointId != res.AccessPointId {
					t.Fatalf("AccessPointId mismatched. Expected: %v, Actual: %v", accessPointId, res.AccessPointId)
				}

				if fsId != res.FileSystemId {
					t.Fatalf("FileSystemId mismatched. Expected: %v, Actual: %v", fsId, res.FileSystemId)
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockCtl)
				c := &cloud{efs: mockEfs}

				req := &AccessPointOptions{
					FileSystemId:   fsId,
					Uid:            uid,
					Gid:            gid,
					DirectoryPerms: directoryPerms,
					DirectoryPath:  directoryPath,
				}

				ctx := context.Background()
				mockEfs.EXPECT().CreateAccessPointWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("CreateAccessPointWithContext failed"))
				_, err := c.CreateAccessPoint(ctx, volName, req)
				if err == nil {
					t.Fatalf("CreateAccessPoint did not fail")
				}
				mockCtl.Finish()
			},
		},
		{
			name: "Fail: Access Denied",
			testFunc: func(t *testing.T) {
				mockCtl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockCtl)
				c := &cloud{efs: mockEfs}

				req := &AccessPointOptions{
					FileSystemId:   fsId,
					Uid:            uid,
					Gid:            gid,
					DirectoryPerms: directoryPerms,
					DirectoryPath:  directoryPath,
				}

				ctx := context.Background()
				mockEfs.EXPECT().CreateAccessPointWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, awserr.New(AccessDeniedException, "Access Denied", errors.New("Access Denied")))
				_, err := c.CreateAccessPoint(ctx, volName, req)
				if err == nil {
					t.Fatalf("CreateAccessPoint did not fail")
				}
				if err != ErrAccessDenied {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrAccessDenied, err)
				}
				mockCtl.Finish()
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestDeleteAccessPoint(t *testing.T) {
	var (
		accessPointId = "fsap-abcd1234xyz987"
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DeleteAccessPointOutput{}
				ctx := context.Background()
				mockEfs.EXPECT().DeleteAccessPointWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				err := c.DeleteAccessPoint(ctx, accessPointId)
				if err != nil {
					t.Fatalf("Delete Access Point failed: %v", err)
				}

				mockctl.Finish()
			},
		},
		{
			name: "Fail: Access Point Not Found",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DeleteAccessPointWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, awserr.New(efs.ErrCodeAccessPointNotFound, "Access Point not found", errors.New("DeleteAccessPointWithContext failed")))
				err := c.DeleteAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DeleteAccessPoint did not fail")
				}
				if err != ErrNotFound {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrNotFound, err)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: Access Denied",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DeleteAccessPointWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, awserr.New(AccessDeniedException, "Access Denied", errors.New("Access Denied")))
				err := c.DeleteAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DeleteAccessPoint did not fail")
				}
				if err != ErrAccessDenied {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrAccessDenied, err)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: Other",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DeleteAccessPointWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DeleteAccessPointWithContext failed"))
				err := c.DeleteAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DeleteAccessPoint did not fail")
				}
				mockctl.Finish()
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestDescribeAccessPoint(t *testing.T) {
	var (
		arn                  = "arn:aws:elasticfilesystem:us-east-1:1234567890:access-point/fsap-abcd1234xyz987"
		accessPointId        = "fsap-abcd1234xyz987"
		fsId                 = "fs-abcd1234"
		uid            int64 = 1001
		gid            int64 = 1001
		directoryPerms       = "0777"
		directoryPath        = "/test"
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeAccessPointsOutput{
					AccessPoints: []*efs.AccessPointDescription{
						{
							AccessPointArn: aws.String(arn),
							AccessPointId:  aws.String(accessPointId),
							ClientToken:    aws.String("test"),
							FileSystemId:   aws.String(fsId),
							OwnerId:        aws.String("1234567890"),
							PosixUser: &efs.PosixUser{
								Gid: aws.Int64(gid),
								Uid: aws.Int64(uid),
							},
							RootDirectory: &efs.RootDirectory{
								CreationInfo: &efs.CreationInfo{
									OwnerGid:    aws.Int64(gid),
									OwnerUid:    aws.Int64(uid),
									Permissions: aws.String(directoryPerms),
								},
								Path: aws.String(directoryPath),
							},
						},
					},
					NextToken: nil,
				}
				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPointsWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.DescribeAccessPoint(ctx, accessPointId)
				if err != nil {
					t.Fatalf("Describe Access Point failed: %v", err)
				}

				if res == nil {
					t.Fatal("Result is nil")
				}

				if accessPointId != res.AccessPointId {
					t.Fatalf("AccessPointId mismatched. Expected: %v, Actual: %v", accessPointId, res.AccessPointId)
				}

				if fsId != res.FileSystemId {
					t.Fatalf("FileSystemId mismatched. Expected: %v, Actual: %v", fsId, res.FileSystemId)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: DescribeAccessPoint result has 0 access points",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeAccessPointsOutput{
					AccessPoints: []*efs.AccessPointDescription{},
					NextToken:    nil,
				}
				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPointsWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				_, err := c.DescribeAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DescribeAccessPoint did not fail")
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: DescribeAccessPoint result has more than 1 access points",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeAccessPointsOutput{
					AccessPoints: []*efs.AccessPointDescription{
						{
							AccessPointArn: aws.String(arn),
							AccessPointId:  aws.String(accessPointId),
							ClientToken:    aws.String("test"),
							FileSystemId:   aws.String(fsId),
							OwnerId:        aws.String("1234567890"),
							PosixUser: &efs.PosixUser{
								Gid: aws.Int64(gid),
								Uid: aws.Int64(uid),
							},
							RootDirectory: &efs.RootDirectory{
								CreationInfo: &efs.CreationInfo{
									OwnerGid:    aws.Int64(gid),
									OwnerUid:    aws.Int64(uid),
									Permissions: aws.String(directoryPerms),
								},
								Path: aws.String(directoryPath),
							},
						},
						{
							AccessPointArn: aws.String(arn),
							AccessPointId:  aws.String(accessPointId),
							ClientToken:    aws.String("test"),
							FileSystemId:   aws.String(fsId),
							OwnerId:        aws.String("1234567890"),
							PosixUser: &efs.PosixUser{
								Gid: aws.Int64(gid),
								Uid: aws.Int64(uid),
							},
							RootDirectory: &efs.RootDirectory{
								CreationInfo: &efs.CreationInfo{
									OwnerGid:    aws.Int64(gid),
									OwnerUid:    aws.Int64(uid),
									Permissions: aws.String(directoryPerms),
								},
								Path: aws.String(directoryPath),
							},
						},
					},
					NextToken: nil,
				}
				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPointsWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				_, err := c.DescribeAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DescribeAccessPoint did not fail")
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: Access Point Not Found",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPointsWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, awserr.New(efs.ErrCodeAccessPointNotFound, "Access Point not found", errors.New("DeleteAccessPointWithContext failed")))
				_, err := c.DescribeAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DescribeAccessPoint did not fail")
				}
				if err != ErrNotFound {
					t.Fatalf("Failed. Expected: %v, Actuak: %v", ErrNotFound, err)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: Access Denied",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPointsWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, awserr.New(AccessDeniedException, "Access Denied", errors.New("Access Denied")))
				_, err := c.DescribeAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DescribeAccessPoint did not fail")
				}
				if err != ErrAccessDenied {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrAccessDenied, err)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: Other",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPointsWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DescribeAccessPointWithContext failed"))
				_, err := c.DescribeAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DescribeAccessPoint did not fail")
				}
				mockctl.Finish()
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestDescribeFileSystem(t *testing.T) {
	var (
		fsId = "fs-abcd1234"
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeFileSystemsOutput{
					FileSystems: []*efs.FileSystemDescription{
						{
							CreationToken: aws.String("test"),
							Encrypted:     aws.Bool(true),
							FileSystemId:  aws.String(fsId),
							Name:          aws.String("test"),
							OwnerId:       aws.String("1234567890"),
						},
					},
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystemsWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.DescribeFileSystem(ctx, fsId)
				if err != nil {
					t.Fatalf("Describe File System failed: %v", err)
				}

				if res == nil {
					t.Fatal("Result is nil")
				}

				if fsId != res.FileSystemId {
					t.Fatalf("FileSystemId mismatched. Expected: %v, Actual: %v", fsId, res.FileSystemId)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: DescribeFileSystems result has 0 file systems",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeFileSystemsOutput{
					FileSystems: []*efs.FileSystemDescription{},
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystemsWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				_, err := c.DescribeFileSystem(ctx, fsId)
				if err == nil {
					t.Fatalf("DescribeFileSystem did not fail")
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: DescribeFileSystem result has more than 1 file-system",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeFileSystemsOutput{
					FileSystems: []*efs.FileSystemDescription{
						{
							CreationToken: aws.String("test"),
							Encrypted:     aws.Bool(true),
							FileSystemId:  aws.String(fsId),
							Name:          aws.String("test"),
							OwnerId:       aws.String("1234567890"),
						},
						{
							CreationToken: aws.String("test"),
							Encrypted:     aws.Bool(true),
							FileSystemId:  aws.String(fsId),
							Name:          aws.String("test"),
							OwnerId:       aws.String("1234567890"),
						},
					},
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystemsWithContext(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				_, err := c.DescribeFileSystem(ctx, fsId)
				if err == nil {
					t.Fatalf("DescribeFileSystem did not fail")
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: File System Not Found",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystemsWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, awserr.New(efs.ErrCodeFileSystemNotFound, "File System not found", errors.New("DescribeFileSystemWithContext failed")))
				_, err := c.DescribeFileSystem(ctx, fsId)
				if err == nil {
					t.Fatalf("DescribeFileSystem did not fail")
				}
				if err != ErrNotFound {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrNotFound, err)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: Access Denied",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystemsWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, awserr.New(AccessDeniedException, "Access Denied", errors.New("Access Denied")))
				_, err := c.DescribeFileSystem(ctx, fsId)
				if err == nil {
					t.Fatalf("DescribeFileSystem did not fail")
				}
				if err != ErrAccessDenied {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrAccessDenied, err)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: Other",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystemsWithContext(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DescribeFileSystemWithContext failed"))
				_, err := c.DescribeFileSystem(ctx, fsId)
				if err == nil {
					t.Fatalf("DescribeFileSystem did not fail")
				}
				mockctl.Finish()
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestDescribeMountTargets(t *testing.T) {
	var (
		fsId = "fs-abcd1234"
		az   = "us-east-1a"
		mtId = "fsmt-abcd1234"
	)

	testCases := []struct {
		name        string
		mockOutput  *efs.DescribeMountTargetsOutput
		mockError   error
		expectError errtyp
	}{
		{
			name: "Success: normal flow",
			mockOutput: &efs.DescribeMountTargetsOutput{
				MountTargets: []*efs.MountTargetDescription{
					{
						AvailabilityZoneId:   aws.String("az-id"),
						AvailabilityZoneName: aws.String(az),
						FileSystemId:         aws.String(fsId),
						IpAddress:            aws.String("127.0.0.1"),
						LifeCycleState:       aws.String("available"),
						MountTargetId:        aws.String(mtId),
						NetworkInterfaceId:   aws.String("eni-abcd1234"),
						OwnerId:              aws.String("1234567890"),
						SubnetId:             aws.String("subnet-abcd1234"),
					},
				},
			},
			expectError: errtyp{},
		},
		{
			name: "Success: Mount target with preferred AZ does not exist. Pick random Az.",
			mockOutput: &efs.DescribeMountTargetsOutput{
				MountTargets: []*efs.MountTargetDescription{
					{
						AvailabilityZoneId:   aws.String("az-id"),
						AvailabilityZoneName: aws.String("us-east-1f"),
						FileSystemId:         aws.String(fsId),
						IpAddress:            aws.String("127.0.0.1"),
						LifeCycleState:       aws.String("available"),
						MountTargetId:        aws.String(mtId),
						NetworkInterfaceId:   aws.String("eni-abcd1234"),
						OwnerId:              aws.String("1234567890"),
						SubnetId:             aws.String("subnet-abcd1234"),
					},
				},
			},
			expectError: errtyp{},
		},
		{
			name: "Fail: File system does not have any mount targets",
			mockOutput: &efs.DescribeMountTargetsOutput{
				MountTargets: []*efs.MountTargetDescription{},
			},
			expectError: errtyp{
				code:    "",
				message: "Cannot find mount targets for file system fs-abcd1234. Please create mount targets for file system.",
			},
		},
		{
			name: "Fail: File system does not have any mount targets in available life cycle state",
			mockOutput: &efs.DescribeMountTargetsOutput{
				MountTargets: []*efs.MountTargetDescription{
					{
						AvailabilityZoneId:   aws.String("az-id"),
						AvailabilityZoneName: aws.String(az),
						FileSystemId:         aws.String(fsId),
						IpAddress:            aws.String("127.0.0.1"),
						LifeCycleState:       aws.String("creating"),
						MountTargetId:        aws.String(mtId),
						NetworkInterfaceId:   aws.String("eni-abcd1234"),
						OwnerId:              aws.String("1234567890"),
						SubnetId:             aws.String("subnet-abcd1234"),
					},
				},
			},
			expectError: errtyp{
				code:    "",
				message: "No mount target for file system fs-abcd1234 is in available state. Please retry in 5 minutes.",
			},
		},
		{
			name:        "Fail: File System Not Found",
			mockError:   awserr.New(efs.ErrCodeFileSystemNotFound, "File system not found", errors.New("File system not found")),
			expectError: errtyp{message: "Resource was not found"},
		},
		{
			name:        "Fail: Access Denied",
			mockError:   awserr.New(AccessDeniedException, "Access Denied", errors.New("Access Denied")),
			expectError: errtyp{message: "Access denied"},
		},
		{
			name:        "Fail: Other",
			mockError:   errors.New("DescribeMountTargetsWithContext failed"),
			expectError: errtyp{message: "Describe Mount Targets failed: DescribeMountTargetsWithContext failed"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockctl := gomock.NewController(t)
			defer mockctl.Finish()
			mockEfs := mocks.NewMockEfs(mockctl)
			c := &cloud{efs: mockEfs}
			ctx := context.Background()

			if tc.mockOutput != nil {
				mockEfs.EXPECT().DescribeMountTargetsWithContext(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(tc.mockOutput, nil)
			}

			if tc.mockError != nil {
				mockEfs.EXPECT().DescribeMountTargetsWithContext(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(nil, tc.mockError)
			}

			res, err := c.DescribeMountTargets(ctx, fsId, az)
			testResult(t, "DescribeMountTargets", res, err, tc.expectError)

		})
	}
}

func testResult(t *testing.T, funcName string, ret interface{}, err error, expectError errtyp) {
	if expectError.message == "" {
		if err != nil {
			t.Fatalf("%s is failed: %v", funcName, err)
		}
		if ret == nil {
			t.Fatal("Expected non-nil return value")
		}
	} else {
		if err == nil {
			t.Fatalf("%s is not failed", funcName)
		}

		if err.Error() != expectError.message {
			t.Fatalf("\nExpected error message: %s\nActual error message:   %s", expectError.message, err.Error())
		}
	}
}
