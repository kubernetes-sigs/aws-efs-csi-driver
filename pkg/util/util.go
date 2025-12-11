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

package util

import (
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
)

func ParseEndpoint(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", fmt.Errorf("could not parse endpoint: %v", err)
	}

	addr := path.Join(u.Host, filepath.FromSlash(u.Path))

	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "tcp":
	case "unix":
		addr = path.Join("/", addr)
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return "", "", fmt.Errorf("could not remove unix domain socket %q: %v", addr, err)
		}
	default:
		return "", "", fmt.Errorf("unsupported protocol: %s", scheme)
	}

	return scheme, addr, nil
}

func GetHttpResponse(client *http.Client, endpoint string) ([]byte, error) {
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("could not get data from %v %v", endpoint, err)
	}
	if resp.Body != nil {
		defer resp.Body.Close()
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("incorrect status code %d", resp.StatusCode)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("unable to read response body: %v", err)
	}
	return body, nil
}

// SanitizeRequest takes a request object and returns a copy of the request with
// the "Secrets" field cleared.
func SanitizeRequest(req interface{}) interface{} {
	v := reflect.ValueOf(&req).Elem()
	e := reflect.New(v.Elem().Type()).Elem()

	e.Set(v.Elem())

	f := reflect.Indirect(e).FieldByName("Secrets")

	if f.IsValid() && f.CanSet() && f.Kind() == reflect.Map {
		f.Set(reflect.MakeMap(f.Type()))
		v.Set(e)
	}
	return req
}

const (
	TopologyZoneKey = "topology.kubernetes.io/zone"
)

// create CSI topology from an availability zone
func BuildTopology(availabilityZone string) *csi.Topology {
	if availabilityZone == "" {
		return nil
	}

	return &csi.Topology{
		Segments: map[string]string{
			TopologyZoneKey: availabilityZone,
		},
	}
}
