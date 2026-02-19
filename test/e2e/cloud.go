/*
Copyright 2019 The Kubernetes Authors.

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

package e2e

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go-v2/service/s3files"
	s3filestypes "github.com/aws/aws-sdk-go-v2/service/s3files/types"
)

type cloud struct {
	efsclient     *efs.Client
	ec2client     *ec2.Client
	s3filesclient *s3files.Client
	s3client      *s3.Client
	iamclient     *iam.Client
	region        string
}

func NewCloud(region string) *cloud {
	cfg, _ := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	return &cloud{
		efsclient:     efs.NewFromConfig(cfg),
		ec2client:     ec2.NewFromConfig(cfg),
		s3filesclient: s3files.NewFromConfig(cfg),
		s3client:      s3.NewFromConfig(cfg),
		iamclient:     iam.NewFromConfig(cfg),
		region:        region,
	}
}

type CreateOptions struct {
	Name             string
	ClusterName      string
	SecurityGroupIds []string
	SubnetIds        []string
}

type S3FilesResources struct {
	FileSystemId string
	BucketName   string
	RoleName     string
}

func (c *cloud) CreateFileSystem(opts CreateOptions) (string, error) {
	tags := []efstypes.Tag{
		{
			Key:   aws.String("Name"),
			Value: aws.String(opts.Name),
		},
		{
			Key:   aws.String("KubernetesCluster"),
			Value: aws.String(opts.ClusterName),
		},
	}

	// Use cluster name as the token
	request := &efs.CreateFileSystemInput{
		CreationToken: aws.String(opts.ClusterName),
		Tags:          tags,
	}
	ctx := context.TODO()
	var fileSystemId *string
	response, err := c.efsclient.CreateFileSystem(ctx, request)
	if err != nil {
		var FileSystemAlreadyExistsErr *efstypes.FileSystemAlreadyExists
		switch {
		case errors.As(err, &FileSystemAlreadyExistsErr):
			fileSystemId = FileSystemAlreadyExistsErr.FileSystemId
		default:
			return "", err
		}
	} else {
		fileSystemId = response.FileSystemId
	}

	err = c.ensureFileSystemStatus(*fileSystemId, "available")
	if err != nil {
		return "", err
	}

	securityGroupIds := opts.SecurityGroupIds
	if len(securityGroupIds) == 0 {
		securityGroupId, err := c.getSecurityGroupId(opts.ClusterName)
		if err != nil {
			return "", err
		}
		securityGroupIds = []string{
			securityGroupId,
		}
	}
	if len(opts.SubnetIds) == 0 {
		matchingSubnetIds, err := c.getSubnetIds(opts.ClusterName)
		if err != nil {
			return "", err
		}
		opts.SubnetIds = append(opts.SubnetIds, matchingSubnetIds...)
	}

	for _, subnetId := range opts.SubnetIds {
		request := &efs.CreateMountTargetInput{
			FileSystemId:   fileSystemId,
			SubnetId:       &subnetId,
			SecurityGroups: securityGroupIds,
		}

		_, err := c.efsclient.CreateMountTarget(ctx, request)
		if err != nil {
			var MountTargetConflictErr *efstypes.MountTargetConflict
			switch {
			case errors.As(err, &MountTargetConflictErr):
				continue
			default:
				return "", err
			}
		}
	}

	err = c.ensureMountTargetStatus(*fileSystemId, "available")
	if err != nil {
		return "", err
	}

	return *fileSystemId, nil
}

func (c *cloud) DeleteFileSystem(fileSystemId string) error {
	err := c.deleteMountTargets(fileSystemId)
	if err != nil {
		return err
	}
	err = c.ensureNoMountTarget(fileSystemId)
	if err != nil {
		return err
	}
	request := &efs.DeleteFileSystemInput{
		FileSystemId: aws.String(fileSystemId),
	}
	ctx := context.TODO()
	_, err = c.efsclient.DeleteFileSystem(ctx, request)
	if err != nil {
		var FileSystemNotFoundErr *efstypes.FileSystemNotFound
		switch {
		case errors.As(err, &FileSystemNotFoundErr):
			return nil
		default:
			return err
		}
	}

	return nil
}

func (c *cloud) CreateAccessPoint(fileSystemId, clusterName string) (string, error) {
	tags := []efstypes.Tag{
		{
			Key:   aws.String("efs.csi.aws.com/cluster"),
			Value: aws.String("true"),
		},
	}

	request := &efs.CreateAccessPointInput{
		ClientToken:  &clusterName,
		FileSystemId: &fileSystemId,
		PosixUser: &efstypes.PosixUser{
			Gid: aws.Int64(1000),
			Uid: aws.Int64(1000),
		},
		RootDirectory: &efstypes.RootDirectory{
			CreationInfo: &efstypes.CreationInfo{
				OwnerGid:    aws.Int64(1000),
				OwnerUid:    aws.Int64(1000),
				Permissions: aws.String("0777"),
			},
			Path: aws.String("/integ-test"),
		},
		Tags: tags,
	}

	ctx := context.TODO()
	var accessPointId *string
	response, err := c.efsclient.CreateAccessPoint(ctx, request)
	if err != nil {
		return "", err
	}

	accessPointId = response.AccessPointId
	err = c.ensureAccessPointStatus(*accessPointId, "available")
	if err != nil {
		return "", err
	}

	return *accessPointId, nil
}

func (c *cloud) DeleteAccessPoint(accessPointId string) error {
	request := &efs.DeleteAccessPointInput{
		AccessPointId: &accessPointId,
	}

	ctx := context.TODO()
	_, err := c.efsclient.DeleteAccessPoint(ctx, request)
	if err != nil {
		return err
	}
	return nil
}

// getSecurityGroupId returns the node security group ID given cluster name
func (c *cloud) getSecurityGroupId(clusterName string) (string, error) {
	// First assume the cluster was installed by kops then fallback to EKS
	groupId, err := c.getKopsSecurityGroupId(clusterName)
	if err != nil {
		fmt.Printf("error getting kops node security group id: %v\n", err)
	} else {
		return groupId, nil
	}

	groupId, err = c.getEksSecurityGroupId(clusterName)
	if err != nil {
		return "", fmt.Errorf("error getting eks node security group id: %v", err)
	}
	return groupId, nil
}

func (c *cloud) getSubnetIds(clusterName string) ([]string, error) {
	// First assume the cluster was installed by kops then fallback to EKS
	subnetIds, err := c.getKopsSubnetIds(clusterName)
	if err != nil {
		fmt.Printf("error getting kops node subnet ids: %v\n", err)
	} else {
		return subnetIds, nil
	}

	subnetIds, err = c.getEksSubnetIds(clusterName)
	if err != nil {
		return nil, fmt.Errorf("error getting eks node subnet ids: %v", err)
	}
	return subnetIds, nil
}

// kops names the node security group nodes.$clustername and tags it
// Name=nodes.$clustername. As opposed to masters.$clustername and
// api.$clustername
func (c *cloud) getKopsSecurityGroupId(clusterName string) (string, error) {
	securityGroups, err := c.getFilteredSecurityGroups(
		[]ec2types.Filter{
			{
				Name: aws.String("tag:Name"),
				Values: []string{
					fmt.Sprintf("nodes.%s", clusterName),
				},
			},
		},
	)
	if err != nil {
		return "", err
	}

	return *securityGroups[0].GroupId, nil
}

// EKS unmanaged node groups:
// The node cloudformation template provided by EKS names the node security
// group *NodeSecurityGroup* and tags it
// aws:cloudformation:logical-id=NodeSecurityGroup
//
// EKS managed node groups:
// EKS doesn't create a separate node security group and instead reuses the
// cluster one: "EKS created security group applied to ENI that is attached to
// EKS Control Plane master nodes, as well as any managed workloads"
//
// In any case the security group is tagged kubernetes.io/cluster/$clustername
// so filter using that and try to find a security group with "node" in it. If
// no such group exists, use the first one in the response
func (c *cloud) getEksSecurityGroupId(clusterName string) (string, error) {
	securityGroups, err := c.getFilteredSecurityGroups(
		[]ec2types.Filter{
			{
				Name: aws.String("tag-key"),
				Values: []string{
					fmt.Sprintf("kubernetes.io/cluster/%s", clusterName),
				},
			},
		},
	)
	if err != nil {
		return "", err
	}

	securityGroupId := *securityGroups[0].GroupId
	for _, securityGroup := range securityGroups {
		if strings.Contains(strings.ToLower(*securityGroup.GroupName), "node") {
			securityGroupId = *securityGroup.GroupId
		}
	}

	return securityGroupId, nil
}

func (c *cloud) getKopsSubnetIds(clusterName string) ([]string, error) {
	return c.getFilteredSubnetIds(
		[]ec2types.Filter{
			{
				Name: aws.String("tag-key"),
				Values: []string{
					fmt.Sprintf("kubernetes.io/cluster/%s", clusterName),
				},
			},
		},
	)
}

func (c *cloud) getEksSubnetIds(clusterName string) ([]string, error) {
	subnetIds, err := c.getEksctlSubnetIds(clusterName)
	if err != nil {
		return nil, err
	} else if len(subnetIds) > 0 {
		return subnetIds, nil
	}
	return c.getEksCloudFormationSubnetIds(clusterName)
}

func (c *cloud) getEksctlSubnetIds(clusterName string) ([]string, error) {
	return c.getFilteredSubnetIds(
		[]ec2types.Filter{
			{
				Name: aws.String("tag:alpha.eksctl.io/cluster-name"),
				Values: []string{
					fmt.Sprintf("%s", clusterName),
				},
			},
		},
	)
}

func (c *cloud) getEksCloudFormationSubnetIds(clusterName string) ([]string, error) {
	// There are no guarantees about subnets created using the template
	// https://docs.aws.amazon.com/eks/latest/userguide/creating-a-vpc.html
	// because the subnet names are derived from the stack name which is
	// user-supplied. Assume that they are prefixed by cluster name and a dash.
	return c.getFilteredSubnetIds(
		[]ec2types.Filter{
			{
				Name: aws.String("tag:Name"),
				Values: []string{
					fmt.Sprintf("%s-*", clusterName),
				},
			},
		},
	)
}

func (c *cloud) getFilteredSecurityGroups(filters []ec2types.Filter) ([]ec2types.SecurityGroup, error) {
	request := &ec2.DescribeSecurityGroupsInput{
		Filters: filters,
	}

	ctx := context.TODO()
	response, err := c.ec2client.DescribeSecurityGroups(ctx, request)
	if err != nil {
		return nil, err
	}

	if len(response.SecurityGroups) == 0 {
		return nil, fmt.Errorf("no security groups found with filters %s", MarshalToString(filters))
	}

	return response.SecurityGroups, nil
}

func (c *cloud) getFilteredSubnetIds(filters []ec2types.Filter) ([]string, error) {
	request := &ec2.DescribeSubnetsInput{
		Filters: filters,
	}

	ctx := context.TODO()
	subnetIds := []string{}
	response, err := c.ec2client.DescribeSubnets(ctx, request)
	if err != nil {
		return subnetIds, err
	}

	if len(response.Subnets) == 0 {
		return []string{}, fmt.Errorf("no subnets found with filters %s", MarshalToString(filters))
	}

	for _, subnet := range response.Subnets {
		subnetIds = append(subnetIds, *subnet.SubnetId)
	}

	return subnetIds, nil
}

func (c *cloud) ensureFileSystemStatus(fileSystemId, status string) error {
	request := &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(fileSystemId),
	}
	ctx := context.TODO()

	for {
		response, err := c.efsclient.DescribeFileSystems(ctx, request)
		if err != nil {
			return err
		}

		if len(response.FileSystems) == 0 {
			return errors.New("no filesystem found")
		}

		if string(response.FileSystems[0].LifeCycleState) == status {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func (c *cloud) ensureAccessPointStatus(accessPointId, status string) error {
	request := &efs.DescribeAccessPointsInput{
		AccessPointId: aws.String(accessPointId),
	}
	ctx := context.TODO()

	for {
		response, err := c.efsclient.DescribeAccessPoints(ctx, request)
		if err != nil {
			return err
		}

		if len(response.AccessPoints) == 0 {
			return errors.New("no access point found")
		}

		if string(response.AccessPoints[0].LifeCycleState) == status {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func (c *cloud) ensureNoMountTarget(fileSystemId string) error {
	request := &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(fileSystemId),
	}
	ctx := context.TODO()

	for {
		response, err := c.efsclient.DescribeFileSystems(ctx, request)
		if err != nil {
			return err
		}

		if len(response.FileSystems) == 0 {
			return errors.New("no filesystem found")
		}

		if int32(response.FileSystems[0].NumberOfMountTargets) == 0 {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func (c *cloud) ensureMountTargetStatus(fileSystemId, status string) error {
	request := &efs.DescribeMountTargetsInput{
		FileSystemId: aws.String(fileSystemId),
	}

	ctx := context.TODO()
	for {
		response, err := c.efsclient.DescribeMountTargets(ctx, request)
		if err != nil {
			return err
		}

		done := true
		for _, target := range response.MountTargets {
			if string(target.LifeCycleState) != status {
				done = false
				break
			}
		}
		if done {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
}

func (c *cloud) deleteMountTargets(fileSystemId string) error {
	request := &efs.DescribeMountTargetsInput{
		FileSystemId: aws.String(fileSystemId),
	}
	ctx := context.TODO()

	response, err := c.efsclient.DescribeMountTargets(ctx, request)
	if err != nil {
		return err
	}

	for _, target := range response.MountTargets {
		request := &efs.DeleteMountTargetInput{
			MountTargetId: target.MountTargetId,
		}

		_, err := c.efsclient.DeleteMountTarget(ctx, request)
		if err != nil {
			var MountTargetNotFoundErr *efstypes.MountTargetNotFound
			switch {
			case errors.As(err, &MountTargetNotFoundErr):
				return nil
			default:
				return err
			}
		}
	}

	return nil
}

func (c *cloud) CreateS3FilesFileSystem(opts CreateOptions) (*S3FilesResources, error) {
	ctx := context.TODO()

	// Track resources for cleanup
	var bucketName string
	var roleName string
	var fileSystemId *string
	success := false

	// Defer cleanup of resources if creation fails
	defer func() {
		if !success {
			if bucketName != "" {
				fmt.Printf("Cleaning up S3 bucket due to failure: %s\n", bucketName)
				if err := c.deleteS3Bucket(bucketName); err != nil {
					fmt.Printf("Warning: failed to delete S3 bucket %s: %v\n", bucketName, err)
				}
			}
			if roleName != "" {
				fmt.Printf("Cleaning up IAM role due to failure: %s\n", roleName)
				if err := c.deleteIAMRole(roleName); err != nil {
					fmt.Printf("Warning: failed to delete IAM role %s: %v\n", roleName, err)
				}
			}
		}
	}()

	// Step 1: Create S3 bucket
	bucketName = fmt.Sprintf("s3files-csi-e2e-%s", strings.ToLower(opts.ClusterName))
	fmt.Printf("Creating S3 bucket: %s\n", bucketName)

	_, err := c.s3client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
		CreateBucketConfiguration: &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(c.region),
		},
	})
	if err != nil {
		var bucketAlreadyExists *s3types.BucketAlreadyExists
		var bucketAlreadyOwnedByYou *s3types.BucketAlreadyOwnedByYou
		switch {
		case errors.As(err, &bucketAlreadyExists):
			fmt.Printf("Bucket %s already exists\n", bucketName)
		case errors.As(err, &bucketAlreadyOwnedByYou):
			fmt.Printf("Bucket %s already owned by you\n", bucketName)
		default:
			return nil, fmt.Errorf("failed to create S3 bucket: %v", err)
		}
	}

	// Enable versioning on the bucket (required for S3 Files)
	fmt.Printf("Enabling versioning on S3 bucket: %s\n", bucketName)
	_, err = c.s3client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucketName),
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: s3types.BucketVersioningStatusEnabled,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to enable versioning on S3 bucket: %v", err)
	}

	// Step 2: Create IAM role
	roleName = bucketName
	fmt.Printf("Creating IAM role: %s\n", roleName)

	// Trust policy for the role
	trustPolicy := `{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {
					"Service": "elasticfilesystem.amazonaws.com"
				},
				"Action": "sts:AssumeRole"
			}
		]
	}`

	createRoleInput := &iam.CreateRoleInput{
		RoleName:                 aws.String(roleName),
		AssumeRolePolicyDocument: aws.String(trustPolicy),
		Description:              aws.String("Role for S3 Files access"),
		Tags: []iamtypes.Tag{
			{
				Key:   aws.String("Name"),
				Value: aws.String(roleName),
			},
			{
				Key:   aws.String("KubernetesCluster"),
				Value: aws.String(opts.ClusterName),
			},
		},
	}

	roleResponse, err := c.iamclient.CreateRole(ctx, createRoleInput)
	if err != nil {
		var entityAlreadyExists *iamtypes.EntityAlreadyExistsException
		if errors.As(err, &entityAlreadyExists) {
			fmt.Printf("IAM role %s already exists\n", roleName)
			getRoleInput := &iam.GetRoleInput{
				RoleName: aws.String(roleName),
			}
			getRoleResponse, getRoleErr := c.iamclient.GetRole(ctx, getRoleInput)
			if getRoleErr != nil {
				return nil, fmt.Errorf("failed to get existing IAM role: %v", getRoleErr)
			}
			roleResponse = &iam.CreateRoleOutput{
				Role: getRoleResponse.Role,
			}
		} else {
			return nil, fmt.Errorf("failed to create IAM role: %v", err)
		}
	}

	// Step 3: Attach policy to role for S3 access
	policyDocument := fmt.Sprintf(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Action": [
					"s3:PutObject",
					"s3:Get*",
					"s3:DeleteObject",
					"s3:List*",
					"s3:AbortMultipartUpload"
				],
				"Resource": [
					"arn:aws:s3:::%s/*",
					"arn:aws:s3:::%s"
				]
			}
		]
	}`, bucketName, bucketName)

	policyName := fmt.Sprintf("S3FilesPolicy-%s", opts.ClusterName)
	_, err = c.iamclient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(policyDocument),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to attach policy to IAM role: %v", err)
	}

	// Wait for IAM role to propagate
	fmt.Printf("Waiting 30 seconds for IAM role to propagate...\n")
	time.Sleep(30 * time.Second)

	// Step 4: Create S3 Files with bucket and role
	tags := []s3filestypes.Tag{
		{
			Key:   aws.String("Name"),
			Value: aws.String(opts.Name),
		},
		{
			Key:   aws.String("KubernetesCluster"),
			Value: aws.String(opts.ClusterName),
		},
	}

	request := &s3files.CreateFileSystemInput{
		Bucket:      aws.String("arn:aws:s3:::" + bucketName),
		ClientToken: aws.String(opts.ClusterName),
		RoleArn:     roleResponse.Role.Arn,
		Tags:        tags,
	}

	response, err := c.s3filesclient.CreateFileSystem(ctx, request)
	if err != nil {
		var S3FilesAlreadyExistsErr *s3filestypes.ConflictException
		switch {
		case errors.As(err, &S3FilesAlreadyExistsErr):
			fileSystemId = S3FilesAlreadyExistsErr.ResourceId
		default:
			return nil, err
		}
	} else {
		fileSystemId = response.FileSystemId
	}
	fmt.Printf("Created S3Files filesystem: %s\n", *fileSystemId)

	err = c.ensureS3FilesFileSystemStatus(*fileSystemId, s3filestypes.LifeCycleStateAvailable)
	if err != nil {
		return nil, err
	}

	// Step 5: Create mount targets
	securityGroupIds := opts.SecurityGroupIds
	if len(securityGroupIds) == 0 {
		securityGroupId, err := c.getSecurityGroupId(opts.ClusterName)
		if err != nil {
			return nil, err
		}
		securityGroupIds = []string{
			securityGroupId,
		}
	}
	if len(opts.SubnetIds) == 0 {
		matchingSubnetIds, err := c.getSubnetIds(opts.ClusterName)
		if err != nil {
			return nil, err
		}
		opts.SubnetIds = append(opts.SubnetIds, matchingSubnetIds...)
	}

	for _, subnetId := range opts.SubnetIds {
		request := &s3files.CreateMountTargetInput{
			FileSystemId:   fileSystemId,
			SubnetId:       &subnetId,
			SecurityGroups: securityGroupIds,
		}

		_, err := c.s3filesclient.CreateMountTarget(ctx, request)
		if err != nil {
			var MountTargetConflictErr *s3filestypes.ConflictException
			switch {
			case errors.As(err, &MountTargetConflictErr):
				continue
			default:
				return nil, err
			}
		}
		fmt.Printf("Created mount target for subnet %s\n", subnetId)
	}

	err = c.ensureS3FilesMountTargetStatus(*fileSystemId, s3filestypes.LifeCycleStateAvailable)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Successfully created S3Files filesystem %s with bucket %s and role %s\n",
		*fileSystemId, bucketName, roleName)

	// Mark success to prevent cleanup
	success = true

	return &S3FilesResources{
		FileSystemId: *fileSystemId,
		BucketName:   bucketName,
		RoleName:     roleName,
	}, nil
}

func (c *cloud) DeleteS3FilesFileSystem(resources *S3FilesResources) error {
	ctx := context.TODO()

	// Delete mount targets
	err := c.deleteS3FilesMountTargets(resources.FileSystemId)
	if err != nil {
		return err
	}
	err = c.ensureNoS3FilesMountTarget(resources.FileSystemId)
	if err != nil {
		return err
	}

	// Delete the S3 Files file system
	request := &s3files.DeleteFileSystemInput{
		FileSystemId: aws.String(resources.FileSystemId),
		ForceDelete:  aws.Bool(true),
	}
	_, err = c.s3filesclient.DeleteFileSystem(ctx, request)
	if err != nil {
		var FileSystemNotFoundErr *s3filestypes.ResourceNotFoundException
		switch {
		case errors.As(err, &FileSystemNotFoundErr):
			fmt.Printf("S3Files file system %s not found, continuing with cleanup\n", resources.FileSystemId)
		default:
			fmt.Printf("S3Files file system %s failed to be deleted, continuing with cleanup: %v\n", resources.FileSystemId, err)
		}
	} else {
		// Wait for the S3 Files file system to be fully deleted before deleting the bucket
		err = c.ensureNoS3FilesFileSystem(resources.FileSystemId)
		if err != nil {
			fmt.Printf("Warning: failed to confirm S3Files file system %s deletion: %v\n", resources.FileSystemId, err)
		} else {
			fmt.Printf("S3Files file system %s is confirmed to be deleted\n", resources.FileSystemId)
		}
	}

	// Clean up S3 bucket
	fmt.Printf("Cleaning up S3 bucket: %s\n", resources.BucketName)
	err = c.deleteS3Bucket(resources.BucketName)
	if err != nil {
		fmt.Printf("Warning: failed to delete S3 bucket %s: %v\n", resources.BucketName, err)
	}

	// Clean up IAM role
	fmt.Printf("Cleaning up IAM role: %s\n", resources.RoleName)
	err = c.deleteIAMRole(resources.RoleName)
	if err != nil {
		fmt.Printf("Warning: failed to delete IAM role %s: %v\n", resources.RoleName, err)
	}
	return nil
}

func (c *cloud) ensureS3FilesFileSystemStatus(fileSystemId string, status s3filestypes.LifeCycleState) error {
	ctx := context.TODO()

	for {
		request := &s3files.ListFileSystemsInput{}
		response, err := c.s3filesclient.ListFileSystems(ctx, request)
		if err != nil {
			return err
		}

		for _, cd := range response.FileSystems {
			if cd.FileSystemId != nil && *cd.FileSystemId == fileSystemId {
				if cd.Status == status {
					return nil
				}
				statusMsg := ""
				if cd.StatusMessage != nil {
					statusMsg = *cd.StatusMessage
				}
				fmt.Printf("S3Files file system %s status: %s (waiting for %s) with message %s\n", fileSystemId, cd.Status, status, statusMsg)
				break
			}
		}
		time.Sleep(10 * time.Second)
	}
}

func (c *cloud) ensureS3FilesMountTargetStatus(fileSystemId string, status s3filestypes.LifeCycleState) error {
	ctx := context.TODO()
	for {
		request := &s3files.ListMountTargetsInput{
			FileSystemId: aws.String(fileSystemId),
		}
		response, err := c.s3filesclient.ListMountTargets(ctx, request)
		if err != nil {
			return err
		}

		done := true
		for _, target := range response.MountTargets {
			if target.Status != status {
				done = false
				statusMsg := ""
				if target.StatusMessage != nil {
					statusMsg = *target.StatusMessage
				}
				fmt.Printf("S3Files mount target %s status: %s (waiting for %s) with message %s\n", *target.MountTargetId, target.Status, status, statusMsg)
				break
			}
		}
		if done {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
}

func (c *cloud) deleteS3FilesMountTargets(fileSystemId string) error {
	request := &s3files.ListMountTargetsInput{
		FileSystemId: aws.String(fileSystemId),
	}
	ctx := context.TODO()

	response, err := c.s3filesclient.ListMountTargets(ctx, request)
	if err != nil {
		return err
	}

	for _, target := range response.MountTargets {
		request := &s3files.DeleteMountTargetInput{
			MountTargetId: target.MountTargetId,
		}

		_, err := c.s3filesclient.DeleteMountTarget(ctx, request)
		if err != nil {
			var MountTargetNotFoundErr *s3filestypes.ResourceNotFoundException
			switch {
			case errors.As(err, &MountTargetNotFoundErr):
				return nil
			default:
				return err
			}
		}
	}

	return nil
}

func (c *cloud) ensureNoS3FilesMountTarget(fileSystemId string) error {
	ctx := context.TODO()

	for {
		request := &s3files.ListMountTargetsInput{
			FileSystemId: aws.String(fileSystemId),
		}
		response, err := c.s3filesclient.ListMountTargets(ctx, request)
		if err != nil {
			return err
		}

		if len(response.MountTargets) == 0 {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func (c *cloud) ensureNoS3FilesFileSystem(fileSystemId string) error {
	ctx := context.TODO()
	timeout := time.After(5 * time.Minute)

	for {
		select {
		case <-timeout:
			return fmt.Errorf("timed out waiting for S3Files file system %s to be deleted", fileSystemId)
		default:
		}

		request := &s3files.ListFileSystemsInput{}
		response, err := c.s3filesclient.ListFileSystems(ctx, request)
		if err != nil {
			return err
		}

		found := false
		for _, fs := range response.FileSystems {
			if fs.FileSystemId != nil && *fs.FileSystemId == fileSystemId {
				fmt.Printf("S3Files file system %s status: %s (waiting for deletion)\n", fileSystemId, fs.Status)
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		time.Sleep(10 * time.Second)
	}
}

// Helper function to delete S3 bucket
func (c *cloud) deleteS3Bucket(bucketName string) error {
	ctx := context.TODO()

	var keyMarker *string
	var versionMarker *string
	totalDeleted := 0

	for {
		// List object versions
		listInput := &s3.ListObjectVersionsInput{
			Bucket:          aws.String(bucketName),
			KeyMarker:       keyMarker,
			VersionIdMarker: versionMarker,
		}

		listOutput, err := c.s3client.ListObjectVersions(ctx, listInput)
		if err != nil {
			return fmt.Errorf("failed to list object versions: %w", err)
		}

		if len(listOutput.Versions) == 0 && len(listOutput.DeleteMarkers) == 0 {
			break
		}

		// Prepare objects to delete
		var objectsToDelete []types.ObjectIdentifier

		// Add all versions
		for _, version := range listOutput.Versions {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
				Key:       version.Key,
				VersionId: version.VersionId,
			})
		}

		// Add all delete markers
		for _, marker := range listOutput.DeleteMarkers {
			objectsToDelete = append(objectsToDelete, types.ObjectIdentifier{
				Key:       marker.Key,
				VersionId: marker.VersionId,
			})
		}

		if len(objectsToDelete) > 0 {
			// Delete objects in batch
			deleteInput := &s3.DeleteObjectsInput{
				Bucket: aws.String(bucketName),
				Delete: &types.Delete{
					Objects: objectsToDelete,
					Quiet:   new(bool), // false to get detailed response
				},
			}

			deleteOutput, err := c.s3client.DeleteObjects(ctx, deleteInput)
			if err != nil {
				return fmt.Errorf("failed to delete objects: %w", err)
			}

			totalDeleted += len(deleteOutput.Deleted)
			fmt.Printf("Deleted %d objects/versions (total: %d)\n", len(deleteOutput.Deleted), totalDeleted)

			// Check for errors
			if len(deleteOutput.Errors) > 0 {
				for _, delErr := range deleteOutput.Errors {
					fmt.Printf("Error deleting %s (version %s): %s - %s\n",
						*delErr.Key, *delErr.VersionId, *delErr.Code, *delErr.Message)
				}
			}
		}

		// Check if there are more objects to list
		if !*listOutput.IsTruncated {
			break
		}

		// Set markers for next iteration
		keyMarker = listOutput.NextKeyMarker
		versionMarker = listOutput.NextVersionIdMarker
	}

	fmt.Printf("Total objects/versions deleted: %d\n", totalDeleted)

	// Delete the bucket
	_, err := c.s3client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		fmt.Printf("Warning: failed to delete S3 bucket %s: %v\n", bucketName, err)
		return err
	}

	fmt.Printf("Successfully deleted S3 bucket: %s\n", bucketName)
	return nil
}

// Helper function to delete IAM role and its policies
func (c *cloud) deleteIAMRole(roleName string) error {
	ctx := context.TODO()

	// First, delete all inline policies attached to the role
	listRolePoliciesInput := &iam.ListRolePoliciesInput{
		RoleName: aws.String(roleName),
	}

	listRolePoliciesOutput, err := c.iamclient.ListRolePolicies(ctx, listRolePoliciesInput)
	if err != nil {
		fmt.Printf("Warning: failed to list policies for role %s: %v\n", roleName, err)
	} else {
		for _, policyName := range listRolePoliciesOutput.PolicyNames {
			deleteRolePolicyInput := &iam.DeleteRolePolicyInput{
				RoleName:   aws.String(roleName),
				PolicyName: aws.String(policyName),
			}

			_, err = c.iamclient.DeleteRolePolicy(ctx, deleteRolePolicyInput)
			if err != nil {
				fmt.Printf("Warning: failed to delete policy %s from role %s: %v\n", policyName, roleName, err)
			}
		}
	}

	// Delete the role
	deleteRoleInput := &iam.DeleteRoleInput{
		RoleName: aws.String(roleName),
	}

	_, err = c.iamclient.DeleteRole(ctx, deleteRoleInput)
	if err != nil {
		fmt.Printf("Warning: failed to delete IAM role %s: %v\n", roleName, err)
		return err
	}

	fmt.Printf("Successfully deleted IAM role: %s\n", roleName)
	return nil
}

func MarshalToString(data interface{}) string {
	jsonBytes, _ := json.MarshalIndent(data, "", "  ")
	return string(jsonBytes)
}
