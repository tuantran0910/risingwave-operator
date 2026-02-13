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

package rwclient

import (
	"testing"

	risingwavev1alpha1 "github.com/risingwavelabs/risingwave-operator/apis/risingwave/v1alpha1"
)

func TestBuildCreateUserSQL(t *testing.T) {
	tests := []struct {
		name     string
		user     *risingwavev1alpha1.RisingWaveUser
		password string
		expected string
	}{
		{
			name: "basic user with password",
			user: &risingwavev1alpha1.RisingWaveUser{
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					Name: "testuser",
					Permissions: []risingwavev1alpha1.UserPermission{
						risingwavev1alpha1.UserPermission("CREATEDB"),
						risingwavev1alpha1.UserPermission("NOCREATEUSER"),
					},
				},
			},
			password: "SecretPassword123",
			expected: `CREATE USER "testuser" WITH PASSWORD 'SecretPassword123' CREATEDB NOCREATEUSER`,
		},
		{
			name: "basic user without password",
			user: &risingwavev1alpha1.RisingWaveUser{
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					Name: "testuser",
					Permissions: []risingwavev1alpha1.UserPermission{
						risingwavev1alpha1.UserPermission("CREATEDB"),
						risingwavev1alpha1.UserPermission("NOCREATEUSER"),
					},
				},
			},
			expected: `CREATE USER "testuser" CREATEDB NOCREATEUSER`,
		},
		{
			name: "superuser",
			user: &risingwavev1alpha1.RisingWaveUser{
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					Name: "testuser",
					Permissions: []risingwavev1alpha1.UserPermission{
						risingwavev1alpha1.UserPermission("SUPERUSER"),
						risingwavev1alpha1.UserPermission("CREATEDB"),
					},
				},
			},
			password: "SuperSecret",
			expected: `CREATE USER "testuser" WITH PASSWORD 'SuperSecret' SUPERUSER CREATEDB`,
		},
		{
			name: "all permissions",
			user: &risingwavev1alpha1.RisingWaveUser{
				Spec: risingwavev1alpha1.RisingWaveUserSpec{
					Name: "testuser",
					Permissions: []risingwavev1alpha1.UserPermission{
						risingwavev1alpha1.UserPermission("SUPERUSER"),
						risingwavev1alpha1.UserPermission("CREATEDB"),
						risingwavev1alpha1.UserPermission("CREATEUSER"),
					},
				},
			},
			password: "AnyPass",
			expected: `CREATE USER "testuser" WITH PASSWORD 'AnyPass' SUPERUSER CREATEDB CREATEUSER`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildCreateUserSQL(tt.user, tt.password)
			if got != tt.expected {
				t.Errorf("BuildCreateUserSQL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildAlterUserPasswordSQL(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		password string
		expected string
	}{
		{
			name:     "basic password change",
			userName: "testuser",
			password: "NewPass123",
			expected: `ALTER USER "testuser" WITH PASSWORD 'NewPass123'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAlterUserPasswordSQL(tt.userName, tt.password)
			if got != tt.expected {
				t.Errorf("BuildAlterUserPasswordSQL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildDropUserSQL(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		expected string
	}{
		{
			name:     "basic drop",
			userName: "testuser",
			expected: `DROP USER IF EXISTS "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildDropUserSQL(tt.userName)
			if got != tt.expected {
				t.Errorf("BuildDropUserSQL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantStatements(t *testing.T) {
	user := &risingwavev1alpha1.RisingWaveUser{
		Spec: risingwavev1alpha1.RisingWaveUserSpec{
			Name: "testuser",
			Grants: &risingwavev1alpha1.GrantSpec{
				Databases: []risingwavev1alpha1.DatabasePrivilege{
					{
						Name: "dev",
						Privileges: []risingwavev1alpha1.DatabasePrivilegeType{
							risingwavev1alpha1.DatabasePrivilegeConnect,
							risingwavev1alpha1.DatabasePrivilegeCreate,
						},
						WithGrantOption: true,
					},
				},
			},
		},
	}

	statements := BuildGrantStatements("testuser", &user.Spec)
	if len(statements) != 1 {
		t.Errorf("BuildGrantStatements() returned %d statements, expected 1", len(statements))
	}

	expectedDB := `GRANT CONNECT, CREATE ON DATABASE "dev" TO "testuser" WITH GRANT OPTION`
	if statements[0] != expectedDB {
		t.Errorf("BuildGrantStatements()[0] = %q, want %q", statements[0], expectedDB)
	}
}

func TestBuildGrantStatements_Hierarchical(t *testing.T) {
	tests := []struct {
		name     string
		spec     *risingwavev1alpha1.RisingWaveUserSpec
		expected int
		verify   func(t *testing.T, statements []string)
	}{
		{
			name: "nested schema and table privileges",
			spec: &risingwavev1alpha1.RisingWaveUserSpec{
				Grants: &risingwavev1alpha1.GrantSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name: "public",
									Tables: []risingwavev1alpha1.NestedTablePrivilege{
										{Name: "orders", Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect}},
										{Name: "customers", Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect, risingwavev1alpha1.TablePrivilegeUpdate}, WithGrantOption: true},
									},
								},
							},
						},
					},
				},
			},
			expected: 2,
			verify: func(t *testing.T, statements []string) {
				if len(statements) != 2 {
					t.Errorf("expected 2 statements, got %d", len(statements))
				}
				expectedOrders := `GRANT SELECT ON TABLE "dev"."public"."orders" TO "testuser"`
				if statements[0] != expectedOrders {
					t.Errorf("expected %q, got %q", expectedOrders, statements[0])
				}
				expectedCustomers := `GRANT SELECT, UPDATE ON TABLE "dev"."public"."customers" TO "testuser" WITH GRANT OPTION`
				if statements[1] != expectedCustomers {
					t.Errorf("expected %q, got %q", expectedCustomers, statements[1])
				}
			},
		},
		{
			name: "wildcard table privileges",
			spec: &risingwavev1alpha1.RisingWaveUserSpec{
				Grants: &risingwavev1alpha1.GrantSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "analytics",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name: "staging",
									Tables: []risingwavev1alpha1.NestedTablePrivilege{
										{Name: "*", Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect, risingwavev1alpha1.TablePrivilegeInsert}},
									},
								},
							},
						},
					},
				},
			},
			expected: 1,
			verify: func(t *testing.T, statements []string) {
				expected := `GRANT SELECT, INSERT ON ALL TABLES IN SCHEMA "staging" TO "testuser"`
				if len(statements) != 1 {
					t.Errorf("expected 1 statement, got %d", len(statements))
				}
				if statements[0] != expected {
					t.Errorf("expected %q, got %q", expected, statements[0])
				}
			},
		},
		{
			name: "all nested object types",
			spec: &risingwavev1alpha1.RisingWaveUserSpec{
				Grants: &risingwavev1alpha1.GrantSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name:       "public",
									Privileges: []risingwavev1alpha1.SchemaPrivilegeType{risingwavev1alpha1.SchemaPrivilegeUsage, risingwavev1alpha1.SchemaPrivilegeCreate},
									Tables: []risingwavev1alpha1.NestedTablePrivilege{
										{Name: "events", Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect}},
									},
									Views: []risingwavev1alpha1.NestedViewPrivilege{
										{Name: "order_summary", Privileges: []risingwavev1alpha1.ViewPrivilegeType{risingwavev1alpha1.ViewPrivilegeSelect}},
									},
									MaterializedViews: []risingwavev1alpha1.NestedMaterializedViewPrivilege{
										{Name: "daily_summary", Privileges: []risingwavev1alpha1.MaterializedViewPrivilegeType{risingwavev1alpha1.MaterializedViewPrivilegeSelect}},
									},
									Sources: []risingwavev1alpha1.NestedSourcePrivilege{
										{Name: "kafka_source", Privileges: []risingwavev1alpha1.SourcePrivilegeType{risingwavev1alpha1.SourcePrivilegeSelect}},
									},
									Sinks: []risingwavev1alpha1.NestedSinkPrivilege{
										{Name: "elasticsearch_sink", Privileges: []risingwavev1alpha1.SinkPrivilegeType{risingwavev1alpha1.SinkPrivilegeSelect}},
									},
									Connections: []risingwavev1alpha1.NestedConnectionPrivilege{
										{Name: "kafka_conn", Privileges: []risingwavev1alpha1.ConnectionPrivilegeType{risingwavev1alpha1.ConnectionPrivilegeUsage}},
									},
									Secrets: []risingwavev1alpha1.NestedSecretPrivilege{
										{Name: "api_keys", Privileges: []risingwavev1alpha1.SecretPrivilegeType{risingwavev1alpha1.SecretPrivilegeUsage}},
									},
									Functions: []risingwavev1alpha1.NestedFunctionPrivilege{
										{Name: "process_data", Privileges: []risingwavev1alpha1.FunctionPrivilegeType{risingwavev1alpha1.FunctionPrivilegeExecute}},
									},
								},
							},
						},
					},
				},
			},
			expected: 9,
			verify: func(t *testing.T, statements []string) {
				if len(statements) != 9 {
					t.Errorf("expected 9 statements, got %d", len(statements))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildGrantStatements("testuser", tt.spec)
			if len(got) != tt.expected {
				t.Errorf("BuildGrantStatements() returned %d statements, expected %d", len(got), tt.expected)
			}
			if tt.verify != nil {
				tt.verify(t, got)
			}
		})
	}
}

