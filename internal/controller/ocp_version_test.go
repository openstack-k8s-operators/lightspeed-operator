/*
Copyright 2025.

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

func TestParseMajorMinorVersion(t *testing.T) {
	tests := []struct {
		name        string
		fullVersion string
		expected    string
		shouldError bool
	}{
		{
			name:        "Standard version",
			fullVersion: "4.16.0",
			expected:    "4.16",
			shouldError: false,
		},
		{
			name:        "Version with build",
			fullVersion: "4.18.0-0.nightly-2024-01-15-123456",
			expected:    "4.18",
			shouldError: false,
		},
		{
			name:        "Invalid version",
			fullVersion: "invalid",
			expected:    "",
			shouldError: true,
		},
		{
			name:        "Empty version",
			fullVersion: "",
			expected:    "",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseMajorMinorVersion(tt.fullVersion)
			if tt.shouldError {
				if err == nil {
					t.Errorf("ParseMajorMinorVersion(%s) expected error, got nil", tt.fullVersion)
				}
			} else {
				if err != nil {
					t.Errorf("ParseMajorMinorVersion(%s) unexpected error: %v", tt.fullVersion, err)
				}
				if result != tt.expected {
					t.Errorf("ParseMajorMinorVersion(%s) = %s, want %s", tt.fullVersion, result, tt.expected)
				}
			}
		})
	}
}
