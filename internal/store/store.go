// Package store é a camada de persistência Postgres (pgx) do gix-server.
// Substitui o internal/db (SQLite) do gix: notas (+tags, embedding, FTS), alertas,
// chat e usuários, todos escopados por user_id. Cada domínio fica em seu arquivo
// (notes.go, users.go); o pool e as migrations ficam aqui.
package store

import (
	"context"
	"fmt"
	"io/fs"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"

	"github.com/Pedro-0101/gix-server/migrations"
)

// Store é o pool de conexões compartilhado por todos os domínios.
type Pagination struct {
	Limit  int
	Offset int
}

func DefaultPagination() Pagination {
	return Pagination{Limit: 50, Offset: 0}
}

type Store struct {
	pool *pgxpool.Pool
}

// Open abre o pool, valida a conexão e aplica as migrations embutidas.
func Open(ctx context.Context, dsn string) (*Store, error) {
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	poolCfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	s := &Store{pool: pool}
	if err := s.migrate(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return s, nil
}

// Close fecha o pool.
func (s *Store) Close() { s.pool.Close() }

// migrate executa os .sql embutidos em ordem de nome. Como cada arquivo é
// idempotente (IF NOT EXISTS), rodar de novo é seguro. Exec sem argumentos usa
// o protocolo simples do pgx, que aceita múltiplos statements por arquivo.
func (s *Store) migrate(ctx context.Context) error {
	names, err := fs.Glob(migrations.FS, "*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		sqlBytes, err := migrations.FS.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, string(sqlBytes)); err != nil {
			return fmt.Errorf("migration %s: %w", name, err)
		}
	}
	return nil
}
