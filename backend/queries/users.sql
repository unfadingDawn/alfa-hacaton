-- name: CreateUser :one
INSERT INTO users (email, password_hash, full_name)
VALUES (@email, @password_hash, @full_name)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE lower(email) = lower(@email);

-- name: GetUserByID :one
SELECT * FROM users WHERE id = @id;

-- name: UpdateUserProfile :one
UPDATE users
SET full_name = @full_name,
    role_code = @role_code,
    position_title = @position_title,
    work_focus = @work_focus,
    updated_at = now()
WHERE id = @id
RETURNING *;

-- name: UpdateUserPersonalization :one
UPDATE users SET personalization = @personalization, updated_at = now()
WHERE id = @id
RETURNING *;

-- name: SetOnboardingCompleted :exec
UPDATE users SET onboarding_completed = @completed, updated_at = now() WHERE id = @id;

-- name: DeleteUserInterests :exec
DELETE FROM user_interests WHERE user_id = @user_id;

-- name: AddUserInterest :exec
INSERT INTO user_interests (user_id, topic_code, weight)
VALUES (@user_id, @topic_code, @weight)
ON CONFLICT (user_id, topic_code) DO UPDATE SET weight = EXCLUDED.weight;

-- name: ListUserInterests :many
SELECT ui.topic_code, ui.weight, t.name
FROM user_interests ui JOIN topics t ON t.code = ui.topic_code
WHERE ui.user_id = @user_id
ORDER BY ui.weight DESC, t.name;

-- name: ListTopics :many
SELECT * FROM topics ORDER BY name;

-- name: CreateSession :one
INSERT INTO sessions (user_id, token_hash, user_agent, expires_at)
VALUES (@user_id, @token_hash, @user_agent, @expires_at)
RETURNING *;

-- name: GetActiveSessionByHash :one
SELECT * FROM sessions
WHERE token_hash = @token_hash AND revoked_at IS NULL AND expires_at > now();

-- name: RevokeSession :exec
UPDATE sessions SET revoked_at = now() WHERE id = @id AND revoked_at IS NULL;

-- name: RevokeSessionByHash :exec
UPDATE sessions SET revoked_at = now() WHERE token_hash = @token_hash AND revoked_at IS NULL;

-- name: AddMembership :exec
INSERT INTO memberships (user_id, company_id, is_primary)
VALUES (@user_id, @company_id, @is_primary)
ON CONFLICT (user_id, company_id) DO UPDATE SET is_primary = EXCLUDED.is_primary;

-- name: ClearPrimaryMembership :exec
UPDATE memberships SET is_primary = false WHERE user_id = @user_id AND is_primary;

-- name: RemoveMembership :exec
DELETE FROM memberships WHERE user_id = @user_id AND company_id = @company_id;

-- name: IsMember :one
SELECT EXISTS (
  SELECT 1 FROM memberships WHERE user_id = @user_id AND company_id = @company_id
) AS is_member;

-- name: GetPrimaryCompanyID :one
SELECT company_id FROM memberships
WHERE user_id = @user_id
ORDER BY is_primary DESC, created_at
LIMIT 1;

-- name: ListUserCompanies :many
SELECT c.id, c.display_name, c.okved_code, c.okved_name, c.region, c.revenue, m.is_primary
FROM memberships m JOIN companies c ON c.id = m.company_id
WHERE m.user_id = @user_id
ORDER BY m.is_primary DESC, m.created_at;
