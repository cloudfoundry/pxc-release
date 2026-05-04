#!/usr/bin/env bash

GENERAL_OPS=(
  "-o operations/update-deployment-name.yml"
  "-o operations/update-vm-type.yml"
	"-o operations/update-persistent-disk-size.yml"
	"-o operations/update-network.yml"
	"-o operations/update-vm-type.yml"
	"-o operations/update-azs.yml"
	"-o operations/local-dev-release.yml"
	"-o operations/add-replicator-job.yml"
)
SOURCE_OPS=(
	"-o operations/set-replicator-source.yml"
)
REPLICA_OPS=(
	"-o operations/set-replicator-target.yml"
)
REPLICA_VARS=(
  "-v deployment_name=replica"
  "-v source_deployment_name=source"
)
SOURCE_VARS=(
  "-v deployment_name=source"
)
GENERAL_VARS=(
  "-v vm_type=medium"
  "-v azs=['az-0','az-1','az-2']"
  "-v network_name=004-dev-scf-eu01-mgmt"
  "-v persistent_disk_size=10000"
)
#shellcheck disable=2068
bosh int pxc-deployment.yml \
  ${GENERAL_OPS[@]} \
  ${GENERAL_VARS[@]} \
  ${REPLICA_OPS[@]} \
  ${REPLICA_VARS[@]} \
  > replica.yml

#shellcheck disable=2068
bosh int pxc-deployment.yml \
  ${GENERAL_OPS[@]} \
  ${GENERAL_VARS[@]} \
  ${SOURCE_OPS[@]} \
  ${SOURCE_VARS[@]} \
  > source.yml
