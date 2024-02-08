package handler

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Managed struct {
	APIVersion string        `json:"apiVersion"`
	Kind       string        `json:"kind"`
	Spec       Spec          `json:"spec"`
	Metadata   v1.ObjectMeta `json:"metadata"`
	Status     Status        `json:"status"`
}

type Status struct {
	AtProvider map[string]any `json:"forProvider"`
	Conditions []Condition    `json:"conditions"`
}

type Condition struct {
	LastTransitionTime string `json:"lastTransitionTime"`
	Message            string `json:"message"`
	Reason             string `json:"reason"`
	Status             string `json:"status"`
	Type               string `json:"type"`
}

type Spec struct {
	ForProvider map[string]any `json:"forProvider"`
}
