// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016 Datadog, Inc.

package sql

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"gopkg.in/DataDog/dd-trace-go.v1/contrib/internal/namingschematest"
	"gopkg.in/DataDog/dd-trace-go.v1/contrib/internal/sqltest"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/ext"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/mocktracer"

	mssql "github.com/denisenkom/go-mssqldb"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tableName holds the SQL table that these tests will be run against. It must be unique cross-repo.
const tableName = "testsql"

func TestMain(m *testing.M) {
	_, ok := os.LookupEnv("INTEGRATION")
	if !ok {
		fmt.Println("--- SKIP: to enable integration test, set the INTEGRATION environment variable")
		os.Exit(0)
	}
	defer sqltest.Prepare(tableName)()
	os.Exit(m.Run())
}

func TestSqlServer(t *testing.T) {
	driverName := "sqlserver"
	Register(driverName, &mssql.Driver{})
	defer unregister(driverName)
	db, err := Open(driverName, "sqlserver://sa:myPassw0rd@127.0.0.1:1433?database=master")
	require.NoError(t, err)
	defer db.Close()

	testConfig := &sqltest.Config{
		DB:         db,
		DriverName: driverName,
		TableName:  tableName,
		ExpectName: "sqlserver.query",
		ExpectTags: map[string]interface{}{
			ext.ServiceName:     "sqlserver.db",
			ext.SpanType:        ext.SpanTypeSQL,
			ext.TargetHost:      "127.0.0.1",
			ext.TargetPort:      "1433",
			ext.DBUser:          "sa",
			ext.DBName:          "master",
			ext.EventSampleRate: nil,
			ext.DBSystem:        "mssql",
		},
	}
	sqltest.RunAll(t, testConfig)
}

func TestMySQL(t *testing.T) {
	driverName := "mysql"
	Register(driverName, &mysql.MySQLDriver{})
	defer unregister(driverName)
	db, err := Open(driverName, "test:test@tcp(127.0.0.1:3306)/test")
	require.NoError(t, err)
	defer db.Close()

	testConfig := &sqltest.Config{
		DB:         db,
		DriverName: driverName,
		TableName:  tableName,
		ExpectName: "mysql.query",
		ExpectTags: map[string]interface{}{
			ext.ServiceName:     "mysql.db",
			ext.SpanType:        ext.SpanTypeSQL,
			ext.TargetHost:      "127.0.0.1",
			ext.TargetPort:      "3306",
			ext.DBUser:          "test",
			ext.DBName:          "test",
			ext.EventSampleRate: nil,
			ext.DBSystem:        "mysql",
		},
	}
	sqltest.RunAll(t, testConfig)
}

func TestPostgres(t *testing.T) {
	driverName := "postgres"
	Register(driverName, &pq.Driver{}, WithServiceName("postgres-test"), WithAnalyticsRate(0.2))
	defer unregister(driverName)
	db, err := Open(driverName, "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable")
	require.NoError(t, err)
	defer db.Close()

	testConfig := &sqltest.Config{
		DB:         db,
		DriverName: driverName,
		TableName:  tableName,
		ExpectName: "postgres.query",
		ExpectTags: map[string]interface{}{
			ext.ServiceName:     "postgres-test",
			ext.SpanType:        ext.SpanTypeSQL,
			ext.TargetHost:      "127.0.0.1",
			ext.TargetPort:      "5432",
			ext.DBUser:          "postgres",
			ext.DBName:          "postgres",
			ext.EventSampleRate: 0.2,
			ext.DBSystem:        "postgresql",
		},
	}
	sqltest.RunAll(t, testConfig)
}

