/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */
package quotaownership

import (
	"encoding/json"
	"fmt"
	kai "github.com/kai-scheduler/kai-resource-management-api/kai/v1alpha1"
)

// Control is read from protected metadata, including when evaluating stale commands.
type Control struct {
	Source   string `json:"source,omitempty"`
	Phase    string `json:"phase,omitempty"`
	Revision int64  `json:"revision,omitempty"`
	Epoch    string `json:"epoch"`
}

func ParseControl(value string) (Control, error) {
	var c Control
	if err := json.Unmarshal([]byte(value), &c); err != nil {
		return c, fmt.Errorf("decode quota control: %w", err)
	}
	if c.Revision == 0 {
		if c.Source != "" || c.Phase != "" {
			return c, fmt.Errorf("legacy quota control must omit native fields")
		}
		c.Source = "kubernetes"
		c.Phase = "stable"
		return c, nil
	}
	if c.Revision < 0 || (c.Source != "runai" && c.Source != "kubernetes") || (c.Phase != "freeze" && c.Phase != "release" && c.Phase != "stable") {
		return c, fmt.Errorf("invalid quota control")
	}
	return c, nil
}

// Fence also runs in admission, so even a delayed trusted-agent write cannot regress control.
func Fence(old, next Control, present bool) error {
	if !present {
		if next.Revision > 0 && next.Phase != "freeze" {
			return fmt.Errorf("native quota control must start frozen")
		}
		return nil
	}
	if old.Revision == 0 && next.Revision == 0 {
		return nil
	}
	if next.Revision < old.Revision {
		return fmt.Errorf("stale quota control revision")
	}
	if next.Revision == old.Revision {
		if old != next {
			return fmt.Errorf("conflicting quota control revision")
		}
		return nil
	}
	if old.Phase == "stable" && next.Phase == "freeze" && old.Epoch != next.Epoch {
		return nil
	}
	if old.Epoch == next.Epoch && old.Source == "runai" && next.Source == "runai" && old.Phase == "stable" && next.Phase == "stable" {
		return nil
	}
	if old.Epoch == next.Epoch && old.Source == next.Source && ((old.Phase == "freeze" && next.Phase == "release") || (old.Phase == "release" && next.Phase == "stable")) {
		return nil
	}
	return fmt.Errorf("invalid quota control phase or epoch")
}

func copyNumbers(to *kai.SystemResource, from kai.SystemResource) {
	to.Deserved = from.Deserved
	to.Limit = from.Limit
	to.OverQuotaWeight = from.OverQuotaWeight
}
