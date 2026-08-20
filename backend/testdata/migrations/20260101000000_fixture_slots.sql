-- Fase: expand
-- Reversible: sí
-- ADR-0002: fixture de verificación de la cadena de generación, no es esquema real.

-- +goose Up
CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE slots (
                       id          uuid PRIMARY KEY,
                       owner_id    uuid NOT NULL,
                       project_id  uuid,
                       starts_at   timestamptz NOT NULL,
                       ends_at     timestamptz NOT NULL,
                       note        text,
                       created_at  timestamptz NOT NULL DEFAULT now(),
                       CONSTRAINT slots_range_valid CHECK (ends_at > starts_at)
);

ALTER TABLE slots ADD CONSTRAINT slots_no_overlap
    EXCLUDE USING gist (
    owner_id WITH =,
    tstzrange(starts_at, ends_at) WITH &&
  );

-- +goose Down
DROP TABLE slots;