func TestOpenOptions(t *testing.T) {
	driverName := "postgres"
	Register(driverName, &pq.Driver{}, WithServiceName("postgres-test"), WithAnalyticsRate(0.2))
	defer unregister(driverName)

	t.Run("Open", func(t *testing.T) {
		db, err := Open(driverName, "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable",
			WithServiceName("override-test"),
			WithAnalytics(true),
		)
		if err != nil {
			log.Fatal(err)
		}
		defer db.Close()

		testConfig := &sqltest.Config{
			DB:         db,
			DriverName: driverName,
			TableName:  tableName,
			ExpectName: "postgres.query",
			ExpectTags: map[string]interface{}{
				ext.ServiceName:     "override-test",
				ext.SpanType:        ext.SpanTypeSQL,
				ext.TargetHost:      "127.0.0.1",
				ext.TargetPort:      "5432",
				ext.DBUser:          "postgres",
				ext.DBName:          "postgres",
				ext.EventSampleRate: 1.0,
				ext.DBSystem:        "postgresql",
			},
		}
		sqltest.RunAll(t, testConfig)
	})

	t.Run("OpenDB", func(t *testing.T) {
		c, err := pq.NewConnector("postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable")
		if err != nil {
			log.Fatal(err)
		}
		db := OpenDB(c)
		defer db.Close()

		testConfig := &sqltest.Config{
			DB:         db,
			DriverName: driverName,
			TableName:  tableName,
			ExpectName: "postgres.query",
			ExpectTags: map[string]interface{}{
				ext.ServiceName:     "postgres-test",
				ext.SpanType:        ext.SpanTypeSQL,
				ext.TargetHost:      nil,
				ext.TargetPort:      nil,
				ext.DBUser:          nil,
				ext.DBName:          nil,
				ext.EventSampleRate: 0.2,
				ext.DBSystem:        "postgresql",
			},
		}
		sqltest.RunAll(t, testConfig)
	})

	t.Run("WithDSN", func(t *testing.T) {
		dsn := "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"
		c, err := pq.NewConnector(dsn)
		if err != nil {
			log.Fatal(err)
		}
		db := OpenDB(c, WithDSN(dsn))
		defer db.Close()

		testConfig := &sqltest.Config{
			DB:         db,
			DriverName: driverName,
			TableName:  tableName,
			ExpectName: "postgres.query",
			ExpectTags: map[string]interface{}{
				ext.ServiceName:     "postgres-test",
				ext.SpanType:        ext.SpanTypeSQL,
				ext.TargetHost:      "127.0.0.1",
				ext.TargetPort:      "5432",
				ext.DBUser:          "postgres",
				ext.DBName:          "postgres",
				ext.EventSampleRate: 0.2,
				ext.DBSystem:        "postgresql",
			},
		}
		sqltest.RunAll(t, testConfig)
	})
}

func TestMySQLUint64(t *testing.T) {
	Register("mysql", &mysql.MySQLDriver{})
	defer unregister("mysql")
	db, err := Open("mysql", "test:test@tcp(127.0.0.1:3306)/test")
	require.NoError(t, err)
	defer db.Close()

	assert := assert.New(t)
	rows, err := db.Query("SELECT ?", uint64(math.MaxUint64))
	assert.NoError(err)
	assert.NotNil(rows)
	assert.True(rows.Next())
	var result uint64
	rows.Scan(&result)
	assert.Equal(uint64(math.MaxUint64), result)
	assert.False(rows.Next())
	assert.NoError(rows.Err())
	assert.NoError(rows.Close())
}

// hangingConnector hangs on Connect until ctx is cancelled.
type hangingConnector struct{}

func (h *hangingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	select {
	case <-ctx.Done():
		return nil, errors.New("context cancelled")
	}
}

func (h *hangingConnector) Driver() driver.Driver {
	panic("hangingConnector: Driver() not implemented")
}

func TestConnectCancelledCtx(t *testing.T) {
	mockTracer := mocktracer.Start()
	defer mockTracer.Stop()
	assert := assert.New(t)
	driverName := "hangingConnector"
	cfg := new(config)
	defaults(cfg, driverName, nil)
	tc := tracedConnector{
		connector:  &hangingConnector{},
		driverName: driverName,
		cfg:        cfg,
	}
	ctx, cancelFunc := context.WithCancel(context.Background())

	go func() {
		tc.Connect(ctx)
	}()
	time.Sleep(time.Millisecond * 100)
	cancelFunc()
	time.Sleep(time.Millisecond * 100)

	spans := mockTracer.FinishedSpans()
	assert.Len(spans, 1)
	s := spans[0]
	assert.Equal("hangingConnector.query", s.OperationName())
	assert.Equal("Connect", s.Tag("sql.query_type"))
}

func TestRegister(_ *testing.T) {
	var wg sync.WaitGroup

	for i := 1; i < 10; i++ {
		wg.Add(1)
		go func(i int64) {
			Register("test"+strconv.FormatInt(i, 10), &mysql.MySQLDriver{})
			wg.Done()
		}(int64(i))
	}

	wg.Wait()
	// cleanup registered drivers
	for i := 1; i < 10; i++ {
		unregister("test" + strconv.FormatInt(int64(i), 10))
	}
}

