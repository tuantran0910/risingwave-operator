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

package utils

import (
	"fmt"
	"testing"
)

func TestGenerateRandomPassword(t *testing.T) {
	// Test default length (16 chars)
	password1 := GenerateRandomPassword(0)
	if len(password1) != 16 {
		t.Errorf("GenerateRandomPassword() with default length returned %d chars, expected 16", len(password1))
	}

	// Test specific lengths
	for length := 8; length <= 32; length += 8 {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			password := GenerateRandomPassword(length)
			if len(password) != length {
				t.Errorf("GenerateRandomPassword(%d) returned %d chars, expected %d", length, len(password), length)
			}
			// Verify password contains at least one of each character type
			hasUpper := false
			hasLower := false
			hasDigit := false
			for _, c := range password {
				switch {
				case c >= 'A' && c <= 'Z':
					hasUpper = true
				case c >= 'a' && c <= 'z':
					hasLower = true
				case c >= '0' && c <= '9':
					hasDigit = true
				}
			}
			if !hasUpper || !hasLower || !hasDigit {
				t.Errorf("Password %q doesn't contain all required character types", password)
			}
		})
	}
}

func TestGenerateRandomPasswordUniqueness(t *testing.T) {
	// Generate multiple passwords and verify they're different
	passwords := make(map[string]bool)
	iterations := 100
	for i := 0; i < iterations; i++ {
		password := GenerateRandomPassword(16)
		if passwords[password] {
			t.Errorf("Generated duplicate password after %d iterations", i)
		}
		passwords[password] = true
	}
}

func TestGenerateRandomPasswordCharacterDistribution(t *testing.T) {
	// Test that passwords have reasonable character distribution
	iterations := 1000
	charCounts := make(map[rune]int)
	for i := 0; i < iterations; i++ {
		password := GenerateRandomPassword(16)
		for _, c := range password {
			charCounts[c]++
		}
	}

	// Check distribution is reasonable (no single character > 20% of total)
	for char, count := range charCounts {
		percentage := float64(count) / float64(iterations*16) * 100
		if percentage > 20 {
			t.Errorf("Character %c appears %.1f%% of time, exceeds 20%% threshold", char, percentage)
		}
	}
}
