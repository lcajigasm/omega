package deepclean

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// scanDocker lists Docker's objects one by one: the images no container
// uses, the stopped containers and the volumes no container mounts. `docker
// image prune -a` would take every unused image together, also one built
// on this machine that no registry can give back; here each says which it
// is.
func scanDocker(ctx context.Context, now time.Time) ([]Finding, error) {
	if _, err := command(ctx, "", "docker", "info", "--format", "{{.ServerVersion}}"); err != nil {
		return nil, fmt.Errorf("docker: the daemon does not answer, so its images, containers and volumes were not looked at")
	}
	var found []Finding
	used := map[string]bool{}

	// Containers first: the images they use are kept.
	out, err := command(ctx, "", "docker", "ps", "-a", "-q", "--no-trunc")
	if err != nil {
		return nil, err
	}
	if ids := strings.Fields(out); len(ids) > 0 {
		info, err := command(ctx, "", "docker", append([]string{"container", "inspect", "--size", "--format",
			"{{.Id}}\t{{.Image}}\t{{.State.Status}}\t{{.Name}}\t{{.State.FinishedAt}}\t{{.SizeRw}}\t{{.Config.Image}}"}, ids...)...)
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(strings.TrimSpace(info), "\n") {
			p := strings.Split(line, "\t")
			if len(p) < 7 {
				continue
			}
			used[p[1]] = true
			status := p[2]
			if status == "running" || status == "paused" || status == "restarting" {
				continue
			}
			name := strings.TrimPrefix(p[3], "/")
			rw, _ := strconv.ParseInt(p[5], 10, 64)
			f := Finding{
				ID: id(KindDockerContainer, p[0]), Category: CatDocker, Kind: KindDockerContainer,
				Title: "container " + name + " (" + p[6] + ", " + status + ")", Ref: p[0], Size: rw,
				Action: "docker container rm " + name,
			}
			if t, err := time.Parse(time.RFC3339Nano, p[4]); err == nil && t.Year() > 1 {
				f.LastUsed = t
				f.Reasons = append(f.Reasons, "stopped "+ago(now.Sub(t)))
			}
			if rw > 0 {
				f.Verdict = Review
				f.Reasons = append(f.Reasons, "what it wrote inside itself is lost with it (its volumes stay)")
			} else {
				f.Verdict = Safe
				f.Reasons = append(f.Reasons, "it changed nothing inside itself: docker run makes it again")
			}
			found = append(found, f)
		}
	}

	// Images no container uses.
	out, err = command(ctx, "", "docker", "image", "ls", "-q", "--no-trunc")
	if err != nil {
		return found, err
	}
	ids := uniq(strings.Fields(out))
	if len(ids) > 0 {
		info, err := command(ctx, "", "docker", append([]string{"image", "inspect", "--format",
			"{{.Id}}\t{{.Size}}\t{{len .RepoDigests}}\t{{join .RepoTags \",\"}}\t{{.Created}}"}, ids...)...)
		if err != nil {
			return found, err
		}
		for _, line := range strings.Split(strings.TrimSpace(info), "\n") {
			p := strings.Split(line, "\t")
			if len(p) < 5 || used[p[0]] {
				continue
			}
			size, _ := strconv.ParseInt(p[1], 10, 64)
			digests, _ := strconv.Atoi(p[2])
			tags := p[3]
			name := tags
			if name == "" {
				name = "<none> " + short(strings.TrimPrefix(p[0], "sha256:"))
			}
			f := Finding{
				ID: id(KindDockerImage, p[0]), Category: CatDocker, Kind: KindDockerImage,
				Title: "image " + name, Ref: p[0], Size: size,
				Action: "docker image rm " + short(strings.TrimPrefix(p[0], "sha256:")),
			}
			if t, err := time.Parse(time.RFC3339Nano, p[4]); err == nil {
				f.LastUsed = t
				f.Reasons = append(f.Reasons, "created "+ago(now.Sub(t))+"; no container uses it")
			}
			switch {
			case tags == "":
				f.Verdict = Safe
				f.Reasons = append(f.Reasons, "dangling: no name points to it any more")
			case digests > 0:
				f.Verdict = Safe
				f.Reasons = append(f.Reasons, "it came from a registry: docker pull "+strings.Split(tags, ",")[0]+" brings it back")
			default:
				f.Verdict = Review
				f.Reasons = append(f.Reasons, "built on this machine: no registry has it, getting it back means building it again")
			}
			found = append(found, f)
		}
	}

	// Volumes no container mounts: they can hold a database, so never safe.
	out, err = command(ctx, "", "docker", "volume", "ls", "-q", "--filter", "dangling=true")
	if err != nil {
		return found, err
	}
	if vols := strings.Fields(out); len(vols) > 0 {
		sizes := volumeSizes(ctx)
		for _, v := range vols {
			found = append(found, Finding{
				ID: id(KindDockerVolume, v), Category: CatDocker, Kind: KindDockerVolume,
				Title: "volume " + v, Ref: v, Size: sizes[v], Verdict: Review,
				Reasons: []string{"no container mounts it", "a volume can hold a database or the only copy of some data: look before removing"},
				Action:  "docker volume rm " + v,
			})
		}
	}
	return found, nil
}

// volumeSizes reads the size of each volume from docker system df -v.
func volumeSizes(ctx context.Context) map[string]int64 {
	out := map[string]int64{}
	raw, err := command(ctx, "", "docker", "system", "df", "-v", "--format", "{{json .}}")
	if err != nil {
		return out
	}
	var df struct {
		Volumes []struct {
			Name string
			Size string
		}
	}
	if json.Unmarshal([]byte(strings.TrimSpace(raw)), &df) != nil {
		return out
	}
	for _, v := range df.Volumes {
		out[v.Name] = ParseSize(v.Size)
	}
	return out
}

// ParseSize reads the sizes Docker prints: 119.5MB, 2.1GB, 0B, 512kB.
func ParseSize(s string) int64 {
	s = strings.TrimSpace(s)
	units := []struct {
		suffix string
		mult   float64
	}{{"TB", 1e12}, {"GB", 1e9}, {"MB", 1e6}, {"kB", 1e3}, {"KB", 1e3}, {"B", 1}}
	for _, u := range units {
		if n, ok := strings.CutSuffix(s, u.suffix); ok {
			v, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
			if err != nil {
				return 0
			}
			return int64(v * u.mult)
		}
	}
	return 0
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
