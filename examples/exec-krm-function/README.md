# Exec function

This variant runs an installed `tfstatesubst` binary as a Kustomize exec KRM
function. Ensure `tfstatesubst` is in `PATH`, then run from the repository
root:

```console
$ kustomize build --enable-alpha-plugins --enable-exec examples/exec-krm-function
```

The wrapper reads the synthetic state from `terraform.tfstate`. Change
`TFSTATESUBST_TFSTATE` in `tfstatesubst` when adapting the example to another
state file or a supported remote state URL.
