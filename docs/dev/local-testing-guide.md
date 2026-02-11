# Local Testing Guide

This guide provides step-by-step instructions for setting up a local Kubernetes cluster to test the RisingWave Operator and RisingWave clusters.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)
- [Go](https://go.dev/doc/install) (for building the operator)
- [psql](https://www.postgresql.org/download/) (for connecting to RisingWave)

## Step 1: Create a Local Cluster

Use `kind` to create a new cluster:

```bash
kind create cluster --name rw-test
```

## Step 2: Install cert-manager

The RisingWave operator requires `cert-manager` for its webhooks.

```bash
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.14.5/cert-manager.yaml

# Wait for all pods to be ready
kubectl wait --for=condition=Ready pods --all -n cert-manager --timeout=300s
```

## Step 3: Build and Deploy the Operator

### 3.1 Build the Operator Image

Build the e2e image (which includes the manager and its dependencies):

```bash
make build-e2e-image
```

This will create an image tagged as `docker.io/risingwavelabs/risingwave-operator:dev`.

### 3.2 Load Image into kind

Load the locally built image into your `kind` cluster:

```bash
kind load docker-image docker.io/risingwavelabs/risingwave-operator:dev --name rw-test
```

### 3.3 Deploy the Operator

Generate the test manifest and apply it:

```bash
make generate-test-yaml
kubectl apply --server-side --force-conflicts -f config/risingwave-operator-test.yaml

# Wait for the operator to be ready
kubectl wait --for=condition=Ready pods -l control-plane=controller-manager -n risingwave-operator-system --timeout=300s
```

## Step 4: Deploy a RisingWave Cluster

Deploy a minimal RisingWave instance using memory-based storage for local testing.

```bash
kubectl apply -f docs/manifests/stable/memory/risingwave.yaml
```

Wait for the RisingWave components to be ready:

```bash
kubectl wait --for=condition=Ready pods -l risingwave/name=risingwave --timeout=300s
```

## Step 5: Verify the Setup

### 5.1 Access RisingWave

Forward the frontend service port to your local machine:

```bash
kubectl port-forward svc/risingwave-frontend 4567:service
```

### 5.2 Test with psql

Connect to the RisingWave cluster:

```bash
psql -h localhost -p 4567 -d dev -U root
```

## Step 6: Testing User Management (Optional)

You can test user management via the `RisingWaveUser` CRD.

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: test-user
spec:
  risingWaveRef:
    name: risingwave
  permissions:
    - CREATEDB
```

Apply the manifest and verify:

```bash
kubectl apply -f your-user-manifest.yaml
psql -h localhost -p 4567 -d dev -U root -c "\du"
```
