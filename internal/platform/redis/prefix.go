package redis

import (
	"context"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"
)

// redisKeyPositions maps the Redis commands the backend issues to the
// positions of their key arguments inside the command argument list.
// A command that is missing from this table keeps its key unprefixed,
// so the table has to stay in sync with the commands the code issues.
//
// The pub/sub commands are deliberately absent: go-redis writes
// SUBSCRIBE and PUBLISH straight to the connection, bypassing the
// hook chain, so a hook can never rewrite a channel symmetrically.
// The fanout channel is prefixed by its publisher instead.
var redisKeyPositions = map[string][]int{
	"get": {1}, "set": {1}, "setnx": {1}, "del": {1}, "unlink": {1},
	"exists": {1}, "incr": {1}, "incrby": {1}, "decr": {1},
	"type": {1}, "ttl": {1}, "pttl": {1},
	"expire": {1}, "pexpire": {1}, "expireat": {1}, "pexpireat": {1},
	"persist": {1},
	"hget":    {1}, "hset": {1}, "hdel": {1}, "hgetall": {1}, "hincrby": {1},
	"sadd": {1}, "srem": {1}, "smembers": {1}, "sismember": {1},
	"zadd": {1}, "zrem": {1}, "zrange": {1}, "zcard": {1}, "zscore": {1},
	"zrangebyscore": {1},
	"lpush":         {1}, "rpush": {1}, "lrange": {1}, "lrem": {1},
	"keys": {1},
}

// redisScanCommands take an optional MATCH pattern whose position
// depends on the options the caller passed, so the pattern is
// located by scanning the argument list instead of by a fixed index.
var redisScanCommands = map[string]bool{
	"scan": true, "hscan": true, "sscan": true, "zscan": true,
}

// keyPrefixHook rewrites the key arguments of every Redis command so
// two deployments can share one Redis instance without sharing rate
// limit buckets, invitations, timers or pub/sub channels.
type keyPrefixHook struct {
	prefix string
}

func (h keyPrefixHook) DialHook(next goredis.DialHook) goredis.DialHook {
	return next
}

func (h keyPrefixHook) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h.rewrite(cmd)
		return next(ctx, cmd)
	}
}

func (h keyPrefixHook) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		for _, cmd := range cmds {
			h.rewrite(cmd)
		}
		return next(ctx, cmds)
	}
}

func (h keyPrefixHook) rewrite(cmd goredis.Cmder) {
	if h.prefix == "" {
		return
	}
	args := cmd.Args()
	if len(args) < 2 {
		return
	}
	name := strings.ToLower(cmd.FullName())
	if name == "eval" || name == "evalsha" {
		h.rewriteScriptKeys(args)
		return
	}
	if redisScanCommands[name] {
		h.rewriteScanArgs(name, args)
		return
	}
	for _, pos := range redisKeyPositions[name] {
		h.prefixArg(args, pos)
	}
}

// rewriteScanArgs prefixes the key and the MATCH pattern of a SCAN
// family command. SCAN itself has no key: its first argument is the
// cursor, which must not be touched.
func (h keyPrefixHook) rewriteScanArgs(name string, args []interface{}) {
	if name != "scan" {
		h.prefixArg(args, 1)
	}
	for i := 1; i < len(args)-1; i++ {
		option, ok := args[i].(string)
		if !ok || !strings.EqualFold(option, "match") {
			continue
		}
		h.prefixArg(args, i+1)
		return
	}
}

// rewriteScriptKeys prefixes the KEYS of an EVAL/EVALSHA command. The
// argument layout is "<script> <numkeys> <key>...".
func (h keyPrefixHook) rewriteScriptKeys(args []interface{}) {
	if len(args) < 3 {
		return
	}
	numKeys, err := strconv.Atoi(stringArg(args[2]))
	if err != nil || numKeys <= 0 {
		return
	}
	for i := 3; i < 3+numKeys && i < len(args); i++ {
		h.prefixArg(args, i)
	}
}

// prefixArg prefixes the string argument at pos. An argument that
// already carries the prefix is left alone: Redis hands physical keys
// back to the caller (SCAN and KEYS results) and those must not be
// prefixed twice when they are fed back into the client.
func (h keyPrefixHook) prefixArg(args []interface{}, pos int) {
	if pos < 0 || pos >= len(args) {
		return
	}
	key, ok := args[pos].(string)
	if !ok || strings.HasPrefix(key, h.prefix) {
		return
	}
	args[pos] = h.prefix + key
}

func stringArg(v interface{}) string {
	switch value := v.(type) {
	case string:
		return value
	case []byte:
		return string(value)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case uint64:
		return strconv.FormatUint(value, 10)
	default:
		return ""
	}
}
