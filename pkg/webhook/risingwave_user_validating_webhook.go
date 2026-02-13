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

package webhook

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	risingwavev1alpha1 "github.com/risingwavelabs/risingwave-operator/apis/risingwave/v1alpha1"
	"github.com/risingwavelabs/risingwave-operator/pkg/metrics"
)

const (
	passwordRotateAnnotation = "risingwave.risingwavelabs.com/rotate-password"
)

// RisingWaveUserValidatingWebhook is the validating webhook for RisingWaveUser.
type RisingWaveUserValidatingWebhook struct {
	client client.Reader
}

// NewRisingWaveUserValidatingWebhook creates a new validator for RisingWaveUser.
func NewRisingWaveUserValidatingWebhook(client client.Reader) admission.Validator[*risingwavev1alpha1.RisingWaveUser] {
	return metrics.NewValidatingWebhookMetricsRecorder(&RisingWaveUserValidatingWebhook{client: client})
}

// ValidateCreate validates creation of a RisingWaveUser.
func (w *RisingWaveUserValidatingWebhook) ValidateCreate(ctx context.Context, obj *risingwavev1alpha1.RisingWaveUser) (warnings admission.Warnings, err error) {
	return w.validateObject(ctx, obj)
}

// ValidateUpdate validates update of a RisingWaveUser.
func (w *RisingWaveUserValidatingWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj *risingwavev1alpha1.RisingWaveUser) (warnings admission.Warnings, err error) {
	// Validate new object first
	baseWarnings, err := w.validateObject(ctx, newObj)
	if err != nil {
		return baseWarnings, err
	}

	fieldErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	// Combine warnings from base validation with any update-specific warnings
	// (no update-specific warnings currently)
	warnings = baseWarnings
	if len(warnings) == 0 {
		warnings = nil
	}

	// risingWaveRef cannot be changed
	if oldObj.Spec.RisingWaveRef.Name != newObj.Spec.RisingWaveRef.Name {
		fieldErrs = append(fieldErrs, field.Forbidden(
			specPath.Child("risingWaveRef"),
			"risingWaveRef.name cannot be changed",
		))
	}

	// User name cannot be changed once set
	oldName := w.getUserName(oldObj)
	newName := w.getUserName(newObj)
	if oldName != "" && newName != "" && oldName != newName {
		fieldErrs = append(fieldErrs, field.Forbidden(
			specPath.Child("name"),
			fmt.Sprintf("user name cannot be changed from %q to %q", oldName, newName),
		))
	}

	// Password configuration cannot be changed if rotate-password annotation is not set
	hasRotateAnnotation := false
	if newObj.Annotations != nil {
		_, hasRotateAnnotation = newObj.Annotations[passwordRotateAnnotation]
	}
	if !hasRotateAnnotation {
		if !w.passwordConfigEqual(oldObj.Spec.Password, newObj.Spec.Password) {
			fieldErrs = append(fieldErrs, field.Forbidden(
				specPath.Child("password"),
				"password configuration cannot be changed without rotate-password annotation",
			))
		}
	}

	// Auth type cannot be changed once set
	oldAuthType := w.getAuthType(oldObj)
	newAuthType := w.getAuthType(newObj)
	if oldAuthType != "" && newAuthType != "" && oldAuthType != newAuthType {
		fieldErrs = append(fieldErrs, field.Forbidden(
			specPath.Child("auth", "type"),
			fmt.Sprintf("auth type cannot be changed from %s to %s", oldAuthType, newAuthType),
		))
	}

	if len(fieldErrs) > 0 {
		gvk := newObj.GroupVersionKind()
		return warnings, apierrors.NewInvalid(gvk.GroupKind(), newObj.Name, fieldErrs)
	}

	return warnings, nil
}

// ValidateDelete validates deletion of a RisingWaveUser.
func (w *RisingWaveUserValidatingWebhook) ValidateDelete(ctx context.Context, obj *risingwavev1alpha1.RisingWaveUser) (warnings admission.Warnings, err error) {
	return nil, nil
}

