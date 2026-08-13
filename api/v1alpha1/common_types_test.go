/*
Copyright 2024.

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

package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGroupVersionKind_GVK(t *testing.T) {
	tests := []struct {
		name     string
		gvk      GroupVersionKindTarget
		expected schema.GroupVersionKind
	}{

		{
			name: "All fields empty",
			gvk:  GroupVersionKindTarget{},
			expected: schema.GroupVersionKind{
				Group:   "",
				Version: "",
				Kind:    "",
			},
		},
		{
			name: "Only Group field set",
			gvk: GroupVersionKindTarget{
				Group: "some-group",
			},
			expected: schema.GroupVersionKind{
				Group:   "some-group",
				Version: "",
				Kind:    "",
			},
		},
		{
			name: "Only Version and Kind fields set",
			gvk: GroupVersionKindTarget{
				Version: "v2",
				Kind:    "SomeKind",
			},
			expected: schema.GroupVersionKind{
				Group:   "",
				Version: "v2",
				Kind:    "SomeKind",
			},
		},
		{
			name: "Group and Version and Kind fields set",
			gvk: GroupVersionKindTarget{
				Group:   "batch",
				Version: "v1",
				Kind:    "Job",
			},
			expected: schema.GroupVersionKind{
				Group:   "batch",
				Version: "v1",
				Kind:    "Job",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.gvk.GVK()
			assert.Equal(t, tt.expected, result)
		})
	}
}
