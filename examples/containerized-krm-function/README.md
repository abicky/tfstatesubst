# Containerized KRM function

This example runs `tfstatesubst` as a containerized KRM function to populate
an Azure Key Vault `SecretProviderClass` and a Microsoft Entra Workload
Identity `ServiceAccount` from Terraform state.

The manifests follow the Microsoft documentation for
[using Workload Identity with the Azure Key Vault CSI driver][csi] and
[configuring a Workload Identity service account][workload-identity].

Run from the repository root:

```console
$ kustomize build --enable-alpha-plugins examples/containerized-krm-function
```

`terraform.tfstate` is synthetic and contains only example values. In a
real kustomization, change the `src` mount in `tfstatesubst.yaml` to the local
Terraform state file that provides these resources, or configure
`TFSTATESUBST_TFSTATE` with a supported remote state URL and credentials.

[csi]: https://learn.microsoft.com/en-us/azure/aks/csi-secrets-store-identity-access?tabs=azure-portal&pivots=access-with-a-microsoft-entra-workload-identity
[workload-identity]: https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview?tabs=dotnet
