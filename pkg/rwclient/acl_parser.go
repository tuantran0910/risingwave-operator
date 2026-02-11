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
	"strings"

	risingwavev1alpha1 "github.com/risingwavelabs/risingwave-operator/apis/risingwave/v1alpha1"
)

// PrivilegeGrant represents a single privilege grant for a user.
type PrivilegeGrant struct {
	Privilege       string
	WithGrantOption bool
}

// UserPrivileges represents the set of privileges a user has on an object.
type UserPrivileges struct {
	User       string
	Privileges []PrivilegeGrant
}

// ParseACL parses a RisingWave ACL string (e.g., "{user1=arwd/root,user2=r/root}").
func ParseACL(aclStr string) []UserPrivileges {
	aclStr = strings.Trim(aclStr, "{}")
	if aclStr == "" {
		return nil
	}

	var results []UserPrivileges
	entries := strings.Split(aclStr, ",")
	for _, entry := range entries {
		// entry format: grantee=privs/grantor
		parts := strings.Split(entry, "=")
		if len(parts) != 2 {
			continue
		}

		grantee := parts[0]
		remaining := parts[1]

		// split privs and grantor
		privsGrantor := strings.Split(remaining, "/")
		if len(privsGrantor) != 2 {
			continue
		}

		privsStr := privsGrantor[0]

		up := UserPrivileges{
			User: grantee,
		}

		for i := 0; i < len(privsStr); i++ {
			char := string(privsStr[i])
			withGrantOption := false
			if i+1 < len(privsStr) && privsStr[i+1] == '*' {
				withGrantOption = true
				i++
			}

			up.Privileges = append(up.Privileges, PrivilegeGrant{
				Privilege:       char,
				WithGrantOption: withGrantOption,
			})
		}
		results = append(results, up)
	}

	return results
}

// MapCharToTablePrivilege maps ACL characters to v1alpha1.TablePrivilegeType.
func MapCharToTablePrivilege(char string) risingwavev1alpha1.TablePrivilegeType {
	switch char {
	case "r":
		return risingwavev1alpha1.TablePrivilegeSelect
	case "a":
		return risingwavev1alpha1.TablePrivilegeInsert
	case "w":
		return risingwavev1alpha1.TablePrivilegeUpdate
	case "d":
		return risingwavev1alpha1.TablePrivilegeDelete
	case "D":
		return risingwavev1alpha1.TablePrivilegeTruncate
	case "x":
		return risingwavev1alpha1.TablePrivilegeReferences
	case "t":
		return risingwavev1alpha1.TablePrivilegeTrigger
	default:
		return ""
	}
}

// MapCharToDatabasePrivilege maps ACL characters to v1alpha1.DatabasePrivilegeType.
func MapCharToDatabasePrivilege(char string) risingwavev1alpha1.DatabasePrivilegeType {
	switch char {
	case "c":
		return risingwavev1alpha1.DatabasePrivilegeConnect
	case "C":
		return risingwavev1alpha1.DatabasePrivilegeCreate
	default:
		return ""
	}
}

// MapCharToSchemaPrivilege maps ACL characters to v1alpha1.SchemaPrivilegeType.
func MapCharToSchemaPrivilege(char string) risingwavev1alpha1.SchemaPrivilegeType {
	switch char {
	case "U":
		return risingwavev1alpha1.SchemaPrivilegeUsage
	case "C":
		return risingwavev1alpha1.SchemaPrivilegeCreate
	default:
		return ""
	}
}
