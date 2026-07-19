package apk

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
)

const (
	uploadSessionTTL    = 24 * time.Hour
	maxActiveUploads    = 2
	uploadMetaKeyFmt    = "upload:v1:%s:meta"
	uploadChunksKeyFmt  = "upload:v1:%s:chunks"
	uploadUserActiveFmt = "upload:v1:user:%d:active"
	uploadExpiresSet    = "upload:v1:expires"
)

type UploadSessionStatus string

const (
	StatusReceiving  UploadSessionStatus = "receiving"
	StatusAssembling UploadSessionStatus = "assembling"
	StatusCompleted  UploadSessionStatus = "completed"
	StatusFailed     UploadSessionStatus = "failed"
	StatusAborted    UploadSessionStatus = "aborted"
)

type UploadSession struct {
	ID             string
	UserID         uint
	FileName       string
	FileSize       int64
	TotalChunks    int
	ChunksReceived int
	BytesReceived  int64
	Status         UploadSessionStatus
	Version        string
	Description    string
	CreatedAt      time.Time
	ExpiresAt      time.Time
}

type ChunkRecord struct {
	Index int
	Hash  string
	Size  int64
}

type BeginUploadRequest struct {
	UserID      uint
	FileName    string
	FileSize    int64
	TotalChunks int
	Version     string
	Description string
}

type UploadStateStore struct {
	client *goredis.Client
}

func NewUploadStateStore(client *goredis.Client) *UploadStateStore {
	return &UploadStateStore{client: client}
}

var uploadBeginScript = goredis.NewScript(`
	local user_active_key = KEYS[1]
	local expires_key = KEYS[2]
	local meta_key = KEYS[3]
	local upload_id = ARGV[1]
	local user_id = ARGV[2]
	local file_name = ARGV[3]
	local file_size = ARGV[4]
	local total_chunks = ARGV[5]
	local version_str = ARGV[6]
	local description = ARGV[7]
	local ttl_ms = ARGV[8]
	local max_active = tonumber(ARGV[9])
	local now_ms = ARGV[10]
	local expires_at = ARGV[11]

	local active = redis.call('SCARD', user_active_key)
	if active >= max_active then
		return {err = 'too many active uploads'}
	end

	local meta = cjson.encode({
		user_id = tonumber(user_id),
		file_name = file_name,
		file_size = tonumber(file_size),
		total_chunks = tonumber(total_chunks),
		chunks_received = 0,
		bytes_received = 0,
		status = 'receiving',
		version = version_str,
		description = description,
		created_at = now_ms,
		expires_at = expires_at
	})

	redis.call('SET', meta_key, meta, 'PX', ttl_ms)
	redis.call('SADD', user_active_key, upload_id)
	redis.call('PEXPIRE', user_active_key, ttl_ms)
	redis.call('ZADD', expires_key, expires_at, upload_id)

	return {'ok', 1}

`)

var uploadRecordChunkScript = goredis.NewScript(`
	local meta_key = KEYS[1]
	local chunks_key = KEYS[2]
	local chunk_index = ARGV[1]
	local chunk_hash = ARGV[2]
	local chunk_size = ARGV[3]
	local ttl_ms = ARGV[4]

	local meta_raw = redis.call('GET', meta_key)
	if not meta_raw then
		return {err = 'upload not found'}
	end

	local meta = cjson.decode(meta_raw)
	if meta.status ~= 'receiving' then
		return {err = 'upload not in receiving state'}
	end

	local chunk_field = 'chunk:' .. chunk_index
	local existing = redis.call('HGET', chunks_key, chunk_field)
	if existing then
		local existing_data = cjson.decode(existing)
		if existing_data.hash == chunk_hash then
			return {'ok', 1, 'duplicate', 1}
		end
		return {err = 'chunk hash mismatch'}
	end

	local chunk_data = cjson.encode({hash = chunk_hash, size = tonumber(chunk_size)})
	redis.call('HSET', chunks_key, chunk_field, chunk_data)
	redis.call('PEXPIRE', chunks_key, ttl_ms)

	meta.chunks_received = meta.chunks_received + 1
	meta.bytes_received = meta.bytes_received + tonumber(chunk_size)
	redis.call('SET', meta_key, cjson.encode(meta), 'PX', ttl_ms)

	return {'ok', 1, 'duplicate', 0}
`)

var uploadBeginAssemblyScript = goredis.NewScript(`
	local meta_key = KEYS[1]
	local chunks_key = KEYS[2]
	local total_chunks = tonumber(ARGV[1])

	local meta_raw = redis.call('GET', meta_key)
	if not meta_raw then
		return {err = 'upload not found'}
	end

	local meta = cjson.decode(meta_raw)
	if meta.status ~= 'receiving' then
		return {err = 'upload not in receiving state'}
	end

	local chunk_count = redis.call('HLEN', chunks_key)
	if chunk_count < total_chunks then
		return {err = 'not all chunks received'}
	end

	meta.status = 'assembling'
	redis.call('SET', meta_key, cjson.encode(meta), 'KEEPTTL')

	return {'ok', 1}
`)

