# Get started

## Difference with [multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) in open-cluster-management

This reposotiy is an extension to [open-cluster-management-io/multicluster-controlplane](https://github.com/open-cluster-management-io/multicluster-controlplane) with the following enhancements:
1. Added managed cluster info addon
2. Added policy addon
3. Added managed service account addon
4. The above addon components are covered in Integration/E2E test cases

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

> **Warning**
> clusteradm version should be v0.4.1 or later


