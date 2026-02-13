# RisingWave User Management

The RisingWave Operator provides a Kubernetes-native way to manage database users for RisingWave clusters through the `RisingWaveUser` custom resource.

This guide covers:
- [Basic User Creation](#basic-user-creation)
- [Password Management](#password-management)
- [Privilege Grants](#privilege-grants)
- [Authentication Methods](#authentication-methods)
- [User Operations](#user-operations)
- [Retrieving Credentials](#retrieving-credentials)

## Basic User Creation

### Minimal User

The simplest way to create a user with auto-generated password:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: my-user
spec:
  risingWaveRef:
    name: risingwave-sample
```

This creates a user named `my-user` in RisingWave with:
- Auto-generated 16-character password
- Default database connection privileges
- Secret created at `risingwave-risingwave-sample-my-user`

### User with Custom Name

Use `spec.name` to set a different RisingWave username than the Kubernetes resource name:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: my-user-resource  # Kubernetes resource name
spec:
  risingWaveRef:
    name: risingwave-sample
  name: "my_db_user"  # Actual RisingWave username
```

### User with Database Permissions

Grant user-level permissions:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: developer-user
spec:
  risingWaveRef:
    name: risingwave-sample
  permissions:
    - CREATEDB    # Can create databases
    - CREATEUSER   # Can create other users
  # Note: SUPERUSER is implicitly granted all permissions
```

**Available User Permissions:**
- `SUPERUSER` - Full superuser privileges
- `CREATEDB` - Can create databases
- `CREATEUSER` - Can create other users
- `NOSUPERUSER` - Explicitly remove superuser
- `NOCREATEDB` - Remove database creation privilege
- `NOCREATEUSER` - Remove user creation privilege

## Password Management

### Auto-Generated Password

By default, a 16-character random password is generated. Customize length:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: secure-user
spec:
  risingWaveRef:
    name: risingwave-sample
  password:
    generateRandomLength: 32  # 8-128 characters allowed
```

### Password from Secret

Reference an existing secret:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: secret-user
spec:
  risingWaveRef:
    name: risingwave-sample
  password:
    secretRef:
      name: my-existing-password
      namespace: default  # Optional, defaults to RisingWaveUser namespace
      key: password         # Optional, defaults to "password"
```

**Secret Format:**
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: my-existing-password
type: Opaque
stringData:
  password: "my-secure-password"
```

### Password Rotation

Trigger password rotation by adding an annotation:

```bash
kubectl annotate risingwaveuser my-user "risingwave.risingwavelabs.com/rotate-password=true" --overwrite
```

The operator will:
1. Generate a new random password
2. Update the RisingWave user
3. Update the Kubernetes Secret
4. Remove the annotation

**Note:** The operator only honors this annotation when using auto-generated passwords. If you use `secretRef`, you must rotate the password manually in the secret.

## Privilege Grants

The `grants` section allows fine-grained access control on database objects using a **hierarchical structure**. Database and schema context is specified once at the parent level, with nested privileges inheriting that context.

### Structure Overview

```mermaid
graph TD
    A[privileges] --> B[databases]
    B --> C[schemas]
    C --> D[tables]
    C --> E[views]
    C --> F[materializedViews]
    C --> G[sources]
    C --> H[sinks]
    C --> I[connections]
    C --> J[secrets]
    C --> K[functions]
```

### Hierarchical Structure Example

```yaml
spec:
  grants:
    databases:
      - name: "dev"
        privileges: [CONNECT, CREATE]
        withGrantOption: true
        schemas:
          - name: "public"
            privileges: [USAGE, CREATE]
            tables:
              - name: "orders"
                privileges: [SELECT, INSERT, UPDATE]
            views:
              - name: "*"  # All views in this schema
                privileges: [SELECT]
```

### Supported Objects and Privilege Types

| Level | Object Type | Supported Privileges |
|-------|-------------|----------------------|
| Database | `databases` | `CONNECT`, `CREATE`, `ALL` |
| Schema | `schemas` | `USAGE`, `CREATE`, `ALL` |
| Table | `tables` | `SELECT`, `INSERT`, `UPDATE`, `DELETE`, `ALL` |
| View | `views` | `SELECT`, `ALL` |
| Materialized View | `materializedViews` | `SELECT`, `ALL` |
| Source | `sources` | `SELECT`, `ALL` |
| Sink | `sinks` | `SELECT`, `ALL` |
| Connection | `connections` | `USAGE`, `ALL` |
| Secret | `secrets` | `USAGE`, `ALL` |
| Function | `functions` | `EXECUTE`, `ALL` |

### Wildcard Support

Use `"*"` as the name to grant privileges on all objects of that type within a schema:

```yaml
grants:
  databases:
    - name: "dev"
      schemas:
        - name: "*"      # All schemas in 'dev'
          tables:
            - name: "*"  # All tables in all schemas
              privileges: [SELECT]
```

## Authentication Methods

### Password Authentication (Default)

If `auth` is not specified, password authentication is used:

```yaml
spec:
  # auth section omitted - defaults to password
  password:
    generateRandomLength: 16
```

### OAuth Authentication

For OAuth/JWT-based authentication:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: oauth-user
spec:
  risingWaveRef:
    name: risingwave-sample
  auth:
    type: oauth
    oauth:
      jwksUrl: "https://auth.example.com/.well-known/jwks.json"
      issuer: "risingwave"
      audience:
        - "risingwave"
        - "https://myapp.example.com"
```

**OAuth Configuration:**
- `jwksUrl` - JWKS (JSON Web Key Set) endpoint for token verification (required)
- `issuer` - Issuer claim to verify in JWT token (required)
- `audience` - Audience claims to verify (optional)

### LDAP Authentication

For LDAP-based authentication:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: ldap-user
spec:
  risingWaveRef:
    name: risingwave-sample
  auth:
    type: ldap
    ldap:
      host: "ldap.example.com"
      port: 389                      # Optional, default: 389
      baseDN: "dc=example,dc=com"
      useSSL: true                 # Optional
      insecureSkipVerify: false    # Optional
```

**LDAP Configuration:**
- `host` - LDAP server hostname or IP (required)
- `port` - LDAP server port (optional, default: 389)
- `baseDN` - Base DN for LDAP searches (required)
- `useSSL` - Use SSL/TLS for connections (optional)
- `insecureSkipVerify` - Skip TLS certificate verification (optional)

## User Operations

### Cross-Namespace Reference

Reference a RisingWave cluster in a different namespace:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: my-user
  namespace: app-namespace           # User namespace
spec:
  risingWaveRef:
    name: risingwave-sample        # Cluster name
    namespace: database-namespace   # Cluster namespace
```

### Deleting a User

```bash
kubectl delete risingwaveuser my-user
```

The operator will:
1. Execute `DROP USER` in RisingWave
2. Delete the Kubernetes Secret containing the password
3. Remove finalizers from the resource

## Retrieving Credentials

The operator stores user credentials in a Kubernetes Secret named:

```
risingwave-<risingwave-name>-<resource-name>
```

For example, a RisingWaveUser named `analytics-user` referencing a RisingWave cluster named `risingwave-sample` creates a secret at:

```
risingwave-risingwave-sample-analytics-user
```

### Get Secret

```bash
kubectl get secret risingwave-risingwave-sample-analytics-user -o yaml
```

### Extract Password

```bash
kubectl get secret risingwave-risingwave-sample-analytics-user -o jsonpath='{.data.password}' | base64 -d
```

### Use with psql

```bash
# Port forward to RisingWave
kubectl port-forward svc/risingwave-sample-frontend 4567:service

# Get password and connect
PASSWORD=$(kubectl get secret risingwave-risingwave-sample-analytics-user -o jsonpath='{.data.password}' | base64 -d)
psql -h localhost -p 4567 -d dev -U analytics-user -W <<< "$PASSWORD"
```

### Connection String Format

For applications, use the retrieved credentials to construct a connection string:

```
postgresql://analytics-user:PASSWORD@localhost:4567/dev
```

## Status and Conditions

Monitor the user status:

```bash
kubectl get risingwaveuser my-user -o yaml
```

**Status Fields:**
- `status.phase` - High-level lifecycle phase (Pending, Creating, Ready, Updating, Deleting, Failed)
- `status.userCreated` - Whether user exists in RisingWave
- `status.secretCreated` - Whether password secret exists
- `status.privilegesSynced` - Whether privileges have been applied
- `status.connectionStatus.connected` - Whether operator can connect to RisingWave
- `status.secretName` - Name of the password secret

**Conditions:**
- `Ready` - User is fully provisioned
- `UserCreated` - User created in RisingWave
- `SecretCreated` - Password secret created
- `PrivilegesSynced` - Privileges applied
- `ConnectionError` - Connection to RisingWave failed

## Common Patterns

### Read-Only Analytics User

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: analytics-readonly
spec:
  risingWaveRef:
    name: risingwave-sample
  permissions:
    - CREATEDB  # Can create analytics databases
  grants:
    databases:
      - name: analytics
        privileges: [CONNECT, CREATE]
    tables:
      - database: analytics
        schema: public
        name: "*"
        privileges: [SELECT]
    views:
      - database: analytics
        schema: public
        name: "*"
        privileges: [SELECT]
    materializedViews:
      - database: analytics
        schema: public
        name: "*"
        privileges: [SELECT]
```

### Data Ingestion User

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: ingestion-writer
spec:
  risingWaveRef:
    name: risingwave-sample
  grants:
    databases:
      - name: production
        privileges: [CONNECT, CREATE]
    schemas:
      - database: production
        name: ingestion
        privileges: [USAGE, CREATE]
    tables:
      - database: production
        schema: ingestion
        name: "*"
        privileges: [SELECT, INSERT, UPDATE, DELETE]
    sources:
      - database: production
        schema: ingestion
        name: "*"
        privileges: [SELECT]
    sinks:
      - database: production
        schema: ingestion
        name: "*"
        privileges: [SELECT]
```

### Delegated Admin User

User who can grant privileges to others:

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: delegated-admin
spec:
  risingWaveRef:
    name: risingwave-sample
  grants:
    databases:
      - name: dev
        privileges: ["ALL PRIVILEGES"]
        withGrantOption: true    # Can grant these privileges
```

## Troubleshooting

### User Not Created

Check the status and conditions:

```bash
kubectl describe risingwaveuser my-user
```

Look for:
- `ConnectionError` condition - Operator cannot connect to RisingWave
- Verify `risingWaveRef.name` points to a valid cluster
- Verify the RisingWave cluster is in `Running` status

### Permission Denied After Creation

The user may not have immediate access to granted objects. This is normal in some cases. Verify privileges:

```sql
-- Connect as superuser and check grants
\du
SELECT * FROM pg_roles WHERE rolname = 'my-user';
SELECT grantee, privilege_type FROM pg_admin.extern_grants WHERE grantee = 'my-user';
```

### Secret Not Found

If the secret doesn't exist:

```bash
kubectl get secrets | grep risingwave
```

Check the `status.secretName` field and ensure the secret is in the same namespace as the RisingWaveUser.

### Password Rotation Not Working

Ensure:
1. The user uses auto-generated password (not `secretRef`)
2. The annotation is exactly: `risingwave.risingwavelabs.com/rotate-password=true`
3. The annotation value is the string `"true"`

### Hierarchical Privilege Structure

The RisingWaveUser CRD now supports a **hierarchical privilege structure** where database and schema context is specified once at the parent level, with nested privileges inheriting that context.

#### Structure Overview

```
grants:
  databases:
    - name: "database_name"
      privileges: [CONNECT, CREATE]
      withGrantOption: true
      schemas:
        - name: "schema_name"
          privileges: [USAGE, CREATE]
          withGrantOption: false
          tables:
            - name: "table_name"  # or "*" for all tables
              privileges: [SELECT, INSERT, UPDATE]
          views:
            - name: "view_name"  # or "*" for all views
              privileges: [SELECT]
          materializedViews:
            - name: "mv_name"  # or "*" for all materialized views
              privileges: [SELECT]
          sources:
            - name: "source_name"  # or "*" for all sources
              privileges: [SELECT]
          sinks:
            - name: "sink_name"  # or "*" for all sinks
              privileges: [SELECT]
          connections:
            - name: "connection_name"  # or "*" for all connections
              privileges: [USAGE]
          secrets:
            - name: "secret_name"  # or "*" for all secrets
              privileges: [USAGE]
          functions:
            - name: "function_name"  # or "*" for all functions
              privileges: [EXECUTE]
```

#### Key Benefits of Hierarchical Structure

1. **DRY - Don't Repeat Yourself**: Database and schema names are specified once at the parent level
2. **Better Organization**: Logical grouping of related privileges under their parent objects
3. **Easier Maintenance**: Changes to a schema's objects only require updating the schema definition
4. **Wildcard Support**: Use "*" for all objects of a type (tables, views, etc.) within a schema

#### Example

```yaml
apiVersion: risingwave.risingwavelabs.com/v1alpha1
kind: RisingWaveUser
metadata:
  name: example-user
spec:
  risingWaveRef:
    name: risingwave-sample
  grants:
    databases:
      - name: dev
        privileges: [CONNECT, CREATE]
        schemas:
          - name: public
            privileges: [USAGE, CREATE]
            tables:
              - name: "events"
                privileges: [SELECT, INSERT, UPDATE, DELETE]
              - name: "metrics"
                privileges: [SELECT]
            materializedViews:
              - name: "daily_summary"
                privileges: [SELECT]
```

#### Migration from Flat Structure

If you have existing manifests using the old flat structure:

**Old (Flat):**
```yaml
privileges:
  schemas:
    - database: dev
      name: public
      privileges: [USAGE]
  tables:
        - database: dev
          schema: public
          name: orders
          privileges: [SELECT]
```

**New (Hierarchical):**
```yaml
grants:
  databases:
    - name: dev
      schemas:
        - name: public
            tables:
              - name: orders
                privileges: [SELECT]
```

**Note:** The old flat structure (`schemas:`, `tables:`, `views: etc. directly under `privileges`) is **deprecated**. Only the hierarchical structure under `databases -> schemas` should be used for new RisingWaveUser resources.

---



For complete API reference, see [API Documentation](api.md).
