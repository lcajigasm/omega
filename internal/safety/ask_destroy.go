package safety

import "strings"

// destroyVerbs are the subcommands that delete a virtual machine, a
// cluster, a model or an environment for good in the tools vmTools names.
const destroyVerbs = "delete rm remove purge destroy reset factory-reset"

// vmTools run virtual machines, local clusters and models: deleting one
// throws away its disks, its images or a download of many gigabytes, like
// docker system prune --volumes does.
const vmTools = "colima limactl lima orb orbctl multipass vagrant minikube kind k3d ollama"

// destroysElsewhere reports a command that deletes something that does not
// come back by itself and that is not a file of the user: a virtual
// machine, a cluster, a model, a conda environment, the crontab, a
// password in the Keychain, or what lives on a remote service (a GitHub
// repository, an S3 bucket, Kubernetes objects, infrastructure managed by
// Terraform or Pulumi, a cloud resource).
func destroysElsewhere(verb string, args []string) bool {
	pos := operands(args, "")
	at := func(i int) string {
		if i < len(pos) {
			return pos[i]
		}
		return ""
	}
	switch {
	case has(vmTools, verb):
		return anyArg(pos, func(a string) bool { return has(destroyVerbs, a) })
	}
	switch verb {
	case "conda", "mamba", "micromamba":
		switch at(0) {
		case "env":
			return has("remove rm", at(1))
		case "remove", "uninstall":
			return anyArg(args, func(a string) bool { return a == "--all" })
		}
	case "crontab":
		// -r removes the whole crontab without a copy; -ir asks first,
		// which nobody answers in a run.
		return anyArg(args, func(a string) bool { return shortFlags.MatchString(a) && strings.Contains(a, "r") })
	case "gh":
		return has("repo release gist secret variable ssh-key gpg-key label cache codespace run", at(0)) && has("delete rm remove", at(1))
	case "aws":
		if at(0) == "s3" {
			switch at(1) {
			case "rb":
				return true
			case "rm", "mv":
				return anyArg(args, func(a string) bool { return a == "--recursive" })
			case "sync":
				return anyArg(args, func(a string) bool { return a == "--delete" })
			}
		}
		return anyArg(pos, func(a string) bool {
			return strings.HasPrefix(a, "delete-") || strings.HasPrefix(a, "terminate-") || strings.HasPrefix(a, "remove-")
		})
	case "kubectl", "oc":
		// Options with a value (-n ns) may come before the subcommand.
		return anyArg(pos, func(a string) bool { return a == "delete" || a == "drain" }) ||
			(anyArg(pos, func(a string) bool { return a == "replace" }) && anyArg(args, func(a string) bool { return a == "--force" }))
	case "helm":
		return anyArg(pos, func(a string) bool { return has("uninstall delete del un", a) })
	case "terraform", "tofu", "terragrunt":
		if at(0) == "destroy" {
			return true
		}
		return has("apply plan", at(0)) && anyArg(args, func(a string) bool { return a == "-destroy" || a == "--destroy" })
	case "pulumi":
		return at(0) == "destroy" || (at(0) == "stack" && at(1) == "rm")
	case "gcloud", "az", "doctl", "flyctl", "fly", "heroku", "vercel", "netlify", "wrangler":
		return anyArg(pos, func(a string) bool {
			return has("delete destroy remove rm purge", a) || strings.HasSuffix(a, ":destroy")
		})
	}
	return false
}
