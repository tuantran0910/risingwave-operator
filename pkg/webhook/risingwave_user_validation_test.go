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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	risingwavev1alpha1 "github.com/risingwavelabs/risingwave-operator/apis/risingwave/v1alpha1"
)

func newTestRisingWave(name string) *risingwavev1alpha1.RisingWave {
	return &risingwavev1alpha1.RisingWave{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Status: risingwavev1alpha1.RisingWaveStatus{
			Conditions: []risingwavev1alpha1.RisingWaveCondition{
				{
					Type:   risingwavev1alpha1.RisingWaveConditionRunning,
					Status: metav1.ConditionTrue,
					Reason: "RisingWaveClusterReady",
				},
			},
		},
	}
}

func newTestUser(name string) *risingwavev1alpha1.RisingWaveUser {
	return &risingwavev1alpha1.RisingWaveUser{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: risingwavev1alpha1.RisingWaveUserSpec{
			RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
				Name:      "test-rw",
				Namespace: "default",
			},
		},
	}
}

func newFakeClient(objects ...runtime.Object) client.Reader {
	scheme := runtime.NewScheme()
	err := risingwavev1alpha1.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, obj := range objects {
		builder.WithRuntimeObjects(obj)
	}
	return builder.Build()
}

func TestRisingWaveUserValidatingWebhook_ValidateCreate(t *testing.T) {
	tests := []struct {
		name    string
		user    *risingwavev1alpha1.RisingWaveUser
		wantErr string
	}{
		{
			name:    "valid user",
			user:    newTestUser("test-user"),
			wantErr: "",
		},
		{
			name: "missing risingWaveRef name",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-user",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Namespace: "default",
					},
				},
			},
			wantErr: "risingWaveRef.name is required",
		},
		{
			name: "user name too long",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-user",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					Name: string(make([]rune, 64)),
				},
			},
			wantErr: "Too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := newFakeClient(newTestRisingWave("test-rw"))
			webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

			warnings, err := webhook.ValidateCreate(context.Background(), tt.user)

			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Empty(t, warnings)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRisingWaveUserValidatingWebhook_ValidateCreate_RisingWaveNotFound(t *testing.T) {
	user := newTestUser("test-user")
	user.Spec.RisingWaveRef.Name = "nonexistent-rw"

	fakeClient := newFakeClient()
	webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

	_, err := webhook.ValidateCreate(context.Background(), user)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "Not found")
}

