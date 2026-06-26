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

	"github.com/go-logr/logr"
)

func TestDetectRHOSOVersion(t *testing.T) {
	logger := logr.Discard()

	tests := []struct {
		name       string
		ocpVersion string
		expected   string
	}{
		{
			name:       "Version below bound returns mapped RHOSO version",
			ocpVersion: "4.16",
			expected:   "18.0",
		},
		{
			name:       "Version at bound returns mapped RHOSO version",
			ocpVersion: "4.21",
			expected:   "18.0",
		},
		{
			name:       "Version with patch at bound returns mapped RHOSO version",
			ocpVersion: "4.21.3",
			expected:   "18.0",
		},
		{
			name:       "Version above all known bounds falls back to default",
			ocpVersion: "4.22",
			expected:   OKPDefaultRHOSOVersion,
		},
		{
			name:       "Far future version falls back to default",
			ocpVersion: "5.0",
			expected:   OKPDefaultRHOSOVersion,
		},
		{
			name:       "Invalid version string falls back to default",
			ocpVersion: "not-a-version",
			expected:   OKPDefaultRHOSOVersion,
		},
		{
			name:       "Empty version string falls back to default",
			ocpVersion: "",
			expected:   OKPDefaultRHOSOVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectRHOSOVersion(tt.ocpVersion, logger)
			if result != tt.expected {
				t.Errorf("detectRHOSOVersion(%q) = %q, want %q", tt.ocpVersion, result, tt.expected)
			}
		})
	}
}

func TestDetectRHOSOVersionMapOrdering(t *testing.T) {
	logger := logr.Discard()

	// Save and restore the global map so this test is self-contained.
	original := ocpToRHOSOVersionMap
	t.Cleanup(func() { ocpToRHOSOVersionMap = original })

	ocpToRHOSOVersionMap = []ocpVersionBound{
		{"4.21", "18.0"},
		{"5.99", "19.0"},
	}

	tests := []struct {
		name       string
		ocpVersion string
		expected   string
	}{
		{
			name:       "Version matched by first entry",
			ocpVersion: "4.16",
			expected:   "18.0",
		},
		{
			name:       "Version at boundary of first entry",
			ocpVersion: "4.21",
			expected:   "18.0",
		},
		{
			name:       "Version matched by second entry",
			ocpVersion: "5.0",
			expected:   "19.0",
		},
		{
			name:       "Version at boundary of second entry",
			ocpVersion: "5.99",
			expected:   "19.0",
		},
		{
			name:       "Version above all bounds falls back to default",
			ocpVersion: "6.0",
			expected:   OKPDefaultRHOSOVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectRHOSOVersion(tt.ocpVersion, logger)
			if result != tt.expected {
				t.Errorf("detectRHOSOVersion(%q) = %q, want %q", tt.ocpVersion, result, tt.expected)
			}
		})
	}
}
