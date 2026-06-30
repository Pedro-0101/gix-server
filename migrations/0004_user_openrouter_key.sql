-- 0004_user_openrouter_key.sql — chave de API da OpenRouter por usuário.
-- Cada usuário pode configurar sua própria chave via PUT /v1/prefs (campo
-- openrouterKey). Se não definida, o servidor usa a chave do ambiente
-- (OPENROUTER_API_KEY) como fallback. Sem chave alguma, intents de IA
-- retornam status "no_api_key".

ALTER TABLE user_prefs ADD COLUMN IF NOT EXISTS openrouter_key TEXT NOT NULL DEFAULT '';
