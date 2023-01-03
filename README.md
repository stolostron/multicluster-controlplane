# Get started

## Difference with [multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) in open-cluster-management

This reposotiy is an extension to [open-cluster-management-io/multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) with the following enhancements:
1. Added managed cluster info addon
2. Added policy addon
3. Added managed service account addon

In line with [open-cluster-management-io/multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane):
1. Using the same API Server, it starts core kubernetes and enables open-cluster-management registration controllers
2. Both embedded etcd and external etcd are supported
3. Test scenarios in [open-cluster-management-io/multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) are all covered in the current repository

## Install multicluster-controlplane

### Option 1: Run multicluster-controlplane as a local binary
- Setup a multicluster-controlplane from the binary
- Join a KinD managed cluster for the controlplane
```bash
$ make setup-integration
```

### Option 2: Run multicluster-controlplane on a KinD cluster(hosting)
- Setup a KinD cluster as the hosting cluster
- Deploy controlplane1 and controlplane2 on the hosting cluster
- Join KinD managed cluster controlplane1-mc1 for controlplane1 
- Join KinD managed cluster controlplane2-mc1 for controlplane2
```bash
$ make setup-e2e
```

### Option 3: Run the multicluster-controlplane binary with external etcd
On KinD cluster(Option 2) we use external etcd by default, but we can also specify external etcd when starting binary multicluster-controlplane server.

Start an etcd server first, then you need to update the following parameters in [setup-integration.sh](https://github.com/stolostron/multicluster-controlplane/blob/main/test/scripts/setup-integration.sh)
```bash
- "--enable-embedded-etcd=false"
- "--etcd-servers=<etcd-servers>"
- "--etcd-cafile=<etcd-cafile>"
- "--etcd-certfile=<etcd-certfile>"
- "--etcd-keyfile=<etcd-keyfile>"
```
After setting the parameters above, you can start the controlplane as you did in Option 1
```bash
$ make setup-integration
```

> **Warning**
> clusteradm version should be v0.4.1 or later


