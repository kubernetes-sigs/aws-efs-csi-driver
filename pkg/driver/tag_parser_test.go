package driver

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseTags(t *testing.T) {
	testCases := []struct {
		name     string
		testFunc func(t *testing.T)
	}{{
		name: "Empty String gives Empty Map",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "", map[string]string{})
		},
	}, {
		name: "One Entry - No Spaces or Odd Characters",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "foo:bar", map[string]string{"foo": "bar"})
		},
	}, {
		name: "Multiple Entries - No Spaces or Odd Characters",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "foo:bar bash:baz flip:flop", map[string]string{"foo": "bar", "bash": "baz", "flip": "flop"})
		},
	}, {
		name: "Containing +, -, =, ., _, /",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "foo+bar-=.:bash_baz/", map[string]string{"foo+bar-=.": "bash_baz/"})
		},
	}, {
		name: "Key contains single :",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "'foo:bar':bash", map[string]string{"foo:bar": "bash"})
		},
	}, {
		name: "Key contains multiple :'s",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "'foo:bar:i:am:fish':bash", map[string]string{"foo:bar:i:am:fish": "bash"})
		},
	}, {
		name: "Value contains multiple :'s",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "foo:bash:bar:i:am:fish", map[string]string{"foo": "bash:bar:i:am:fish"})
		},
	}, {
		name: "Value behaves with quotes",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "foo:'bashbariamfish'", map[string]string{"foo": "bashbariamfish"})
		},
	}, {
		name: "Multiple Entries with and without quotes and colons",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "foo:'bashbariamfish' 'a:b:c':fowl bash:baz",
				map[string]string{"foo": "bashbariamfish", "a:b:c": "fowl", "bash": "baz"})
		},
	}, {
		name: "Key with spaces",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "'foo and another thing':bar", map[string]string{"foo and another thing": "bar"})
		},
	}, {
		name: "Value with spaces",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "foo:'and another thing'", map[string]string{"foo": "and another thing"})
		},
	}, {
		name: "Case Sensitivity Is Retained",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "Foo:'and anOther ThiNg'", map[string]string{"Foo": "and anOther ThiNg"})
		},
	}, {
		name: "Unmatched Quotes are rejected",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "'Foo:bar", map[string]string{})
		},
	}, {
		name: "Check the empty key is rejected",
		testFunc: func(t *testing.T) {
			compareParseTags(t, ":bar", map[string]string{})
		},
	}, {
		name: "Check the empty key is rejected when quoted",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "'':bar", map[string]string{})
		},
	}, {
		name: "Check the empty value is registered properly",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "Foo: Bar:Bash", map[string]string{"Foo": "", "Bar": "Bash"})
		},
	}, {
		name: "Check the empty value is registered properly when quoted",
		testFunc: func(t *testing.T) {
			compareParseTags(t, "Foo:'' Bar:Bash", map[string]string{"Foo": "", "Bar": "Bash"})
		},
	}}

	for _, tc := range testCases {
		t.Run(tc.name, tc.testFunc)
	}
}

func compareParseTags(testCase *testing.T, input string, wanted map[string]string) {
	got := parseTagsFromStr(input)
	if !reflect.DeepEqual(got, wanted) {
		wanted, _ := json.Marshal(wanted)
		got, _ := json.Marshal(got)
		testCase.Fatalf("Wanted %s, but got %s", wanted, got)
	}
}
