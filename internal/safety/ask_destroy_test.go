package safety

import "testing"

// A5 of the fifth review: deleting a virtual machine, a cluster, a model,
// an environment, the crontab, a Keychain item or something remote is
// asked about every time, as docker system prune --volumes is.
func TestAlwaysConfirmDestroysElsewhere(t *testing.T) {
	for cmd, want := range map[string]bool{
		`colima delete`:                                  true,
		`colima delete -f default`:                       true,
		`colima list`:                                    false,
		`limactl delete default`:                         true,
		`orb delete ubuntu`:                              true,
		`multipass purge`:                                true,
		`vagrant destroy -f`:                             true,
		`vagrant status`:                                 false,
		`minikube delete --all`:                          true,
		`kind delete clusters --all`:                     true,
		`ollama rm llama3`:                               true,
		`ollama list`:                                    false,
		`conda env remove -n old`:                        true,
		`conda remove --all -n old`:                      true,
		`conda remove -n env numpy`:                      false,
		`crontab -r`:                                     true,
		`crontab -l`:                                     false,
		`security delete-generic-password -s x`:          true,
		`security delete-keychain ~/x.keychain`:          true,
		`gh repo delete me/x --yes`:                      true,
		`gh repo view`:                                   false,
		`aws s3 rm s3://bucket/ --recursive`:             true,
		`aws s3 rm s3://bucket/key`:                      false,
		`aws s3 rb s3://bucket --force`:                  true,
		`aws ec2 terminate-instances --instance-ids i-1`: true,
		`aws s3 ls`:                                      false,
		`kubectl delete pod x`:                           true,
		`kubectl -n prod delete deploy x`:                true,
		`kubectl get pods`:                               false,
		`terraform destroy -auto-approve`:                true,
		`terraform apply -destroy`:                       true,
		`terraform plan`:                                 false,
		`helm uninstall app`:                             true,
		`gcloud compute instances delete vm-1`:           true,
		`podman machine rm`:                              true,
	} {
		if got := AlwaysConfirm(cmd); got != want {
			t.Errorf("AlwaysConfirm(%q) = %v, want %v", cmd, got, want)
		}
	}
}
