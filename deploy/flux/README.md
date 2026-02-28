# FluxCD GitOps Configuration
#
# After the CAPI bootstrap + pivot, FluxCD is installed on the workload
# cluster so it can self-manage all services from this Git repository.
#
# Directory layout:
#
#   deploy/flux/
#     flux-system.yaml        ← GitRepository + top-level Kustomization
#     kustomizations/
#       cnpg-operator.yaml    ← CloudNative-PG operator (HelmRelease)
#       cnpg-cluster.yaml     ← DKP bot PostgreSQL cluster
#       observability.yaml    ← Full observability stack (HelmReleases)
#       dkpbot.yaml           ← DKP bot HelmRelease
#
# Bootstrap:
#   flux bootstrap github \
#     --owner=jensholdgaard \
#     --repository=discord-dkp-bot \
#     --branch=main \
#     --path=deploy/flux \
#     --personal
#
# After bootstrap Flux watches this repo and reconciles all resources
# defined in the kustomizations/ folder, in dependency order.
