/*
 * Copyright 2023 RisingWave Labs
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package rwclient

import (
	"reflect"
	"sort"
	"testing"

	risingwavev1alpha1 "github.com/risingwavelabs/risingwave-operator/apis/risingwave/v1alpha1"
)

func TestCalculateDatabaseDiff(t *testing.T) {
	userName := "test-user"
	tests := []struct {
		name    string
		actual  DatabasePrivilegeSnapshot
		desired *risingwavev1alpha1.DatabasePrivilege
		want    PrivilegeDiff
	}{
		{
			name: "no change",
			actual: DatabasePrivilegeSnapshot{
				Name:       "db",
				Privileges: []string{"CONNECT"},
			},
			desired: &risingwavev1alpha1.DatabasePrivilege{
				Name:       "db",
				Privileges: []risingwavev1alpha1.DatabasePrivilegeType{"CONNECT"},
			},
			want: PrivilegeDiff{},
		},
		{
			name:   "add privilege",
			actual: DatabasePrivilegeSnapshot{Name: "db"},
			desired: &risingwavev1alpha1.DatabasePrivilege{
				Name:       "db",
				Privileges: []risingwavev1alpha1.DatabasePrivilegeType{"CONNECT"},
			},
			want: PrivilegeDiff{
				ToGrant: []string{"GRANT CONNECT ON DATABASE \"db\" TO \"test-user\""},
			},
		},
		{
			name: "revoke privilege",
			actual: DatabasePrivilegeSnapshot{
				Name:       "db",
				Privileges: []string{"CONNECT", "CREATE"},
			},
			desired: &risingwavev1alpha1.DatabasePrivilege{
				Name:       "db",
				Privileges: []risingwavev1alpha1.DatabasePrivilegeType{"CONNECT"},
			},
			want: PrivilegeDiff{
				ToRevoke: []string{"REVOKE CREATE ON DATABASE \"db\" FROM \"test-user\""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateDatabaseDiff(userName, tt.actual, tt.desired); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CalculateDatabaseDiff() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCalculateObjectDiff(t *testing.T) {
	tests := []struct {
		name         string
		userName     string
		schemaName   string
		objectType   string
		actual       ObjectPrivilege
		desiredName  string
		desiredPrivs []string
		expected     PrivilegeDiff
	}{
		{
			name:       "grant new privileges",
			userName:   "testuser",
			schemaName: "public",
			objectType: "TABLE",
			actual: ObjectPrivilege{
				Name:       "t1",
				Privileges: []string{},
			},
			desiredName:  "t1",
			desiredPrivs: []string{"SELECT", "INSERT"},
			expected: PrivilegeDiff{
				ToGrant: []string{`GRANT INSERT, SELECT ON TABLE "t1" TO "testuser"`},
			},
		},
		{
			name:       "revoke removed privileges",
			userName:   "testuser",
			schemaName: "public",
			objectType: "TABLE",
			actual: ObjectPrivilege{
				Name:       "t1",
				Privileges: []string{"SELECT", "INSERT", "UPDATE"},
			},
			desiredName:  "t1",
			desiredPrivs: []string{"SELECT"},
			expected: PrivilegeDiff{
				ToRevoke: []string{`REVOKE INSERT, UPDATE ON TABLE "t1" FROM "testuser"`},
			},
		},
		{
			name:       "mix grant and revoke",
			userName:   "testuser",
			schemaName: "public",
			objectType: "TABLE",
			actual: ObjectPrivilege{
				Name:       "t1",
				Privileges: []string{"SELECT", "INSERT"},
			},
			desiredName:  "t1",
			desiredPrivs: []string{"SELECT", "UPDATE"},
			expected: PrivilegeDiff{
				ToGrant:  []string{`GRANT UPDATE ON TABLE "t1" TO "testuser"`},
				ToRevoke: []string{`REVOKE INSERT ON TABLE "t1" FROM "testuser"`},
			},
		},
		{
			name:       "wildcard table grant",
			userName:   "testuser",
			schemaName: "public",
			objectType: "TABLE",
			actual: ObjectPrivilege{
				Name:       "*",
				Privileges: []string{},
			},
			desiredName:  "*",
			desiredPrivs: []string{"SELECT"},
			expected: PrivilegeDiff{
				ToGrant: []string{`GRANT SELECT ON ALL TABLES IN SCHEMA "public" TO "testuser"`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateObjectDiff(tt.userName, tt.schemaName, tt.objectType, tt.actual, tt.desiredName, tt.desiredPrivs)

			// Sort slices for comparison
			sort.Strings(got.ToGrant)
			sort.Strings(tt.expected.ToGrant)
			sort.Strings(got.ToRevoke)
			sort.Strings(tt.expected.ToRevoke)

			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("CalculateObjectDiff() = %v, want %v", got, tt.expected)
			}
		})
	}
}
