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
	"fmt"
	"strings"
)

const (
	commaDelim     = ","
	equalDelim     = "="
	expectedTokens = 2
)

func parseCommaSeparatedKVPairs(combined string) (map[string]string, error) {
	pairs := map[string]string{}
	for _, combinedTokens := range strings.Split(combined, commaDelim) {
		tokens := strings.Split(combinedTokens, equalDelim)
		if len(tokens) != expectedTokens {
			return nil, fmt.Errorf("failed to parse key=value pair %v", combinedTokens)
		}
		pairs[tokens[0]] = tokens[1]
	}
	return pairs, nil
}

func parseCommaSeparatedStrings(combined string) []string {
	if combined == "" {
		return []string{}
	}
	return strings.Split(combined, commaDelim)
}
