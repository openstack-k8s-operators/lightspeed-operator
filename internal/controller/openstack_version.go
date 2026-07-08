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
	"github.com/Masterminds/semver/v3"
	"github.com/go-logr/logr"
)

// ocpVersionBound maps an OCP version upper bound to a RHOSO version.
type ocpVersionBound struct {
	maxOCPVersion string
	rhosoVersion  string
}

// ocpToRHOSOVersionMap maps OCP version upper bounds to RHOSO versions.
// Entries must be in ascending order of maxOCPVersion.
// Versions above the highest bound fall back to OKPDefaultRHOSOVersion.
// Add a new entry here when a new RHOSO version's content becomes available in the knowledge base.
var ocpToRHOSOVersionMap = []ocpVersionBound{
	{"4.21", "18.0"},
	// When RHOSO 19.0 content is available, add: {"4.XX", "19.0"}
}

// detectRHOSOVersion returns the RHOSO version corresponding to the given OCP version.
// Falls back to OKPDefaultRHOSOVersion if the version cannot be parsed or is above all defined bounds.
func detectRHOSOVersion(ocpVersion string, logger logr.Logger) string {
	detected, err := semver.NewVersion(ocpVersion)
	if err != nil {
		logger.Info("Failed to parse OCP version, using default RHOSO version",
			"ocpVersion", ocpVersion, "default", OKPDefaultRHOSOVersion)
		return OKPDefaultRHOSOVersion
	}

	for _, entry := range ocpToRHOSOVersionMap {
		bound, err := semver.NewVersion(entry.maxOCPVersion)
		if err != nil {
			logger.Info("Invalid bound in RHOSO version map, using default",
				"bound", entry.maxOCPVersion, "default", OKPDefaultRHOSOVersion)
			return OKPDefaultRHOSOVersion
		}
		if detected.Compare(bound) <= 0 {
			return entry.rhosoVersion
		}
	}

	logger.Info("OCP version above all known bounds, using default RHOSO version",
		"ocpVersion", ocpVersion, "default", OKPDefaultRHOSOVersion)
	return OKPDefaultRHOSOVersion
}
