-- +goose Up
-- A provider is either an OpenAI-compatible server at an address, or the
-- hub's own runtime, which runs llama-server itself and needs no address:
-- its routes name a GGUF file in the models folder.
ALTER TABLE model_providers ADD COLUMN kind text NOT NULL DEFAULT 'openai_compatible'
    CHECK (kind IN ('openai_compatible', 'hub_runtime'));
ALTER TABLE model_providers DROP CONSTRAINT model_providers_base_url_check;
ALTER TABLE model_providers ADD CONSTRAINT model_providers_base_url_check
    CHECK (kind = 'hub_runtime' OR base_url ~ '^https?://');

-- +goose Down
DELETE FROM model_providers WHERE kind = 'hub_runtime';
ALTER TABLE model_providers DROP CONSTRAINT model_providers_base_url_check;
ALTER TABLE model_providers ADD CONSTRAINT model_providers_base_url_check CHECK (base_url ~ '^https?://');
ALTER TABLE model_providers DROP COLUMN kind;