var uploadCompleteScript = goredis.NewScript(`
	local meta_key = KEYS[1]
	local user_active_key = KEYS[2]
	local expires_key = KEYS[3]
	local upload_id = ARGV[1]

	local meta_raw = redis.call('GET', meta_key)
	if not meta_raw then
		return {err = 'upload not found'}
	end

	local meta = cjson.decode(meta_raw)
	if meta.status ~= 'assembling' then
		return {err = 'upload not in assembling state'}
	end

	meta.status = 'completed'
	redis.call('SET', meta_key, cjson.encode(meta), 'KEEPTTL')
	redis.call('SREM', user_active_key, upload_id)
	redis.call('ZREM', expires_key, upload_id)

	return {'ok', 1}
`)

var uploadAbortScript = goredis.NewScript(`
	local meta_key = KEYS[1]
	local user_active_key = KEYS[2]
	local expires_key = KEYS[3]
	local chunks_key = KEYS[4]
	local upload_id = ARGV[1]

	local meta_raw = redis.call('GET', meta_key)
	if not meta_raw then
		return {err = 'upload not found'}
	end

	local meta = cjson.decode(meta_raw)
	if meta.status == 'completed' then
		return {err = 'upload already completed'}
	end

	meta.status = 'aborted'
	redis.call('SET', meta_key, cjson.encode(meta), 'KEEPTTL')
	redis.call('SREM', user_active_key, upload_id)
	redis.call('ZREM', expires_key, upload_id)
	redis.call('DEL', chunks_key)

	return {'ok', 1}
`)

