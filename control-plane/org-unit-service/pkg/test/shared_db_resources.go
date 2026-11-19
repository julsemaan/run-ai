/*
 * SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
 * SPDX-License-Identifier: Apache-2.0
 */

package testing

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	org_unit_config "github.com/dsx-ai-factory/run-ai/control-plane/org-unit-service/pkg/config"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/integdb"
	"github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb"
	dbconfig "github.com/dsx-ai-factory/run-ai/runai-common-packages/runaidb/config"
)

var (
	activeTests   int
	testMutex     sync.Mutex
	dbMutex       sync.Mutex
	dbCond        *sync.Cond
	isInitialized bool
	isCleaning    bool
)

// localPostgresConfig opts into a separately provisioned disposable local database.
// The fixed host, database and role prevent ordinary production DB settings from
// redirecting destructive test cleanup. The caller owns the database lifecycle.
func localPostgresConfig() (*dbconfig.Config, error) {
	portText := os.Getenv("ORGUNIT_TEST_POSTGRES_PORT")
	if portText == "" {
		return nil, nil
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("ORGUNIT_TEST_POSTGRES_PORT must be a valid TCP port")
	}
	password := os.Getenv("ORGUNIT_TEST_POSTGRES_PASSWORD")
	if password == "" {
		return nil, fmt.Errorf("ORGUNIT_TEST_POSTGRES_PASSWORD is required for the disposable local database")
	}
	conf := &dbconfig.Config{}
	conf.SetDefaults()
	conf.Connection.Host = "127.0.0.1"
	conf.Connection.Port = port
	conf.Connection.Name = "quota_poc_test"
	conf.Connection.User = "quota_poc_test"
	conf.Connection.Password = password
	conf.Connection.Schema = org_unit_config.Schema
	conf.Migration.Files = "file://../database/migrations"
	conf.ServiceName = "test-orgunit-local"
	conf.EncryptionKey = "0123456789abcdef0123456789abcdef"
	return conf, nil
}

func init() {
	dbCond = sync.NewCond(&dbMutex)
}

// RegisterTest marks that a test has started
// This should be called at the beginning of each test
func RegisterTest() {
	testMutex.Lock()
	defer testMutex.Unlock()
	activeTests++
}

// UnregisterTest marks that a test has finished
// This should be called at the end of each test
func UnregisterTest() {
	if activeTests == 0 {
		return
	}
	testMutex.Lock()
	defer testMutex.Unlock()
	activeTests--

	// If this was the last test, trigger cleanup
	if activeTests == 0 {
		SharedCleanup()
	}
}

// SharedCleanup ensures that table cleanup only happens once
// This prevents conflicts when multiple tests run together
func SharedCleanup() {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Only cleanup if database is initialized and not already cleaning
	if isInitialized && !isCleaning {
		isCleaning = true
		CleanTables()
		isInitialized = false
		isCleaning = false
		// Signal any waiting initialization operations
		dbCond.Broadcast()
	}
}

// SharedOpenPostgresForTesting ensures that database initialization only happens once
// This prevents conflicts when multiple tests try to initialize the database simultaneously
func SharedOpenPostgresForTesting(t *testing.T) {
	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Wait for any ongoing cleanup to complete
	for isCleaning {
		dbCond.Wait()
	}

	// Only initialize if not already initialized
	if !isInitialized {
		conf, setupErr := localPostgresConfig()
		cleanup := func() {}
		if setupErr == nil && conf == nil {
			conf, cleanup, setupErr = integdb.SetupPostgresDB(&integdb.IntegdbConfig{
				Schema:           org_unit_config.Schema,
				MigrationDirPath: "file://../database/migrations",
			})
		}
		if setupErr != nil {
			t.Fatalf("failed to set up test postgres: %v", setupErr)
		}
		t.Cleanup(cleanup)
		conf.FullSaveAssociations = true
		conf.SkipDefaultTransaction = true
		if err := runaidb.Init(conf); err != nil {
			t.Fatalf("failed to init database: %v", err)
		}
		t.Cleanup(func() { _ = runaidb.Close() })
		isInitialized = true
	}
}
