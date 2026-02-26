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
)

func TestParseACL(t *testing.T) {
	tests := []struct {
		name   string
		aclStr string
		want   []UserPrivileges
	}{
		{
			name:   "empty",
			aclStr: "{}",
			want:   nil,
		},
		{
			name:   "single user single priv",
			aclStr: "{user1=r/root}",
			want: []UserPrivileges{
				{
					User: "user1",
					Privileges: []PrivilegeGrant{
						{Privilege: "r", WithGrantOption: false},
					},
				},
			},
		},
		{
			name:   "single user multiple privs",
			aclStr: "{user1=arwd/root}",
			want: []UserPrivileges{
				{
					User: "user1",
					Privileges: []PrivilegeGrant{
						{Privilege: "a", WithGrantOption: false},
						{Privilege: "r", WithGrantOption: false},
						{Privilege: "w", WithGrantOption: false},
						{Privilege: "d", WithGrantOption: false},
					},
				},
			},
		},
		{
			name:   "multiple users",
			aclStr: "{user1=r/root,user2=rw/root}",
			want: []UserPrivileges{
				{
					User: "user1",
					Privileges: []PrivilegeGrant{
						{Privilege: "r", WithGrantOption: false},
					},
				},
				{
					User: "user2",
					Privileges: []PrivilegeGrant{
						{Privilege: "r", WithGrantOption: false},
						{Privilege: "w", WithGrantOption: false},
					},
				},
			},
		},
		{
			name:   "with grant option",
			aclStr: "{user1=r*w/root}",
			want: []UserPrivileges{
				{
					User: "user1",
					Privileges: []PrivilegeGrant{
						{Privilege: "r", WithGrantOption: true},
						{Privilege: "w", WithGrantOption: false},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseACL(tt.aclStr); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseACL() = %v, want %v", got, tt.want)
			}
		})
	}
}
