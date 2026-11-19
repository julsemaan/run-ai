/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package testing

import (
	"testing"
)

func TestSharedCleanup(t *testing.T) {
	// Reset state for testing
	activeTests = 0

	// Simulate multiple tests starting
	RegisterTest()
	RegisterTest()
	RegisterTest()

	// Verify that we have 3 active tests
	if activeTests != 3 {
		t.Errorf("Expected 3 active tests, got %d", activeTests)
	}

	// Simulate first test finishing
	UnregisterTest()

	// Verify that we still have 2 active tests
	if activeTests != 2 {
		t.Errorf("Expected 2 active tests, got %d", activeTests)
	}

	// Simulate second test finishing
	UnregisterTest()

	// Verify that we still have 1 active test
	if activeTests != 1 {
		t.Errorf("Expected 1 active test, got %d", activeTests)
	}

	// Simulate last test finishing
	UnregisterTest()

	// Verify that we have 0 active tests
	if activeTests != 0 {
		t.Errorf("Expected 0 active tests, got %d", activeTests)
	}

	// Verify that calling UnregisterTest again doesn't cause issues
	UnregisterTest()

	// Verify that we still have 0 active tests (shouldn't go negative)
	if activeTests != 0 {
		t.Errorf("Expected 0 active tests, got %d", activeTests)
	}
}

func TestLocalPostgresConfig(t *testing.T) {
	t.Setenv("ORGUNIT_TEST_POSTGRES_PORT", "")
	conf, err := localPostgresConfig()
	if conf != nil || err != nil {
		t.Fatal("unset override must preserve testcontainers default")
	}
	t.Setenv("ORGUNIT_TEST_POSTGRES_PORT", "invalid")
	if _, err = localPostgresConfig(); err == nil {
		t.Fatal("invalid port accepted")
	}
	t.Setenv("ORGUNIT_TEST_POSTGRES_PORT", "25432")
	t.Setenv("ORGUNIT_TEST_POSTGRES_PASSWORD", "")
	if _, err = localPostgresConfig(); err == nil {
		t.Fatal("missing password accepted")
	}
	t.Setenv("ORGUNIT_TEST_POSTGRES_PASSWORD", "test-only")
	conf, err = localPostgresConfig()
	if err != nil {
		t.Fatal(err)
	}
	if conf.Connection.Host != "127.0.0.1" || conf.Connection.Name != "quota_poc_test" || conf.Connection.User != "quota_poc_test" || conf.Connection.Port != 25432 || !*conf.Migration.Auto {
		t.Fatalf("unexpected local test configuration: host=%s name=%s", conf.Connection.Host, conf.Connection.Name)
	}
}
