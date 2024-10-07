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
	"github.com/aws/aws-sdk-go-v2/config"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/efs"
	efstypes "github.com/aws/aws-sdk-go-v2/service/efs/types"
)

type cloud struct {
	efsclient *efs.Client
	ec2client *ec2.Client
}

func NewCloud(region string) *cloud {
	cfg, _ := config.LoadDefaultConfig(context.TODO(), config.WithRegion(region))
	return &cloud{
		efsclient: efs.NewFromConfig(cfg),
		ec2client: ec2.NewFromConfig(cfg),
	}
}

type CreateOptions struct {
	Name             string
	ClusterName      string
	SecurityGroupIds []string
	SubnetIds        []string
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

func MarshalToString(data interface{}) string {
	jsonBytes, _ := json.MarshalIndent(data, "", "  ")
	return string(jsonBytes)
}