// validateObject performs common validation for both create and update.
func (w *RisingWaveUserValidatingWebhook) validateObject(ctx context.Context, obj *risingwavev1alpha1.RisingWaveUser) (warnings admission.Warnings, err error) {
	var validationWarnings admission.Warnings
	fieldErrs := field.ErrorList{}
	specPath := field.NewPath("spec")

	// Validate risingWaveRef
	rwRefPath := specPath.Child("risingWaveRef")
	if obj.Spec.RisingWaveRef.Name == "" {
		fieldErrs = append(fieldErrs, field.Required(rwRefPath.Child("name"), "risingWaveRef.name is required"))
	}

	// Verify referenced RisingWave exists
	if obj.Spec.RisingWaveRef.Name != "" {
		namespace := obj.Spec.RisingWaveRef.Namespace
		if namespace == "" {
			namespace = obj.Namespace
		}

		var risingWave risingwavev1alpha1.RisingWave
		err := w.client.Get(ctx, types.NamespacedName{
			Namespace: namespace,
			Name:      obj.Spec.RisingWaveRef.Name,
		}, &risingWave)

		if err != nil {
			if apierrors.IsNotFound(err) {
				fieldErrs = append(fieldErrs, field.NotFound(rwRefPath.Child("name"),
					fmt.Sprintf("RisingWave %s/%s not found", namespace, obj.Spec.RisingWaveRef.Name)))
			} else {
				return nil, fmt.Errorf("failed to get RisingWave: %w", err)
			}
		}
	}

	// Validate user name
	if obj.Spec.Name != "" {
		namePath := specPath.Child("name")
		if len(obj.Spec.Name) > 63 {
			fieldErrs = append(fieldErrs, field.TooLong(namePath, obj.Spec.Name, 63))
		}
		if strings.HasPrefix(obj.Spec.Name, "pg_") || strings.HasPrefix(obj.Spec.Name, "rw_") {
			validationWarnings = append(validationWarnings,
				fmt.Sprintf("User name %q starts with reserved prefix (pg_ or rw_), this may cause conflicts", obj.Spec.Name))
		}
	}

	// Validate password configuration
	fieldErrs = append(fieldErrs, w.validatePasswordConfig(obj, specPath)...)

	// Validate auth configuration
	fieldErrs = append(fieldErrs, w.validateAuthConfig(obj, specPath)...)

	// Validate privileges (structural only - value validation delegated to RisingWave)
	if obj.Spec.Grants != nil {
		fieldErrs = append(fieldErrs, w.validatePrivileges(obj, specPath)...)
	}

	if len(fieldErrs) > 0 {
		gvk := obj.GroupVersionKind()
		return validationWarnings, apierrors.NewInvalid(gvk.GroupKind(), obj.Name, fieldErrs)
	}

	return validationWarnings, nil
}

// validatePasswordConfig validates password configuration.
func (w *RisingWaveUserValidatingWebhook) validatePasswordConfig(obj *risingwavev1alpha1.RisingWaveUser, specPath *field.Path) field.ErrorList {
	if obj.Spec.Password == nil {
		return nil
	}

	var fieldErrs field.ErrorList
	passwordPath := specPath.Child("password")

	// Cannot have both secretRef and generateRandomLength
	if obj.Spec.Password.SecretRef != nil && obj.Spec.Password.GenerateRandomLength != nil {
		fieldErrs = append(fieldErrs, field.Invalid(
			passwordPath.Child("generateRandomLength"),
			*obj.Spec.Password.GenerateRandomLength,
			"cannot specify both secretRef and generateRandomLength",
		))
	}

	// Validate secretRef
	if obj.Spec.Password.SecretRef != nil {
		secretRefPath := passwordPath.Child("secretRef")
		if obj.Spec.Password.SecretRef.Name == "" {
			fieldErrs = append(fieldErrs, field.Required(secretRefPath.Child("name"), "secretRef.name is required"))
		}
	}

	// Validate generateRandomLength
	if obj.Spec.Password.GenerateRandomLength != nil {
		lengthPath := passwordPath.Child("generateRandomLength")
		length := *obj.Spec.Password.GenerateRandomLength
		if length < 8 {
			fieldErrs = append(fieldErrs, field.Invalid(
				lengthPath,
				length,
				fmt.Sprintf("must be greater than or equal to %d", 8),
			))
		}
		if length > 128 {
			fieldErrs = append(fieldErrs, field.Invalid(
				lengthPath,
				length,
				fmt.Sprintf("must be less than or equal to %d", 128),
			))
		}
	}

	return fieldErrs
}

