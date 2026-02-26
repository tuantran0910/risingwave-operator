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
	"fmt"
	"strings"

	risingwavev1alpha1 "github.com/risingwavelabs/risingwave-operator/apis/risingwave/v1alpha1"
)

// QuoteIdentifier quotes an SQL identifier (table name, column name, etc.).
func QuoteIdentifier(name string) string {
	if name == "*" {
		return name
	}
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(name, "\"", "\"\""))
}

// QuoteUser quotes a user name for SQL.
func QuoteUser(name string) string {
	return fmt.Sprintf("\"%s\"", strings.ReplaceAll(name, "\"", "\"\""))
}

// BuildCreateUserSQL builds a CREATE USER statement.
func BuildCreateUserSQL(user *risingwavev1alpha1.RisingWaveUser, password string) string {
	var sb strings.Builder

	sb.WriteString("CREATE USER ")
	sb.WriteString(QuoteUser(getUserName(user)))

	if password != "" {
		sb.WriteString(" WITH PASSWORD '")
		sb.WriteString(escapeStringLiteral(password))
		sb.WriteString("'")
	}

	// Add user permissions
	for _, perm := range user.Spec.Permissions {
		sb.WriteString(" ")
		sb.WriteString(string(perm))
	}

	return sb.String()
}

// BuildAlterUserPasswordSQL builds an ALTER USER ... WITH PASSWORD statement.
func BuildAlterUserPasswordSQL(userName string, password string) string {
	return fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s'",
		QuoteUser(userName),
		escapeStringLiteral(password))
}

// BuildDropUserSQL builds a DROP USER statement.
func BuildDropUserSQL(userName string) string {
	return fmt.Sprintf("DROP USER IF EXISTS %s", QuoteUser(userName))
}

// BuildCreateUserWithOAuthSQL builds a CREATE USER statement with OAuth authentication.
func BuildCreateUserWithOAuthSQL(userName string, oauth *risingwavev1alpha1.OAuthConfig) string {
	var sb strings.Builder

	sb.WriteString("CREATE USER ")
	sb.WriteString(QuoteUser(userName))
	sb.WriteString(" WITH CREATE_USER_ONLY")

	return sb.String()
}

// BuildAlterUserOAuthSQL builds ALTER USER statements for OAuth configuration.
// Note: Currently returns empty slice as OAuth is configured at cluster level in RisingWave.
// This function is a placeholder for future extensions if needed.
func BuildAlterUserOAuthSQL(userName string, oauth *risingwavev1alpha1.OAuthConfig) []string {
	var statements []string

	// RisingWave uses CREATE USER ONLY for OAuth, then alters with authentication method
	// The actual OAuth configuration is set at cluster level
	// Users created with OAuth can log in via JWT tokens

	return statements
}

// BuildCreateUserWithLDAPSQL builds a CREATE USER statement with LDAP authentication.
func BuildCreateUserWithLDAPSQL(userName string, ldap *risingwavev1alpha1.LDAPConfig) string {
	var sb strings.Builder

	sb.WriteString("CREATE USER ")
	sb.WriteString(QuoteUser(userName))
	sb.WriteString(" WITH CREATE_USER_ONLY")

	return sb.String()
}

// BuildGrantStatements builds GRANT statements for all grants in spec.
func BuildGrantStatements(userName string, spec *risingwavev1alpha1.RisingWaveUserSpec) []string {
	var statements []string

	if spec.Grants == nil {
		return statements
	}

	// Process hierarchical structure (DatabasePrivilege with nested Schemas)
	for _, dbPriv := range spec.Grants.Databases {
		statements = append(statements, buildDatabasePrivilegesHierarchical(userName, &dbPriv)...)
	}

	return statements
}

