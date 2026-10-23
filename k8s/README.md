# Worker deployment

Push to `main` runs **Lab setup** (`.github/workflows/lab-setup.yml`). After it succeeds, **Worker deployment** (`.github/workflows/worker-deploy.yml`) builds and deploys the Go worker on the same Raspberry Pi runner.

The deployment checks that the setup commit is still the current `main` commit. If a newer commit has arrived, it skips deployment and waits for that commit's setup. Both workflow files must be present on the default branch for this trigger to work. This lab assumes one runner with the `malinka` label.

The worker Deployment selects `node-pool: workloads`, so its pods run on `lab-worker`. Ansible also adds the role label after node registration, making `kubectl get nodes` display the role `workloads`. The control-plane node remains dedicated to cluster components. An older single-node cluster must be explicitly recreated with the new kind configuration first; otherwise worker pods remain Pending.

The workflow separates these operations into two steps:

1. **Build and load image:** build the image tagged with the setup commit SHA, then load it into kind. No registry is needed.
2. **Deploy worker:** apply the manifest with the same image tag, wait for rollout and print recent logs. This step runs only after a successful build and load.

The image tag is defined once in the job environment, so both steps use the same revision.

The Dockerfile lives in `cmd/worker`, but the build context is the repository root because it needs `go.mod` and the source tree:

```bash
docker build --file cmd/worker/Dockerfile --tag k8s-lab-go-worker:local .
```

`--file` selects the Dockerfile, `--tag` names the image, and the final `.` supplies files from the current directory to the build.

To inspect the deployed worker on the Pi:

```bash
export KUBECONFIG="$HOME/.kube/k8s-lab.yaml"
kubectl --context kind-lab get deployment go-worker
kubectl --context kind-lab logs deployment/go-worker --tail=20
```

The scratch image is unchanged and does not include CA certificates yet. HTTPS calls can fail certificate verification. A successful rollout confirms the container starts; this worker has no readiness probe, so it does not prove successful HTTP requests.
