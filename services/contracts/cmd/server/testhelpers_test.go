package main

import (
	"context"
	"database/sql"
	"testing"

	"github.com/copycatsh/hire-flow/services/contracts/migrations"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	_ "github.com/go-sql-driver/mysql"
)

func setupMySQL(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()

	mysqlContainer, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("test_contracts"),
		tcmysql.WithUsername("test"),
		tcmysql.WithPassword("test"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mysqlContainer.Terminate(context.Background()) })

	connStr, err := mysqlContainer.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)

	db, err := sql.Open("mysql", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	goose.SetBaseFS(migrations.FS)
	require.NoError(t, goose.SetDialect("mysql"))
	require.NoError(t, goose.Up(db, "."))

	return db
}
