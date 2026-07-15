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

package v1beta1

import (
	"fmt"
	"reflect"
	"testing"
)

// TestOpenStackLightspeedImagesFieldTypes guards the mergeImages
// reflection-based implementation against future struct changes.
// mergeImages uses reflect and IsZero to copy non-zero fields; this
// only works correctly for simple types (string, int) where the zero
// value reliably means "not set". Adding an unexported field, or a
// complex type (slice, map, pointer, struct), would cause silent
// misbehavior or a panic.
func TestOpenStackLightspeedImagesFieldTypes(t *testing.T) {
	allowedKinds := map[reflect.Kind]bool{
		reflect.String: true,
	}

	typ := reflect.TypeOf(OpenStackLightspeedImages{})
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		if !field.IsExported() {
			t.Errorf("field %q is unexported; mergeImages uses reflect to set fields "+
				"and cannot write unexported fields (will panic)", field.Name)
			continue
		}

		if !allowedKinds[field.Type.Kind()] {
			t.Errorf("field %q has type %s (kind %s); mergeImages relies on IsZero "+
				"to detect unset values, which is only reliable for string — "+
				"add handling in mergeImages before using this type",
				field.Name, field.Type, field.Type.Kind())
		}
	}
}

func TestMergeImages(t *testing.T) {
	dst := OpenStackLightspeedImages{
		RAGImageURL:   "original-rag",
		LCoreImageURL: "original-lcore",
	}
	src := OpenStackLightspeedImages{
		RAGImageURL:       "override-rag",
		ExporterImageURL:  "override-exporter",
		MCPServerImageURL: "override-mcp",
	}

	mergeImages(&dst, &src)

	checks := []struct {
		name string
		got  string
		want string
	}{
		{"RAGImageURL (overridden)", dst.RAGImageURL, "override-rag"},
		{"LCoreImageURL (kept)", dst.LCoreImageURL, "original-lcore"},
		{"ExporterImageURL (set from zero)", dst.ExporterImageURL, "override-exporter"},
		{"MCPServerImageURL (set from zero)", dst.MCPServerImageURL, "override-mcp"},
		{"PostgresImageURL (both zero)", dst.PostgresImageURL, ""},
	}
	for _, tc := range checks {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestMergeImages_EmptySrc(t *testing.T) {
	dst := OpenStackLightspeedImages{
		RAGImageURL: "keep-this",
	}
	src := OpenStackLightspeedImages{}

	mergeImages(&dst, &src)

	if dst.RAGImageURL != "keep-this" {
		t.Errorf("RAGImageURL changed unexpectedly to %q", dst.RAGImageURL)
	}
}

func TestMergeDefaults_GlobalWriteBackDoesNotCorruptBase(t *testing.T) {
	SetupDefaults()
	original := OpenStackLightspeedDefaultValues

	specImages := &OpenStackLightspeedImages{RAGImageURL: "custom-rag"}
	merged := MergeDefaults(specImages)
	OpenStackLightspeedDefaultValues = merged

	reverted := MergeDefaults(nil)
	if reverted.RAGImageURL != original.RAGImageURL {
		t.Errorf("RAGImageURL not reverted: got %q, want %q",
			reverted.RAGImageURL, original.RAGImageURL)
	}
}

func TestMergeImages_AllFields(t *testing.T) {
	// Verify mergeImages touches every field by setting all src fields
	// to non-zero and confirming they all arrive in dst.
	typ := reflect.TypeOf(OpenStackLightspeedImages{})
	src := OpenStackLightspeedImages{}
	srcVal := reflect.ValueOf(&src).Elem()
	for i := 0; i < typ.NumField(); i++ {
		f := srcVal.Field(i)
		f.SetString(fmt.Sprintf("val-%d", i))
	}

	dst := OpenStackLightspeedImages{}
	mergeImages(&dst, &src)

	if !reflect.DeepEqual(dst, src) {
		t.Errorf("after merging fully-populated src into zero dst, values differ:\n  dst: %+v\n  src: %+v", dst, src)
	}
}