func (s *UploadStateStore) Begin(ctx context.Context, req BeginUploadRequest) (*UploadSession, error) {
	if s.client == nil {
		return nil, fmt.Errorf("redis unavailable")
	}

	id := uuid.New().String()
	now := time.Now().UTC()
	ttl := uploadSessionTTL
	expiresAt := now.Add(ttl)

	userActiveKey := fmt.Sprintf(uploadUserActiveFmt, req.UserID)
	metaKey := fmt.Sprintf(uploadMetaKeyFmt, id)

	result, err := uploadBeginScript.Run(ctx, s.client,
		[]string{userActiveKey, uploadExpiresSet, metaKey},
		id,
		strconv.FormatUint(uint64(req.UserID), 10),
		req.FileName,
		strconv.FormatInt(req.FileSize, 10),
		strconv.Itoa(req.TotalChunks),
		req.Version,
		req.Description,
		strconv.FormatInt(ttl.Milliseconds(), 10),
		strconv.Itoa(maxActiveUploads),
		strconv.FormatInt(now.UnixMilli(), 10),
		strconv.FormatInt(expiresAt.UnixMilli(), 10),
	).Result()
	if err != nil {
		return nil, fmt.Errorf("begin upload: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) == 0 {
		return nil, fmt.Errorf("unexpected script result")
	}

	resultMap := make(map[string]string)
	for i := 0; i < len(resultSlice); i += 2 {
		resultMap[resultSlice[i].(string)] = fmt.Sprintf("%v", resultSlice[i+1])
	}

	if errMsg, hasErr := resultMap["err"]; hasErr {
		return nil, fmt.Errorf("%s", errMsg)
	}

	return &UploadSession{
		ID:             id,
		UserID:         req.UserID,
		FileName:       req.FileName,
		FileSize:       req.FileSize,
		TotalChunks:    req.TotalChunks,
		ChunksReceived: 0,
		BytesReceived:  0,
		Status:         StatusReceiving,
		Version:        req.Version,
		Description:    req.Description,
		CreatedAt:      now,
		ExpiresAt:      expiresAt,
	}, nil
}

func (s *UploadStateStore) Get(ctx context.Context, id uuid.UUID) (*UploadSession, error) {
	if s.client == nil {
		return nil, fmt.Errorf("redis unavailable")
	}

	metaKey := fmt.Sprintf(uploadMetaKeyFmt, id.String())
	raw, err := s.client.Get(ctx, metaKey).Result()
	if err != nil {
		if err == goredis.Nil {
			return nil, fmt.Errorf("upload not found")
		}
		return nil, fmt.Errorf("get upload: %w", err)
	}

	return parseUploadSession(id.String(), raw)
}

func (s *UploadStateStore) RecordChunk(ctx context.Context, id uuid.UUID, chunk ChunkRecord) error {
	if s.client == nil {
		return fmt.Errorf("redis unavailable")
	}

	metaKey := fmt.Sprintf(uploadMetaKeyFmt, id.String())
	chunksKey := fmt.Sprintf(uploadChunksKeyFmt, id.String())
	ttlMs := strconv.FormatInt(uploadSessionTTL.Milliseconds(), 10)

	result, err := uploadRecordChunkScript.Run(ctx, s.client,
		[]string{metaKey, chunksKey},
		strconv.Itoa(chunk.Index),
		chunk.Hash,
		strconv.FormatInt(chunk.Size, 10),
		ttlMs,
	).Result()
	if err != nil {
		return fmt.Errorf("record chunk: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) == 0 {
		return fmt.Errorf("unexpected script result")
	}

	resultMap := make(map[string]interface{})
	for i := 0; i < len(resultSlice); i += 2 {
		resultMap[resultSlice[i].(string)] = resultSlice[i+1]
	}

	if errMsg, hasErr := resultMap["err"].(string); hasErr {
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

func (s *UploadStateStore) BeginAssembly(ctx context.Context, id uuid.UUID, totalChunks uint) error {
	if s.client == nil {
		return fmt.Errorf("redis unavailable")
	}

	metaKey := fmt.Sprintf(uploadMetaKeyFmt, id.String())
	chunksKey := fmt.Sprintf(uploadChunksKeyFmt, id.String())

	result, err := uploadBeginAssemblyScript.Run(ctx, s.client,
		[]string{metaKey, chunksKey},
		strconv.Itoa(int(totalChunks)),
	).Result()
	if err != nil {
		return fmt.Errorf("begin assembly: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) == 0 {
		return fmt.Errorf("unexpected script result")
	}

	resultMap := make(map[string]interface{})
	for i := 0; i < len(resultSlice); i += 2 {
		resultMap[resultSlice[i].(string)] = resultSlice[i+1]
	}

	if errMsg, hasErr := resultMap["err"].(string); hasErr {
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

func (s *UploadStateStore) Complete(ctx context.Context, id uuid.UUID) error {
	if s.client == nil {
		return fmt.Errorf("redis unavailable")
	}

	uid := id.String()
	metaKey := fmt.Sprintf(uploadMetaKeyFmt, uid)
	session, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	userActiveKey := fmt.Sprintf(uploadUserActiveFmt, session.UserID)

	result, err := uploadCompleteScript.Run(ctx, s.client,
		[]string{metaKey, userActiveKey, uploadExpiresSet},
		uid,
	).Result()
	if err != nil {
		return fmt.Errorf("complete upload: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) == 0 {
		return fmt.Errorf("unexpected script result")
	}

	resultMap := make(map[string]string)
	for i := 0; i < len(resultSlice); i += 2 {
		resultMap[resultSlice[i].(string)] = fmt.Sprintf("%v", resultSlice[i+1])
	}

	if errMsg, hasErr := resultMap["err"]; hasErr {
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

func (s *UploadStateStore) Abort(ctx context.Context, id uuid.UUID) error {
	if s.client == nil {
		return fmt.Errorf("redis unavailable")
	}

	uid := id.String()
	metaKey := fmt.Sprintf(uploadMetaKeyFmt, uid)
	session, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	userActiveKey := fmt.Sprintf(uploadUserActiveFmt, session.UserID)
	chunksKey := fmt.Sprintf(uploadChunksKeyFmt, uid)

	result, err := uploadAbortScript.Run(ctx, s.client,
		[]string{metaKey, userActiveKey, uploadExpiresSet, chunksKey},
		uid,
	).Result()
	if err != nil {
		return fmt.Errorf("abort upload: %w", err)
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) == 0 {
		return fmt.Errorf("unexpected script result")
	}

	resultMap := make(map[string]string)
	for i := 0; i < len(resultSlice); i += 2 {
		resultMap[resultSlice[i].(string)] = fmt.Sprintf("%v", resultSlice[i+1])
	}

	if errMsg, hasErr := resultMap["err"]; hasErr {
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

func (s *UploadStateStore) ListExpired(ctx context.Context, before time.Time, limit int) ([]uuid.UUID, error) {
	if s.client == nil {
		return nil, nil
	}

	raw, err := s.client.ZRangeByScore(ctx, uploadExpiresSet, &goredis.ZRangeBy{
		Min:    "0",
		Max:    strconv.FormatInt(before.UnixMilli(), 10),
		Offset: 0,
		Count:  int64(limit),
	}).Result()
	if err != nil {
		return nil, fmt.Errorf("list expired: %w", err)
	}

	ids := make([]uuid.UUID, 0, len(raw))
	for _, r := range raw {
		id, err := uuid.Parse(r)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func parseUploadSession(id string, raw string) (*UploadSession, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("parse upload session: %w", err)
	}

	s := &UploadSession{ID: id}

	if v, ok := data["user_id"].(float64); ok {
		s.UserID = uint(v)
	}
	if v, ok := data["file_name"].(string); ok {
		s.FileName = v
	}
	if v, ok := data["file_size"].(float64); ok {
		s.FileSize = int64(v)
	}
	if v, ok := data["total_chunks"].(float64); ok {
		s.TotalChunks = int(v)
	}
	if v, ok := data["chunks_received"].(float64); ok {
		s.ChunksReceived = int(v)
	}
	if v, ok := data["bytes_received"].(float64); ok {
		s.BytesReceived = int64(v)
	}
	if v, ok := data["status"].(string); ok {
		s.Status = UploadSessionStatus(v)
	}
	if v, ok := data["version"].(string); ok {
		s.Version = v
	}
	if v, ok := data["description"].(string); ok {
		s.Description = v
	}
	if v, ok := data["created_at"].(float64); ok {
		s.CreatedAt = time.UnixMilli(int64(v))
	}
	if v, ok := data["expires_at"].(float64); ok {
		s.ExpiresAt = time.UnixMilli(int64(v))
	}

	return s, nil
}