func TestRisingWaveUserValidatingWebhook_ValidateUpdate(t *testing.T) {
	tests := []struct {
		name    string
		oldUser *risingwavev1alpha1.RisingWaveUser
		newUser *risingwavev1alpha1.RisingWaveUser
		wantErr string
	}{
		{
			name: "valid update",
			oldUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
				},
			},
			newUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
				},
			},
			wantErr: "",
		},
		{
			name: "user name cannot be changed",
			oldUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "oldname",
				},
			},
			newUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "newname",
				},
			},
			wantErr: "user name cannot be changed",
		},
		{
			name: "password config changed without annotation",
			oldUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						GenerateRandomLength: ptr.To(int32(16)),
					},
				},
			},
			newUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						GenerateRandomLength: ptr.To(int32(32)),
					},
				},
			},
			wantErr: "password configuration cannot be changed without rotate-password annotation",
		},
		{
			name: "auth type changed",
			oldUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypePassword),
					},
				},
			},
			newUser: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeOAuth),
					},
				},
			},
			wantErr: "oauth configuration is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := newFakeClient(newTestRisingWave("test-rw"))
			webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

			warnings, err := webhook.ValidateUpdate(context.Background(), tt.oldUser, tt.newUser)

			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Empty(t, warnings)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRisingWaveUserValidatingWebhook_validatePasswordConfig(t *testing.T) {
	tests := []struct {
		name    string
		user    *risingwavev1alpha1.RisingWaveUser
		wantErr string
	}{
		{
			name: "valid password with generateRandomLength",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						GenerateRandomLength: ptr.To(int32(16)),
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid password with secretRef",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						SecretRef: &risingwavev1alpha1.SecretReference{
							Name: "my-secret",
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "both secretRef and generateRandomLength",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						SecretRef: &risingwavev1alpha1.SecretReference{
							Name: "my-secret",
						},
						GenerateRandomLength: ptr.To(int32(16)),
					},
				},
			},
			wantErr: "cannot specify both secretRef and generateRandomLength",
		},
		{
			name: "generateRandomLength too small",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						GenerateRandomLength: ptr.To(int32(7)),
					},
				},
			},
			wantErr: "must be greater than or equal to 8",
		},
		{
			name: "generateRandomLength too large",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						GenerateRandomLength: ptr.To(int32(129)),
					},
				},
			},
			wantErr: "must be less than or equal to 128",
		},
		{
			name: "secretRef without name",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Password: &risingwavev1alpha1.PasswordConfig{
						SecretRef: &risingwavev1alpha1.SecretReference{
							Namespace: "default",
						},
					},
				},
			},
			wantErr: "secretRef.name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := newFakeClient(newTestRisingWave("test-rw"))
			webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

			warnings, err := webhook.ValidateCreate(context.Background(), tt.user)

			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Empty(t, warnings)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRisingWaveUserValidatingWebhook_validateAuthConfig_OAuth(t *testing.T) {
	tests := []struct {
		name    string
		user    *risingwavev1alpha1.RisingWaveUser
		wantErr string
	}{
		{
			name: "valid OAuth config",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeOAuth),
						OAuth: &risingwavev1alpha1.OAuthConfig{
							JWKSUrl: "https://auth.example.com/.well-known/jwks.json",
							Issuer:  "risingwave",
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "OAuth without oauth config",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeOAuth),
					},
				},
			},
			wantErr: "oauth configuration is required",
		},
		{
			name: "OAuth without jwksUrl",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeOAuth),
						OAuth: &risingwavev1alpha1.OAuthConfig{
							Issuer: "risingwave",
						},
					},
				},
			},
			wantErr: "jwksUrl is required",
		},
		{
			name: "OAuth without issuer",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeOAuth),
						OAuth: &risingwavev1alpha1.OAuthConfig{
							JWKSUrl: "https://auth.example.com/.well-known/jwks.json",
						},
					},
				},
			},
			wantErr: "issuer is required",
		},
		{
			name: "valid OAuth with audience",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeOAuth),
						OAuth: &risingwavev1alpha1.OAuthConfig{
							JWKSUrl:  "https://auth.example.com/.well-known/jwks.json",
							Issuer:   "risingwave",
							Audience: []string{"risingwave", "https://myapp.example.com"},
						},
					},
				},
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := newFakeClient(newTestRisingWave("test-rw"))
			webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

			warnings, err := webhook.ValidateCreate(context.Background(), tt.user)

			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Empty(t, warnings)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRisingWaveUserValidatingWebhook_validateAuthConfig_LDAP(t *testing.T) {
	tests := []struct {
		name    string
		user    *risingwavev1alpha1.RisingWaveUser
		wantErr string
	}{
		{
			name: "valid LDAP config",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeLDAP),
						LDAP: &risingwavev1alpha1.LDAPConfig{
							Host:   "ldap.example.com",
							BaseDN: "dc=example,dc=com",
							Port:   389,
						},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "LDAP without ldap config",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeLDAP),
					},
				},
			},
			wantErr: "ldap configuration is required",
		},
		{
			name: "LDAP without host",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeLDAP),
						LDAP: &risingwavev1alpha1.LDAPConfig{
							BaseDN: "dc=example,dc=com",
						},
					},
				},
			},
			wantErr: "host is required",
		},
		{
			name: "LDAP without baseDN",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeLDAP),
						LDAP: &risingwavev1alpha1.LDAPConfig{
							Host: "ldap.example.com",
						},
					},
				},
			},
			wantErr: "baseDN is required",
		},
		{
			name: "LDAP port too large",
			user: &risingwavev1alpha1.RisingWaveUser{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-user",
					Namespace: "default",
				},
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
						Name: "test-rw",
					},
					Name: "testuser",
					Auth: &risingwavev1alpha1.AuthConfig{
						Type: ptr.To(risingwavev1alpha1.AuthTypeLDAP),
						LDAP: &risingwavev1alpha1.LDAPConfig{
							Host:   "ldap.example.com",
							BaseDN: "dc=example,dc=com",
							Port:   65536,
						},
					},
				},
			},
			wantErr: "port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := newFakeClient(newTestRisingWave("test-rw"))
			webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

			warnings, err := webhook.ValidateCreate(context.Background(), tt.user)

			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Empty(t, warnings)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRisingWaveUserValidatingWebhook_validatePermissions(t *testing.T) {
	tests := []struct {
		name        string
		permissions []risingwavev1alpha1.UserPermission
		wantErr     string
	}{
		{
			name:        "empty permissions",
			permissions: []risingwavev1alpha1.UserPermission{},
			wantErr:     "",
		},
		{
			name: "valid permissions",
			permissions: []risingwavev1alpha1.UserPermission{
				"CREATEDB",
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := newTestUser("test-user")
			user.Spec.Permissions = tt.permissions

			fakeClient := newFakeClient(newTestRisingWave("test-rw"))
			webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

			warnings, err := webhook.ValidateCreate(context.Background(), user)

			if tt.wantErr == "" {
				require.NoError(t, err)
				assert.Empty(t, warnings)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			}
		})
	}
}

