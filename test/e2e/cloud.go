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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/efs"
)

type cloud struct {
	efsclient *efs.EFS
	ec2client *ec2.EC2
}

func NewCloud(region string) *cloud {
	config := &aws.Config{
		Region: aws.String(region),
	}
	sess := session.Must(session.NewSession(config))

	return &cloud{
		efsclient: efs.New(sess),
		ec2client: ec2.New(sess),
	}
}

func (c *cloud) CreateFileSystem(clusterName string) (string, error) {
	tags := []*efs.Tag{
		{
			Key:   aws.String("KubernetesCluster"),
			Value: aws.String(clusterName),
		},
	}

	// Use cluster name as the token
	request := &efs.CreateFileSystemInput{
		CreationToken: aws.String(clusterName),
		Tags:          tags,
	}

	var fileSystemId *string
	response, err := c.efsclient.CreateFileSystem(request)
	if err != nil {
		switch t := err.(type) {
		case *efs.FileSystemAlreadyExists:
			fileSystemId = t.FileSystemId
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

	securityGroupId, err := c.getSecurityGroup(clusterName)
	if err != nil {
		return "", err
	}

	subnetIds, err := c.getSubnetIds(clusterName)
	if err != nil {
		return "", err
	}

	for _, subnetId := range subnetIds {
		request := &efs.CreateMountTargetInput{
			FileSystemId: fileSystemId,
			SubnetId:     aws.String(subnetId),
			SecurityGroups: []*string{
				aws.String(securityGroupId),
			},
		}

		_, err := c.efsclient.CreateMountTarget(request)
		if err != nil {
			switch err.(type) {
			case *efs.MountTargetConflict:
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

	return aws.StringValue(fileSystemId), nil
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
	_, err = c.efsclient.DeleteFileSystem(request)
	if err != nil {
		switch err.(type) {
		case *efs.FileSystemNotFound:
			return nil
		default:
			return err
		}
	}

	return nil
}

// getSecurityGroup returns the node security group ID given cluster name
func (c *cloud) getSecurityGroup(clusterName string) (string, error) {
	// First assume the cluster was installed by kops then fallback to EKS
	groupId, err := c.getKopsSecurityGroup(clusterName)
	if err != nil {
		fmt.Printf("error getting kops node security group: %v\n", err)
	} else {
		return groupId, nil
	}

	groupid, err := c.getEksSecurityGroup(clusterName)
	if err != nil {
		return "", fmt.Errorf("error getting eks node security group: %v", err)
	}
	return groupid, nil

}

// kops names the node security group nodes.$clustername and tags it
// Name=nodes.$clustername. As opposed to masters.$clustername and
// api.$clustername
func (c *cloud) getKopsSecurityGroup(clusterName string) (string, error) {
	request := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("tag:Name"),
				Values: []*string{
					aws.String(fmt.Sprintf("nodes.%s", clusterName)),
				},
			},
		},
	}

	response, err := c.ec2client.DescribeSecurityGroups(request)
	if err != nil {
		return "", err
	}

	if len(response.SecurityGroups) == 0 {
		return "", fmt.Errorf("no security group found for cluster %s", clusterName)
	}

	return aws.StringValue(response.SecurityGroups[0].GroupId), nil
}

// EKS unmanaged node groups:
// The node cloudformation template provided by EKS names the node security
// group *NodeSecurityGroup* and tags it
// aws:cloudformation:logical-id=NodeSecurityGroup
//
// EKS managed node groups:
// EKS doesn't create a separate node security group and instead reuses the the
// cluster one: "EKS created security group applied to ENI that is attached to
// EKS Control Plane master nodes, as well as any managed workloads"
//
// In any case the security group is tagged kubernetes.io/cluster/$clustername
// so filter using that and try to find a security group with "node" in it. If
// no such group exists, use the first one in the response
func (c *cloud) getEksSecurityGroup(clusterName string) (string, error) {
	request := &ec2.DescribeSecurityGroupsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("tag-key"),
				Values: []*string{
					aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)),
				},
			},
		},
	}

	response, err := c.ec2client.DescribeSecurityGroups(request)
	if err != nil {
		return "", err
	}

	if len(response.SecurityGroups) == 0 {
		return "", fmt.Errorf("no security group found for cluster %s", clusterName)
	}

	groupId := aws.StringValue(response.SecurityGroups[0].GroupId)
	for _, sg := range response.SecurityGroups {
		if strings.Contains(strings.ToLower(*sg.GroupName), "node") {
			groupId = aws.StringValue(sg.GroupId)
		}
	}

	return groupId, nil
}

func (c *cloud) getSubnetIds(clusterName string) ([]string, error) {
	request := &ec2.DescribeSubnetsInput{
		Filters: []*ec2.Filter{
			{
				Name: aws.String("tag-key"),
				Values: []*string{
					aws.String(fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)),
				},
			},
		},
	}

	subnetIds := []string{}
	response, err := c.ec2client.DescribeSubnets(request)
	if err != nil {
		return subnetIds, err
	}

	if len(response.Subnets) == 0 {
		return []string{}, fmt.Errorf("no subnets found for cluster %s", clusterName)
	}

	for _, subnet := range response.Subnets {
		subnetIds = append(subnetIds, aws.StringValue(subnet.SubnetId))
	}

	return subnetIds, nil
}

func (c *cloud) ensureFileSystemStatus(fileSystemId, status string) error {
	request := &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(fileSystemId),
	}

	for {
		response, err := c.efsclient.DescribeFileSystems(request)
		if err != nil {
			return err
		}

		if len(response.FileSystems) == 0 {
			return errors.New("no filesystem found")
		}

		if *response.FileSystems[0].LifeCycleState == status {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func (c *cloud) ensureNoMountTarget(fileSystemId string) error {
	request := &efs.DescribeFileSystemsInput{
		FileSystemId: aws.String(fileSystemId),
	}

	for {
		response, err := c.efsclient.DescribeFileSystems(request)
		if err != nil {
			return err
		}

		if len(response.FileSystems) == 0 {
			return errors.New("no filesystem found")
		}

		if *response.FileSystems[0].NumberOfMountTargets == 0 {
			return nil
		}
		time.Sleep(time.Second)
	}
}

func (c *cloud) ensureMountTargetStatus(fileSystemId, status string) error {
	request := &efs.DescribeMountTargetsInput{
		FileSystemId: aws.String(fileSystemId),
	}

	for {
		response, err := c.efsclient.DescribeMountTargets(request)
		if err != nil {
			return err
		}

		done := true
		for _, target := range response.MountTargets {
			if *target.LifeCycleState != status {
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

	response, err := c.efsclient.DescribeMountTargets(request)
	if err != nil {
		return err
	}

	for _, target := range response.MountTargets {
		request := &efs.DeleteMountTargetInput{
			MountTargetId: target.MountTargetId,
		}

		_, err := c.efsclient.DeleteMountTarget(request)
		if err != nil {
			switch err.(type) {
			case *efs.MountTargetNotFound:
				return nil
			default:
				return err
			}
		}
	}

	return nil
}
