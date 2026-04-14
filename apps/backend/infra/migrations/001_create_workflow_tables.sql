-- 001_create_workflow_tables.sql
-- Creates core tables for storing AI workflow definitions and execution history.

BEGIN;

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TYPE workflow_state AS ENUM ('draft', 'active', 'archived');
CREATE TYPE run_status AS ENUM ('pending', 'running', 'succeeded', 'failed', 'cancelled');

CREATE TABLE workflows (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug text UNIQUE NOT NULL,
    name text NOT NULL,
    description text,
    owner_id uuid,
    state workflow_state DEFAULT 'draft',
    default_version_id uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workflow_versions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    version_number integer NOT NULL,
    config jsonb NOT NULL,
    config_hash bytea,
    changelog text,
    created_by uuid,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (workflow_id, version_number)
);

CREATE TABLE workflow_nodes (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES workflow_versions(id) ON DELETE CASCADE,
    node_key text NOT NULL,
    node_type text NOT NULL,
    name text,
    spec jsonb NOT NULL,
    position jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (version_id, node_key)
);

CREATE TABLE workflow_edges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id uuid NOT NULL REFERENCES workflow_versions(id) ON DELETE CASCADE,
    source_node_id uuid NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    target_node_id uuid NOT NULL REFERENCES workflow_nodes(id) ON DELETE CASCADE,
    condition jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (version_id, source_node_id, target_node_id, condition)
);

CREATE TABLE workflow_runs (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    version_id uuid REFERENCES workflow_versions(id),
    trigger text,
    status run_status NOT NULL,
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    metrics jsonb,
    context jsonb,
    error text
);

CREATE TABLE workflow_run_events (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    node_id uuid REFERENCES workflow_nodes(id),
    step text,
    event_type text NOT NULL,
    payload jsonb,
    occurred_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE artifacts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id uuid NOT NULL REFERENCES workflow_runs(id) ON DELETE CASCADE,
    node_id uuid REFERENCES workflow_nodes(id),
    artifact_type text NOT NULL,
    location text,
    metadata jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workflow_tags (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text UNIQUE NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE workflow_tag_map (
    workflow_id uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    tag_id uuid NOT NULL REFERENCES workflow_tags(id) ON DELETE CASCADE,
    PRIMARY KEY (workflow_id, tag_id)
);

CREATE INDEX workflow_versions_workflow_idx ON workflow_versions (workflow_id, version_number DESC);
CREATE INDEX workflow_nodes_version_key_idx ON workflow_nodes (version_id, node_key);
CREATE INDEX workflow_runs_workflow_idx ON workflow_runs (workflow_id, started_at DESC);
CREATE INDEX workflow_run_events_run_idx ON workflow_run_events (run_id, occurred_at);
CREATE INDEX workflow_nodes_spec_gin_idx ON workflow_nodes USING GIN (spec jsonb_path_ops);
CREATE INDEX workflow_runs_metrics_gin_idx ON workflow_runs USING GIN (metrics jsonb_path_ops);

COMMIT;
