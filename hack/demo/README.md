[comment]: # ( Copyright Contributors to the Open Cluster Management project )
# Get started 

## Prerequisites
1. An OpenShift cluster as the management cluster, and deploy a controlplane in this cluster
    ```bash
        export SELF_MANAGEMENT=true
        make deploy
    ```
2. An OpenShift cluster as the managed cluster `hosted-cluster1`
3. An OpenShift cluster as the managed cluster `hosted-cluster2`
4. A KinD cluster as managed cluster

## Run

```bash
export MANAGEMENT_KUBECONFIG=<your management cluster kubeconfig file path>
export HOSTED_CLUSTER1_KUBECONFIG=<your hosted cluster1 kubeconfig file path>
export HOSTED_CLUSTER2_KUBECONFIG=<your hosted cluster2 kubeconfig file path>
export KIND_CLUSTER_KUBECONFIG=<your kind cluster kubeconfig file path>

./next-generation.sh
```

## Clean up

```bash
export MANAGEMENT_KUBECONFIG=<your management cluster kubeconfig file path>
export HOSTED_CLUSTER1_KUBECONFIG=<your hosted cluster1 kubeconfig file path>
export HOSTED_CLUSTER2_KUBECONFIG=<your hosted cluster2 kubeconfig file path>
export KIND_CLUSTER_KUBECONFIG=<your kind cluster kubeconfig file path>

./clean-up.sh
```
