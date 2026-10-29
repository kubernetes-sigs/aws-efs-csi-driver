/*
Copyright 2026 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cloud

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	"github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/aws/aws-sdk-go-v2/service/s3files"
	s3filestypes "github.com/aws/aws-sdk-go-v2/service/s3files/types"
	"github.com/aws/smithy-go"
	"github.com/golang/mock/gomock"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
)

func TestGetAccessPointTags(t *testing.T) {
	ctx := context.Background()
	want := map[string]string{"example.com/cluster": "my-cluster"}

	t.Run("efs", func(t *testing.T) {
		mockCtl, c, mockEfs, _ := setupTest(t)
		defer mockCtl.Finish()
		mockEfs.EXPECT().DescribeAccessPoints(gomock.Any(), &efs.DescribeAccessPointsInput{AccessPointId: aws.String("fsap-abcd1234")}, gomock.Any()).
			Return(&efs.DescribeAccessPointsOutput{AccessPoints: []types.AccessPointDescription{{
				Tags: []types.Tag{{Key: aws.String("example.com/cluster"), Value: aws.String("my-cluster")}},
			}}}, nil)
		got, err := c.GetAccessPointTags(ctx, "fsap-abcd1234", util.FileSystemTypeEFS)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("efs not found", func(t *testing.T) {
		mockCtl, c, mockEfs, _ := setupTest(t)
		defer mockCtl.Finish()
		mockEfs.EXPECT().DescribeAccessPoints(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &types.AccessPointNotFound{})
		if _, err := c.GetAccessPointTags(ctx, "fsap-abcd1234", util.FileSystemTypeEFS); err != ErrNotFound {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("s3files", func(t *testing.T) {
		mockCtl, c, _, mockS3Files := setupTest(t)
		defer mockCtl.Finish()
		mockS3Files.EXPECT().GetAccessPoint(gomock.Any(), &s3files.GetAccessPointInput{AccessPointId: aws.String("fsap-abcd1234")}, gomock.Any()).
			Return(&s3files.GetAccessPointOutput{Tags: []s3filestypes.Tag{{Key: aws.String("example.com/cluster"), Value: aws.String("my-cluster")}}}, nil)
		got, err := c.GetAccessPointTags(ctx, "fsap-abcd1234", util.FileSystemTypeS3Files)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v", got, err)
		}
	})
}

func TestGetFileSystemTags(t *testing.T) {
	ctx := context.Background()
	want := map[string]string{"example.com/cluster": "my-cluster"}

	t.Run("efs", func(t *testing.T) {
		mockCtl, c, mockEfs, _ := setupTest(t)
		defer mockCtl.Finish()
		mockEfs.EXPECT().DescribeFileSystems(gomock.Any(), &efs.DescribeFileSystemsInput{FileSystemId: aws.String("fs-abcd1234")}, gomock.Any()).
			Return(&efs.DescribeFileSystemsOutput{FileSystems: []types.FileSystemDescription{{
				Tags: []types.Tag{{Key: aws.String("example.com/cluster"), Value: aws.String("my-cluster")}},
			}}}, nil)
		got, err := c.GetFileSystemTags(ctx, "fs-abcd1234", util.FileSystemTypeEFS)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v", got, err)
		}
	})

	t.Run("s3files", func(t *testing.T) {
		mockCtl, c, _, mockS3Files := setupTest(t)
		defer mockCtl.Finish()
		mockS3Files.EXPECT().GetFileSystem(gomock.Any(), &s3files.GetFileSystemInput{FileSystemId: aws.String("fs-abcd1234")}, gomock.Any()).
			Return(&s3files.GetFileSystemOutput{Tags: []s3filestypes.Tag{{Key: aws.String("example.com/cluster"), Value: aws.String("my-cluster")}}}, nil)
		got, err := c.GetFileSystemTags(ctx, "fs-abcd1234", util.FileSystemTypeS3Files)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, %v", got, err)
		}
	})
}

func TestTagAccessPoint(t *testing.T) {
	ctx := context.Background()
	tags := map[string]string{"example.com/cluster": "my-cluster"}

	t.Run("efs sends only the given tags", func(t *testing.T) {
		mockCtl, c, mockEfs, _ := setupTest(t)
		defer mockCtl.Finish()
		mockEfs.EXPECT().TagResource(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, in *efs.TagResourceInput, _ ...func(*efs.Options)) (*efs.TagResourceOutput, error) {
				if *in.ResourceId != "fsap-abcd1234" || len(in.Tags) != 1 || *in.Tags[0].Key != "example.com/cluster" || *in.Tags[0].Value != "my-cluster" {
					t.Fatalf("unexpected TagResource input %+v", in)
				}
				return &efs.TagResourceOutput{}, nil
			})
		if err := c.TagAccessPoint(ctx, "fsap-abcd1234", tags, util.FileSystemTypeEFS); err != nil {
			t.Fatalf("TagAccessPoint failed: %v", err)
		}
	})

	t.Run("s3files access denied", func(t *testing.T) {
		mockCtl, c, _, mockS3Files := setupTest(t)
		defer mockCtl.Finish()
		mockS3Files.EXPECT().TagResource(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, &smithy.GenericAPIError{Code: AccessDeniedException, Message: "denied"})
		err := c.TagAccessPoint(ctx, "fsap-abcd1234", tags, util.FileSystemTypeS3Files)
		if !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("expected ErrAccessDenied, got %v", err)
		}
	})
}
