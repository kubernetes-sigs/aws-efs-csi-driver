package cloud

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/smithy-go"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/cloud/mocks"
)

type errtyp struct {
	code    string
	message string
}

var (
	ErrCodeAccessPointNotFound = "AccessPointNotFound"
	ErrCodeFileSystemNotFound  = "FileSystemNotFound"
)

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
		clientToken          = volName
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
					PosixUser: &types.PosixUser{
						Gid: aws.Int64(gid),
						Uid: aws.Int64(uid),
					},
					RootDirectory: &types.RootDirectory{
						CreationInfo: &types.CreationInfo{
							OwnerGid:    aws.Int64(gid),
							OwnerUid:    aws.Int64(uid),
							Permissions: aws.String(directoryPerms),
						},
						Path: aws.String(directoryPath),
					},
				}

				ctx := context.Background()
				mockEfs.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.CreateAccessPoint(ctx, clientToken, req)

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
				mockEfs.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("CreateAccessPoint failed"))
				_, err := c.CreateAccessPoint(ctx, clientToken, req)
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
				mockEfs.EXPECT().CreateAccessPoint(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&smithy.GenericAPIError{
						Code:    AccessDeniedException,
						Message: "Access Denied",
					})
				_, err := c.CreateAccessPoint(ctx, clientToken, req)
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
				mockEfs.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
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
				mockEfs.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&types.AccessPointNotFound{
						Message: aws.String("Access Point not found"),
					})
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
				mockEfs.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&smithy.GenericAPIError{
						Code:    AccessDeniedException,
						Message: "Access Denied",
					})
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
				mockEfs.EXPECT().DeleteAccessPoint(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DeleteAccessPoint failed"))
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
					AccessPoints: []types.AccessPointDescription{
						{
							AccessPointArn: aws.String(arn),
							AccessPointId:  aws.String(accessPointId),
							ClientToken:    aws.String("test"),
							FileSystemId:   aws.String(fsId),
							OwnerId:        aws.String("1234567890"),
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(gid),
								Uid: aws.Int64(uid),
							},
							RootDirectory: &types.RootDirectory{
								CreationInfo: &types.CreationInfo{
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
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
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
			name: "Success - nil Posix user",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeAccessPointsOutput{
					AccessPoints: []types.AccessPointDescription{
						{
							AccessPointArn: aws.String(arn),
							AccessPointId:  aws.String(accessPointId),
							ClientToken:    aws.String("test"),
							FileSystemId:   aws.String(fsId),
							OwnerId:        aws.String("1234567890"),
							RootDirectory: &types.RootDirectory{
								CreationInfo: &types.CreationInfo{
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
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
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
					AccessPoints: []types.AccessPointDescription{},
					NextToken:    nil,
				}
				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
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
					AccessPoints: []types.AccessPointDescription{
						{
							AccessPointArn: aws.String(arn),
							AccessPointId:  aws.String(accessPointId),
							ClientToken:    aws.String("test"),
							FileSystemId:   aws.String(fsId),
							OwnerId:        aws.String("1234567890"),
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(gid),
								Uid: aws.Int64(uid),
							},
							RootDirectory: &types.RootDirectory{
								CreationInfo: &types.CreationInfo{
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
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(gid),
								Uid: aws.Int64(uid),
							},
							RootDirectory: &types.RootDirectory{
								CreationInfo: &types.CreationInfo{
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
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
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
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&types.AccessPointNotFound{
						Message: aws.String("Access Point not found"),
					})
				_, err := c.DescribeAccessPoint(ctx, accessPointId)
				if err == nil {
					t.Fatalf("DescribeAccessPoint did not fail")
				}
				if err != ErrNotFound {
					t.Fatalf("Failed. Expected: %v, Actual: %v", ErrNotFound, err)
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
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&smithy.GenericAPIError{
						Code:    AccessDeniedException,
						Message: "Access Denied",
					})
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
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DescribeAccessPoint failed"))
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

func TestFindAccessPointByClientToken(t *testing.T) {
	var (
		fsId                = "fs-abcd1234"
		accessPointId       = "ap-abc123"
		clientToken         = "token"
		path                = "/myDir"
		Gid           int64 = 1000
		Uid           int64 = 1000
	)
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success - clientToken found",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeAccessPointsOutput{
					AccessPoints: []types.AccessPointDescription{
						{
							AccessPointId: aws.String(accessPointId),
							FileSystemId:  aws.String(fsId),
							ClientToken:   aws.String(clientToken),
							RootDirectory: &types.RootDirectory{
								Path: aws.String(path),
							},
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(Gid),
								Uid: aws.Int64(Uid),
							},
						},
					},
					NextToken: nil,
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.FindAccessPointByClientToken(ctx, clientToken, fsId)
				if err != nil {
					t.Fatalf("Find Access Point by Client Token failed: %v", err)
				}

				if res == nil {
					t.Fatal("Result is nil")
				}

				mockctl.Finish()
			},
		},
		{
			name: "Success - nil result if clientToken is not found",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeAccessPointsOutput{
					AccessPoints: []types.AccessPointDescription{
						{
							AccessPointId: aws.String(accessPointId),
							FileSystemId:  aws.String(fsId),
							ClientToken:   aws.String("differentToken"),
							RootDirectory: &types.RootDirectory{
								Path: aws.String(path),
							},
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(Gid),
								Uid: aws.Int64(Uid),
							},
						},
					},
					NextToken: nil,
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.FindAccessPointByClientToken(ctx, clientToken, fsId)
				if err != nil {
					t.Fatalf("Find Access Point by Client Token failed: %v", err)
				}

				if res != nil {
					t.Fatal("Result should be nil. No access point with the specified token")
				}

				mockctl.Finish()
			},
		},
		{
			name: "Fail - Access Denied",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}
				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&smithy.GenericAPIError{
						Code:    AccessDeniedException,
						Message: "Access Denied",
					})
				_, err := c.FindAccessPointByClientToken(ctx, clientToken, fsId)
				if err == nil {
					t.Fatalf("Find Access Point by Client Token should have failed: %v", err)
				}

				mockctl.Finish()
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func TestListAccessPoints(t *testing.T) {
	var (
		fsId                = "fs-abcd1234"
		accessPointId       = "ap-abc123"
		Gid           int64 = 1000
		Uid           int64 = 1000
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
					AccessPoints: []types.AccessPointDescription{
						{
							AccessPointId: aws.String(accessPointId),
							FileSystemId:  aws.String(fsId),
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(Gid),
								Uid: aws.Int64(Uid),
							},
						},
					},
					NextToken: nil,
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.ListAccessPoints(ctx, fsId)
				if err != nil {
					t.Fatalf("List Access Points failed: %v", err)
				}

				if res == nil {
					t.Fatal("Result is nil")
				}

				if len(res) != 1 {
					t.Fatalf("Expected only one AccessPoint in response but got: %v", res)
				}

				mockctl.Finish()
			},
		},
		{
			name: "Success - multiple access points",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				output := &efs.DescribeAccessPointsOutput{
					AccessPoints: []types.AccessPointDescription{
						{
							AccessPointId: aws.String(accessPointId),
							FileSystemId:  aws.String(fsId),
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(Gid),
								Uid: aws.Int64(Uid),
							},
						},
						{
							AccessPointId: aws.String(accessPointId),
							FileSystemId:  aws.String(fsId),
							PosixUser: &types.PosixUser{
								Gid: aws.Int64(1001),
								Uid: aws.Int64(1001),
							},
						},
					},
					NextToken: nil,
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.ListAccessPoints(ctx, fsId)
				if err != nil {
					t.Fatalf("List Access Points failed: %v", err)
				}

				if res == nil {
					t.Fatal("Result is nil")
				}

				if len(res) != 2 {
					t.Fatalf("Expected two AccessPoints in response but got: %v", res)
				}

				mockctl.Finish()
			},
		},
		{
			name: "Fail - Access Denied",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}
				ctx := context.Background()
				mockEfs.EXPECT().DescribeAccessPoints(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&smithy.GenericAPIError{
						Code:    AccessDeniedException,
						Message: "Access Denied",
					})
				_, err := c.ListAccessPoints(ctx, fsId)
				if err == nil {
					t.Fatalf("List Access Points should have failed: %v", err)
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
					FileSystems: []types.FileSystemDescription{
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
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				res, err := c.DescribeFileSystemById(ctx, fsId)
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
					FileSystems: []types.FileSystemDescription{},
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				_, err := c.DescribeFileSystemById(ctx, fsId)
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
					FileSystems: []types.FileSystemDescription{
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
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(output, nil)
				_, err := c.DescribeFileSystemById(ctx, fsId)
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
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&types.FileSystemNotFound{
						Message: aws.String("File System not found"),
					})
				_, err := c.DescribeFileSystemById(ctx, fsId)
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
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(nil,
					&smithy.GenericAPIError{
						Code:    AccessDeniedException,
						Message: "Access Denied",
					})
				_, err := c.DescribeFileSystemById(ctx, fsId)
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
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(nil, errors.New("DescribeFileSystem failed"))
				_, err := c.DescribeFileSystemById(ctx, fsId)
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
				MountTargets: []types.MountTargetDescription{
					{
						AvailabilityZoneId:   aws.String("az-id"),
						AvailabilityZoneName: aws.String(az),
						FileSystemId:         aws.String(fsId),
						IpAddress:            aws.String("127.0.0.1"),
						LifeCycleState:       types.LifeCycleStateAvailable,
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
				MountTargets: []types.MountTargetDescription{
					{
						AvailabilityZoneId:   aws.String("az-id"),
						AvailabilityZoneName: aws.String("us-east-1f"),
						FileSystemId:         aws.String(fsId),
						IpAddress:            aws.String("127.0.0.1"),
						LifeCycleState:       types.LifeCycleStateAvailable,
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
				MountTargets: []types.MountTargetDescription{},
			},
			expectError: errtyp{
				code:    "",
				message: "Cannot find mount targets for file system fs-abcd1234. Please create mount targets for file system.",
			},
		},
		{
			name: "Fail: File system does not have any mount targets in available life cycle state",
			mockOutput: &efs.DescribeMountTargetsOutput{
				MountTargets: []types.MountTargetDescription{
					{
						AvailabilityZoneId:   aws.String("az-id"),
						AvailabilityZoneName: aws.String(az),
						FileSystemId:         aws.String(fsId),
						IpAddress:            aws.String("127.0.0.1"),
						LifeCycleState:       types.LifeCycleStateCreating,
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
			name: "Fail: File System Not Found",
			mockError: &types.FileSystemNotFound{
				Message: aws.String("File System not found"),
			},
			expectError: errtyp{message: "Resource was not found"},
		},
		{
			name: "Fail: Access Denied",
			mockError: &smithy.GenericAPIError{
				Code:    AccessDeniedException,
				Message: "Access Denied",
			},
			expectError: errtyp{message: "Access denied"},
		},
		{
			name:        "Fail: Other",
			mockError:   errors.New("DescribeMountTargets failed"),
			expectError: errtyp{message: "Describe Mount Targets failed: DescribeMountTargets failed"},
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
				mockEfs.EXPECT().DescribeMountTargets(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(tc.mockOutput, nil)
			}

			if tc.mockError != nil {
				mockEfs.EXPECT().DescribeMountTargets(gomock.Eq(ctx), gomock.Any(), gomock.Any()).Return(nil, tc.mockError)
			}

			res, err := c.DescribeMountTargets(ctx, fsId, az)
			testResult(t, "DescribeMountTargets", res, err, tc.expectError)

		})
	}
}

func TestDescribeFileSystemByToken(t *testing.T) {
	var (
		fsId          = []string{"fs-abcd1234", "fs-efgh5678"}
		fsArn         = []string{"arn:aws:elasticfilesystem:us-west-2:1111333322228888:file-system/fs-0123456789abcdef8", "arn:aws:elasticfilesystem:us-west-2:1111333322228888:file-system/fs-987654321abcdef0"}
		creationToken = "efs-for-discovery"
		az            = "us-east-1a"
	)

	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{
		{
			name: "Success: Normal flow",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				fs := &efs.DescribeFileSystemsOutput{
					FileSystems: []types.FileSystemDescription{
						{
							FileSystemId:         aws.String(fsId[0]),
							FileSystemArn:        aws.String(fsArn[0]),
							Encrypted:            aws.Bool(true),
							CreationToken:        aws.String("efs-for-discovery"),
							AvailabilityZoneName: aws.String(az),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("prod"),
								},
								{
									Key:   aws.String("owner"),
									Value: aws.String("avanishpatil23@gmail.com"),
								},
							},
						},
						{
							FileSystemId:         aws.String(fsId[1]),
							FileSystemArn:        aws.String(fsArn[1]),
							Encrypted:            aws.Bool(true),
							CreationToken:        aws.String("efs-not-for-discovery"),
							AvailabilityZoneName: aws.String(az),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("prod"),
								},
								{
									Key:   aws.String("owner"),
									Value: aws.String("avanishpatil23@gmail.com"),
								},
							},
						},
					},
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).DoAndReturn(func(ctx context.Context, input *efs.DescribeFileSystemsInput, opts ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error) {
					res := &efs.DescribeFileSystemsOutput{}
					for _, fileSystem := range fs.FileSystems {
						if input.CreationToken != nil && *fileSystem.CreationToken == *input.CreationToken {
							res.FileSystems = append(res.FileSystems, fileSystem)
						} else if input.CreationToken == nil {
							res.FileSystems = append(res.FileSystems, fileSystem)
						}
					}
					return res, nil
				})

				efsList, err := c.DescribeFileSystemByToken(ctx, creationToken)
				if err != nil {
					t.Fatalf("DescribeFileSystem failed")
				}
				if len(efsList) != 1 {
					t.Fatalf("Expected 1 fileSystems got %d", len(efsList))
				}
				mockctl.Finish()
			},
		},
		{
			name: "Success: Normal flow without creation token",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				fs := &efs.DescribeFileSystemsOutput{
					FileSystems: []types.FileSystemDescription{
						{
							FileSystemId:         aws.String(fsId[0]),
							FileSystemArn:        aws.String(fsArn[0]),
							Encrypted:            aws.Bool(true),
							CreationToken:        aws.String("efs-for-discovery"),
							AvailabilityZoneName: aws.String(az),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("prod"),
								},
								{
									Key:   aws.String("owner"),
									Value: aws.String("avanishpatil23@gmail.com"),
								},
							},
						},
						{
							FileSystemId:         aws.String(fsId[1]),
							FileSystemArn:        aws.String(fsArn[1]),
							Encrypted:            aws.Bool(true),
							CreationToken:        aws.String("efs-not-for-discovery"),
							AvailabilityZoneName: aws.String(az),
							Tags: []types.Tag{
								{
									Key:   aws.String("env"),
									Value: aws.String("prod"),
								},
								{
									Key:   aws.String("owner"),
									Value: aws.String("avanishpatil23@gmail.com"),
								},
							},
						},
					},
				}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).DoAndReturn(func(ctx context.Context, input *efs.DescribeFileSystemsInput, opts ...func(*efs.Options)) (*efs.DescribeFileSystemsOutput, error) {
					res := &efs.DescribeFileSystemsOutput{}
					for _, fileSystem := range fs.FileSystems {
						if input.CreationToken != nil && *fileSystem.CreationToken == *input.CreationToken {
							res.FileSystems = append(res.FileSystems, fileSystem)
						} else if input.CreationToken == nil {
							res.FileSystems = append(res.FileSystems, fileSystem)
						}
					}
					return res, nil
				})

				efsList, err := c.DescribeFileSystemByToken(ctx, "")
				if err != nil {
					t.Fatalf("DescribeFileSystem failed")
				}
				if len(efsList) != len(fs.FileSystems) {
					t.Fatalf("Expected 1 fileSystems got %d", len(efsList))
				}
				for i, fileSystem := range fs.FileSystems {
					for _, v := range fileSystem.Tags {
						if val, exists := efsList[i].Tags[*v.Key]; !exists || val != *v.Value {
							t.Fatalf("Tags list is corrupted, expected %s for %s but got %s", *v.Value, *v.Key, val)
						}
					}
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
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(nil, &smithy.GenericAPIError{
					Code:    AccessDeniedException,
					Message: "Access Denied",
				})

				_, err := c.DescribeFileSystemByToken(ctx, "efs-discovery")
				if err == nil {
					t.Fatalf("DescribeFileSystemByToken did not fail")
				}
				if err != ErrAccessDenied {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrAccessDenied, err)
				}
				mockctl.Finish()
			},
		},
		{
			name: "Fail: File System not found",
			testFunc: func(t *testing.T) {
				mockctl := gomock.NewController(t)
				mockEfs := mocks.NewMockEfs(mockctl)
				c := &cloud{efs: mockEfs}

				ctx := context.Background()
				mockEfs.EXPECT().DescribeFileSystems(gomock.Eq(ctx), gomock.Any()).Return(nil, &types.FileSystemNotFound{
					Message: aws.String("File System not found"),
				})

				_, err := c.DescribeFileSystemByToken(ctx, "efs-discovery")
				if err == nil {
					t.Fatalf("DescribeFileSystemByToken did not fail")
				}
				if err != ErrNotFound {
					t.Fatalf("Failed. Expected: %v, Actual:%v", ErrNotFound, err)
				}
				mockctl.Finish()
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
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

func Test_findAccessPointByPath(t *testing.T) {
	fsId := "testFsId"
	clientToken := "testPvcName"
	dirPath := "testPath"
	diffClientToken := aws.String("diff")

	mockctl := gomock.NewController(t)
	defer mockctl.Finish()
	mockEfs := mocks.NewMockEfs(mockctl)

	expectedSingleAP := &AccessPoint{
		AccessPointId:      "testApId",
		AccessPointRootDir: dirPath,
		FileSystemId:       fsId,
	}

	type args struct {
		clientToken     string
		accessPointOpts *AccessPointOptions
	}
	tests := []struct {
		name            string
		args            args
		prepare         func(*mocks.MockEfs)
		wantAccessPoint *AccessPoint
		wantErr         bool
	}{
		{name: "Expected_ClientToken_Not_Found", args: args{clientToken, &AccessPointOptions{FileSystemId: fsId, DirectoryPath: dirPath}}, prepare: func(mockEfs *mocks.MockEfs) {
			mockEfs.EXPECT().DescribeAccessPoints(gomock.Any(), gomock.Any()).Return(&efs.DescribeAccessPointsOutput{
				AccessPoints: []types.AccessPointDescription{{FileSystemId: aws.String(fsId), ClientToken: diffClientToken, AccessPointId: aws.String(expectedSingleAP.AccessPointId), RootDirectory: &types.RootDirectory{Path: aws.String("differentPath")}}},
			}, nil)
		}, wantAccessPoint: nil, wantErr: false},
		{name: "Expected_Path_Found_In_Multiple_APs_And_One_AP_Filtered_By_ClientToken", args: args{clientToken, &AccessPointOptions{FileSystemId: fsId, DirectoryPath: dirPath}}, prepare: func(mockEfs *mocks.MockEfs) {
			mockEfs.EXPECT().DescribeAccessPoints(gomock.Any(), gomock.Any()).Return(&efs.DescribeAccessPointsOutput{
				AccessPoints: []types.AccessPointDescription{
					{FileSystemId: aws.String(fsId), ClientToken: diffClientToken, AccessPointId: aws.String("differentApId"), RootDirectory: &types.RootDirectory{Path: aws.String(expectedSingleAP.AccessPointRootDir)}},
					{FileSystemId: aws.String(fsId), ClientToken: &clientToken, AccessPointId: aws.String(expectedSingleAP.AccessPointId), RootDirectory: &types.RootDirectory{Path: aws.String(expectedSingleAP.AccessPointRootDir)}},
				},
			}, nil)
		}, wantAccessPoint: expectedSingleAP, wantErr: false},
		{name: "Fail_DescribeAccessPoints", args: args{clientToken, &AccessPointOptions{FileSystemId: fsId, DirectoryPath: dirPath}}, prepare: func(mockEfs *mocks.MockEfs) {
			mockEfs.EXPECT().DescribeAccessPoints(gomock.Any(), gomock.Any()).Return(nil, errors.New("access_denied"))
		}, wantAccessPoint: nil, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cloud{efs: mockEfs}
			ctx := context.Background()

			if tt.prepare != nil {
				tt.prepare(mockEfs)
			}

			gotAccessPoint, err := c.FindAccessPointByClientToken(ctx, tt.args.clientToken, tt.args.accessPointOpts.FileSystemId)
			if (err != nil) != tt.wantErr {
				t.Errorf("findAccessPointByClientToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotAccessPoint, tt.wantAccessPoint) {
				t.Errorf("findAccessPointByClientToken() gotAccessPoint = %v, want %v", gotAccessPoint, tt.wantAccessPoint)
			}
		})
	}
}
