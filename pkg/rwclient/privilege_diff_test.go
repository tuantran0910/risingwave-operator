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
	userName := "test-user"
	tests := []struct {
		name         string
		objectType   string
		actual       ObjectPrivilege
		desiredName  string
		desiredPrivs []string
		want         PrivilegeDiff
	}{
		{
			name:       "revoke select",
			objectType: "TABLE",
			actual: ObjectPrivilege{
				Name:       "tab",
				Privileges: []string{"SELECT", "INSERT"},
			},
			desiredName:  "tab",
			desiredPrivs: []string{"INSERT"},
			want: PrivilegeDiff{
				ToRevoke: []string{"REVOKE SELECT ON TABLE \"tab\" FROM \"test-user\""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateObjectDiff(userName, tt.objectType, tt.actual, tt.desiredName, tt.desiredPrivs); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CalculateObjectDiff() = %v, want %v", got, tt.want)
			}
		})
	}
}
