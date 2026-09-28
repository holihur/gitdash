# Kubernetes / Helm (k8s) repository example

A minimal Helm chart plus the commands to publish it to gitdash's `k8s`
registry and consume it with the native `helm` client.

## Publish (HTTP repository)

Package the chart and upload the archive. gitdash reads `name` / `version`
from `Chart.yaml` and generates the repository `index.yaml`:

```bash
helm package hello                       # -> hello-0.1.0.tgz
curl -u <owner>:<PAT> \
  -F file=@hello-0.1.0.tgz \
  http://<host>/api/packages/k8s/<owner>/<repo>/publish
```

## Consume

```bash
helm repo add gitdash http://<host>/api/packages/k8s/<owner>/<repo>
helm repo update
helm install hello gitdash/hello
```

Private repositories need credentials on `helm repo add`:

```bash
helm repo add gitdash http://<host>/api/packages/k8s/<owner>/<repo> \
  --username <owner> --password <PAT>
```

## OCI (optional)

`helm push` targets an OCI registry; gitdash's Docker / OCI endpoint already
implements the Distribution API, so you can also do:

```bash
helm package hello
helm push hello-0.1.0.tgz oci://<host>/<owner>
```
