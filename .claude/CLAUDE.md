# RisingWave Operator - Project Guide

## Project Overview

RisingWave Operator is a Kubernetes operator for managing [RisingWave](https://github.com/risingwavelabs/risingwave), a distributed streaming processing platform. The operator automates deployment, scaling, and lifecycle management of RisingWave clusters on Kubernetes.

- **Language**: Go 1.19+
- **Framework**: Kubebuilder v3 with controller-runtime
- **License**: Apache 2.0
- **Compatibility**: K8s v1.21+, RisingWave v0.19.0+
- **CI**: Buildkite, Codecov

## Project Structure

```
risingwave-operator/
├── apis/risingwave/v1alpha1/     # CRD type definitions
│   ├── risingwave_types.go          # Main RisingWave CRD
│   ├── risingwave_user_types.go     # RisingWaveUser CRD (new)
│   └── zz_generated.deepcopy.go    # Auto-generated
├── cmd/manager/                   # Entry point
│   └── main.go                    # Main() with controller/webhook setup
├── config/
│   ├── crd/bases/                  # Generated CRD manifests
│   ├── rbac/                       # Generated RBAC rules
│   ├── webhook/                     # Webhook configurations
│   ├── samples/                     # Example manifests
│   └── local/                      # Local dev configs
├── docs/
│   └── general/api.md               # Auto-generated API docs
├── pkg/
│   ├── controller/                  # Reconciliation controllers
│   │   ├── risingwave_controller.go
│   │   └── risingwave_user_controller.go
│   ├── webhook/                    # Mutating/validating webhooks
│   ├── manager/                    # State machine (ctrlkit)
│   ├── factory/                    # K8s object factory
│   ├── object/                     # Object managers
│   ├── features/                   # Feature flags
│   ├── metrics/                    # Prometheus metrics
│   ├── rwclient/                   # DB connection pool (new)
│   ├── utils/                      # Utilities
│   └── consts/                     # Constants & labels
├── test/e2e/                      # Shell-based E2E tests
├── Makefile                        # Build system
├── go.mod/go.sum                   # Dependencies
└── .golangci.yaml                  # Lint configuration
```

## CRDs

### RisingWave (`risingwave.risingwavelabs.com`)

Manages a RisingWave cluster with components:
- **Meta** - Metadata management (StatefulSet)
- **Frontend** - Query serving (Deployment)
- **Compute** - Stream processing (StatefulSet)
- **Compactor** - Data compaction (Deployment)
- **Standalone** - Single-node deployment

### RisingWaveUser (`risingwaveuser.risingwavelabs.com`)

Manages database users for RisingWave:
- User creation/deletion via PostgreSQL protocol
- Password management (auto-generate or from Secret)
- Privilege grants (databases, schemas, tables, views, sources, sinks, functions)
- Authentication: password, OAuth (JWT/JWKS), LDAP

## Development Commands

### Essential Make Targets

```bash
# === Build ===
make build              # Build binary to bin/manager
make docker-build       # Build operator image (default: ghcr.io/.../risingwave-operator:latest)
make docker-cross-build # Multi-arch build (linux/amd64,linux/arm64)

# === Code Generation ===
make manifests          # Generate CRD YAMLs from // +kubebuilder markers
make generate          # Generate deepcopy, client, conversion code
make generate-all       # Run all generators

# === Testing ===
make test              # Run unit tests (includes manifests, fmt, vet, lint)
make test-e2e          # Run E2E tests (requires kind cluster)
make manifest-test     # Test CRD manifests

# === Quality ===
make fmt               # Format code
make vet               # Run go vet
make lint              # Run golangci-lint
make spellcheck        # Run cspell

# === Local Development ===
make install-local      # Install CRDs + webhooks to local cluster
make copy-local-certs  # Copy certs for Docker Desktop K8s
make run-local          # Run operator locally (requires install-local first)
```

### Testing

**Unit Tests** (`*_test.go` files):
```bash
make test                           # Runs all unit tests with coverage
go test ./pkg/controller/... -v       # Run specific package
go test ./pkg/controller/... -run TestSomething  # Run specific test
```

**E2E Tests** (shell scripts in `test/e2e/`):
```bash
make e2e-test                       # Run all E2E tests
./test/e2e/e2e.sh -l              # List available tests
E2E_TEST_CASE_PREFIX=risingwave::storage_support make e2e-test  # Run subset
```

### Local Development Workflow

```bash
# 1. Install CRDs and webhooks
make install-local

# 2. For Docker Desktop, copy certs
make copy-local-certs

# 3. Run operator locally
make run-local

# 4. In another terminal, apply sample
kubectl apply -f config/samples/risingwave_v1alpha1_risingwave.yaml
```

**Debugging**:
```bash
# Enable debug logs
make run-local -- -zap-devel -zap-log-level debug

# View operator logs
kubectl logs -f deployment/risingwave-operator-controller-manager -n risingwave-operator-system

# Port forward to test RisingWave
kubectl port-forward svc/risingwave-sample-frontend 4567:service
psql -h localhost -p 4567 -d dev -U root
```

## Architecture Patterns

### Controller Pattern (Standard controller-runtime)

```go
func (r *RisingWaveReconciler) Reconcile(
    ctx context.Context,
    req ctrl.Request,
) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    // 1. Fetch resource
    risingwave := &risingwavev1alpha1.RisingWave{}
    if err := r.Get(ctx, req.NamespacedName, risingwave); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Handle deletion (finalizer pattern)
    if !risingwave.DeletionTimestamp.IsZero() {
        return r.cleanup(ctx, risingwave)
    }

    // 3. Reconcile desired state
    if err := r.reconcileComponents(ctx, risingwave); err != nil {
        logger.Error(err, "failed to reconcile")
        return ctrl.Result{}, err
    }

    // 4. Update status
    if err := r.Status().Update(ctx, risingwave); err != nil {
        return ctrl.Result{}, err
    }

    return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
}
```

### State Machine Pattern (ctrlkit)

The RisingWave controller uses `ctrlkit` for complex workflows:

**Definition file**: `pkg/manager/risingwave_controller_manager.cm`
**Generated code**: `pkg/manager/risingwave_controller_manager_generated.go`

**Pattern**:
```
state { ... }    // Define K8s resources to manage
action { ... }   // Define operations
barrier { ... }   // Define conditional checks

workflow {        // Compose the flow
    ParallelJoin(
        SyncMetaService(...)
        SyncFrontendDeployment(...)
    )
    If(HasComponent("compactor"), Then(
        SyncCompactorDeployment(...)
    ))
}
```

After modifying `.cm` file, run `make generate-manager` to regenerate.

### Webhook Pattern

```go
// Mutating webhook (sets defaults)
func (w *RisingWaveMutatingWebhook) Default(
    ctx context.Context,
    obj runtime.Object,
) error {
    rw := obj.(*risingwavev1alpha1.RisingWave)
    if rw.Spec.DataDirectory == "" {
        rw.Spec.DataDirectory = "/risingwave/data"
    }
    return nil
}

// Validating webhook (validates specs)
func (w *RisingWaveValidatingWebhook) ValidateCreate(
    ctx context.Context,
    obj runtime.Object,
) (warnings admission.Warnings, err error) {
    rw := obj.(*risingwavev1alpha1.RisingWave)
    if rw.Spec.MetaStore == nil {
        return nil, fmt.Errorf("metaStore is required")
    }
    return nil, nil
}
```

## Adding Features

### Add a New CRD

1. Create `apis/<group>/<version>/<resource>_types.go`:
```go
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type MyResource struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec   MyResourceSpec   `json:"spec,omitempty"`
    Status MyResourceStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:Optional
type MyResourceSpec struct {
    Field string `json:"field,omitempty"`
}
```

2. Add `// +kubebuilder:rbac` markers to controller for RBAC

3. Generate code:
```bash
make manifests    # Generate CRD YAML
make generate     # Generate deepcopy methods
```

4. Create controller in `pkg/controller/my_resource_controller.go`

5. Register in `cmd/manager/main.go`:
```go
if err = myresourcecontroller.NewController(mgr.GetClient(), mgr.GetEventRecorder("myresource")).SetupWithManager(mgr); err != nil {
    setupLog.Error(err, "unable to create controller", "controller", "MyResource")
    os.Exit(1)
}
```

6. Add webhook in `pkg/webhook/my_resource_webhook.go` (optional)

7. Register webhook in `pkg/webhook/webhook.go`

### Add RBAC Markers

```go
// +kubebuilder:rbac:groups=risingwave.risingwavelabs.com,resources=myresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=risingwave.risingwavelabs.com,resources=myresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;delete
```

After running `make manifests`, RBAC rules are generated in `config/rbac/role.yaml`.

## Code Conventions

### Naming

- **CRDs**: `RisingWave`, `RisingWaveUser`, `MyResource`
- **Controllers**: `RisingWaveController`, `MyResourceController`
- **Webhooks**: `RisingWaveMutatingWebhook`, `MyResourceValidatingWebhook`
- **Interfaces**: Descriptive names or `-er` suffix

### Logging

```go
logger := log.FromContext(ctx)
logger.Info("message", "key", value)
logger.Error(err, "error message", "context", data)
```

### Error Handling

```go
if err != nil {
    return ctrl.Result{}, fmt.Errorf("action failed: %w", err)
}

// For NotFound errors that are expected
return ctrl.Result{}, client.IgnoreNotFound(err)
```

### Labels & Annotations

From `pkg/consts/consts.go`:

**Standard Labels**:
- `risingwave/component` - Component name: `meta`, `frontend`, `compute`, `compactor`, `standalone`
- `risingwave/name` - RisingWave instance name
- `risingwave/group` - Group name

**Annotations**:
- `risingwave/restart-at` - Trigger pod restart (timestamp)
- `risingwave.risingwavelabs.com/pause-reconcile` - Pause reconciliation

### Status Conditions

Condition pattern: `RisingWaveCondition<Name>`
```go
import (
    "k8s.io/apimachinery/pkg/api/meta"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

meta.SetStatusCondition(&risingwave.Status.Conditions, metav1.Condition{
    Type:    risingwavev1alpha1.RisingWaveConditionReady,
    Status:  metav1.ConditionTrue,
    Reason:   "AllComponentsReady",
    Message:  "All components are ready",
})
```

## Feature Flags

From `pkg/features/`:

| Feature | Stage | Default | Description |
|---------|--------|----------|-------------|
| `EnableOpenKruise` | Beta | false | Use OpenKruise for CloneSets/AdvancedStatefulSets |
| `EnableForceUpdate` | Beta | true | Allow force updates via annotation |
| `RandomSecretStorePrivateKey` | Alpha | false | Generate random secret store private keys |

**Usage**:
```bash
# Enable OpenKruise
--feature-gates=EnableOpenKruise=true

# Disable force update
--feature-gates=EnableForceUpdate=false

# Multiple gates
--feature-gates=EnableOpenKruise=true,EnableForceUpdate=false
```

## Environment Variables

**Pod Info** (injected by K8s):
- `POD_IP`, `POD_NAME`, `POD_NAMESPACE`

**RisingWave Configuration**:
- `RW_WORKER_THREADS` - Number of worker threads
- `JAVA_OPTS` - Java options for compactor

**Cloud Storage** (for state backend):
- `AWS_*` - S3 credentials
- `GCS_*` - Google Cloud Storage
- `AZURE_*` - Azure Blob Storage
- `MINIO_*` - MinIO

## Common Workflows

### Full Development Cycle

```bash
# 1. Make code changes
vim pkg/controller/risingwave_controller.go

# 2. Generate code (if CRD/struct changes)
make generate-all

# 3. Run tests
make test

# 4. Run locally
make install-local && make run-local

# 5. In another terminal, test
kubectl apply -f config/samples/risingwave_v1alpha1_risingwave.yaml

# 6. Commit when ready
git add .
git commit -m "feat: add feature"
```

### E2E Testing Cycle

```bash
# 1. Build E2E image
make build-e2e-image

# 2. Run all E2E tests
make e2e-test

# 3. Run specific test
E2E_TEST_CASE_PREFIX=risingwave::storage_support make e2e-test
```

### Deploy Custom Operator

```bash
# 1. Build image with custom tag
REGISTRY=docker.io/myorg TAG=v1.0.0 make docker-build

# 2. Load into kind (if using kind)
kind load docker-image docker.io/myorg/risingwave-operator:v1.0.0

# 3. Edit deployment manifest
# Update config/risingwave-operator.yaml with new image reference

# 4. Deploy
kubectl apply -f config/risingwave-operator.yaml
```

## Key Files Reference

| Purpose | File Path |
|----------|------------|
| Main entry | `cmd/manager/main.go` |
| Main controller | `pkg/controller/risingwave_controller.go` |
| User controller | `pkg/controller/risingwave_user_controller.go` |
| Webhook setup | `pkg/webhook/webhook.go` |
| Manager definition | `pkg/manager/risingwave_controller_manager.cm` |
| Generated manager | `pkg/manager/risingwave_controller_manager_generated.go` |
| CRD types | `apis/risingwave/v1alpha1/*.go` |
| Constants | `pkg/consts/consts.go` |
| Feature flags | `pkg/features/feature_gate.go` |
| Local webhook config | `config/local/webhook.yaml` |
| Development docs | `docs/dev/development.md` |
| Makefile | `Makefile` |

## Dependencies

**Go Modules**:
- `sigs.k8s.io/controller-runtime` - Operator framework
- `github.com/lib/pq` - PostgreSQL driver (for RisingWaveUser)
- `sigs.k8s.io/kustomize` - Manifest templating
- `github.com/rwcrr/ctrlkit` - State management library

**Required Tools**:
- Go 1.19+
- controller-gen >= 0.9
- kustomize >= v0.13
- golangci-lint >= 1.45
- goimports-reviser >= 2.5

## Troubleshooting

**NOTE**: Use relative paths instead of absolute paths

### Webhook Failures

```bash
# Check webhook configuration
kubectl get mutatingwebhookconfiguration,validatingwebhookconfiguration

# Check certificates
kubectl get secret -n risingwave-operator-system risingwave-operator-webhook-cert

# For Docker Desktop, ensure certs copied
make copy-local-certs
```

### Reconciliation Loops

Check logs for generation mismatch:
```bash
kubectl logs -f deployment/risingwave-operator-controller-manager | grep "generation"
```

### Image Pull Issues

For kind/minikube, preload images:
```bash
kind load docker-image ghcr.io/risingwavelabs/risingwave-operator:latest
minikube image load ghcr.io/risingwavelabs/risingwave-operator:latest
```

## Git Workflow

- **Main branch**: `main`
- **PR Requirements**: Tests pass, lint passes
- **Coverage**: Tracked via Codecov
- **License**: Apache 2.0 header required in all files