func TestRisingWaveUserValidatingWebhook_validatePrivileges(t *testing.T) {
	t.Run("valid database privileges", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Privileges: []risingwavev1alpha1.DatabasePrivilegeType{
								risingwavev1alpha1.DatabasePrivilegeConnect,
							},
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		warnings, err := webhook.ValidateCreate(context.Background(), user)

		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("database without name", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Privileges: []risingwavev1alpha1.DatabasePrivilegeType{
								risingwavev1alpha1.DatabasePrivilegeConnect,
							},
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		_, err := webhook.ValidateCreate(context.Background(), user)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "database name is required")
	})

	t.Run("database without privileges", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		_, err := webhook.ValidateCreate(context.Background(), user)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one privilege is required")
	})

	t.Run("database wildcard not supported", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "*",
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		_, err := webhook.ValidateCreate(context.Background(), user)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "wildcard '*' is not supported")
	})

	t.Run("valid hierarchical schema privileges", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Privileges: []risingwavev1alpha1.DatabasePrivilegeType{
								risingwavev1alpha1.DatabasePrivilegeConnect,
							},
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name: "analytics",
									Privileges: []risingwavev1alpha1.SchemaPrivilegeType{
										risingwavev1alpha1.SchemaPrivilegeUsage,
										risingwavev1alpha1.SchemaPrivilegeCreate,
									},
									Tables: []risingwavev1alpha1.NestedTablePrivilege{
										{
											Name:       "events",
											Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		warnings, err := webhook.ValidateCreate(context.Background(), user)

		require.NoError(t, err)
		assert.Empty(t, warnings)
	})

	t.Run("schema without name", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Privileges: []risingwavev1alpha1.SchemaPrivilegeType{
										risingwavev1alpha1.SchemaPrivilegeUsage,
									},
								},
							},
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		_, err := webhook.ValidateCreate(context.Background(), user)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "schema name is required")
	})

	t.Run("schema without privileges", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name: "analytics",
								},
							},
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		_, err := webhook.ValidateCreate(context.Background(), user)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one privilege is required")
	})

	t.Run("table without name", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name: "analytics",
									Privileges: []risingwavev1alpha1.SchemaPrivilegeType{
										risingwavev1alpha1.SchemaPrivilegeUsage,
									},
									Tables: []risingwavev1alpha1.NestedTablePrivilege{
										{
											Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		_, err := webhook.ValidateCreate(context.Background(), user)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "table name is required")
	})

	t.Run("table without privileges", func(t *testing.T) {
		user := &risingwavev1alpha1.RisingWaveUser{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-user",
				Namespace: "default",
			},
			Spec: risingwavev1alpha1.RisingWaveUserSpec{
				RisingWaveRef: risingwavev1alpha1.RisingWaveReference{
					Name: "test-rw",
				},
				Name: "testuser",
				Privileges: &risingwavev1alpha1.PrivilegeSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name: "analytics",
									Tables: []risingwavev1alpha1.NestedTablePrivilege{
										{
											Name: "events",
										},
									},
								},
							},
						},
					},
				},
			},
		}

		fakeClient := newFakeClient(newTestRisingWave("test-rw"))
		webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

		_, err := webhook.ValidateCreate(context.Background(), user)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least one privilege is required")
	})
}

func TestRisingWaveUserValidatingWebhook_ValidateDelete(t *testing.T) {
	user := newTestUser("test-user")

	fakeClient := newFakeClient(newTestRisingWave("test-rw"))
	webhook := NewRisingWaveUserValidatingWebhook(fakeClient)

	warnings, err := webhook.ValidateDelete(context.Background(), user)

	require.NoError(t, err)
	assert.Empty(t, warnings)
}
