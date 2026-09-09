package driver

import (
	"fmt"
	"strings"
)

// ConfOverride represents a single section:key=value config override.
type ConfOverride struct {
	Section string
	Key     string
	Value   string
}

// efsUtilsConfOverridesDenylist is the set of security-sensitive keys that must
// never be settable via --efs-utils-conf-overrides. These keys control the
// mount transport DNS target and TLS certificate verification; allowing them to
// be set through the conf-override interface lets a caller redirect the
// transport or disable certificate checks. This is a defense-in-depth layer
// that backs the EKS add-on schema denylist: it applies on every install path
// (EKS add-on, self-managed Helm, and manual), not just the add-on API. Keys
// are matched by name regardless of section.
var efsUtilsConfOverridesDenylist = map[string]bool{
	"dns_name_suffix":             true,
	"dns_name_format":             true,
	"stunnel_cafile":              true,
	"stunnel_check_cert_hostname": true,
	"stunnel_check_cert_validity": true,
}

// s3filesUtilsConfOverridesDenylist is the set of security-sensitive keys that
// must never be settable via --s3files-utils-conf-overrides. See
// efsUtilsConfOverridesDenylist for the rationale. It is defined separately so
// the two conf files' denied sets can evolve independently.
var s3filesUtilsConfOverridesDenylist = map[string]bool{
	"dns_name_suffix":             true,
	"dns_name_format":             true,
	"stunnel_cafile":              true,
	"stunnel_check_cert_hostname": true,
	"stunnel_check_cert_validity": true,
}

// validateConfOverridesDenylist returns an error if any override targets a key
// present in the supplied denylist. Comparison is case-insensitive: efs-utils
// reads the written conf with Python ConfigParser, whose default optionxform
// lowercases option names, so a case variant (e.g. DNS_NAME_SUFFIX) would
// otherwise be applied as the denied key. flagName is used only for error
// messages (e.g. "efs-utils-conf-overrides").
func validateConfOverridesDenylist(overrides []ConfOverride, denylist map[string]bool, flagName string) error {
	for _, o := range overrides {
		if denylist[strings.ToLower(o.Key)] {
			return fmt.Errorf("override key %q is not permitted for %s: this key controls transport security and cannot be set via conf overrides", o.Key, flagName)
		}
	}
	return nil
}

// parseConfOverrides parses a comma-separated "section:key=value" string into ConfOverride structs.
func parseConfOverrides(raw string) ([]ConfOverride, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var overrides []ConfOverride
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.SplitN(entry, ":", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid override entry %q: missing colon separator", entry)
		}
		rest := parts[1]
		kv := strings.SplitN(rest, "=", 2)
		if len(kv) != 2 || kv[0] == "" {
			return nil, fmt.Errorf("invalid override entry %q: missing equals separator", entry)
		}
		overrides = append(overrides, ConfOverride{
			Section: strings.TrimSpace(parts[0]),
			Key:     strings.TrimSpace(kv[0]),
			Value:   strings.TrimSpace(kv[1]),
		})
	}
	return overrides, nil
}

// applyConfOverrides applies parsed overrides to an INI-style config string.
// Returns an error if a section or key does not exist in the config (including commented-out keys).
func applyConfOverrides(config string, overrides []ConfOverride) (string, error) {
	if len(overrides) == 0 {
		return config, nil
	}
	lines := strings.Split(config, "\n")
	for _, o := range overrides {
		sectionHeader := "[" + o.Section + "]"
		sectionIdx := -1
		for i, line := range lines {
			if strings.TrimSpace(line) == sectionHeader {
				sectionIdx = i
				break
			}
		}
		if sectionIdx < 0 {
			return "", fmt.Errorf("section [%s] not found in config", o.Section)
		}

		nextSectionIdx := len(lines)
		for i := sectionIdx + 1; i < len(lines); i++ {
			trimmed := strings.TrimSpace(lines[i])
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				nextSectionIdx = i
				break
			}
		}

		replaced := false
		for i := sectionIdx + 1; i < nextSectionIdx; i++ {
			trimmed := strings.TrimSpace(lines[i])
			// Check uncommented key=value lines
			if !strings.HasPrefix(trimmed, "#") {
				eqPos := strings.Index(trimmed, "=")
				if eqPos < 0 {
					continue
				}
				existingKey := strings.TrimSpace(trimmed[:eqPos])
				if existingKey == o.Key {
					lines[i] = o.Key + " = " + o.Value
					replaced = true
					break
				}
			} else {
				// Check commented-out key=value lines (e.g., "# key = value")
				uncommented := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
				eqPos := strings.Index(uncommented, "=")
				if eqPos < 0 {
					continue
				}
				existingKey := strings.TrimSpace(uncommented[:eqPos])
				if existingKey == o.Key {
					lines[i] = o.Key + " = " + o.Value
					replaced = true
					break
				}
			}
		}

		if !replaced {
			return "", fmt.Errorf("key %q not found in section [%s]", o.Key, o.Section)
		}
	}
	return strings.Join(lines, "\n"), nil
}
