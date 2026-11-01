// Package formula rewrites Formula/omega.rb from the checksums of a release.
package formula

import (
	"fmt"
	"regexp"
	"strings"
)

// Archives are the release files whose checksums the formula records.
func Archives(version string) []string {
	var names []string
	for _, pair := range []struct{ os, arch, ext string }{
		{"darwin", "amd64", "tar.gz"},
		{"darwin", "arm64", "tar.gz"},
		{"linux", "amd64", "tar.gz"},
		{"linux", "arm64", "tar.gz"},
		{"windows", "amd64", "zip"},
		{"windows", "arm64", "zip"},
	} {
		names = append(names, archive(version, pair.os, pair.arch, pair.ext))
	}
	return names
}

func archive(version, osName, arch, ext string) string {
	return fmt.Sprintf("omega_%s_%s_%s.%s", version, osName, arch, ext)
}

// ParseSums reads a sha256sum file. The result maps a file name to its hex digest.
func ParseSums(data string) (map[string]string, error) {
	out := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\uFEFF"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 || len(fields[0]) != 64 {
			return nil, fmt.Errorf("bad checksum line %q", line)
		}
		out[strings.TrimPrefix(fields[1], "*")] = fields[0]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no checksums")
	}
	return out, nil
}

var (
	versionLine = regexp.MustCompile(`^  version "[0-9]+\.[0-9]+\.[0-9]+"`)
	assetURL    = regexp.MustCompile(`https://github.com/lcajigasm/omega/releases/download/v[0-9]+\.[0-9]+\.[0-9]+/omega_[0-9]+\.[0-9]+\.[0-9]+_(darwin|linux|windows)_(amd64|arm64)\.(tar\.gz|zip)`)
	winComment  = regexp.MustCompile(`^(\s*# )omega_[0-9]+\.[0-9]+\.[0-9]+_windows_(amd64|arm64)\.zip sha256 [0-9a-f]{64}`)
	semver      = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

// Update sets the formula's version and points every archive at version,
// using sums (file name to sha256) for the digests.
func Update(src, version string, sums map[string]string) (string, error) {
	if !semver.MatchString(version) {
		return "", fmt.Errorf("version %q is not x.y.z", version)
	}
	for _, name := range Archives(version) {
		if len(sums[name]) != 64 {
			return "", fmt.Errorf("missing sha256 for %s", name)
		}
	}
	lines := strings.Split(src, "\n")
	sawVersion := false
	urls := 0
	wins := 0
	for i := 0; i < len(lines); i++ {
		if versionLine.MatchString(lines[i]) {
			lines[i] = `  version "` + version + `"`
			sawVersion = true
			continue
		}
		if m := winComment.FindStringSubmatch(lines[i]); m != nil {
			ext := "zip"
			name := archive(version, "windows", m[2], ext)
			lines[i] = m[1] + name + " sha256 " + sums[name]
			wins++
			continue
		}
		loc := assetURL.FindStringSubmatchIndex(lines[i])
		if loc == nil {
			continue
		}
		sub := assetURL.FindStringSubmatch(lines[i])
		osName, arch := sub[1], sub[2]
		ext := "tar.gz"
		if strings.HasSuffix(sub[0], ".zip") {
			ext = "zip"
		}
		name := archive(version, osName, arch, ext)
		url := "https://github.com/lcajigasm/omega/releases/download/v" + version + "/" + name
		lines[i] = lines[i][:loc[0]] + url + lines[i][loc[1]:]
		if i+1 >= len(lines) || !strings.Contains(lines[i+1], "sha256") {
			return "", fmt.Errorf("url for %s has no sha256 line", name)
		}
		indent := lines[i+1][:len(lines[i+1])-len(strings.TrimLeft(lines[i+1], " \t"))]
		lines[i+1] = indent + `sha256 "` + sums[name] + `"`
		urls++
	}
	if !sawVersion {
		return "", fmt.Errorf("formula has no version line")
	}
	if urls != 4 {
		return "", fmt.Errorf("formula has %d archive urls, want 4", urls)
	}
	if wins != 2 {
		return "", fmt.Errorf("formula has %d windows checksum comments, want 2", wins)
	}
	return strings.Join(lines, "\n"), nil
}
