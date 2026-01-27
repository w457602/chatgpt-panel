CREATE TEMP TABLE import_lines (payload jsonb);
COPY import_lines FROM '/tmp/chatgpt_accounts_api.ndjson';

WITH src AS (
    SELECT
        NULLIF(payload->>'email', '') AS email,
        NULLIF(payload->>'access_token', '') AS access_token,
        NULLIF(payload->>'refresh_token', '') AS refresh_token,
        NULLIF(payload->>'checkout_url', '') AS checkout_url,
        NULLIF(payload->>'account_id', '') AS account_id,
        payload->'cookies' AS session_cookies,
        NULLIF(payload->>'created_at', '') AS created_at_raw,
        NULLIF(payload->>'last_refresh', '') AS last_refresh_raw,
        NULLIF(payload->>'expired', '') AS expired_raw,
        NULLIF(payload->>'type', '') AS plan_type
    FROM import_lines
)
INSERT INTO accounts (
    email,
    password,
    access_token,
    refresh_token,
    checkout_url,
    account_id,
    session_cookies,
    status,
    registered_at,
    token_expired,
    created_at,
    updated_at,
    notes
)
SELECT
    email,
    'imported' AS password,
    access_token,
    refresh_token,
    checkout_url,
    account_id,
    session_cookies,
    CASE
        WHEN access_token IS NOT NULL AND access_token <> '' THEN 'active'
        ELSE 'pending'
    END,
    COALESCE(created_at_raw::timestamptz, NOW()),
    CASE WHEN expired_raw IS NOT NULL THEN expired_raw::timestamptz ELSE NULL END,
    COALESCE(created_at_raw::timestamptz, NOW()),
    COALESCE(last_refresh_raw::timestamptz, NOW()),
    CASE WHEN plan_type IS NOT NULL THEN 'plan=' || plan_type ELSE NULL END
FROM src
WHERE email IS NOT NULL
ON CONFLICT (email) DO UPDATE SET
    access_token = EXCLUDED.access_token,
    refresh_token = COALESCE(EXCLUDED.refresh_token, accounts.refresh_token),
    checkout_url = COALESCE(EXCLUDED.checkout_url, accounts.checkout_url),
    account_id = COALESCE(EXCLUDED.account_id, accounts.account_id),
    session_cookies = COALESCE(EXCLUDED.session_cookies, accounts.session_cookies),
    token_expired = COALESCE(EXCLUDED.token_expired, accounts.token_expired),
    status = CASE
        WHEN EXCLUDED.access_token IS NOT NULL AND EXCLUDED.access_token <> '' THEN 'active'
        ELSE accounts.status
    END,
    updated_at = NOW(),
    notes = COALESCE(EXCLUDED.notes, accounts.notes);
