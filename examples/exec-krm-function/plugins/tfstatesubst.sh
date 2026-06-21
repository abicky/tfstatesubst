#!/bin/sh

TFSTATESUBST_TFSTATE="${TFSTATESUBST_TFSTATE:-$(dirname "$0")/../terraform.tfstate}"
TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION="^(?:azurerm_key_vault\.example\.name|azurerm_user_assigned_identity\.workload\.client_id|data\.azurerm_client_config\.current\.tenant_id)$"
export TFSTATESUBST_TFSTATE
export TFSTATESUBST_ALLOW_REFERENCE_EXPRESSION

exec tfstatesubst