// validateAuthConfig validates authentication configuration.
func (w *RisingWaveUserValidatingWebhook) validateAuthConfig(obj *risingwavev1alpha1.RisingWaveUser, specPath *field.Path) field.ErrorList {
	if obj.Spec.Auth == nil {
		return nil
	}

	var fieldErrs field.ErrorList
	authPath := specPath.Child("auth")
	authType := w.getAuthType(obj)

	switch authType {
	case risingwavev1alpha1.AuthTypeOAuth:
		if obj.Spec.Auth.OAuth == nil {
			fieldErrs = append(fieldErrs, field.Required(authPath.Child("oauth"),
				"oauth configuration is required when auth.type is oauth"))
			break
		}
		oauthPath := authPath.Child("oauth")
		if obj.Spec.Auth.OAuth.JWKSUrl == "" {
			fieldErrs = append(fieldErrs, field.Required(oauthPath.Child("jwksUrl"),
				"jwksUrl is required for OAuth authentication"))
		}
		if !strings.HasPrefix(obj.Spec.Auth.OAuth.JWKSUrl, "http://") &&
			!strings.HasPrefix(obj.Spec.Auth.OAuth.JWKSUrl, "https://") {
			fieldErrs = append(fieldErrs, field.Invalid(oauthPath.Child("jwksUrl"),
				obj.Spec.Auth.OAuth.JWKSUrl,
				"must be a valid HTTP(S) URL"))
		}
		if obj.Spec.Auth.OAuth.Issuer == "" {
			fieldErrs = append(fieldErrs, field.Required(oauthPath.Child("issuer"),
				"issuer is required for OAuth authentication"))
		}

	case risingwavev1alpha1.AuthTypeLDAP:
		if obj.Spec.Auth.LDAP == nil {
			fieldErrs = append(fieldErrs, field.Required(authPath.Child("ldap"),
				"ldap configuration is required when auth.type is ldap"))
			break
		}
		ldapPath := authPath.Child("ldap")
		if obj.Spec.Auth.LDAP.Host == "" {
			fieldErrs = append(fieldErrs, field.Required(ldapPath.Child("host"),
				"host is required for LDAP authentication"))
		}
		if obj.Spec.Auth.LDAP.BaseDN == "" {
			fieldErrs = append(fieldErrs, field.Required(ldapPath.Child("baseDN"),
				"baseDN is required for LDAP authentication"))
		}
		// Validate port if explicitly set (default port 389 is used if not set)
		port := obj.Spec.Auth.LDAP.Port
		if port != 0 && (port < 1 || port > 65535) {
			fieldErrs = append(fieldErrs, field.Invalid(ldapPath.Child("port"),
				port,
				"port must be between 1 and 65535"))
		}
	}

	return fieldErrs
}

