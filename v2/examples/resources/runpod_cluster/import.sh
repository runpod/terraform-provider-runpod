# Clusters are imported by cluster id. The cluster object the API returns does
# not carry the container configuration, so an import leaves image, args, disk,
# env, ports, data_center_ids, mounts, start_ssh and start_jupyter empty in
# state. Each of them forces replacement: any one the configuration sets plans
# a new cluster on the first apply, even when it matches what the cluster was
# created with. Leave them out of the configuration, or name every one it sets
# in lifecycle { ignore_changes = [...] }, before the first apply.
terraform import runpod_cluster.training cluster_abc123
