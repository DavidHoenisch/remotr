-- name: UpsertEndpointCapabilityDocument :one
INSERT INTO endpoint_capability_documents (endpoint_id, digest, canonical_document, received_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (endpoint_id) DO UPDATE
SET digest = EXCLUDED.digest,
    canonical_document = EXCLUDED.canonical_document,
    received_at = EXCLUDED.received_at
WHERE endpoint_capability_documents.digest IS DISTINCT FROM EXCLUDED.digest
RETURNING *;

-- name: GetEndpointCapabilityDocument :one
SELECT * FROM endpoint_capability_documents
WHERE endpoint_id = $1;