// buildDatabasePrivilegesHierarchical recursively builds GRANT statements for hierarchical structure.
func buildDatabasePrivilegesHierarchical(userName string, dbPriv *risingwavev1alpha1.DatabasePrivilege) []string {
	var statements []string

	// Database-level privileges
	if len(dbPriv.Privileges) > 0 {
		stmt := buildGrantDatabasePrivilege(userName, dbPriv)
		statements = append(statements, stmt)
	}

	// Nested schema-level privileges
	for _, schemaPriv := range dbPriv.Schemas {
		statements = append(statements, buildSchemaPrivilegesHierarchical(userName, dbPriv.Name, &schemaPriv)...)
	}

	return statements
}

// buildSchemaPrivilegesHierarchical recursively builds GRANT statements for nested schema objects.
func buildSchemaPrivilegesHierarchical(userName string, database string, schemaPriv *risingwavev1alpha1.NestedSchemaPrivilege) []string {
	var statements []string

	// Schema-level privileges
	if len(schemaPriv.Privileges) > 0 {
		stmt := buildGrantNestedSchemaPrivilege(userName, schemaPriv)
		statements = append(statements, stmt)
	}

	// Nested table privileges
	for _, tablePriv := range schemaPriv.Tables {
		statements = append(statements, buildGrantNestedTablePrivilege(userName, database, schemaPriv.Name, &tablePriv))
	}

	// Nested view privileges
	for _, viewPriv := range schemaPriv.Views {
		statements = append(statements, buildGrantNestedViewPrivilege(userName, database, schemaPriv.Name, &viewPriv))
	}

	// Nested materialized view privileges
	for _, mvPriv := range schemaPriv.MaterializedViews {
		statements = append(statements, buildGrantNestedMaterializedViewPrivilege(userName, database, schemaPriv.Name, &mvPriv))
	}

	// Nested source privileges
	for _, sourcePriv := range schemaPriv.Sources {
		statements = append(statements, buildGrantNestedSourcePrivilege(userName, database, schemaPriv.Name, &sourcePriv))
	}

	// Nested sink privileges
	for _, sinkPriv := range schemaPriv.Sinks {
		statements = append(statements, buildGrantNestedSinkPrivilege(userName, database, schemaPriv.Name, &sinkPriv))
	}

	// Nested connection privileges
	for _, connPriv := range schemaPriv.Connections {
		statements = append(statements, buildGrantNestedConnectionPrivilege(userName, database, schemaPriv.Name, &connPriv))
	}

	// Nested secret privileges
	for _, secretPriv := range schemaPriv.Secrets {
		statements = append(statements, buildGrantNestedSecretPrivilege(userName, database, schemaPriv.Name, &secretPriv))
	}

	// Nested function privileges
	for _, funcPriv := range schemaPriv.Functions {
		statements = append(statements, buildGrantNestedFunctionPrivilege(userName, database, schemaPriv.Name, &funcPriv))
	}

	return statements
}

// DatabaseGroupedStatements groups grant statements by database.
type DatabaseGroupedStatements struct {
	Database   string
	Statements []string
}

// BuildGrantStatementsByDatabase builds GRANT statements grouped by target database.
// Returns groups where Database is the target database for statements.
// Database-level privileges are grouped under empty string "".
func BuildGrantStatementsByDatabase(userName string, spec *risingwavev1alpha1.RisingWaveUserSpec) []DatabaseGroupedStatements {
	var grouped []DatabaseGroupedStatements

	if spec.Grants == nil {
		return grouped
	}

	// Process hierarchical structure
	for _, dbPriv := range spec.Grants.Databases {
		grouped = append(grouped, buildDatabasePrivilegesHierarchicalGrouped(userName, &dbPriv)...)
	}

	return grouped
}

// buildDatabasePrivilegesHierarchicalGrouped builds grouped GRANT statements for hierarchical structure.
func buildDatabasePrivilegesHierarchicalGrouped(userName string, dbPriv *risingwavev1alpha1.DatabasePrivilege) []DatabaseGroupedStatements {
	var grouped []DatabaseGroupedStatements

	// Database-level privileges - execute on current database (no switch needed)
	if len(dbPriv.Privileges) > 0 {
		stmt := buildGrantDatabasePrivilege(userName, dbPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   "",
			Statements: []string{stmt},
		})
	}

	// Nested schema-level privileges
	for _, schemaPriv := range dbPriv.Schemas {
		grouped = append(grouped, buildSchemaPrivilegesHierarchicalGrouped(userName, dbPriv.Name, &schemaPriv)...)
	}

	return grouped
}

