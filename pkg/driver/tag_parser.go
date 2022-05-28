package driver

import (
	"fmt"

	"k8s.io/klog"
)

func parseTagsFromStr(tagStr string) map[string]string {
	defer func() {
		if r := recover(); r != nil {
			klog.Errorf("Failed to parse input tag string: %v", tagStr)
		}
	}()

	m := make(map[string]string)
	if tagStr == "" {
		klog.Infof("Did not find any input tags.")
		return m
	}
	tagsSplit := extractPairsFromRawString(tagStr)

	for _, pair := range tagsSplit {
		k, v, err := extractKeyAndValueFromRawPair(pair)
		if err != nil {
			klog.Warningf("Could not extract key and value from %s - %v", pair, err)
			continue
		}
		m[k] = v
	}
	return m
}

func extractPairsFromRawString(raw string) []string {
	result := make([]string, 0)
	accumulator := ""
	chars := []rune(raw)
	literal := false
	for i := 0; i < len(chars); i++ {
		switch chars[i] {
		case '\'':
			literal = !literal
		case ' ':
			if !literal {
				result = append(result, accumulator)
				accumulator = ""
				continue
			}
		}
		accumulator += string(chars[i])
	}
	if accumulator != "" {
		result = append(result, accumulator)
	}
	return result
}

func extractKeyAndValueFromRawPair(pair string) (string, string, error) {
	chars := []rune(pair)
	key := ""
	literal := false
	finalKeyIndex := -1
	for i := 0; i < len(chars); i++ {
		switch chars[i] {
		case '\'':
			literal = !literal
			continue
		case ':':
			if !literal {
				finalKeyIndex = i
				break
			}
		}
		if finalKeyIndex >= 0 {
			break
		}
		key += string(chars[i])
	}

	if literal {
		return "", "", fmt.Errorf("unmatched quotes in tag string")
	} else if key == "" {
		return "", "", fmt.Errorf("cannot have empty key")
	}

	return key, stripOuterQuotes(string(chars[finalKeyIndex+1:])), nil
}

func stripOuterQuotes(value string) string {
	if len(value) > 0 && value[0] == '\'' {
		value = value[1:]
	}
	if len(value) > 0 && value[len(value)-1] == '\'' {
		value = value[:len(value)-1]
	}
	return value
}
