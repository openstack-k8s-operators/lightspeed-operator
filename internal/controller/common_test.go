/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"
)

func TestGenerateRandomStringLength(t *testing.T) {
	t.Run("Below minimum length returns error", func(t *testing.T) {
		_, err := generateRandomString(15)
		if err == nil {
			t.Error("generateRandomString(15) expected error, got nil")
		}
	})

	tests := []struct {
		name   string
		length int
	}{
		{name: "Min length 16", length: 16},
		{name: "Even length 32", length: 32},
		{name: "Odd length 33", length: 33},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := generateRandomString(tt.length)
			if err != nil {
				t.Errorf("generateRandomString(%d) unexpected error: %v", tt.length, err)
			}
			if len(result) != tt.length {
				t.Errorf("generateRandomString(%d) returned length %d, want %d", tt.length, len(result), tt.length)
			}
		})
	}
}

func TestGenerateRandomStringCharacters(t *testing.T) {
	result, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("generateRandomString(32) unexpected error: %v", err)
	}

	var hasLower, hasUpper, hasDigit bool
	for i, c := range result {
		switch {
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= '0' && c <= '9':
			hasDigit = true
		default:
			t.Errorf("generateRandomString(32) character at index %d is %q, not alphanumeric", i, c)
		}
	}
	if !hasLower {
		t.Error("generateRandomString(32) result has no lowercase letter")
	}
	if !hasUpper {
		t.Error("generateRandomString(32) result has no uppercase letter")
	}
	if !hasDigit {
		t.Error("generateRandomString(32) result has no digit")
	}
}

func TestGenerateRandomStringUniqueness(t *testing.T) {
	const length = 16
	a, err := generateRandomString(length)
	if err != nil {
		t.Fatalf("first call unexpected error: %v", err)
	}
	b, err := generateRandomString(length)
	if err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}
	if a == b {
		t.Errorf("generateRandomString(%d) returned identical values across two calls: %q", length, a)
	}
}
