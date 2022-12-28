# Get started

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