func TestNamingSchema(t *testing.T) {
	newGenSpansFunc := func(t *testing.T, driverName string) namingschematest.GenSpansFn {
		return func(t *testing.T, serviceOverride string) []mocktracer.Span {
			var registerOpts []RegisterOption
			if serviceOverride != "" {
				registerOpts = append(registerOpts, WithServiceName(serviceOverride))
			}
			var openOpts []Option
			if serviceOverride != "" {
				openOpts = append(openOpts, WithServiceName(serviceOverride))
			}
			mt := mocktracer.Start()
			defer mt.Stop()

			var (
				dv  driver.Driver
				dsn string
			)
			switch driverName {
			case "sqlserver":
				dv = &mssql.Driver{}
				dsn = "sqlserver://sa:myPassw0rd@127.0.0.1:1433?database=master"
			case "postgres":
				dv = &pq.Driver{}
				dsn = "postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable"
			case "mysql":
				dv = &mysql.MySQLDriver{}
				dsn = "test:test@tcp(127.0.0.1:3306)/test"
			default:
				t.Fatal("unknown driver: ", driverName)
			}
			Register(driverName, dv, registerOpts...)
			defer unregister(driverName)
			db, err := Open(driverName, dsn, openOpts...)
			require.NoError(t, err)

			err = db.Ping()
			require.NoError(t, err)

			spans := mt.FinishedSpans()
			require.Len(t, spans, 2)
			return spans
		}
	}
	t.Run("SQLServer", func(t *testing.T) {
		genSpans := newGenSpansFunc(t, "sqlserver")
		assertOpV0 := func(t *testing.T, spans []mocktracer.Span) {
			require.Len(t, spans, 2)
			assert.Equal(t, "sqlserver.query", spans[0].OperationName())
			assert.Equal(t, "sqlserver.query", spans[1].OperationName())
		}
		assertOpV1 := func(t *testing.T, spans []mocktracer.Span) {
			require.Len(t, spans, 2)
			assert.Equal(t, "mssql.query", spans[0].OperationName())
			assert.Equal(t, "mssql.query", spans[1].OperationName())
		}
		wantServiceNameV0 := namingschematest.ServiceNameAssertions{
			WithDefaults:             []string{"sqlserver.db", "sqlserver.db"},
			WithDDService:            []string{"sqlserver.db", "sqlserver.db"},
			WithDDServiceAndOverride: []string{namingschematest.TestServiceOverride, namingschematest.TestServiceOverride},
		}
		t.Run("ServiceName", namingschematest.NewServiceNameTest(genSpans, "sqlserver.db", wantServiceNameV0))
		t.Run("SpanName", namingschematest.NewOpNameTest(genSpans, assertOpV0, assertOpV1))
	})
	t.Run("Postgres", func(t *testing.T) {
		genSpans := newGenSpansFunc(t, "postgres")
		assertOpV0 := func(t *testing.T, spans []mocktracer.Span) {
			require.Len(t, spans, 2)
			assert.Equal(t, "postgres.query", spans[0].OperationName())
			assert.Equal(t, "postgres.query", spans[1].OperationName())
		}
		assertOpV1 := func(t *testing.T, spans []mocktracer.Span) {
			require.Len(t, spans, 2)
			assert.Equal(t, "postgresql.query", spans[0].OperationName())
			assert.Equal(t, "postgresql.query", spans[1].OperationName())
		}
		wantServiceNameV0 := namingschematest.ServiceNameAssertions{
			WithDefaults:             []string{"postgres.db", "postgres.db"},
			WithDDService:            []string{"postgres.db", "postgres.db"},
			WithDDServiceAndOverride: []string{namingschematest.TestServiceOverride, namingschematest.TestServiceOverride},
		}
		t.Run("ServiceName", namingschematest.NewServiceNameTest(genSpans, "postgres.db", wantServiceNameV0))
		t.Run("SpanName", namingschematest.NewOpNameTest(genSpans, assertOpV0, assertOpV1))
	})
	t.Run("MySQL", func(t *testing.T) {
		genSpans := newGenSpansFunc(t, "mysql")
		assertOpV0 := func(t *testing.T, spans []mocktracer.Span) {
			require.Len(t, spans, 2)
			assert.Equal(t, "mysql.query", spans[0].OperationName())
			assert.Equal(t, "mysql.query", spans[1].OperationName())
		}
		assertOpV1 := func(t *testing.T, spans []mocktracer.Span) {
			require.Len(t, spans, 2)
			assert.Equal(t, "mysql.query", spans[0].OperationName())
			assert.Equal(t, "mysql.query", spans[1].OperationName())
		}
		wantServiceNameV0 := namingschematest.ServiceNameAssertions{
			WithDefaults:             []string{"mysql.db", "mysql.db"},
			WithDDService:            []string{"mysql.db", "mysql.db"},
			WithDDServiceAndOverride: []string{namingschematest.TestServiceOverride, namingschematest.TestServiceOverride},
		}
		t.Run("ServiceName", namingschematest.NewServiceNameTest(genSpans, "mysql.db", wantServiceNameV0))
		t.Run("SpanName", namingschematest.NewOpNameTest(genSpans, assertOpV0, assertOpV1))
	})
}
