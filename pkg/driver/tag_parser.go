package driver

import (
	"fmt"
	"strings"

	"k8s.io/klog"
)

const tagEntrySeparator = ' '
const tagKeyValueSeparator = ':'
const tagKeyValueQuote = '\''

/*
parseTagsFromStr allows you to turn a space-separated, colon-delimited string, including quotes, into a set of tags.
This is based on the AWS specification for tags which can be seen here
https://docs.aws.amazon.com/general/latest/gr/aws_tagging.html and
https://docs.aws.amazon.com/efs/latest/ug/manage-fs-tags.html

parseTagsFromStr supports single quotes, so you can include spaces and colons in the keys and values themselves,
for example:
* 'a:b':foo maps to a tag with key "a:b" and value "foo"
* a:'a and b and c' maps to a tag with key "a" and value "a and b and c"
*/
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
	return splitStringWithQuotes(raw, tagKeyValueQuote, tagEntrySeparator)
}

func extractKeyAndValueFromRawPair(pair string) (string, string, error) {
	result := splitStringWithQuotes(pair, tagKeyValueQuote, tagKeyValueSeparator)

	if len(result) == 0 {
		return "", "", fmt.Errorf("could not extract key, value from %s", pair)
	}

	// If we get an empty key, or it's the case that the first element is actually a suffix of the string (i.e. it
	// occurs at the end) then this is invalid because keys cannot be empty.
	if isKeyEmpty(result[0]) || strings.HasSuffix(pair, result[0]) {
		return "", "", fmt.Errorf("cannot have empty key")
	}

	// If we have fewer than 2 elements then either we have an unmatched quote (which we can check by looking
	// for a colon inside the element), or a key with an empty value (which is allowed)
	if len(result) < 2 {
		if strings.Contains(result[0], ":") {
			return "", "", fmt.Errorf("unmatched quotes in tag string")
		} else {
			result = append(result, "")
		}
	}

	return stripOuterQuotes(result[0]), stripOuterQuotes(result[1]), nil
}

func splitStringWithQuotes(raw string, quote rune, separator rune) []string {
	quoted := false
	result := strings.FieldsFunc(raw, func(r rune) bool {
		if r == quote {
			quoted = !quoted
		}
		return !quoted && r == separator
	})
	return result
}

func isKeyEmpty(key string) bool {
	return key == "" || key == string(tagKeyValueQuote) || key == fmt.Sprintf("%c%c", tagKeyValueQuote, tagKeyValueQuote)
}

func stripOuterQuotes(value string) string {
	if len(value) > 0 && value[0] == tagKeyValueQuote {
		value = value[1:]
	}
	if len(value) > 0 && value[len(value)-1] == tagKeyValueQuote {
		value = value[:len(value)-1]
	}
	return value
}
