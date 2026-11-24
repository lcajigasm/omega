package safety

import (
	"path"
	"regexp"
	"strings"
)

// Docker, Podman and nerdctl: the cleanups and removals that are asked
// about every time.

// dockerDestructive are the cleanups that delete what cannot be rebuilt.
// The build cache and dangling images (image prune without -a) are not
// here: they come back on the next build, and topgrade's containers step
// prunes them without asking. containerDeletes still catches image prune -a
// and builder prune -a, which throw away every image and every layer.
var dockerDestructive = regexp.MustCompile(`\b(docker|podman|nerdctl)(\.exe)?\s+((system|container|volume|network|pod)\s+prune|rmi|rm|volume\s+(rm|remove)|image\s+(rm|remove)|container\s+(rm|remove)|buildx\s+rm|stack\s+(rm|remove))\b`)

// containerGlobals are the options docker, podman and nerdctl take before
// the subcommand with a value after them.
const containerGlobals = "--context -c -H --host --config -l --log-level --tlscacert --tlscert --tlskey " +
	"--connection --url --identity --root --runroot --storage-driver --storage-opt --namespace -n --address -a " +
	"--cgroup-manager --data-root --snapshotter --hosts-dir --cni-path --tmpdir --runtime"

// AlwaysConfirm reports commands that must be confirmed by the user every
// time, even under --yes, and skipped when nobody is there: Docker cleanups
// can delete images, volumes and databases that are expensive or impossible
// to recreate, and alwaysAsks lists the rest (deletions in the home folder,
// global configuration, sudo, code the command does not show).
func AlwaysConfirm(command string) bool {
	if dockerDestructive.MatchString(command) || alwaysAsks(command) {
		return true
	}
	for _, seg := range shellSegments(command) {
		run := afterWrappers(seg)
		if len(run) == 0 {
			continue
		}
		switch strings.TrimSuffix(strings.ToLower(path.Base(run[0])), ".exe") {
		case "docker", "podman", "nerdctl":
			if containerDeletes(run[1:]) {
				return true
			}
		case "docker-compose", "podman-compose":
			if composeDeletes(run[1:]) {
				return true
			}
		}
	}
	return false
}

// containerDeletes reads the arguments of docker (podman, nerdctl) past
// the global options, which may sit between the program and the subcommand:
// docker --context default system prune.
func containerDeletes(args []string) bool {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if has(containerGlobals, args[i]) {
			i++
		}
		i++
	}
	if i >= len(args) {
		return false
	}
	sub, next := args[i], ""
	if i+1 < len(args) {
		next = args[i+1]
	}
	switch sub {
	case "rm", "rmi":
		return true
	case "image", "builder", "buildx":
		// Dangling images and the build cache are rebuilt on demand; with
		// -a every image (or every cached layer) goes.
		if next == "prune" {
			return pruneAll(args[i+2:])
		}
		return next == "rm" || next == "remove"
	case "system", "container", "volume", "network", "pod", "stack", "machine":
		return next == "prune" || next == "rm" || next == "remove"
	case "compose":
		return composeDeletes(args[i+1:])
	}
	return false
}

// pruneAll reports -a or --all (also in a cluster such as -af) among the
// arguments of a prune.
func pruneAll(args []string) bool {
	for _, a := range args {
		name, value, _ := strings.Cut(a, "=")
		if (abbrev(name, "--all") && value != "false") || (shortFlags.MatchString(a) && strings.Contains(a, "a")) {
			return true
		}
	}
	return false
}

// composeDeletes reports "compose down" with the volumes or the images,
// and "compose rm".
func composeDeletes(args []string) bool {
	down := false
	for _, a := range args {
		down = down || a == "down"
		if a == "rm" {
			return true
		}
	}
	if !down {
		return false
	}
	for _, a := range args {
		name, value, _ := strings.Cut(a, "=")
		if (name == "--volumes" && value != "false") || strings.HasPrefix(a, "--rmi") || (shortFlags.MatchString(a) && strings.Contains(a, "v")) {
			return true
		}
	}
	return false
}
