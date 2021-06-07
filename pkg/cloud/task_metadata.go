/*
Copyright 2021 The Kubernetes Authors.

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
	"encoding/json"
	"fmt"
	"github.com/kubernetes-sigs/aws-efs-csi-driver/pkg/util"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	taskMetadataV4EnvName = "ECS_CONTAINER_METADATA_URI_V4"
)

type TaskMetadataService interface {
	GetTMDSV4Response() ([]byte, error)
}

type taskMetadata struct {
}

type TMDSV4Response struct {
	Cluster          string `json:"Cluster"`
	TaskARN          string `json:"TaskARN"`
	AvailabilityZone string `json:"AvailabilityZone"`
}

func (taskMetadata taskMetadata) GetTMDSV4Response() ([]byte, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}
	metadataUrl := os.Getenv(taskMetadataV4EnvName)
	if metadataUrl == "" {
		return nil, fmt.Errorf("unable to get taskMetadataV4 environment variable")
	}
	respBody, err := util.GetHttpResponse(client, metadataUrl+"/task")
	if err != nil {
		return nil, fmt.Errorf("unable to get task metadata response: %v", err)
	}

	return respBody, nil
}

// getTaskMetadata return a new ECS MetadataServiceImplementation
func getTaskMetadata(svc TaskMetadataService) (MetadataService, error) {
	metadataResp, err := svc.GetTMDSV4Response()
	if err != nil {
		return nil, fmt.Errorf("unable to get TaskMetadataService %v", err)
	}

	tmdsResp := &TMDSV4Response{}
	err = json.Unmarshal(metadataResp, tmdsResp)
	if err != nil {
		return nil, fmt.Errorf("unable to parse task metadata response body %v", metadataResp)
	}
	taskSplit := strings.Split(tmdsResp.TaskARN, "/")
	taskId := taskSplit[len(taskSplit)-1]
	az := tmdsResp.AvailabilityZone
	region := az[:len(az)-1]
	return &metadata{
		// does not need, but taskId would be a unique better choice
		instanceID:       taskId,
		availabilityZone: az,
		region:           region,
	}, nil
}
