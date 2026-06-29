-- 0001_init.sql — schema inicial do gix-server (Postgres + pgvector).
--
-- Multi-user: toda linha de dado é escopada por user_id. Substitui o SQLite
-- local do gix (notes/note_tags/note_vectors/notes_fts/conversations/messages/
-- alerts) pelo modelo canônico no servidor.
--
-- Mapeamento vindo do gix:
--   note_vectors (BLOB float32, dim=384)  -> notes.embedding  vector(384) (pgvector)
--   notes_fts (FTS5 unicode61)            -> notes.fts        tsvector (gerada) + GIN
--   note_tags (join)                      -> notes.tags       text[] + GIN
--
-- Embeddings: e5-small (384 dims), gerados no servidor. Busca híbrida = FTS
-- (ts_rank) + similaridade de cosseno (pgvector), fundidas por RRF na aplicação.

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS unaccent;

-- FTS source das notas. Wrapper marcado IMMUTABLE para poder alimentar a coluna
-- gerada (to_tsvector com config literal e unaccent são, na prática, imutáveis;
-- sem o wrapper o Postgres recusa por enxergá-los como STABLE). unaccent replica
-- o remove_diacritics do FTS5 do gix: "ruido" casa "ruído".
CREATE OR REPLACE FUNCTION gix_note_tsv(p_title text, p_content text, p_tags text[])
RETURNS tsvector LANGUAGE sql IMMUTABLE AS $$
  SELECT to_tsvector('portuguese',
    unaccent(coalesce(p_title, '') || ' ' ||
             coalesce(p_content, '') || ' ' ||
             array_to_string(coalesce(p_tags, '{}'), ' ')))
$$;

-- ---------------------------------------------------------------------------
-- contas
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id            BIGSERIAL PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- notas (+ tags, embedding e FTS na própria linha)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS notes (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    content    TEXT NOT NULL,
    tags       TEXT[] NOT NULL DEFAULT '{}',
    char_limit INTEGER NOT NULL DEFAULT 0,  -- 0 = herda o limite global do usuário
    embedding  vector(384),                 -- NULL até ser embedada
    fts        tsvector GENERATED ALWAYS AS (gix_note_tsv(title, content, tags)) STORED,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_notes_user ON notes(user_id);
CREATE INDEX IF NOT EXISTS idx_notes_fts  ON notes USING GIN(fts);
CREATE INDEX IF NOT EXISTS idx_notes_tags ON notes USING GIN(tags);
-- Índice ANN para busca vetorial por cosseno (e5 normaliza, então cosseno == dot).
CREATE INDEX IF NOT EXISTS idx_notes_embedding
    ON notes USING hnsw (embedding vector_cosine_ops);

-- ---------------------------------------------------------------------------
-- chat: conversas e mensagens
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS conversations (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    model      TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_conversations_user ON conversations(user_id);

CREATE TABLE IF NOT EXISTS messages (
    id              BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,  -- user | assistant | system
    content         TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_messages_conversation ON messages(conversation_id);

-- ---------------------------------------------------------------------------
-- alertas (agendamento e disparo são server-side; ver fase 2 do roteiro)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS alerts (
    id         BIGSERIAL PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message    TEXT NOT NULL,
    note_id    BIGINT REFERENCES notes(id) ON DELETE SET NULL,
    fire_at    TIMESTAMPTZ NOT NULL,
    recurrence TEXT NOT NULL DEFAULT '',         -- '' = one-shot, senão regra JSON
    status     TEXT NOT NULL DEFAULT 'pending',  -- pending | done | cancelled
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Índice do poll do scheduler: alertas pendentes vencendo (por usuário).
CREATE INDEX IF NOT EXISTS idx_alerts_due ON alerts(status, fire_at);

-- ---------------------------------------------------------------------------
-- preferências do usuário (config que era local: model, system_prompt, idioma,
-- tema, limite global de caracteres). api_key da OpenRouter NÃO mora aqui —
-- a chave é única do servidor (env), nenhum canal a vê.
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS user_prefs (
    user_id         BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    model           TEXT NOT NULL DEFAULT '',
    system_prompt   TEXT NOT NULL DEFAULT '',
    language        TEXT NOT NULL DEFAULT 'pt',
    note_char_limit INTEGER NOT NULL DEFAULT 8000
);

-- NOTA (fases futuras, fora desta migration):
--   * fase 2 (scheduler + push): tabela de canais/tokens de push por usuário
--     (ex.: user_channels(user_id, kind, address, push_token, preferred)).
--   * fase 4 (WhatsApp/Telegram): vínculo usuário<->identidade no canal.
