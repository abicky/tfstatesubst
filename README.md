# tfstatesubst

`tfstatesubst` is a command-line filter that reads a restricted Go template
from standard input, replaces calls to the `tfstate` function with values from
Terraform state, and writes the result to standard output.

```console
$ echo '{{ tfstate `data.aws_iam_role.lambda.arn` }}' | tfstatesubst
arn:aws:iam::123456789012:role/lambda
```

The default state location is `terraform.tfstate`. To use a different local
path or a supported URL, set `--state` (`-s`) or
`TFSTATESUBST_TFSTATE`:

```console
$ tfstatesubst --state .terraform/terraform.tfstate < input.yaml
$ TFSTATESUBST_TFSTATE=s3://example/state.tfstate tfstatesubst < input.yaml
```

Local files, HTTP(S), S3, Google Cloud Storage, Azure Blob Storage, and
Terraform Cloud/Enterprise state are supported through
[`tfstate-lookup`](https://github.com/fujiwara/tfstate-lookup). The relevant
credentials for the selected backend must be available in the environment.

## Installation

### Precompiled binary

Download the binary archive from the
[releases page](https://github.com/abicky/tfstatesubst/releases), unpack it,
and move the `tfstatesubst` executable to a directory in your `PATH` (for
example, `/usr/local/bin`).

The default `tfstatesubst_<os>_<arch>.tar.gz` archive supports all backends.
Backend-specific variants that exclude the other cloud provider dependencies
are also available as `tfstatesubst_<backend>_<os>_<arch>.tar.gz`, where
`<backend>` is `azurerm`, `gcs`, `s3`, or `tfe`.

For example, install the latest binary on a Mac with Apple silicon as follows:

```sh
curl -LO https://github.com/abicky/tfstatesubst/releases/latest/download/tfstatesubst_darwin_arm64.tar.gz
tar xvf tfstatesubst_darwin_arm64.tar.gz
mv tfstatesubst_darwin_arm64/tfstatesubst /usr/local/bin/
```

If you downloaded the archive with a browser on macOS and it cannot be opened
because the developer cannot be verified, remove the `com.apple.quarantine`
attribute:

```sh
xattr -d com.apple.quarantine /path/to/tfstatesubst
```

### Homebrew

```sh
brew install --cask abicky/tools/tfstatesubst
```

### Docker

The release image is available from Docker Hub and GitHub Container Registry:

```sh
docker run --rm abicky/tfstatesubst --help
docker run --rm ghcr.io/abicky/tfstatesubst --help
```

The unsuffixed image supports all backends and is tagged as `latest`,
`<major>.<minor>`, and the full release version. Backend-specific images are
available with `-azurerm`, `-gcs`, `-s3`, or `-tfe` appended to the version tag,
for example `ghcr.io/abicky/tfstatesubst:1.2.3-s3`. These variants exclude the
other cloud provider dependencies and are published to both registries.

### From source

```sh
go install github.com/abicky/tfstatesubst@latest
```

Alternatively, clone the repository and build the binary manually:

```sh
git clone https://github.com/abicky/tfstatesubst
cd tfstatesubst
make install
```

## Template syntax

Only direct `tfstate` calls with a single literal string argument are
accepted:

```gotemplate
{{ tfstate `aws_vpc.main.id` }}
{{ tfstate `aws_subnet.public['ap-northeast-1a'].id` }}
{{ tfstate "module.app.aws_lb.main.dns_name" }}
```

All other functions, data expressions, variables, pipelines, template
definitions, and control actions such as `if`, `range`, and `with` are
rejected. The complete input is validated and rendered before output is
written, so an error never produces partial output.

Input that does not call the `tfstate` function passes through without opening
the state file.

By default, a reference whose value is `null` produces an error. Pass
`--allow-null` to render such values as `null` instead:

```console
$ echo '{{ tfstate `aws_instance.example.optional_attribute` }}' | \
    tfstatesubst --allow-null
null
```

Set `TFSTATESUBST_ALLOW_NULL=true` to enable the same behavior through the
environment.

## Restricting accessible references

Use `--allow-reference-expression` to restrict the `tfstate` references that a
template may resolve. Its value is a Go regular expression matched against the
complete reference:

```console
$ tfstatesubst \
    --allow-reference-expression '^(?:aws_vpc\.main|data\.aws_iam_role\.lambda)\.' \
    < input.yaml
```

Regular expressions use Go syntax. By default, an expression may match a
substring; anchor it with `^` and `$` to require it to match the entire
reference. The equivalent environment variable is
`TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION`. If neither is set, references are
unrestricted.

## Kustomize KRM functions

The binary follows the KRM function transport contract: it accepts a
`ResourceList` on stdin and emits the transformed `ResourceList` on stdout.
Calls to the `tfstate` function can therefore be embedded in string-valued
resource fields.

See the complete Kustomize configurations for the
[containerized](examples/containerized-krm-function) and
[exec](examples/exec-krm-function) variants.

### Containerized function

Mount the state read-only into the release image:

```yaml
# tfstatesubst.yaml
apiVersion: example.com/v1alpha1
kind: TfstatesubstTransformer
metadata:
  name: tfstatesubst
  annotations:
    config.kubernetes.io/function: |
      container:
        image: ghcr.io/abicky/tfstatesubst:latest
        mounts:
          - type: bind
            src: terraform.tfstate
            dst: /workspace/terraform.tfstate
        envs:
          - TFSTATESUBST_TFSTATE=/workspace/terraform.tfstate
```

Add `tfstatesubst.yaml` to `transformers` in `kustomization.yaml`, then run:

```console
$ kustomize build --enable-alpha-plugins .
```

Kustomize restricts bind-mount sources to the current kustomization directory.
`TFSTATESUBST_TFSTATE` supplies the absolute path to the mounted state file.

### Exec function

Place an executable wrapper relative to the kustomization directory:

```sh
#!/bin/sh
TFSTATESUBST_TFSTATE="${TFSTATESUBST_TFSTATE:-$(dirname "$0")/../terraform.tfstate}"
export TFSTATESUBST_TFSTATE
exec tfstatesubst
```

Reference it from the function configuration:

```yaml
apiVersion: example.com/v1alpha1
kind: TfstatesubstTransformer
metadata:
  name: tfstatesubst
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: ./plugins/tfstatesubst.sh
```

Then enable exec functions explicitly:

```console
$ kustomize build --enable-alpha-plugins --enable-exec .
```

## Argo CD integration

Argo CD can run the exec function through its built-in Kustomize support. The
following example applies specifically to Argo CD running on Azure Kubernetes
Service (AKS). It assumes that the Terraform state is stored in Azure Blob
Storage and that the repo-server accesses it through Microsoft Entra Workload
Identity. Other environments require the corresponding state backend and
authentication configuration.

> [!WARNING]
> `--enable-exec` allows application repositories to execute arbitrary code in
> the repo-server container. An exec KRM function can also declare `envs` that
> overwrite environment variables inherited from the repo-server, including
> `TFSTATESUBST_TFSTATE` and `TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION`. Do not
> rely on repo-server environment variables to enforce a security boundary, and
> enable exec functions only when every Kustomize application repository is
> trusted. To avoid allowing every Kustomize application repository to define an exec
function, consider running `tfstatesubst` through an Argo CD [Config Management
Plugin](https://argo-cd.readthedocs.io/en/stable/operator-manual/config-management-plugins/).

Before installing the Argo CD Helm chart, configure an AKS federated identity
credential for the `argocd-repo-server` service account. Grant the associated
managed identity the `Storage Blob Data Reader` role for the storage account or
state container.

Choose the example that matches the AKS cluster's Kubernetes version, then use
its values to install or upgrade the Argo CD chart. Application repositories
can then run `tfstatesubst` as an exec function without selecting a custom Argo
CD source plugin.

> [!NOTE]
> Instead of installing `tfstatesubst` on `argocd-repo-server`, the executable
> can be committed to the repository that manages the application manifests.
> Set the exec function's `path` to the repository-relative location, and make
> sure the executable targets the repo-server's OS and architecture and retains
> its executable bit.

### Kubernetes 1.34 and 1.35

[Image volumes](https://kubernetes.io/docs/tasks/configure-pod-container/image-volumes/) are beta and disabled by default in
these versions, and the mounted files may not be executable. Use an init
container to make `tfstatesubst` available to the repo-server container:

```yaml
configs:
  cm:
    kustomize.buildOptions: --enable-alpha-plugins --enable-exec

repoServer:
  env:
  # Use the managed identity instead of a storage account access key.
  # See https://github.com/fujiwara/tfstate-lookup#azure-blob-storage-authentication
  - name: ARM_USE_AZUREAD
    value: "true"
  - name: TFSTATESUBST_TFSTATE
    value: azurerm://RESOURCE_GROUP/STORAGE_ACCOUNT/CONTAINER/terraform.tfstate
  - name: TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION
    # Only allow client_id references
    value: (?:^|\.)azurerm_user_assigned_identity\.[^.]+\.client_id$
  podLabels:
    azure.workload.identity/use: "true"
  serviceAccount:
    create: true
    name: argocd-repo-server
    annotations:
      azure.workload.identity/client-id: MANAGED_IDENTITY_CLIENT_ID
  initContainers:
  - name: install-tfstatesubst
    image: curlimages/curl:8.17.0
    command: [sh, -c]
    securityContext:
      runAsNonRoot: true
      runAsUser: 100
      runAsGroup: 101
      readOnlyRootFilesystem: true
      allowPrivilegeEscalation: false
      capabilities:
        drop:
        - ALL
    args:
    - |
      set -eu
      case "$(uname -m)" in
        x86_64) arch=amd64 ;;
        aarch64) arch=arm64 ;;
        *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
      esac
      archive="tfstatesubst_linux_${arch}"
      curl -fsSL https://github.com/abicky/tfstatesubst/releases/latest/download/${archive}.tar.gz \
      | tar xz -C /custom-tools --strip-components=1 "${archive}/tfstatesubst"
    volumeMounts:
    - name: custom-tools
      mountPath: /custom-tools
  volumes:
  - name: custom-tools
    emptyDir: {}
  volumeMounts:
  - name: custom-tools
    mountPath: /usr/local/bin/tfstatesubst
    subPath: tfstatesubst
    readOnly: true
```

### Kubernetes 1.36 or later

Starting with Kubernetes 1.36, [image volumes](https://kubernetes.io/docs/tasks/configure-pod-container/image-volumes/) are
stable and enabled by default. Mount the `tfstatesubst` container image
directly in the repo-server container:

```yaml
configs:
  cm:
    kustomize.buildOptions: --enable-alpha-plugins --enable-exec

repoServer:
  env:
  # Use the managed identity instead of a storage account access key.
  # See https://github.com/fujiwara/tfstate-lookup#azure-blob-storage-authentication
  - name: ARM_USE_AZUREAD
    value: "true"
  - name: TFSTATESUBST_TFSTATE
    value: azurerm://RESOURCE_GROUP/STORAGE_ACCOUNT/CONTAINER/terraform.tfstate
  - name: TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION
    # Only allow client_id references
    value: (?:^|\.)azurerm_user_assigned_identity\.[^.]+\.client_id$
  podLabels:
    azure.workload.identity/use: "true"
  serviceAccount:
    create: true
    name: argocd-repo-server
    annotations:
      azure.workload.identity/client-id: MANAGED_IDENTITY_CLIENT_ID
  volumes:
  - name: tfstatesubst
    image:
      reference: ghcr.io/abicky/tfstatesubst:latest
  volumeMounts:
  - name: tfstatesubst
    mountPath: /usr/local/bin/tfstatesubst
    subPath: ko-app/tfstatesubst
    readOnly: true
```

### KRM function

Add the following function configuration to an application repository:

```yaml
# tfstatesubst.yaml
apiVersion: example.com/v1alpha1
kind: TfstatesubstTransformer
metadata:
  name: tfstatesubst
  annotations:
    config.kubernetes.io/function: |
      exec:
        path: tfstatesubst
```

Then reference it as a transformer in the same kustomization:

```yaml
# kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
- deployment.yaml
transformers:
- tfstatesubst.yaml
```

## Author

Takeshi Arabiki ([@abicky](https://github.com/abicky))
