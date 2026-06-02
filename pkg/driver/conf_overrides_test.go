package driver

import (
	"strings"
	"testing"
)

func TestParseConfOverrides(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantCount int
		wantErr   bool
	}{
		{name: "empty string", input: "", wantCount: 0, wantErr: false},
		{name: "whitespace only", input: "   ", wantCount: 0, wantErr: false},
		{name: "single override", input: "mount-watchdog:stunnel_health_check_interval_min=1", wantCount: 1, wantErr: false},
		{name: "multiple overrides", input: "mount-watchdog:stunnel_health_check_interval_min=1,mount-watchdog:tls_cert_renewal_interval_min=30", wantCount: 2, wantErr: false},
		{name: "value with equals", input: "section:key=val=ue", wantCount: 1, wantErr: false},
		{name: "missing colon", input: "sectionkey=value", wantCount: 0, wantErr: true},
		{name: "missing equals", input: "section:keyvalue", wantCount: 0, wantErr: true},
		{name: "empty section", input: ":key=value", wantCount: 0, wantErr: true},
		{name: "empty key", input: "section:=value", wantCount: 0, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseConfOverrides(tc.input)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseConfOverrides(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
				return
			}
			if !tc.wantErr && len(got) != tc.wantCount {
				t.Errorf("parseConfOverrides(%q) returned %d overrides, want %d", tc.input, len(got), tc.wantCount)
			}
		})
	}

	// Verify parsed values
	overrides, _ := parseConfOverrides("mount-watchdog:stunnel_health_check_interval_min=1")
	if overrides[0].Section != "mount-watchdog" || overrides[0].Key != "stunnel_health_check_interval_min" || overrides[0].Value != "1" {
		t.Errorf("Unexpected parsed override: %+v", overrides[0])
	}

	// Value with equals
	overrides, _ = parseConfOverrides("section:key=val=ue")
	if overrides[0].Value != "val=ue" {
		t.Errorf("Expected value 'val=ue', got %q", overrides[0].Value)
	}
}

func TestApplyConfOverrides(t *testing.T) {
	tests := []struct {
		name      string
		config    string
		overrides []ConfOverride
		wantErr   bool
		errMsg    string
		check     func(string) bool
		desc      string
	}{
		{
			name:      "no overrides",
			config:    "[section]\nkey = old\n",
			overrides: nil,
			check:     func(s string) bool { return s == "[section]\nkey = old\n" },
			desc:      "config unchanged",
		},
		{
			name:      "replace existing key",
			config:    "[mount-watchdog]\nstunnel_health_check_interval_min = 5\n",
			overrides: []ConfOverride{{Section: "mount-watchdog", Key: "stunnel_health_check_interval_min", Value: "1"}},
			check:     func(s string) bool { return strings.Contains(s, "stunnel_health_check_interval_min = 1") },
			desc:      "key replaced",
		},
		{
			name:   "multiple overrides same section",
			config: "[mount-watchdog]\nstunnel_health_check_interval_min = 5\ntls_cert_renewal_interval_min = 60\n",
			overrides: []ConfOverride{
				{Section: "mount-watchdog", Key: "stunnel_health_check_interval_min", Value: "1"},
				{Section: "mount-watchdog", Key: "tls_cert_renewal_interval_min", Value: "30"},
			},
			check: func(s string) bool {
				return strings.Contains(s, "stunnel_health_check_interval_min = 1") &&
					strings.Contains(s, "tls_cert_renewal_interval_min = 30")
			},
			desc: "both keys replaced",
		},
		{
			name:   "overrides across different sections",
			config: "[mount-watchdog]\nenabled = true\n\n[proxy]\nmetrics_enabled = true\n",
			overrides: []ConfOverride{
				{Section: "mount-watchdog", Key: "enabled", Value: "false"},
				{Section: "proxy", Key: "metrics_enabled", Value: "false"},
			},
			check: func(s string) bool {
				return strings.Contains(s, "[mount-watchdog]\nenabled = false") &&
					strings.Contains(s, "metrics_enabled = false")
			},
			desc: "both sections updated",
		},
		{
			name:      "non-existent section returns error",
			config:    "[mount-watchdog]\nenabled = true\n",
			overrides: []ConfOverride{{Section: "nonexistent", Key: "key", Value: "value"}},
			wantErr:   true,
			errMsg:    "section [nonexistent] not found",
		},
		{
			name:      "non-existent key returns error",
			config:    "[mount-watchdog]\nenabled = true\n",
			overrides: []ConfOverride{{Section: "mount-watchdog", Key: "no_such_key", Value: "value"}},
			wantErr:   true,
			errMsg:    "key \"no_such_key\" not found in section [mount-watchdog]",
		},
		{
			name:      "commented-out key gets uncommented and replaced",
			config:    "[proxy]\nmetrics_enabled = true\n# read_bypass_denylist_size = 10000\n",
			overrides: []ConfOverride{{Section: "proxy", Key: "read_bypass_denylist_size", Value: "20000"}},
			check:     func(s string) bool { return strings.Contains(s, "read_bypass_denylist_size = 20000") },
			desc:      "commented key uncommented and replaced",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := applyConfOverrides(tc.config, tc.overrides)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errMsg)
				}
				if !strings.Contains(err.Error(), tc.errMsg) {
					t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.check(result) {
				t.Errorf("applyConfOverrides failed (%s), got:\n%s", tc.desc, result)
			}
		})
	}
}
