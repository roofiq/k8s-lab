# Ansible lab: Kubernetes on malinka

The GitHub runner and the managed machine are the same Raspberry Pi. Ansible runs locally; this setup does not use SSH.

## Raspberry Pi prerequisite: memory cgroups

Before running the setup pipeline, check that the memory controller is enabled:

```bash
cat /sys/fs/cgroup/cgroup.controllers
```

The output must include `memory`. On this Raspberry Pi OS installation it was missing, and `/proc/cmdline` contained `cgroup_disable=memory`. Docker can still start in this state, but the Kubernetes setup needs the memory controller. The playbooks currently do not configure this boot setting.

If `memory` is missing, back up and edit the boot command line:

```bash
# Keep an existing backup if one is already present.
sudo cp -n /boot/firmware/cmdline.txt /boot/firmware/cmdline.txt.bak
sudo nano /boot/firmware/cmdline.txt
```

Append `cgroup_enable=memory` after a space at the end of the existing line. Keep the entire file on **one line** and preserve the other boot parameters. Save with Ctrl+O, Enter, then exit with Ctrl+X.

```bash
# Apply the boot setting; SSH disconnects during the reboot.
sudo reboot
```

Reconnect over SSH and run `cat /sys/fs/cgroup/cgroup.controllers` again. Confirm that `memory` is now listed before running the setup pipeline or rerunning its failed job. If it is still missing, resolve the kernel configuration before retrying cluster creation.