func TestNormalizePrivileges(t *testing.T) {
	tests := []struct {
		name       string
		privileges []risingwavev1alpha1.DatabasePrivilegeType
		expected   string
	}{
		{
			name:       "single privilege",
			privileges: []risingwavev1alpha1.DatabasePrivilegeType{risingwavev1alpha1.DatabasePrivilegeConnect},
			expected:   "CONNECT",
		},
		{
			name: "multiple privileges",
			privileges: []risingwavev1alpha1.DatabasePrivilegeType{
				risingwavev1alpha1.DatabasePrivilegeConnect,
				risingwavev1alpha1.DatabasePrivilegeCreate,
			},
			expected: "CONNECT, CREATE",
		},
		{
			name:       "ALL PRIVILEGES becomes ALL",
			privileges: []risingwavev1alpha1.DatabasePrivilegeType{"ALL PRIVILEGES"},
			expected:   "ALL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizePrivileges(tt.privileges)
			if got != tt.expected {
				t.Errorf("normalizePrivileges() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestQuoteIdentifier(t *testing.T) {
	tests := []struct {
		name       string
		identifier string
		expected   string
	}{
		{
			name:       "simple identifier",
			identifier: "table_name",
			expected:   `"table_name"`,
		},
		{
			name:       "asterisk",
			identifier: "*",
			expected:   "*",
		},
		{
			name:       "identifier with single quote",
			identifier: `my"table`,
			expected:   `"my""table"`,
		},
		{
			name:       "identifier with multiple quotes",
			identifier: `my"table"name`,
			expected:   `"my""table""name"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteIdentifier(tt.identifier)
			if got != tt.expected {
				t.Errorf("QuoteIdentifier() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestEscapeStringLiteral(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "string with single quote",
			input:    "it's",
			expected: "it''s",
		},
		{
			name:     "string with multiple quotes",
			input:    `contain's'quote`,
			expected: `contain''s''quote`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeStringLiteral(tt.input)
			if got != tt.expected {
				t.Errorf("escapeStringLiteral() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestQuoteUser(t *testing.T) {
	tests := []struct {
		name     string
		userName string
		expected string
	}{
		{
			name:     "simple username",
			userName: "testuser",
			expected: `"testuser"`,
		},
		{
			name:     "username with special chars",
			userName: "user-123",
			expected: `"user-123"`,
		},
		{
			name:     "username with quotes",
			userName: `my"user`,
			expected: `"my""user"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := QuoteUser(tt.userName)
			if got != tt.expected {
				t.Errorf("QuoteUser() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildCreateUserWithOAuthSQL(t *testing.T) {
	oauth := &risingwavev1alpha1.OAuthConfig{
		JWKSUrl: "https://auth.example.com/.well-known/jwks.json",
		Issuer:  "risingwave",
	}

	got := BuildCreateUserWithOAuthSQL("oauthuser", oauth)
	expected := `CREATE USER "oauthuser" WITH CREATE_USER_ONLY`

	if got != expected {
		t.Errorf("BuildCreateUserWithOAuthSQL() = %q, want %q", got, expected)
	}
}

func TestBuildAlterUserOAuthSQL(t *testing.T) {
	oauth := &risingwavev1alpha1.OAuthConfig{
		JWKSUrl: "https://auth.example.com/.well-known/jwks.json",
		Issuer:  "risingwave",
	}

	got := BuildAlterUserOAuthSQL("oauthuser", oauth)

	if len(got) != 0 {
		t.Errorf("BuildAlterUserOAuthSQL() returned %d statements, expected 0", len(got))
	}
}

func TestBuildCreateUserWithLDAPSQL(t *testing.T) {
	ldap := &risingwavev1alpha1.LDAPConfig{
		Host:   "ldap.example.com",
		Port:   389,
		BaseDN: "dc=example,dc=com",
	}

	got := BuildCreateUserWithLDAPSQL("ldapuser", ldap)
	expected := `CREATE USER "ldapuser" WITH CREATE_USER_ONLY`

	if got != expected {
		t.Errorf("BuildCreateUserWithLDAPSQL() = %q, want %q", got, expected)
	}
}

func TestBuildAlterUserPermissionsSQL(t *testing.T) {
	tests := []struct {
		name        string
		userName    string
		permissions []risingwavev1alpha1.UserPermission
		expected    string
	}{
		{
			name:        "empty permissions",
			userName:    "testuser",
			permissions: []risingwavev1alpha1.UserPermission{},
			expected:    "",
		},
		{
			name:     "single permission",
			userName: "testuser",
			permissions: []risingwavev1alpha1.UserPermission{
				risingwavev1alpha1.UserPermission("CREATEDB"),
			},
			expected: `ALTER USER "testuser" CREATEDB`,
		},
		{
			name:     "multiple permissions",
			userName: "testuser",
			permissions: []risingwavev1alpha1.UserPermission{
				risingwavev1alpha1.UserPermission("SUPERUSER"),
				risingwavev1alpha1.UserPermission("CREATEDB"),
			},
			expected: `ALTER USER "testuser" SUPERUSER CREATEDB`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildAlterUserPermissionsSQL(tt.userName, tt.permissions)
			if got != tt.expected {
				t.Errorf("BuildAlterUserPermissionsSQL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildRevokeDatabasePrivilege(t *testing.T) {
	priv := &risingwavev1alpha1.DatabasePrivilege{
		Name:       "dev",
		Privileges: []risingwavev1alpha1.DatabasePrivilegeType{risingwavev1alpha1.DatabasePrivilegeConnect},
	}

	got := buildRevokeDatabasePrivilege("testuser", priv)
	expected := `REVOKE CONNECT ON DATABASE "dev" FROM "testuser"`

	if got != expected {
		t.Errorf("buildRevokeDatabasePrivilege() = %q, want %q", got, expected)
	}
}

func TestBuildGrantNestedTablePrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedTablePrivilege
		expected string
	}{
		{
			name: "specific table",
			priv: &risingwavev1alpha1.NestedTablePrivilege{
				Name:       "orders",
				Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect},
			},
			expected: `GRANT SELECT ON TABLE "dev"."public"."orders" TO "testuser"`,
		},
		{
			name: "table with multiple privileges",
			priv: &risingwavev1alpha1.NestedTablePrivilege{
				Name: "products",
				Privileges: []risingwavev1alpha1.TablePrivilegeType{
					risingwavev1alpha1.TablePrivilegeSelect,
					risingwavev1alpha1.TablePrivilegeInsert,
					risingwavev1alpha1.TablePrivilegeUpdate,
				},
				WithGrantOption: true,
			},
			expected: `GRANT SELECT, INSERT, UPDATE ON TABLE "dev"."public"."products" TO "testuser" WITH GRANT OPTION`,
		},
		{
			name: "wildcard all tables",
			priv: &risingwavev1alpha1.NestedTablePrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect, risingwavev1alpha1.TablePrivilegeDelete},
			},
			expected: `GRANT SELECT, DELETE ON ALL TABLES IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedTablePrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedTablePrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantNestedViewPrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedViewPrivilege
		expected string
	}{
		{
			name: "specific view",
			priv: &risingwavev1alpha1.NestedViewPrivilege{
				Name:       "order_summary",
				Privileges: []risingwavev1alpha1.ViewPrivilegeType{risingwavev1alpha1.ViewPrivilegeSelect},
			},
			expected: `GRANT SELECT ON VIEW "dev"."public"."order_summary" TO "testuser"`,
		},
		{
			name: "view with multiple privileges",
			priv: &risingwavev1alpha1.NestedViewPrivilege{
				Name: "customer_view",
				Privileges: []risingwavev1alpha1.ViewPrivilegeType{
					risingwavev1alpha1.ViewPrivilegeSelect,
					risingwavev1alpha1.ViewPrivilegeDelete,
				},
				WithGrantOption: true,
			},
			expected: `GRANT SELECT, DELETE ON VIEW "dev"."public"."customer_view" TO "testuser" WITH GRANT OPTION`,
		},
		{
			name: "wildcard all views",
			priv: &risingwavev1alpha1.NestedViewPrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.ViewPrivilegeType{risingwavev1alpha1.ViewPrivilegeSelect, risingwavev1alpha1.ViewPrivilegeInsert},
			},
			expected: `GRANT SELECT, INSERT ON ALL VIEWS IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedViewPrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedViewPrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantNestedMaterializedViewPrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedMaterializedViewPrivilege
		expected string
	}{
		{
			name: "specific materialized view",
			priv: &risingwavev1alpha1.NestedMaterializedViewPrivilege{
				Name:       "daily_summary",
				Privileges: []risingwavev1alpha1.MaterializedViewPrivilegeType{risingwavev1alpha1.MaterializedViewPrivilegeSelect},
			},
			expected: `GRANT SELECT ON MATERIALIZED VIEW "dev"."public"."daily_summary" TO "testuser"`,
		},
		{
			name: "wildcard all materialized views",
			priv: &risingwavev1alpha1.NestedMaterializedViewPrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.MaterializedViewPrivilegeType{risingwavev1alpha1.MaterializedViewPrivilegeSelect},
			},
			expected: `GRANT SELECT ON ALL MATERIALIZED VIEWS IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedMaterializedViewPrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedMaterializedViewPrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantNestedSourcePrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedSourcePrivilege
		expected string
	}{
		{
			name: "specific source",
			priv: &risingwavev1alpha1.NestedSourcePrivilege{
				Name:       "kafka_source",
				Privileges: []risingwavev1alpha1.SourcePrivilegeType{risingwavev1alpha1.SourcePrivilegeSelect},
			},
			expected: `GRANT SELECT ON SOURCE "dev"."public"."kafka_source" TO "testuser"`,
		},
		{
			name: "wildcard all sources",
			priv: &risingwavev1alpha1.NestedSourcePrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.SourcePrivilegeType{risingwavev1alpha1.SourcePrivilegeSelect},
			},
			expected: `GRANT SELECT ON ALL SOURCES IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedSourcePrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedSourcePrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantNestedSinkPrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedSinkPrivilege
		expected string
	}{
		{
			name: "specific sink",
			priv: &risingwavev1alpha1.NestedSinkPrivilege{
				Name:       "elasticsearch_sink",
				Privileges: []risingwavev1alpha1.SinkPrivilegeType{risingwavev1alpha1.SinkPrivilegeSelect},
			},
			expected: `GRANT SELECT ON SINK "dev"."public"."elasticsearch_sink" TO "testuser"`,
		},
		{
			name: "wildcard all sinks",
			priv: &risingwavev1alpha1.NestedSinkPrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.SinkPrivilegeType{risingwavev1alpha1.SinkPrivilegeSelect},
			},
			expected: `GRANT SELECT ON ALL SINKS IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedSinkPrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedSinkPrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantNestedConnectionPrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedConnectionPrivilege
		expected string
	}{
		{
			name: "specific connection",
			priv: &risingwavev1alpha1.NestedConnectionPrivilege{
				Name:       "kafka_conn",
				Privileges: []risingwavev1alpha1.ConnectionPrivilegeType{risingwavev1alpha1.ConnectionPrivilegeUsage},
			},
			expected: `GRANT USAGE ON CONNECTION "dev"."public"."kafka_conn" TO "testuser"`,
		},
		{
			name: "wildcard all connections",
			priv: &risingwavev1alpha1.NestedConnectionPrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.ConnectionPrivilegeType{risingwavev1alpha1.ConnectionPrivilegeUsage},
			},
			expected: `GRANT USAGE ON ALL CONNECTIONS IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedConnectionPrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedConnectionPrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantNestedSecretPrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedSecretPrivilege
		expected string
	}{
		{
			name: "specific secret",
			priv: &risingwavev1alpha1.NestedSecretPrivilege{
				Name:       "api_key",
				Privileges: []risingwavev1alpha1.SecretPrivilegeType{risingwavev1alpha1.SecretPrivilegeUsage},
			},
			expected: `GRANT USAGE ON SECRET "dev"."public"."api_key" TO "testuser"`,
		},
		{
			name: "wildcard all secrets",
			priv: &risingwavev1alpha1.NestedSecretPrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.SecretPrivilegeType{risingwavev1alpha1.SecretPrivilegeUsage},
			},
			expected: `GRANT USAGE ON ALL SECRETS IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedSecretPrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedSecretPrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantNestedFunctionPrivilege(t *testing.T) {
	tests := []struct {
		name     string
		priv     *risingwavev1alpha1.NestedFunctionPrivilege
		expected string
	}{
		{
			name: "specific function",
			priv: &risingwavev1alpha1.NestedFunctionPrivilege{
				Name:       "calculate_total",
				Privileges: []risingwavev1alpha1.FunctionPrivilegeType{risingwavev1alpha1.FunctionPrivilegeExecute},
			},
			expected: `GRANT EXECUTE ON FUNCTION "dev"."public"."calculate_total" TO "testuser"`,
		},
		{
			name: "wildcard all functions",
			priv: &risingwavev1alpha1.NestedFunctionPrivilege{
				Name:       "*",
				Privileges: []risingwavev1alpha1.FunctionPrivilegeType{risingwavev1alpha1.FunctionPrivilegeExecute},
			},
			expected: `GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA "public" TO "testuser"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantNestedFunctionPrivilege("testuser", "dev", "public", tt.priv)
			if got != tt.expected {
				t.Errorf("buildGrantNestedFunctionPrivilege() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildRevokeNestedSchemaPrivilege(t *testing.T) {
	priv := &risingwavev1alpha1.NestedSchemaPrivilege{
		Name:       "public",
		Privileges: []risingwavev1alpha1.SchemaPrivilegeType{risingwavev1alpha1.SchemaPrivilegeUsage},
	}

	got := buildRevokeNestedSchemaPrivilege("testuser", priv)
	expected := `REVOKE USAGE ON SCHEMA "public" FROM "testuser"`

	if got != expected {
		t.Errorf("buildRevokeNestedSchemaPrivilege() = %q, want %q", got, expected)
	}
}

func TestBuildGrantOption(t *testing.T) {
	tests := []struct {
		name      string
		withGrant bool
		expected  string
	}{
		{
			name:      "without grant option",
			withGrant: false,
			expected:  "",
		},
		{
			name:      "with grant option",
			withGrant: true,
			expected:  " WITH GRANT OPTION",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildGrantOption(tt.withGrant)
			if got != tt.expected {
				t.Errorf("buildGrantOption() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildGrantStatementsByDatabase(t *testing.T) {
	tests := []struct {
		name   string
		spec   *risingwavev1alpha1.RisingWaveUserSpec
		verify func(t *testing.T, result []DatabaseGroupedStatements)
	}{
		{
			name: "single database with table privileges",
			spec: &risingwavev1alpha1.RisingWaveUserSpec{
				Grants: &risingwavev1alpha1.GrantSpec{
					Databases: []risingwavev1alpha1.DatabasePrivilege{
						{
							Name: "dev",
							Schemas: []risingwavev1alpha1.NestedSchemaPrivilege{
								{
									Name: "public",
									Tables: []risingwavev1alpha1.NestedTablePrivilege{
										{Name: "orders", Privileges: []risingwavev1alpha1.TablePrivilegeType{risingwavev1alpha1.TablePrivilegeSelect}},
									},
								},
							},
						},
					},
				},
			},
			verify: func(t *testing.T, result []DatabaseGroupedStatements) {
				if len(result) != 1 {
					t.Errorf("expected 1 group, got %d", len(result))
				}
				if result[0].Database != "dev" {
					t.Errorf("expected database 'dev', got '%s'", result[0].Database)
				}
				if len(result[0].Statements) != 1 {
					t.Errorf("expected 1 statement, got %d", len(result[0].Statements))
				}
				expected := `GRANT SELECT ON TABLE "dev"."public"."orders" TO "testuser"`
				if result[0].Statements[0] != expected {
					t.Errorf("expected %q, got %q", expected, result[0].Statements[0])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildGrantStatementsByDatabase("testuser", tt.spec)
			if tt.verify != nil {
				tt.verify(t, got)
			}
		})
	}
}

func TestDatabaseGroupedStatementsStruct(t *testing.T) {
	group := DatabaseGroupedStatements{
		Database: "dev",
		Statements: []string{
			`GRANT SELECT ON TABLE "dev"."public"."orders" TO "testuser"`,
			`GRANT SELECT ON TABLE "dev"."public"."customers" TO "testuser"`,
		},
	}

	if group.Database != "dev" {
		t.Errorf("Expected database 'dev', got '%s'", group.Database)
	}

	if len(group.Statements) != 2 {
		t.Errorf("Expected 2 statements, got %d", len(group.Statements))
	}
}
