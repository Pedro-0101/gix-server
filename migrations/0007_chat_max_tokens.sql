-- 0007_chat_max_tokens.sql — limite de tokens de contexto por conversa.
-- 0 = usa o padrão do servidor (96000). O servidor trunca o histórico
-- automaticamente quando o total estimado de tokens ultrapassa este valor.

ALTER TABLE user_prefs ADD COLUMN IF NOT EXISTS chat_max_tokens INTEGER NOT NULL DEFAULT 0;
