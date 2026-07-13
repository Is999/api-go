-- 原子轮换前台用户会话，确保同一个旧 token 最多刷新成功一次。
-- KEYS[1]: 用户 session Hash；KEYS[2]: 用户 session ZSET 索引；KEYS[3]: 用户认证版本 String。
-- ARGV: now_ms, expected_auth_version, sid, previous_token, new_token, expires_at_ms。
local now_ms = tonumber(ARGV[1])
local expected_version = ARGV[2]
local sid = ARGV[3]
local previous_token = ARGV[4]
local new_token = ARGV[5]
local expires_at_ms = tonumber(ARGV[6])

if not now_ms or expected_version == '' or sid == '' or previous_token == '' or new_token == '' or not expires_at_ms or expires_at_ms <= now_ms then
    return redis.error_reply('invalid user session rotate arguments')
end
if redis.call('GET', KEYS[3]) ~= expected_version then
    return -1
end

local expired = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now_ms)
if #expired > 0 then
    redis.call('HDEL', KEYS[1], unpack(expired))
    redis.call('ZREM', KEYS[2], unpack(expired))
end

local saved_token = redis.call('HGET', KEYS[1], sid)
local previous_expires_at = tonumber(redis.call('ZSCORE', KEYS[2], sid))
if not saved_token or saved_token ~= previous_token or not previous_expires_at or previous_expires_at <= now_ms then
    return 0
end

redis.call('HSET', KEYS[1], sid, new_token)
redis.call('ZADD', KEYS[2], expires_at_ms, sid)

local latest = redis.call('ZRANGE', KEYS[2], -1, -1, 'WITHSCORES')
local ttl_seconds = math.max(1, math.ceil((tonumber(latest[2]) - now_ms) / 1000))
redis.call('EXPIRE', KEYS[1], ttl_seconds)
redis.call('EXPIRE', KEYS[2], ttl_seconds)
redis.call('EXPIRE', KEYS[3], ttl_seconds)
return 1
