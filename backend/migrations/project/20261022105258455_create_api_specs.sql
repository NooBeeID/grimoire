-- T06 project/apispec. Butuh tabel projects (T03).

CREATE TABLE api_specs (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id       uuid        NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name             text        NOT NULL,
    source_path      text        NULL,
    title            text        NOT NULL,
    api_version      text        NOT NULL,
    openapi_version  text        NOT NULL,
    content          text        NOT NULL,
    content_format   text        NOT NULL,
    content_hash     bytea       NOT NULL,
    size_bytes       int         NOT NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    created_by       uuid        NOT NULL,  -- tanpa FK (rule 8.5)
    created_by_label text        NOT NULL,
    updated_at       timestamptz NOT NULL DEFAULT now(),
    updated_by       uuid        NOT NULL,
    updated_by_label text        NOT NULL,
    CONSTRAINT api_specs_project_name_key UNIQUE (project_id, name),
    CONSTRAINT api_specs_name_check CHECK (name ~ '^[a-z0-9._-]{1,80}$' AND name NOT IN ('.', '..')),
    CONSTRAINT api_specs_format_check CHECK (content_format IN ('yaml', 'json')),
    CONSTRAINT api_specs_size_check CHECK (size_bytes >= 0),
    CONSTRAINT api_specs_hash_len CHECK (octet_length(content_hash) = 32)
);

CREATE TABLE api_spec_endpoints (
    spec_id      uuid    NOT NULL REFERENCES api_specs(id) ON DELETE CASCADE,
    method       text    NOT NULL,
    path         text    NOT NULL,
    operation_id text    NULL,
    summary      text    NOT NULL DEFAULT '',
    tags         text[]  NOT NULL DEFAULT '{}',
    deprecated   boolean NOT NULL DEFAULT false,
    CONSTRAINT api_spec_endpoints_pkey PRIMARY KEY (spec_id, method, path),
    CONSTRAINT api_spec_endpoints_method_check
        CHECK (method IN ('GET','PUT','POST','DELETE','OPTIONS','HEAD','PATCH','TRACE'))
);
CREATE INDEX api_spec_endpoints_opid_idx ON api_spec_endpoints (spec_id, operation_id)
    WHERE operation_id IS NOT NULL;
