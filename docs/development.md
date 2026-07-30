# Local Development

You'll need a Kubernetes cluster to run against. Use [KIND](https://sigs.k8s.io/kind) for local development, or point at a remote cluster via your kubeconfig.

## Prerequisites

Install: Go, Docker, kind, kubectl, [task](https://taskfile.dev/)

## Quickstart

1. Clone the repository and initialize the build submodule:

```bash
git clone https://github.com/openmcp-project/metrics-operator
cd metrics-operator
git submodule update --init
```

2. Configure a DataSink for local testing. Copy the example and fill in your credentials:

```bash
cp examples/datasink/basic-datasink.yaml examples/datasink/dynatrace-prod-setup.yaml
# edit dynatrace-prod-setup.yaml with your endpoint and credentials
```

> The `examples/datasink/dynatrace-prod-setup.yaml` path is in `.gitignore` — safe to put real credentials here locally.

3. Set up a local kind cluster with all CRDs, Crossplane, and sample resources:

```bash
task dev:local:all
```

4. Run the operator locally:

```bash
task run
```

5. Check your data sink for incoming metrics.

## Common Tasks

| Command              | Description                                                       |
| -------------------- | ----------------------------------------------------------------- |
| `task dev:local:all` | Set up local kind cluster with CRDs, Crossplane, sample resources |
| `task run`           | Run the operator locally                                          |
| `task dev:clean`     | Delete the local kind cluster                                     |
| `task test`          | Run all Go tests                                                  |
| `task generate`      | Regenerate CRDs and deepcopy code after API changes               |
| `task validate:lint` | Run golangci-lint                                                 |

Run `task` with no arguments for the full list of available tasks.
