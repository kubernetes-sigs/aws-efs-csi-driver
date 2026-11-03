package driver

import (
	"strings"
	"testing"
)

func TestValidateConfOverridesDenylist(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		denylist map[string]bool
		wantErr  bool
	}{
		// Safe keys observed in production usage — must pass.
		{name: "efs fips_mode_enabled allowed", input: "mount:fips_mode_enabled=true", denylist: efsUtilsConfOverridesDenylist, wantErr: false},
		{name: "efs retry timeout allowed", input: "mount:retry_nfs_mount_command_timeout_sec=60", denylist: efsUtilsConfOverridesDenylist, wantErr: false},
		{name: "efs stunnel health check timeout allowed", input: "mount-watchdog:stunnel_health_check_command_timeout_sec=90", denylist: efsUtilsConfOverridesDenylist, wantErr: false},
		{name: "efs stunnel health check interval allowed", input: "mount-watchdog:stunnel_health_check_interval_min=1", denylist: efsUtilsConfOverridesDenylist, wantErr: false},
		{name: "efs unmount count allowed", input: "mount-watchdog:unmount_count_for_consistency=10", denylist: efsUtilsConfOverridesDenylist, wantErr: false},
		{name: "s3files readahead cache allowed", input: "proxy:readahead_cache_enabled=false", denylist: s3filesUtilsConfOverridesDenylist, wantErr: false},
		{name: "efs multiple safe keys", input: "mount:fips_mode_enabled=true,mount-watchdog:unmount_count_for_consistency=10", denylist: efsUtilsConfOverridesDenylist, wantErr: false},
		{name: "empty overrides pass", input: "", denylist: efsUtilsConfOverridesDenylist, wantErr: false},

		// Denied security-sensitive keys — must be rejected regardless of section, for both files.
		{name: "efs reject dns_name_suffix", input: "mount:dns_name_suffix=attacker.example.internal", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "efs reject dns_name_format", input: "mount:dns_name_format={az}.{fs_id}.efs.{region}.evil.internal", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "efs reject stunnel_cafile", input: "mount:stunnel_cafile=/tmp/evil.pem", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "efs reject stunnel_check_cert_hostname", input: "mount:stunnel_check_cert_hostname=false", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "efs reject stunnel_check_cert_validity", input: "mount:stunnel_check_cert_validity=false", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "s3files reject dns_name_suffix", input: "mount:dns_name_suffix=attacker.example.internal", denylist: s3filesUtilsConfOverridesDenylist, wantErr: true},
		{name: "s3files reject dns_name_format", input: "mount:dns_name_format={az_id}.{fs_id}.s3files.{region}.evil.internal", denylist: s3filesUtilsConfOverridesDenylist, wantErr: true},
		{name: "s3files reject stunnel_cafile", input: "mount:stunnel_cafile=/tmp/evil.pem", denylist: s3filesUtilsConfOverridesDenylist, wantErr: true},
		{name: "reject denied key in any section", input: "mount-watchdog:dns_name_suffix=evil.internal", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "reject one denied among safe", input: "mount:fips_mode_enabled=true,mount:dns_name_suffix=attacker.example.internal", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "reject full attack string", input: "mount:dns_name_suffix=attacker.example.internal,mount:stunnel_check_cert_hostname=false,mount:stunnel_check_cert_validity=false", denylist: efsUtilsConfOverridesDenylist, wantErr: true},

		// Case variants — must be rejected (efs-utils ConfigParser lowercases option names).
		{name: "reject uppercase dns_name_suffix", input: "mount:DNS_NAME_SUFFIX=evil", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "reject mixed-case dns_name_suffix", input: "mount:Dns_Name_Suffix=evil", denylist: efsUtilsConfOverridesDenylist, wantErr: true},
		{name: "reject uppercase stunnel_check_cert_hostname", input: "mount:STUNNEL_CHECK_CERT_HOSTNAME=false", denylist: s3filesUtilsConfOverridesDenylist, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			overrides, err := parseConfOverrides(tc.input)
			if err != nil {
				t.Fatalf("parseConfOverrides(%q) unexpected error: %v", tc.input, err)
			}
			err = validateConfOverridesDenylist(overrides, tc.denylist, "test-flag")
			if (err != nil) != tc.wantErr {
				t.Errorf("validateConfOverridesDenylist(%q) error = %v, wantErr %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

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

// TestApplyConfOverridesOnInstalledConf exercises applyConfOverrides against
// copies of the conf files efs-utils installs rather than a hand-written config,
// because that is the input the driver actually gives it. The region and CloudWatch cases
// matter most: efs-utils ships those keys commented out, so uncommenting them is
// the only way the driver's own flags reach efs-utils.
func TestApplyConfOverridesOnInstalledConf(t *testing.T) {
	tests := []struct {
		name string
		// fileName is the conf fixture the override is applied to.
		fileName string
		// commentedOut says the key starts out commented in that file, so
		// applying the override has to uncomment it.
		commentedOut bool
		override     ConfOverride
	}{
		{
			name:         "commented-out region in efs-utils.conf",
			fileName:     efsConfigFileName,
			commentedOut: true,
			override:     ConfOverride{Section: "mount", Key: "region", Value: "us-west-2"},
		},
		{
			name:         "commented-out region in s3files-utils.conf",
			fileName:     s3filesConfigFileName,
			commentedOut: true,
			override:     ConfOverride{Section: "mount", Key: "region", Value: "eu-west-1"},
		},
		{
			name:         "commented-out cloudwatch-log enabled in efs-utils.conf",
			fileName:     efsConfigFileName,
			commentedOut: true,
			override:     ConfOverride{Section: "cloudwatch-log", Key: "enabled", Value: "true"},
		},
		{
			name:         "commented-out proxy readbypass key in s3files-utils.conf",
			fileName:     s3filesConfigFileName,
			commentedOut: true,
			override:     ConfOverride{Section: "proxy", Key: "read_bypass_denylist_size", Value: "20000"},
		},
		{
			name:     "uncommented proxy key in s3files-utils.conf",
			fileName: s3filesConfigFileName,
			override: ConfOverride{Section: "proxy", Key: "metrics_enabled", Value: "false"},
		},
		{
			name:     "uncommented cloudwatch-log enabled in s3files-utils.conf",
			fileName: s3filesConfigFileName,
			override: ConfOverride{Section: "cloudwatch-log", Key: "enabled", Value: "false"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := readConfFixture(t, tt.fileName)
			key := confKey{tt.override.Section, tt.override.Key}
			// The assertion below would pass for the wrong reason if the key
			// already held the value, or if the case claims a commented-out key
			// that efs-utils in fact ships set, so check the starting state.
			before, set := confValue(config, key)
			if tt.commentedOut && set {
				t.Fatalf("%s is already set to %q in %s, so this case does not exercise the commented-out path", key, before, tt.fileName)
			}
			if !tt.commentedOut && !set {
				t.Fatalf("%s is not set in %s, so this case does not exercise the replace path", key, tt.fileName)
			}
			if before == tt.override.Value {
				t.Fatalf("%s already holds %q, so this case proves nothing", key, before)
			}

			got, err := applyConfOverrides(config, []ConfOverride{tt.override})
			if err != nil {
				t.Fatalf("applyConfOverrides: %v", err)
			}
			after, set := confValue(got, key)
			if !set {
				t.Fatalf("%s is still not set after the override", key)
			}
			if after != tt.override.Value {
				t.Errorf("%s = %q, want %q", key, after, tt.override.Value)
			}
		})
	}
}

// TestSetClientInfoSource covers the one section the driver adds rather than
// overrides, since efs-utils does not ship a [client-info] section.
func TestSetClientInfoSource(t *testing.T) {
	t.Run("appends the section when absent", func(t *testing.T) {
		conf := readConfFixture(t, efsConfigFileName)
		if strings.Contains(conf, "[client-info]") {
			t.Fatalf("efs-utils.conf now ships [client-info]; setClientInfoSource should override it instead of appending")
		}
		got := setClientInfoSource(conf, "k8s")
		if !strings.Contains(got, "\n[client-info]\nsource = k8s\n") {
			t.Errorf("expected an appended [client-info] section, got tail:\n%s", got[max(0, len(got)-120):])
		}
	})

	t.Run("overrides an existing source", func(t *testing.T) {
		got := setClientInfoSource("[client-info]\nsource = old\n", "k8s")
		if got != "[client-info]\nsource = k8s\n" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("adds source to an existing empty section", func(t *testing.T) {
		got := setClientInfoSource("[client-info]\n\n[other]\nkey = v\n", "k8s")
		if !strings.Contains(got, "[client-info]\nsource = k8s\n") {
			t.Errorf("got %q", got)
		}
		if !strings.Contains(got, "[other]\nkey = v\n") {
			t.Errorf("the following section was damaged: %q", got)
		}
	})
}

// TestSetConfKeyPrefersLiveLine checks a live line wins over a commented-out one
// for the same key. Uncommenting the commented line instead would leave the key
// set twice, which ConfigParser rejects with DuplicateOptionError.
func TestSetConfKeyPrefersLiveLine(t *testing.T) {
	for _, tc := range []struct{ name, conf string }{
		{"commented line first", "[mount]\n# region = us-east-1\nregion = us-east-2\n"},
		{"live line first", "[mount]\nregion = us-east-2\n# region = us-east-1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyConfOverrides(tc.conf, []ConfOverride{{Section: "mount", Key: "region", Value: "us-west-2"}})
			if err != nil {
				t.Fatalf("applyConfOverrides: %v", err)
			}
			if n := strings.Count(got, "\nregion = "); n != 1 {
				t.Errorf("expected exactly one live region line, got %d in:\n%s", n, got)
			}
			if !strings.Contains(got, "region = us-west-2") || !strings.Contains(got, "# region = us-east-1") {
				t.Errorf("expected the live line rewritten and the comment kept, got:\n%s", got)
			}
		})
	}
}

// TestSetDriverConf covers the lenient path the driver's own keys take.
func TestSetDriverConf(t *testing.T) {
	conf := "[DEFAULT]\nlogging_level = INFO\n\n[mount]\n#region = us-east-1\ndns_name_suffix = amazonaws.com\n\n[proxy]\nmetrics_enabled = true\n"
	got, added := setDriverConf(conf, []ConfOverride{
		{Section: "mount", Key: "region", Value: "us-west-2"},        // commented out: uncommented, not added
		{Section: "proxy", Key: "metrics_enabled", Value: "false"},   // live: replaced, not added
		{Section: "mount", Key: "fips_mode_enabled", Value: "false"}, // missing key: added to [mount]
		{Section: "cloudwatch-log", Key: "enabled", Value: "true"},   // missing section: appended
	})
	want := "[DEFAULT]\nlogging_level = INFO\n\n[mount]\nregion = us-west-2\ndns_name_suffix = amazonaws.com\nfips_mode_enabled = false\n\n[proxy]\nmetrics_enabled = false\n\n[cloudwatch-log]\nenabled = true\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if len(added) != 2 || added[0].Key != "fips_mode_enabled" || added[1].Section != "cloudwatch-log" {
		t.Errorf("added = %+v, want fips_mode_enabled and [cloudwatch-log] enabled", added)
	}
}
