package leaderboard

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const embedSessionTTL = 30 * time.Minute
const embedSessionKeyPrefix = "leaderboard:embed:session:"
const embedWorkspaceIndexKeyPrefix = "leaderboard:embed:workspace:"

type EmbedSessionStore struct {
	client *redis.Client
}

func NewEmbedSessionStore(client *redis.Client) *EmbedSessionStore {
	return &EmbedSessionStore{client: client}
}

func (s *EmbedSessionStore) Save(ctx context.Context, token string, session EmbedSession) error {
	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}
	sessionKey := embedSessionKey(token)
	pipe := s.client.TxPipeline()
	pipe.Set(ctx, sessionKey, payload, embedSessionTTL)
	if strings.TrimSpace(session.UserID) != "" && strings.TrimSpace(session.AdminAccountID) != "" {
		indexKey := embedWorkspaceIndexKey(session.UserID, session.AdminAccountID)
		pipe.SAdd(ctx, indexKey, sessionKey)
		pipe.Expire(ctx, indexKey, embedSessionTTL)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (s *EmbedSessionStore) Get(ctx context.Context, token string) (*EmbedSession, error) {
	if token == "" {
		return nil, nil
	}
	raw, err := s.client.Get(ctx, embedSessionKey(token)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
		return nil, err
	}
	var session EmbedSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

// DeleteWorkspace deletes all short-lived leaderboard embed sessions for a
// workspace. New sessions are found through the workspace index; old unreleased
// sessions without an index are safely discovered by scanning only leaderboard
// session keys and matching the JSON workspace fields exactly.
func (s *EmbedSessionStore) DeleteWorkspace(ctx context.Context, userID string, adminAccountID string) error {
	userID = strings.TrimSpace(userID)
	adminAccountID = strings.TrimSpace(adminAccountID)
	if userID == "" || adminAccountID == "" {
		return nil
	}
	indexKey := embedWorkspaceIndexKey(userID, adminAccountID)
	indexedKeys, err := s.client.SMembers(ctx, indexKey).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	deleteSet := make(map[string]struct{}, len(indexedKeys))
	for _, key := range indexedKeys {
		if strings.HasPrefix(key, embedSessionKeyPrefix) {
			deleteSet[key] = struct{}{}
		}
	}

	iter := s.client.Scan(ctx, 0, embedSessionKeyPrefix+"*", 100).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		raw, err := s.client.Get(ctx, key).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			return err
		}
		if embedSessionBelongsToWorkspace(raw, userID, adminAccountID) {
			deleteSet[key] = struct{}{}
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}

	keys := make([]string, 0, len(deleteSet)+1)
	for key := range deleteSet {
		keys = append(keys, key)
	}
	keys = append(keys, indexKey)
	return s.client.Del(ctx, keys...).Err()
}

func embedSessionKey(token string) string {
	return embedSessionKeyPrefix + token
}

func embedWorkspaceIndexKey(userID string, adminAccountID string) string {
	return embedWorkspaceIndexKeyPrefix + strings.TrimSpace(userID) + ":" + strings.TrimSpace(adminAccountID)
}

func embedSessionBelongsToWorkspace(raw string, userID string, adminAccountID string) bool {
	var session EmbedSession
	if err := json.Unmarshal([]byte(raw), &session); err != nil {
		return false
	}
	return session.UserID == userID && session.AdminAccountID == adminAccountID
}
