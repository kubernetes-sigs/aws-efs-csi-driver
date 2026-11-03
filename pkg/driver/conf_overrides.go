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
// The driver's own flag-derived keys use setDriverConf instead, which never fails.
func applyConfOverrides(config string, overrides []ConfOverride) (string, error) {
	if len(overrides) == 0 {
		return config, nil
	}
	lines := strings.Split(config, "\n")
	for _, o := range overrides {
		sectionIdx, nextSectionIdx := findConfSection(lines, o.Section)
		if sectionIdx < 0 {
			return "", fmt.Errorf("section [%s] not found in config", o.Section)
		}
		if !setConfKeyInSection(lines, sectionIdx, nextSectionIdx, o.Key, o.Value) {
			return "", fmt.Errorf("key %q not found in section [%s]", o.Key, o.Section)
		}
	}
	return strings.Join(lines, "\n"), nil
}

// findConfSection returns the index of the given section's header line and the
// index one past its last line. The header index is -1 if the section is absent.
func findConfSection(lines []string, section string) (int, int) {
	sectionHeader := "[" + section + "]"
	sectionIdx := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == sectionHeader {
			sectionIdx = i
			break
		}
	}
	if sectionIdx < 0 {
		return -1, len(lines)
	}

	nextSectionIdx := len(lines)
	for i := sectionIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			nextSectionIdx = i
			break
		}
	}
	return sectionIdx, nextSectionIdx
}

// setConfKeyInSection rewrites the line holding key within the half-open range
// (sectionIdx, nextSectionIdx), and reports whether such a line was found. A live
// "key = value" line is preferred; only when there is none does a commented-out
// line (e.g. "#region = us-east-1", which is how efs-utils ships several of the
// keys the driver sets) match, and it is uncommented. Rewriting a commented line
// while a live one exists would leave the key set twice, which Python's
// ConfigParser rejects with DuplicateOptionError.
func setConfKeyInSection(lines []string, sectionIdx, nextSectionIdx int, key, value string) bool {
	for _, commented := range []bool{false, true} {
		for i := sectionIdx + 1; i < nextSectionIdx; i++ {
			trimmed := strings.TrimSpace(lines[i])
			if strings.HasPrefix(trimmed, "#") != commented {
				continue
			}
			// A commented-out key is written as "# key = value" or "#key = value".
			trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
			eqPos := strings.Index(trimmed, "=")
			if eqPos < 0 {
				continue
			}
			if strings.TrimSpace(trimmed[:eqPos]) == key {
				lines[i] = key + " = " + value
				return true
			}
		}
	}
	return false
}

// setDriverConf sets each key in config and, unlike applyConfOverrides, never
// fails. A key the config does not have is added at the end of its section, and
// a missing section is appended with the key in it, so the driver's value is
// always written, as it was when the driver rendered the whole file from its own
// template. It returns the keys it had to add so the caller can warn about them:
// one usually means the installed efs-utils renamed or dropped the key.
func setDriverConf(config string, conf []ConfOverride) (string, []ConfOverride) {
	lines := strings.Split(config, "\n")
	var added []ConfOverride
	for _, c := range conf {
		sectionIdx, nextSectionIdx := findConfSection(lines, c.Section)
		if sectionIdx < 0 {
			lines = strings.Split(strings.TrimRight(strings.Join(lines, "\n"), "\n")+
				"\n\n["+c.Section+"]\n"+c.Key+" = "+c.Value+"\n", "\n")
			added = append(added, c)
			continue
		}
		if setConfKeyInSection(lines, sectionIdx, nextSectionIdx, c.Key, c.Value) {
			continue
		}
		// Insert after the section's last non-blank line, so the key stays
		// clear of the blank lines that separate it from the next section.
		insertAt := sectionIdx + 1
		for i := nextSectionIdx - 1; i > sectionIdx; i-- {
			if strings.TrimSpace(lines[i]) != "" {
				insertAt = i + 1
				break
			}
		}
		lines = append(lines[:insertAt], append([]string{c.Key + " = " + c.Value}, lines[insertAt:]...)...)
		added = append(added, c)
	}
	return strings.Join(lines, "\n"), added
}

// clientInfoSection is the section efs-utils reads the client source from. The
// conf files efs-utils installs do not have it, so the driver adds it instead
// of overriding it; INI section order does not matter to the readers (efs-utils
// parses these files with Python's ConfigParser).
const clientInfoSection = "client-info"

// setClientInfoSource sets [client-info] source, appending the section when the
// config does not already have one.
func setClientInfoSource(config, source string) string {
	content, _ := setDriverConf(config, []ConfOverride{{Section: clientInfoSection, Key: "source", Value: source}})
	return content
}