// validatePrivileges validates privilege specifications.
// Performs structural validation only (required fields, names, wildcard restrictions).
// Privilege value validation is delegated to RisingWave during SQL execution.
func (w *RisingWaveUserValidatingWebhook) validatePrivileges(obj *risingwavev1alpha1.RisingWaveUser, specPath *field.Path) field.ErrorList {
	if obj.Spec.Grants == nil {
		return nil
	}

	var fieldErrs field.ErrorList
	privPath := specPath.Child("privileges")

	// Validate hierarchical database privileges
	for i, dbPriv := range obj.Spec.Grants.Databases {
		p := privPath.Child("databases").Index(i)
		if dbPriv.Name == "" {
			fieldErrs = append(fieldErrs, field.Required(p.Child("name"), "database name is required"))
		}
		if dbPriv.Name == "*" {
			fieldErrs = append(fieldErrs, field.Invalid(p.Child("name"), dbPriv.Name,
				"wildcard '*' is not supported for database names, use specific database names"))
		}
		// Validate nested schemas
		for j, schemaPriv := range dbPriv.Schemas {
			schemaPath := p.Child("schemas").Index(j)
			if schemaPriv.Name == "" {
				fieldErrs = append(fieldErrs, field.Required(schemaPath.Child("name"), "schema name is required"))
			}
			if len(schemaPriv.Privileges) == 0 {
				fieldErrs = append(fieldErrs, field.Required(schemaPath.Child("privileges"), "at least one privilege is required"))
			}
			// Validate nested tables
			for k, tablePriv := range schemaPriv.Tables {
				tablePath := schemaPath.Child("tables").Index(k)
				if tablePriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(tablePath.Child("name"), "table name is required"))
				}
				if len(tablePriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(tablePath.Child("privileges"), "at least one privilege is required"))
				}
			}
			// Validate nested views
			for k, viewPriv := range schemaPriv.Views {
				viewPath := schemaPath.Child("views").Index(k)
				if viewPriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(viewPath.Child("name"), "view name is required"))
				}
				if len(viewPriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(viewPath.Child("privileges"), "at least one privilege is required"))
				}
			}
			// Validate nested materialized views
			for k, mvPriv := range schemaPriv.MaterializedViews {
				mvPath := schemaPath.Child("materializedViews").Index(k)
				if mvPriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(mvPath.Child("name"), "materialized view name is required"))
				}
				if len(mvPriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(mvPath.Child("privileges"), "at least one privilege is required"))
				}
			}
			// Validate nested sources
			for k, sourcePriv := range schemaPriv.Sources {
				sourcePath := schemaPath.Child("sources").Index(k)
				if sourcePriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(sourcePath.Child("name"), "source name is required"))
				}
				if len(sourcePriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(sourcePath.Child("privileges"), "at least one privilege is required"))
				}
			}
			// Validate nested sinks
			for k, sinkPriv := range schemaPriv.Sinks {
				sinkPath := schemaPath.Child("sinks").Index(k)
				if sinkPriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(sinkPath.Child("name"), "sink name is required"))
				}
				if len(sinkPriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(sinkPath.Child("privileges"), "at least one privilege is required"))
				}
			}
			// Validate nested connections
			for k, connPriv := range schemaPriv.Connections {
				connPath := schemaPath.Child("connections").Index(k)
				if connPriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(connPath.Child("name"), "connection name is required"))
				}
				if len(connPriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(connPath.Child("privileges"), "at least one privilege is required"))
				}
			}
			// Validate nested secrets
			for k, secretPriv := range schemaPriv.Secrets {
				secretPath := schemaPath.Child("secrets").Index(k)
				if secretPriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(secretPath.Child("name"), "secret name is required"))
				}
				if len(secretPriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(secretPath.Child("privileges"), "at least one privilege is required"))
				}
			}
			// Validate nested functions
			for k, funcPriv := range schemaPriv.Functions {
				funcPath := schemaPath.Child("functions").Index(k)
				if funcPriv.Name == "" {
					fieldErrs = append(fieldErrs, field.Required(funcPath.Child("name"), "function name is required"))
				}
				if len(funcPriv.Privileges) == 0 {
					fieldErrs = append(fieldErrs, field.Required(funcPath.Child("privileges"), "at least one privilege is required"))
				}
			}
		}
		// Validate database-level privileges
		if len(dbPriv.Privileges) == 0 {
			fieldErrs = append(fieldErrs, field.Required(p.Child("privileges"), "at least one privilege is required"))
		}
	}

	return fieldErrs
}

// getUserName returns user name from spec, defaulting to metadata.name.
func (w *RisingWaveUserValidatingWebhook) getUserName(obj *risingwavev1alpha1.RisingWaveUser) string {
	if obj.Spec.Name != "" {
		return obj.Spec.Name
	}
	return obj.Name
}

// getAuthType returns authentication type, defaulting to password.
func (w *RisingWaveUserValidatingWebhook) getAuthType(obj *risingwavev1alpha1.RisingWaveUser) risingwavev1alpha1.AuthType {
	if obj.Spec.Auth != nil && obj.Spec.Auth.Type != nil {
		return *obj.Spec.Auth.Type
	}
	return risingwavev1alpha1.AuthTypePassword
}

// passwordConfigEqual checks if two password configs are functionally equal.
func (w *RisingWaveUserValidatingWebhook) passwordConfigEqual(a, b *risingwavev1alpha1.PasswordConfig) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Compare secret references
	aSecret := a.SecretRef
	bSecret := b.SecretRef
	if aSecret != nil && bSecret != nil {
		if aSecret.Name == bSecret.Name && aSecret.Namespace == bSecret.Namespace && aSecret.Key == bSecret.Key {
			return true
		}
		return false
	}

	// Compare generateRandomLength
	aLen := a.GenerateRandomLength
	bLen := b.GenerateRandomLength
	if aLen != nil && bLen != nil {
		return *aLen == *bLen
	}
	if aLen == nil || bLen == nil {
		return true
	}

	return false
}