Reference: [Raspberry Pi kernel support for cgroup_enable](https://github.com/raspberrypi/linux/blob/rpi-6.18.y/kernel/cgroup/cgroup.c).

## Files and concepts

- `inventory.ini`: the machines to manage. `[lab]` is a group and `malinka` is a host alias. `ansible_connection=local` means the machine running Ansible, regardless of its hostname. Only use this inventory on the Pi.
- `site.yml`: the main entry point. It imports component playbooks in order; the workflow always calls this file.
- `playbooks/docker.yml`: the Docker component playbook, containing one play for the `lab` group. Tasks run in order. It installs Docker, starts its service and verifies the daemon.
- `playbooks/kind.yml` and `playbooks/kubectl.yml`: install pinned Linux ARM64 binaries in `/usr/local/bin` and verify that the commands run.
- `playbooks/cluster.yml`: gives the runner Docker access, creates the `lab` cluster if missing and waits for its nodes to become Ready.
- `../kind/cluster.yml`: kind configuration with one control-plane node and one worker node, both using a pinned Kubernetes image. The API listens only on the Pi's loopback interface.
- `../ansible.cfg`: forces sudo even when switching to the same user, so newly granted Docker group membership takes effect immediately. Run Ansible from the repository root.
- `become: true`: execute tasks with elevated privileges, using sudo by default.
- `ansible_facts`: information collected from the machine. The pre-task verifies Debian 13 before installing its packages.
- `ansible.builtin.apt`: a built-in module for Debian packages. We install `docker.io` (Engine) and `docker-cli` (client), plus their dependencies.
- `state: present`: install missing packages; do not request an upgrade on every run. `latest` would request the newest available package version.
- `cache_valid_time: 3600`: refresh the package index when it is older than an hour.
- `ansible.builtin.systemd_service`: `state: started` ensures Docker is running now; `enabled: true` enables startup after reboot.

Ansible describes the desired state. Running the playbook again should leave installed packages and a running, enabled service unchanged. Package-index refreshes may still report a change. This is idempotency: repeating the setup does not reinstall everything.

## Pipeline

Push to `main` -> `lab-setup.yml` -> checkout -> check sudo -> install Ansible tooling -> validate syntax -> apply `site.yml` -> successful setup triggers the separate `worker-deploy.yml` workflow.

Ansible must be installed before it can execute a playbook, so the workflow bootstraps `ansible-core` and `python3-apt` with apt. The playbooks install Docker, kind and kubectl, add the current runner user to the Docker group, and configure the cluster. Application deployment belongs to the separate workflow; see [worker deployment](../k8s/README.md). Docker group membership grants control over the host through Docker.

The runner currently runs as `roofi`. That account needs non-interactive sudo for this pipeline. On the Pi, check:

```bash
sudo -k
sudo -n true
```

A password-required error means an administrator must decide how to grant automation privileges first. The workflow intentionally fails early in that case. It does not change sudoers or store a sudo password.

A runner with unrestricted sudo can execute arbitrary root commands from workflows. Only trusted workflow code should run on this lab machine. A job-level branch condition prevents accidental runs; it does not enforce runner access control.

## Try it manually on the Pi

From the repository root, after installing Ansible tooling:

```bash
sudo apt-get update
sudo apt-get install -y ansible-core python3-apt

# Show which machines are selected and which tasks would run.
ansible-inventory -i ansible/inventory.ini --graph
ansible-playbook -i ansible/inventory.ini ansible/site.yml --list-tasks

# Parse the playbook without installing anything.
ansible-playbook -i ansible/inventory.ini ansible/site.yml --syntax-check

# Apply it; -K prompts for your sudo password.
ansible-playbook -i ansible/inventory.ini ansible/site.yml -K

# Run again to observe ok versus changed, then verify the daemon.
ansible-playbook -i ansible/inventory.ini ansible/site.yml -K
sudo docker --host unix:///var/run/docker.sock version
```

`--check --diff` previews supported changes instead of applying them. The cluster-management block is skipped in check mode. It is not a full test: on a fresh machine, the service task can fail because check mode did not actually install Docker. Try it after the first installation. Syntax checks do not verify package availability, sudo access or service health.

## Binary installation: kind and kubectl

We use kind `v0.33.0` and kubectl `v1.37.0`. This matches the Kubernetes version in kind's default node image for that release. Versions and SHA256 checksums are defined together in each component's `vars` section; change both when upgrading. The playbooks intentionally support Linux ARM64 only and require working system CA certificates for HTTPS downloads.

Unlike Docker, these tools are downloaded as standalone executables:

- `ansible.builtin.get_url` downloads a file onto the managed machine.
- `{{ kind_version }}` inserts a variable into the download URL.
- `checksum` verifies the expected SHA256 hash. A matching existing binary is not downloaded again.
- `dest` is the installed file path. `/usr/local/bin` is the standard location for locally installed commands.
- `owner: root`, `group: root`, `mode: '0755'` allow everyone to read and execute the binary, but only root to modify it.
- `changed_when: false` marks a version check as read-only. `when: not ansible_check_mode` skips execution during a dry run, when the binary might not have been installed.

To install only these two components on the Pi from the repository root:

```bash
ansible-playbook -i ansible/inventory.ini ansible/site.yml --tags kind,kubectl -K
```

`--tags kind,kubectl` selects those components; `-K` asks for the sudo password and can be omitted when passwordless sudo is configured. The pipeline runs the whole `site.yml`, so it needs no new steps for these components.

To check the tools after the pipeline succeeds:

```bash
kind version
kubectl version --client
```

`kind version` reports the installed kind version. `kubectl version --client` reports only the local client version, without requiring a running cluster. Installing these tools does not create a Kubernetes cluster.

`get_url` has limited check-mode support: it may contact the URL but does not download and verify the binary. A successful syntax check or dry run is not proof of a working installation.

Sources: [kind release](https://github.com/kubernetes-sigs/kind/releases/tag/v0.33.0), [kind ARM64 checksum](https://github.com/kubernetes-sigs/kind/releases/download/v0.33.0/kind-linux-arm64.sha256sum), [kubectl installation](https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/), [kubectl ARM64 checksum](https://dl.k8s.io/release/v1.37.0/bin/linux/arm64/kubectl.sha256).

## Cluster configuration

`site.yml` runs the cluster playbook after installing the tools. It creates a two-node cluster named `lab` as the runner user (`roofi`): `lab-control-plane` and `lab-worker`. The worker starts with the label `node-pool: workloads`. After registration, Ansible adds `node-role.kubernetes.io/workloads: ""`, so kubectl displays its role as `workloads`. Kubernetes restricts role labels during kubelet registration, so they cannot go directly in the kind node labels. The kind configuration still uses `role: worker`; kind supports only `control-plane` and `worker` roles. The pinned node image and topology live in `kind/cluster.yml`.

The playbook checks whether the cluster exists before creating it. On later runs it preserves the cluster and workloads, starts any exited node containers, refreshes the kubeconfig and waits for nodes to become Ready. This recovery runs during provisioning, not automatically on boot. Changing the node image or topology does not rebuild an existing cluster or add nodes to it; recreation is a separate, destructive operation. If the old single-node cluster already exists, it must be explicitly recreated before deploying the worker with its workloads node selector.

The dedicated kubeconfig is `/home/roofi/.kube/k8s-lab.yaml`, readable only by its owner. Commands explicitly select it, so they do not overwrite or rely on another default Kubernetes context.

After the initial setup, run only the cluster component or inspect it:

```bash
# From the repository root, as roofi. Requires the tools and Docker installed.
ansible-playbook -i ansible/inventory.ini ansible/site.yml --tags cluster

# Select the lab connection for commands in this terminal session.
export KUBECONFIG="$HOME/.kube/k8s-lab.yaml"
kubectl --context kind-lab get nodes
kubectl --context kind-lab get pods
```

`--tags cluster` selects the imported cluster playbook. `export KUBECONFIG` tells kubectl which credentials and API address to use; `--context kind-lab` selects the cluster entry in that file.

The setup and deployment use fresh sudo sessions for Docker operations, so the running GitHub runner does not need a restart to pick up the group change. For plain `docker` commands in an existing SSH session, log out and back in first.

## Add another component

Create its playbook under `playbooks/` and add an `ansible.builtin.import_playbook` entry in `site.yml`. Imports are resolved relative to `site.yml` and run in the listed order. Keep component verification in its playbook so the pipeline does not need component-specific steps.

The pipeline always runs:

```bash
ansible-playbook -i ansible/inventory.ini ansible/site.yml
```

To select only Docker manually, append `--tags docker` (and `-K` if sudo needs a password). `site.yml` is a convention, not an automatically discovered filename. Ansible runs only when invoked; it does not continuously monitor configuration drift.

## Other choices

- **Docker source:** Debian packages keep this first exercise small. Docker's own apt repository offers its release stream, but requires a signing key and repository configuration. Do not mix the two package sets.
- **Execution:** running Ansible locally fits one Pi. Later, a separate Linux runner can connect to the Pi over SSH; the inventory changes while most tasks stay the same.
- **Structure:** `site.yml` imports small component playbooks. Roles can group reusable tasks, defaults, templates and handlers as the lab grows; the current small lab does not need them yet.
- **Tools:** shell scripts are useful for bootstrapping but need explicit state checks. Ansible manages OS configuration; Terraform is mainly useful when creating infrastructure such as VMs and networks.

References: [reusing playbooks](https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_reuse.html), [apt module](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/apt_module.html), [systemd module](https://docs.ansible.com/projects/ansible/latest/collections/ansible/builtin/systemd_service_module.html), [check mode](https://docs.ansible.com/projects/ansible/latest/playbook_guide/playbooks_checkmode.html), [Debian Docker package](https://packages.debian.org/trixie/docker.io).
