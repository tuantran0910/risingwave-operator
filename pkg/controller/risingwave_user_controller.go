/*
 * Copyright 2023 RisingWave Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package controller

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	risingwavev1alpha1 "github.com/risingwavelabs/risingwave-operator/apis/risingwave/v1alpha1"
	"github.com/risingwavelabs/risingwave-operator/pkg/rwclient"
	"github.com/risingwavelabs/risingwave-operator/pkg/utils"
)

const (
	// RisingWaveUserFinalizer is the finalizer for RisingWaveUser.
	risingWaveUserFinalizer = "risingwaveuser.risingwave.risingwavelabs.com/finalizer"

	// passwordRotateAnnotation is the annotation to trigger password rotation.
	passwordRotateAnnotation = "risingwave.risingwavelabs.com/rotate-password"

	// pauseReconcileAnnotation pauses reconciliation.
	pauseReconcileAnnotation = "risingwave.risingwavelabs.com/pause-reconcile"

	// Default connection settings.
	defaultFrontendPort = int32(4567)
	defaultDatabase     = "dev"
	defaultUsername     = "root"
	defaultPassword     = "root"
)

// +kubebuilder:rbac:groups=risingwave.risingwavelabs.com,resources=risingwaveusers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=risingwave.risingwavelabs.com,resources=risingwaveusers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=risingwave.risingwavelabs.com,resources=risingwaveusers/finalizers,verbs=update
// +kubebuilder:rbac:groups=risingwave.risingwavelabs.com,resources=risingwaves,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;delete
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch

// RisingWaveUserController reconciles RisingWaveUser objects.
type RisingWaveUserController struct {
	client.Client
	Recorder       events.EventRecorder
	ConnectionPool *rwclient.Pool
}

// Database is an interface for database operations.
type Database interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) Row
	QueryContext(ctx context.Context, query string, args ...any) (rwclient.Rows, error)
}

// Row is an interface for sql.Row.
type Row interface {
	Scan(dest ...any) error
}

// sqlDB is a wrapper around sql.DB that implements the Database interface.
type sqlDB struct {
	*sql.DB
}

func (s *sqlDB) QueryContext(ctx context.Context, query string, args ...any) (rwclient.Rows, error) {
	return s.DB.QueryContext(ctx, query, args...)
}

func (db *sqlDB) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return db.DB.QueryRowContext(ctx, query, args...)
}

// RisingWaveUserReconciler holds the state for a single reconciliation.
type RisingWaveUserReconciler struct {
	*RisingWaveUserController
	logger        logr.Logger
	rwUser        *risingwavev1alpha1.RisingWaveUser
	risingWave    *risingwavev1alpha1.RisingWave
	conn          Database
	connectionKey rwclient.ConnectionKey
}

// Reconcile reconciles a RisingWaveUser.
func (c *RisingWaveUserController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("risingwaveuser", req.NamespacedName)

	// Fetch RisingWaveUser
	rwUser := &risingwavev1alpha1.RisingWaveUser{}
	err := c.Get(ctx, req.NamespacedName, rwUser)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.V(1).Info("RisingWaveUser not found, skipping")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get RisingWaveUser")
		return ctrl.Result{}, err
	}

	// Create reconciler
	reconciler := &RisingWaveUserReconciler{
		RisingWaveUserController: c,
		logger:                   logger,
		rwUser:                   rwUser,
	}

	// Check for pause annotation
	if rwUser.Annotations != nil {
		if _, ok := rwUser.Annotations[pauseReconcileAnnotation]; ok {
			logger.Info("Reconciliation paused via annotation")
			return ctrl.Result{}, nil
		}
	}

	// Handle deletion
	if !rwUser.DeletionTimestamp.IsZero() {
		return reconciler.reconcileDelete(ctx)
	}

	// Handle normal reconciliation
	return reconciler.reconcileNormal(ctx)
}

// reconcileNormal handles normal reconciliation.
func (r *RisingWaveUserReconciler) reconcileNormal(ctx context.Context) (ctrl.Result, error) {
	// Add finalizer if not present
	hasFinalizer := false
	for _, f := range r.rwUser.Finalizers {
		if f == risingWaveUserFinalizer {
			hasFinalizer = true
			break
		}
	}
	if !hasFinalizer {
		r.rwUser.Finalizers = append(r.rwUser.Finalizers, risingWaveUserFinalizer)
		if err := r.Update(ctx, r.rwUser); err != nil {
			r.logger.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Fetch referenced RisingWave cluster
	if err := r.fetchRisingWave(ctx); err != nil {
		r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
		r.setErrorCondition("FailedToFetchRisingWave", err.Error())
		if updateErr := r.updateStatus(ctx); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// Check if RisingWave is ready
	if !r.isRisingWaveReady() {
		r.setPhase(risingwavev1alpha1.RisingWaveUserPhasePending)
		r.setCondition(metav1.Condition{
			Type:    string(risingwavev1alpha1.RisingWaveUserConditionReady),
			Status:  metav1.ConditionFalse,
			Reason:  "RisingWaveNotReady",
			Message: "RisingWave cluster is not ready yet",
		})
		if err := r.updateStatus(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	// Establish database connection
	if err := r.establishConnection(ctx); err != nil {
		r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
		r.setErrorCondition("ConnectionFailed", err.Error())
		if updateErr := r.updateStatus(ctx); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}
	defer r.closeConnection()

	// Handle password rotation
	var rotatePassword bool
	if r.rwUser.Annotations != nil {
		// Check if rotation annotation is set
		if r.rwUser.Annotations[passwordRotateAnnotation] == "true" {
			rotatePassword = true
		}
	}
	if rotatePassword {
		if err := r.rotatePassword(ctx); err != nil {
			r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
			r.setErrorCondition("PasswordRotationFailed", err.Error())
			if updateErr := r.updateStatus(ctx); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}
		// Remove annotation after successful rotation
		delete(r.rwUser.Annotations, passwordRotateAnnotation)
		if err := r.Update(ctx, r.rwUser); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Get or generate password
	password, err := r.getPassword(ctx)
	if err != nil {
		r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
		r.setErrorCondition("PasswordError", err.Error())
		if updateErr := r.updateStatus(ctx); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// 1. Ensure user exists
	if err := r.ensureUserExists(ctx, password); err != nil {
		r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
		r.setErrorCondition("UserCreationFailed", err.Error())
		if updateErr := r.updateStatus(ctx); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// 2. Sync user-level permissions (Attributes)
	if err := r.reconcilePermissions(ctx); err != nil {
		r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
		r.setErrorCondition("UserPermissionSyncFailed", err.Error())
		if updateErr := r.updateStatus(ctx); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}

	// 3. Sync object-level privileges (Grants)
	r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseUpdating)
	if err := r.applyPrivileges(ctx); err != nil {
		r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
		r.setErrorCondition("PrivilegeSyncFailed", err.Error())
		if updateErr := r.updateStatus(ctx); updateErr != nil {
			return ctrl.Result{}, updateErr
		}
		return ctrl.Result{}, err
	}
	r.rwUser.Status.PrivilegesSynced = true
	r.setCondition(metav1.Condition{
		Type:               string(risingwavev1alpha1.RisingWaveUserConditionPrivilegesSynced),
		Status:             metav1.ConditionTrue,
		Reason:             "PrivilegesSynced",
		Message:            "Privileges synced successfully",
		LastTransitionTime: metav1.Now(),
	})

	// Create or update Secret with password
	authType := r.getAuthType()
	if authType == risingwavev1alpha1.AuthTypePassword || r.rwUser.Spec.Auth == nil {
		if err := r.createOrUpdateSecret(ctx, password); err != nil {
			r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseFailed)
			r.setErrorCondition("SecretCreationFailed", err.Error())
			if updateErr := r.updateStatus(ctx); updateErr != nil {
				return ctrl.Result{}, updateErr
			}
			return ctrl.Result{}, err
		}
	}

	// Update status
	r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseReady)
	r.setCondition(metav1.Condition{
		Type:    string(risingwavev1alpha1.RisingWaveUserConditionReady),
		Status:  metav1.ConditionTrue,
		Reason:  "Ready",
		Message: "RisingWaveUser is ready",
	})
	r.rwUser.Status.ObservedGeneration = r.rwUser.Generation
	r.updateConnectionStatus(true, "")

	if err := r.updateStatus(ctx); err != nil {
		return ctrl.Result{}, err
	}

	r.logger.Info("RisingWaveUser reconciled successfully")
	return ctrl.Result{}, nil
}

// reconcileDelete handles deletion of RisingWaveUser.
func (r *RisingWaveUserReconciler) reconcileDelete(ctx context.Context) (ctrl.Result, error) {
	r.logger.Info("Deleting RisingWaveUser")
	r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseDeleting)

	// If user was created, drop it from RisingWave
	if r.rwUser.Status.UserCreated {
		if err := r.fetchRisingWave(ctx); err != nil {
			r.logger.Error(err, "Failed to fetch RisingWave during deletion")
			return ctrl.Result{}, err
		}

		if r.isRisingWaveReady() {
			if err := r.establishConnection(ctx); err != nil {
				r.logger.Error(err, "Failed to establish connection during deletion")
				// Continue anyway to remove finalizer
			} else {
				defer r.closeConnection()

				dropSQL := rwclient.BuildDropUserSQL(r.getUserName())
				if _, err := r.conn.ExecContext(ctx, dropSQL); err != nil {
					r.logger.Error(err, "Failed to drop user", "sql", dropSQL)
					return ctrl.Result{}, err
				}
				r.logger.Info("User dropped from RisingWave")
			}
		}
	}

	// Remove finalizer
	var finalizers []string
	for _, f := range r.rwUser.Finalizers {
		if f != risingWaveUserFinalizer {
			finalizers = append(finalizers, f)
		}
	}
	r.rwUser.Finalizers = finalizers
	if err := r.Update(ctx, r.rwUser); err != nil {
		r.logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.logger.Info("Finalizer removed, deletion complete")
	return ctrl.Result{}, nil
}

// fetchRisingWave fetches the referenced RisingWave cluster.
func (r *RisingWaveUserReconciler) fetchRisingWave(ctx context.Context) error {
	namespace := r.rwUser.Spec.RisingWaveRef.Namespace
	if namespace == "" {
		namespace = r.rwUser.Namespace
	}

	r.risingWave = &risingwavev1alpha1.RisingWave{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      r.rwUser.Spec.RisingWaveRef.Name,
	}, r.risingWave)

	return err
}

// isRisingWaveReady checks if the RisingWave cluster is ready.
func (r *RisingWaveUserReconciler) isRisingWaveReady() bool {
	if r.risingWave == nil {
		return false
	}

	for _, cond := range r.risingWave.Status.Conditions {
		if string(cond.Type) == string(risingwavev1alpha1.RisingWaveConditionRunning) &&
			cond.Status == metav1.ConditionTrue {
			return true
		}
	}

	return false
}

// establishConnection establishes a database connection to RisingWave.
func (r *RisingWaveUserReconciler) establishConnection(ctx context.Context) error {
	r.connectionKey = rwclient.ConnectionKeyFrom(
		r.risingWave.Namespace,
		r.risingWave.Name,
		r.risingWave.UID,
	)

	// Get frontend service
	svc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: r.risingWave.Namespace,
		Name:      r.risingWave.Name + "-frontend",
	}, svc)
	if err != nil {
		return fmt.Errorf("failed to get frontend service: %w", err)
	}

	// Build connection info
	host := fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace)
	connInfo := rwclient.DefaultConnectionInfo(host, defaultFrontendPort, defaultUsername, defaultPassword)

	// Get connection from pool
	db, err := r.ConnectionPool.Get(ctx, r.connectionKey, connInfo)
	if err != nil {
		return fmt.Errorf("failed to get database connection: %w", err)
	}

	r.conn = &sqlDB{db}
	r.updateConnectionStatus(true, "")

	return nil
}

// closeConnection clears the current connection reference (but keeps it in the pool).
func (r *RisingWaveUserReconciler) closeConnection() {
	r.conn = nil
}

// getUserName returns the actual user name.
func (r *RisingWaveUserReconciler) getUserName() string {
	if r.rwUser.Spec.Name != "" {
		return r.rwUser.Spec.Name
	}
	return r.rwUser.Name
}

// getAuthType returns the authentication type.
func (r *RisingWaveUserReconciler) getAuthType() risingwavev1alpha1.AuthType {
	if r.rwUser.Spec.Auth != nil && r.rwUser.Spec.Auth.Type != nil {
		return *r.rwUser.Spec.Auth.Type
	}
	return risingwavev1alpha1.AuthTypePassword
}

// getPassword gets or generates a password for the user.
func (r *RisingWaveUserReconciler) getPassword(ctx context.Context) (string, error) {
	// Check if password is specified via SecretRef
	if r.rwUser.Spec.Password != nil && r.rwUser.Spec.Password.SecretRef != nil {
		return r.getPasswordFromSecret(ctx)
	}

	// Check if secret already exists with password
	secretName := r.getSecretName()
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: r.rwUser.Namespace,
		Name:      secretName,
	}, secret)

	if err == nil {
		// Secret exists, return existing password
		if password, ok := secret.Data["password"]; ok {
			return string(password), nil
		}
	}

	// Generate new password
	length := int32(16)
	if r.rwUser.Spec.Password != nil && r.rwUser.Spec.Password.GenerateRandomLength != nil {
		length = *r.rwUser.Spec.Password.GenerateRandomLength
	}

	password := utils.GenerateRandomPassword(int(length))
	return password, nil
}

// getPasswordFromSecret retrieves password from a referenced Secret.
func (r *RisingWaveUserReconciler) getPasswordFromSecret(ctx context.Context) (string, error) {
	ref := r.rwUser.Spec.Password.SecretRef
	namespace := ref.Namespace
	if namespace == "" {
		namespace = r.rwUser.Namespace
	}

	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: namespace,
		Name:      ref.Name,
	}, secret)
	if err != nil {
		return "", fmt.Errorf("failed to get password secret: %w", err)
	}

	key := ref.Key
	if key == "" {
		key = "password"
	}

	password, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s", key, ref.Name)
	}

	return string(password), nil
}

// getSecretName returns the name of the secret for this user.
func (r *RisingWaveUserReconciler) getSecretName() string {
	return "risingwave-" + r.risingWave.Name + "-" + r.rwUser.Name
}

// createOrUpdateSecret creates or updates the secret containing the password.
func (r *RisingWaveUserReconciler) createOrUpdateSecret(ctx context.Context, password string) error {
	secretName := r.getSecretName()
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: r.rwUser.Namespace,
		},
		Data: map[string][]byte{
			"username": []byte(r.getUserName()),
			"password": []byte(password),
		},
	}

	// Set owner reference
	if err := ctrl.SetControllerReference(r.rwUser, secret, r.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Create or update
	found := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: r.rwUser.Namespace,
		Name:      secretName,
	}, found)

	if err != nil && apierrors.IsNotFound(err) {
		// Create new secret
		if err := r.Create(ctx, secret); err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}
		r.logger.Info("Secret created", "name", secretName)
	} else if err == nil {
		// Update existing secret
		found.Data = secret.Data
		if err := r.Update(ctx, found); err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}
		r.logger.Info("Secret updated", "name", secretName)
	} else {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	r.rwUser.Status.SecretCreated = true
	r.rwUser.Status.SecretName = secretName
	r.setCondition(metav1.Condition{
		Type:    string(risingwavev1alpha1.RisingWaveUserConditionSecretCreated),
		Status:  metav1.ConditionTrue,
		Reason:  "SecretCreated",
		Message: "Secret created successfully",
	})

	return nil
}

// applyPrivileges applies privileges to the user.
func (r *RisingWaveUserReconciler) applyPrivileges(ctx context.Context) error {
	userName := r.getUserName()

	// 1. Snapshot actual state
	actual, err := rwclient.FetchUserPrivilegeSnapshot(ctx, r.conn, userName)
	if err != nil {
		return fmt.Errorf("failed to fetch actual privilege snapshot: %w", err)
	}

	// 2. Identify all databases in the system to ensure full reconciliation
	allSystemDBs, err := r.getAllDatabases(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch all databases: %w", err)
	}

	relevantDatabases := make(map[string]bool)
	for _, dbName := range allSystemDBs {
		relevantDatabases[dbName] = true
	}
	// Also ensure databases in spec are included (though they should be in allSystemDBs if they exist)
	if r.rwUser.Spec.Privileges != nil {
		for _, dbPriv := range r.rwUser.Spec.Privileges.Databases {
			relevantDatabases[dbPriv.Name] = true
		}
	}

	// 3. Collect statements per database
	currentDB, _ := r.getCurrentDatabase(ctx)
	originalDB := currentDB

	type dbStatements struct {
		toGrant  []string
		toRevoke []string
	}
	statementsPerDB := make(map[string]*dbStatements)
	getDBStats := func(dbName string) *dbStatements {
		if _, ok := statementsPerDB[dbName]; !ok {
			statementsPerDB[dbName] = &dbStatements{}
		}
		return statementsPerDB[dbName]
	}

	// First, reconcile database-level privileges (global connection is fine for this)
	for dbName := range relevantDatabases {
		var actualDB rwclient.DatabasePrivilegeSnapshot
		for _, adb := range actual.Databases {
			if adb.Name == dbName {
				actualDB = adb
				break
			}
		}

		var desiredDB *risingwavev1alpha1.DatabasePrivilege
		if r.rwUser.Spec.Privileges != nil {
			for i := range r.rwUser.Spec.Privileges.Databases {
				if r.rwUser.Spec.Privileges.Databases[i].Name == dbName {
					desiredDB = &r.rwUser.Spec.Privileges.Databases[i]
					break
				}
			}
		}

		if desiredDB == nil {
			if len(actualDB.Privileges) > 0 {
				getDBStats(originalDB).toRevoke = append(getDBStats(originalDB).toRevoke,
					fmt.Sprintf("REVOKE ALL ON DATABASE %s FROM %s",
						rwclient.QuoteIdentifier(dbName), rwclient.QuoteUser(userName)))
			}
			continue
		}

		diff := rwclient.CalculateDatabaseDiff(userName, actualDB, desiredDB)
		getDBStats(originalDB).toGrant = append(getDBStats(originalDB).toGrant, diff.ToGrant...)
		getDBStats(originalDB).toRevoke = append(getDBStats(originalDB).toRevoke, diff.ToRevoke...)
	}

	// 4. Reconcile Schema and Object privileges (requires switching databases)
	currentDB = originalDB
	for dbName := range relevantDatabases {
		if currentDB != dbName {
			if err := r.switchDatabase(ctx, dbName); err != nil {
				r.logger.V(1).Info("Skipping schema reconciliation for database", "database", dbName, "error", err)
				continue
			}
			currentDB = dbName
		}

		actualSchemas, err := rwclient.FetchSchemaPrivileges(ctx, r.conn, userName)
		if err != nil {
			return fmt.Errorf("failed to fetch schema privileges for database %q: %w", dbName, err)
		}

		var desiredSchemas []risingwavev1alpha1.NestedSchemaPrivilege
		if r.rwUser.Spec.Privileges != nil {
			for _, dbPriv := range r.rwUser.Spec.Privileges.Databases {
				if dbPriv.Name == dbName {
					desiredSchemas = dbPriv.Schemas
					break
				}
			}
		}

		allSchemas := make(map[string]bool)
		for _, s := range actualSchemas {
			allSchemas[s.Name] = true
		}
		for _, s := range desiredSchemas {
			allSchemas[s.Name] = true
		}

		for sName := range allSchemas {
			var actualS rwclient.SchemaPrivilegeSnapshot
			for _, asc := range actualSchemas {
				if asc.Name == sName {
					actualS = asc
					break
				}
			}

			var desiredS *risingwavev1alpha1.NestedSchemaPrivilege
			for i := range desiredSchemas {
				if desiredSchemas[i].Name == sName {
					desiredS = &desiredSchemas[i]
					break
				}
			}

			dbStats := getDBStats(dbName)
			if desiredS == nil {
				if len(actualS.Privileges) > 0 {
					dbStats.toRevoke = append(dbStats.toRevoke, fmt.Sprintf("REVOKE ALL ON SCHEMA %s FROM %s",
						rwclient.QuoteIdentifier(sName), rwclient.QuoteUser(userName)))
				}
				r.reconcileObjectPrivileges(userName, "TABLE", actualS.Tables, nil, &dbStats.toGrant, &dbStats.toRevoke)
				r.reconcileObjectPrivileges(userName, "VIEW", actualS.Views, nil, &dbStats.toGrant, &dbStats.toRevoke)
				r.reconcileObjectPrivileges(userName, "MATERIALIZED VIEW", actualS.MaterializedViews, nil, &dbStats.toGrant, &dbStats.toRevoke)
				r.reconcileObjectPrivileges(userName, "SOURCE", actualS.Sources, nil, &dbStats.toGrant, &dbStats.toRevoke)
				continue
			}

			diff := rwclient.CalculateSchemaDiff(userName, actualS, desiredS)
			dbStats.toGrant = append(dbStats.toGrant, diff.ToGrant...)
			dbStats.toRevoke = append(dbStats.toRevoke, diff.ToRevoke...)

			r.reconcileObjectPrivileges(userName, "TABLE", actualS.Tables, desiredS.Tables, &dbStats.toGrant, &dbStats.toRevoke)
			r.reconcileObjectPrivileges(userName, "VIEW", actualS.Views, r.toViewPrivs(desiredS.Views), &dbStats.toGrant, &dbStats.toRevoke)
			r.reconcileObjectPrivileges(userName, "MATERIALIZED VIEW", actualS.MaterializedViews, r.toMVPrivs(desiredS.MaterializedViews), &dbStats.toGrant, &dbStats.toRevoke)
			r.reconcileObjectPrivileges(userName, "SOURCE", actualS.Sources, r.toSourcePrivs(desiredS.Sources), &dbStats.toGrant, &dbStats.toRevoke)
		}
	}

	// 5. Execute statements (Switching DB as needed)
	var errs []string
	for dbName, stats := range statementsPerDB {
		if len(stats.toGrant) == 0 && len(stats.toRevoke) == 0 {
			continue
		}

		if currentDB != dbName {
			if err := r.switchDatabase(ctx, dbName); err != nil {
				errs = append(errs, fmt.Sprintf("failed to switch to database %q for execution: %v", dbName, err))
				continue
			}
			currentDB = dbName
		}

		r.logger.Info("Executing statements for database", "database", dbName, "toRevoke", len(stats.toRevoke), "toGrant", len(stats.toGrant))

		// REVOKE first
		for _, stmt := range stats.toRevoke {
			if _, err := r.conn.ExecContext(ctx, stmt); err != nil {
				if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "does not exist") {
					r.logger.V(1).Info("Ignore revoke error", "database", dbName, "sql", stmt, "error", err)
					continue
				}
				errs = append(errs, fmt.Sprintf("[%s] failed to execute REVOKE: %s: %v", dbName, stmt, err))
			} else {
				r.logger.Info("Executed REVOKE successfully", "database", dbName, "user", userName, "sql", stmt)
			}
		}

		// GRANT second
		for _, stmt := range stats.toGrant {
			if _, err := r.conn.ExecContext(ctx, stmt); err != nil {
				errs = append(errs, fmt.Sprintf("[%s] failed to execute GRANT: %s: %v", dbName, stmt, err))
			} else {
				r.logger.Info("Executed GRANT successfully", "database", dbName, "user", userName, "sql", stmt)
			}
		}
	}

	// Switch back to original database
	if originalDB != currentDB {
		_ = r.switchDatabase(ctx, originalDB)
	}

	if len(errs) > 0 {
		return fmt.Errorf("reconciliation encountered errors: %s", strings.Join(errs, "; "))
	}

	return nil
}

func (r *RisingWaveUserReconciler) getCurrentDatabase(ctx context.Context) (string, error) {
	var currentDB string
	row := r.conn.QueryRowContext(ctx, "SELECT current_database()")
	if err := row.Scan(&currentDB); err != nil {
		return "dev", err
	}
	return currentDB, nil
}

func (r *RisingWaveUserReconciler) switchDatabase(ctx context.Context, dbName string) error {
	setDBSQL := fmt.Sprintf("SET DATABASE TO %s", rwclient.QuoteIdentifier(dbName))
	_, err := r.conn.ExecContext(ctx, setDBSQL)
	return err
}

func (r *RisingWaveUserReconciler) getAllDatabases(ctx context.Context) ([]string, error) {
	rows, err := r.conn.QueryContext(ctx, "SELECT name FROM rw_catalog.rw_databases")
	if err != nil {
		return nil, err
	}
	defer rows.Close() // nolint:errcheck

	var results []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		results = append(results, name)
	}
	return results, nil
}

func (r *RisingWaveUserReconciler) reconcileObjectPrivileges(userName, objectType string, actual []rwclient.ObjectPrivilege, desired []risingwavev1alpha1.NestedTablePrivilege, toGrant, toRevoke *[]string) {
	// Map all objects
	allObjects := make(map[string]bool)
	for _, o := range actual {
		allObjects[o.Name] = true
	}
	for _, o := range desired {
		allObjects[o.Name] = true
	}

	for oName := range allObjects {
		var actualO rwclient.ObjectPrivilege
		for _, ao := range actual {
			if ao.Name == oName {
				actualO = ao
				break
			}
		}

		var desiredO *risingwavev1alpha1.NestedTablePrivilege
		for i := range desired {
			if desired[i].Name == oName {
				desiredO = &desired[i]
				break
			}
		}

		if desiredO == nil {
			*toRevoke = append(*toRevoke, fmt.Sprintf("REVOKE ALL ON %s %s FROM %s",
				objectType, rwclient.QuoteIdentifier(oName), rwclient.QuoteUser(userName)))
			continue
		}

		var dPrivs []string
		for _, p := range desiredO.Privileges {
			dPrivs = append(dPrivs, string(p))
		}

		diff := rwclient.CalculateObjectDiff(userName, objectType, actualO, oName, dPrivs)
		*toGrant = append(*toGrant, diff.ToGrant...)
		*toRevoke = append(*toRevoke, diff.ToRevoke...)
	}
}

func (r *RisingWaveUserReconciler) toViewPrivs(views []risingwavev1alpha1.NestedViewPrivilege) []risingwavev1alpha1.NestedTablePrivilege {
	res := make([]risingwavev1alpha1.NestedTablePrivilege, len(views))
	for i, v := range views {
		res[i] = risingwavev1alpha1.NestedTablePrivilege{
			Name:            v.Name,
			WithGrantOption: v.WithGrantOption,
		}
		for _, p := range v.Privileges {
			res[i].Privileges = append(res[i].Privileges, risingwavev1alpha1.TablePrivilegeType(p))
		}
	}
	return res
}

func (r *RisingWaveUserReconciler) toMVPrivs(mvs []risingwavev1alpha1.NestedMaterializedViewPrivilege) []risingwavev1alpha1.NestedTablePrivilege {
	res := make([]risingwavev1alpha1.NestedTablePrivilege, len(mvs))
	for i, v := range mvs {
		res[i] = risingwavev1alpha1.NestedTablePrivilege{
			Name:            v.Name,
			WithGrantOption: v.WithGrantOption,
		}
		for _, p := range v.Privileges {
			res[i].Privileges = append(res[i].Privileges, risingwavev1alpha1.TablePrivilegeType(p))
		}
	}
	return res
}

func (r *RisingWaveUserReconciler) toSourcePrivs(sources []risingwavev1alpha1.NestedSourcePrivilege) []risingwavev1alpha1.NestedTablePrivilege {
	res := make([]risingwavev1alpha1.NestedTablePrivilege, len(sources))
	for i, s := range sources {
		res[i] = risingwavev1alpha1.NestedTablePrivilege{
			Name:            s.Name,
			WithGrantOption: s.WithGrantOption,
		}
		for _, p := range s.Privileges {
			res[i].Privileges = append(res[i].Privileges, risingwavev1alpha1.TablePrivilegeType(p))
		}
	}
	return res
}

// ensureUserExists ensures the user exists in RisingWave.
func (r *RisingWaveUserReconciler) ensureUserExists(ctx context.Context, password string) error {
	userName := r.getUserName()
	authType := r.getAuthType()

	var createSQL string
	switch authType {
	case risingwavev1alpha1.AuthTypeOAuth:
		createSQL = rwclient.BuildCreateUserWithOAuthSQL(userName, r.rwUser.Spec.Auth.OAuth)
	case risingwavev1alpha1.AuthTypeLDAP:
		createSQL = rwclient.BuildCreateUserWithLDAPSQL(userName, r.rwUser.Spec.Auth.LDAP)
	default:
		createSQL = rwclient.BuildCreateUserSQL(r.rwUser, password)
	}

	r.setPhase(risingwavev1alpha1.RisingWaveUserPhaseCreating)
	_, err := r.conn.ExecContext(ctx, createSQL)
	if err != nil {
		if isUserAlreadyExistsError(err) {
			r.logger.Info("User already exists in RisingWave", "user", userName)
			r.setCondition(metav1.Condition{
				Type:    string(risingwavev1alpha1.RisingWaveUserConditionUserCreated),
				Status:  metav1.ConditionTrue,
				Reason:  "UserAlreadyExists",
				Message: "User already exists in RisingWave",
			})
		} else {
			r.logger.Error(err, "Failed to create user in RisingWave", "user", userName, "sql", createSQL)
			return err
		}
	} else {
		r.logger.Info("User created successfully", "user", userName)
		r.setCondition(metav1.Condition{
			Type:    string(risingwavev1alpha1.RisingWaveUserConditionUserCreated),
			Status:  metav1.ConditionTrue,
			Reason:  "UserCreated",
			Message: "User created successfully",
		})
	}

	r.rwUser.Status.UserCreated = true
	return nil
}

// reconcilePermissions ensures the user permissions match the spec.
func (r *RisingWaveUserReconciler) reconcilePermissions(ctx context.Context) error {
	userName := r.getUserName()
	alterPermsSQL := rwclient.BuildAlterUserPermissionsSQL(userName, r.rwUser.Spec.Permissions)
	if alterPermsSQL == "" {
		return nil
	}

	r.logger.Info("Syncing user permissions", "user", userName, "sql", alterPermsSQL)
	if _, err := r.conn.ExecContext(ctx, alterPermsSQL); err != nil {
		r.logger.Error(err, "Failed to sync user permissions", "user", userName, "sql", alterPermsSQL)
		return err
	}

	return nil
}

// rotatePassword rotates the user's password.
func (r *RisingWaveUserReconciler) rotatePassword(ctx context.Context) error {
	newPassword := utils.GenerateRandomPassword(16)
	userName := r.getUserName()

	alterSQL := rwclient.BuildAlterUserPasswordSQL(userName, newPassword)
	if _, err := r.conn.ExecContext(ctx, alterSQL); err != nil {
		return fmt.Errorf("failed to rotate password: %w", err)
	}

	// Update secret
	return r.createOrUpdateSecret(ctx, newPassword)
}

// setPhase sets the phase of the RisingWaveUser.
func (r *RisingWaveUserReconciler) setPhase(phase string) {
	r.rwUser.Status.Phase = phase
}

// setCondition sets a condition on the RisingWaveUser status.
func (r *RisingWaveUserReconciler) setCondition(cond metav1.Condition) {
	if r.rwUser.Status.Conditions == nil {
		r.rwUser.Status.Conditions = []metav1.Condition{}
	}

	// Find existing condition
	found := false
	for i, c := range r.rwUser.Status.Conditions {
		if c.Type == cond.Type {
			// Update existing condition
			if c.Status != cond.Status || c.Reason != cond.Reason || c.Message != cond.Message {
				cond.LastTransitionTime = metav1.Now()
			} else {
				cond.LastTransitionTime = c.LastTransitionTime
			}
			r.rwUser.Status.Conditions[i] = cond
			found = true
			break
		}
	}

	// Add new condition
	if !found {
		cond.LastTransitionTime = metav1.Now()
		r.rwUser.Status.Conditions = append(r.rwUser.Status.Conditions, cond)
	}
}

// setErrorCondition sets an error condition.
func (r *RisingWaveUserReconciler) setErrorCondition(reason, message string) {
	r.setCondition(metav1.Condition{
		Type:    string(risingwavev1alpha1.RisingWaveUserConditionConnectionError),
		Status:  metav1.ConditionTrue,
		Reason:  reason,
		Message: message,
	})
}

// updateConnectionStatus updates the connection status.
func (r *RisingWaveUserReconciler) updateConnectionStatus(connected bool, errMsg string) {
	r.rwUser.Status.ConnectionStatus = &risingwavev1alpha1.ConnectionStatus{
		Connected: connected,
	}

	if connected {
		now := metav1.Now()
		r.rwUser.Status.ConnectionStatus.LastConnectedTime = &now
	}

	if errMsg != "" {
		r.rwUser.Status.ConnectionStatus.ErrorMessage = errMsg
	}
}

// updateStatus updates the status subresource.
func (r *RisingWaveUserReconciler) updateStatus(ctx context.Context) error {
	return r.Status().Update(ctx, r.rwUser)
}

// SetupWithManager sets up the controller with the ControllerManager.
func (c *RisingWaveUserController) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize connection pool
	c.ConnectionPool = rwclient.NewPool(rwclient.DefaultPoolConfig())

	return ctrl.NewControllerManagedBy(mgr).
		For(&risingwavev1alpha1.RisingWaveUser{}).
		Owns(&corev1.Secret{}).
		Watches(
			&risingwavev1alpha1.RisingWave{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				var userList risingwavev1alpha1.RisingWaveUserList
				if err := c.List(ctx, &userList); err != nil {
					return nil
				}
				var reqs []reconcile.Request
				for _, user := range userList.Items {
					if user.Spec.RisingWaveRef.Name != "" {
						reqs = append(reqs, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      user.Name,
								Namespace: user.Namespace,
							},
						})
					}
				}
				return reqs
			}),
		).
		Complete(c)
}

// NewRisingWaveUserController creates a new RisingWaveUserController.
func NewRisingWaveUserController(cli client.Client, recorder events.EventRecorder) *RisingWaveUserController {
	return &RisingWaveUserController{
		Client:   cli,
		Recorder: recorder,
	}
}

// isUserAlreadyExistsError checks if error indicates a user already exists.
// RisingWave returns a specific error format when attempting to create a duplicate user.
func isUserAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	// First try standard Postgres SQLState for duplicate object (42710)
	if pqErr, ok := err.(*pq.Error); ok {
		if pqErr.Code == "42710" {
			return true
		}
	}

	errMsg := strings.ToLower(err.Error())
	// Fallback to string matching:
	// "user with name <username> exists"
	return strings.Contains(errMsg, "user with name") && strings.Contains(errMsg, "exists")
}