// buildSchemaPrivilegesHierarchicalGrouped builds grouped GRANT statements for nested schema objects.
func buildSchemaPrivilegesHierarchicalGrouped(userName string, database string, schemaPriv *risingwavev1alpha1.NestedSchemaPrivilege) []DatabaseGroupedStatements {
	var grouped []DatabaseGroupedStatements

	// Schema-level privileges
	if len(schemaPriv.Privileges) > 0 {
		stmt := buildGrantNestedSchemaPrivilege(userName, schemaPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// All nested object types inherit the same database context
	// Tables
	for _, tablePriv := range schemaPriv.Tables {
		stmt := buildGrantNestedTablePrivilege(userName, database, schemaPriv.Name, &tablePriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// Views
	for _, viewPriv := range schemaPriv.Views {
		stmt := buildGrantNestedViewPrivilege(userName, database, schemaPriv.Name, &viewPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// Materialized Views
	for _, mvPriv := range schemaPriv.MaterializedViews {
		stmt := buildGrantNestedMaterializedViewPrivilege(userName, database, schemaPriv.Name, &mvPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// Sources
	for _, sourcePriv := range schemaPriv.Sources {
		stmt := buildGrantNestedSourcePrivilege(userName, database, schemaPriv.Name, &sourcePriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// Sinks
	for _, sinkPriv := range schemaPriv.Sinks {
		stmt := buildGrantNestedSinkPrivilege(userName, database, schemaPriv.Name, &sinkPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// Connections
	for _, connPriv := range schemaPriv.Connections {
		stmt := buildGrantNestedConnectionPrivilege(userName, database, schemaPriv.Name, &connPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// Secrets
	for _, secretPriv := range schemaPriv.Secrets {
		stmt := buildGrantNestedSecretPrivilege(userName, database, schemaPriv.Name, &secretPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	// Functions
	for _, funcPriv := range schemaPriv.Functions {
		stmt := buildGrantNestedFunctionPrivilege(userName, database, schemaPriv.Name, &funcPriv)
		grouped = append(grouped, DatabaseGroupedStatements{
			Database:   database,
			Statements: []string{stmt},
		})
	}

	return grouped
}

// BuildRevokeStatements builds REVOKE statements for all grants.
func BuildRevokeStatements(userName string, spec *risingwavev1alpha1.RisingWaveUserSpec) []string {
	var statements []string

	if spec.Grants == nil {
		return statements
	}

	// Process hierarchical structure
	for _, dbPriv := range spec.Grants.Databases {
		stmt := buildRevokeDatabasePrivilege(userName, &dbPriv)
		statements = append(statements, stmt)

		// Nested schemas
		for _, schemaPriv := range dbPriv.Schemas {
			if len(schemaPriv.Privileges) > 0 {
				stmt := buildRevokeNestedSchemaPrivilege(userName, &schemaPriv)
				statements = append(statements, stmt)
			}
		}
	}

	return statements
}

func buildGrantDatabasePrivilege(userName string, priv *risingwavev1alpha1.DatabasePrivilege) string {
	return fmt.Sprintf("GRANT %s ON DATABASE %s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildRevokeDatabasePrivilege(userName string, priv *risingwavev1alpha1.DatabasePrivilege) string {
	return fmt.Sprintf("REVOKE %s ON DATABASE %s FROM %s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName))
}

func buildGrantNestedSchemaPrivilege(userName string, priv *risingwavev1alpha1.NestedSchemaPrivilege) string {
	return fmt.Sprintf("GRANT %s ON SCHEMA %s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildRevokeNestedSchemaPrivilege(userName string, priv *risingwavev1alpha1.NestedSchemaPrivilege) string {
	return fmt.Sprintf("REVOKE %s ON SCHEMA %s FROM %s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName))
}

func buildGrantNestedTablePrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedTablePrivilege) string {
	if priv.Name == "*" {
		// ALL TABLES IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL TABLES IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON TABLE %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildGrantNestedViewPrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedViewPrivilege) string {
	if priv.Name == "*" {
		// ALL VIEWS IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL VIEWS IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON VIEW %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildGrantNestedMaterializedViewPrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedMaterializedViewPrivilege) string {
	if priv.Name == "*" {
		// ALL MATERIALIZED VIEWS IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL MATERIALIZED VIEWS IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON MATERIALIZED VIEW %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildGrantNestedSourcePrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedSourcePrivilege) string {
	if priv.Name == "*" {
		// ALL SOURCES IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL SOURCES IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON SOURCE %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildGrantNestedSinkPrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedSinkPrivilege) string {
	if priv.Name == "*" {
		// ALL SINKS IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL SINKS IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON SINK %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildGrantNestedConnectionPrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedConnectionPrivilege) string {
	if priv.Name == "*" {
		// ALL CONNECTIONS IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL CONNECTIONS IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON CONNECTION %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildGrantNestedSecretPrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedSecretPrivilege) string {
	if priv.Name == "*" {
		// ALL SECRETS IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL SECRETS IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON SECRET %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

func buildGrantNestedFunctionPrivilege(userName string, database, schema string, priv *risingwavev1alpha1.NestedFunctionPrivilege) string {
	if priv.Name == "*" {
		// ALL FUNCTIONS IN SCHEMA syntax
		return fmt.Sprintf("GRANT %s ON ALL FUNCTIONS IN SCHEMA %s TO %s%s",
			normalizePrivileges(priv.Privileges),
			QuoteIdentifier(schema),
			QuoteUser(userName),
			buildGrantOption(priv.WithGrantOption))
	}
	return fmt.Sprintf("GRANT %s ON FUNCTION %s.%s.%s TO %s%s",
		normalizePrivileges(priv.Privileges),
		QuoteIdentifier(database),
		QuoteIdentifier(schema),
		QuoteIdentifier(priv.Name),
		QuoteUser(userName),
		buildGrantOption(priv.WithGrantOption))
}

// normalizePrivileges normalizes privilege list to a comma-separated string.
// Handles "ALL PRIVILEGES" -> "ALL" normalization.
func normalizePrivileges[T ~string](privs []T) string {
	if len(privs) == 0 {
		return "USAGE"
	}

	var normalized []string
	for _, p := range privs {
		ps := string(p)
		if ps == "ALL PRIVILEGES" {
			ps = "ALL"
		}
		normalized = append(normalized, ps)
	}

	return strings.Join(normalized, ", ")
}

// buildGrantOption returns the "WITH GRANT OPTION" clause if enabled.
func buildGrantOption(withGrant bool) string {
	if withGrant {
		return " WITH GRANT OPTION"
	}
	return ""
}

// escapeStringLiteral escapes a string for use in SQL string literals.
func escapeStringLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// getUserName returns the actual user name from spec, defaulting to metadata.name.
func getUserName(user *risingwavev1alpha1.RisingWaveUser) string {
	if user.Spec.Name != "" {
		return user.Spec.Name
	}
	return user.Name
}

// BuildAlterUserPermissionsSQL builds ALTER USER statements for permission changes.
// Used to update user permissions when they change in RisingWaveUser spec.
func BuildAlterUserPermissionsSQL(userName string, permissions []risingwavev1alpha1.UserPermission) string {
	if len(permissions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("ALTER USER ")
	sb.WriteString(QuoteUser(userName))

	for _, perm := range permissions {
		sb.WriteString(" ")
		sb.WriteString(string(perm))
	}

	return sb.String()
}
