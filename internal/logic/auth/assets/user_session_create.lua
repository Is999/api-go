-- 原子创建前台用户会话，并按认证版本隔离旧登录态、清理过期会话和执行每用户硬上限。
-- KEYS[1]: 用户 session Hash；KEYS[2]: 用户 session ZSET 索引；KEYS[3]: 用户认证版本 String。
-- ARGV: now_ms, expected_auth_version, sid, token, expires_at_ms, max_sessions。
local now_ms = tonumber(ARGV[1])
local expected_version = ARGV[2]
local sid = ARGV[3]
local token = ARGV[4]
local expires_at_ms = tonumber(ARGV[5])
local max_sessions = tonumber(ARGV[6])

if not now_ms or not expected_version or not string.match(expected_version, '^%d+$') or not string.match(expected_version, '[1-9]') or not expires_at_ms or expires_at_ms <= now_ms or not max_sessions or max_sessions < 1 or sid == '' or token == '' then
    return redis.error_reply('invalid user session create arguments')
end

local function compare_uint(left, right)
    left = string.gsub(left, '^0+', '')
    right = string.gsub(right, '^0+', '')
    if #left ~= #right then
        return #left < #right and -1 or 1
    end
    if left == right then
        return 0
    end
    return left < right and -1 or 1
end

local current_version_text = redis.call('GET', KEYS[3])
if current_version_text then
    if not string.match(current_version_text, '^%d+$') or not string.match(current_version_text, '[1-9]') then
        return redis.error_reply('invalid cached auth version')
    end
    local compared = compare_uint(current_version_text, expected_version)
    if compared > 0 then
        return -1
    end
    if compared < 0 then
        redis.call('DEL', KEYS[1], KEYS[2])
    end
end
redis.call('SET', KEYS[3], expected_version)

local expired = redis.call('ZRANGEBYSCORE', KEYS[2], '-inf', now_ms)
if #expired > 0 then
    redis.call('HDEL', KEYS[1], unpack(expired))
    redis.call('ZREM', KEYS[2], unpack(expired))
end

redis.call('HSET', KEYS[1], sid, token)
redis.call('ZADD', KEYS[2], expires_at_ms, sid)

local overflow = redis.call('ZCARD', KEYS[2]) - max_sessions
local evicted = 0
if overflow > 0 then
    local oldest = redis.call('ZRANGE', KEYS[2], 0, overflow - 1)
    if #oldest > 0 then
        redis.call('HDEL', KEYS[1], unpack(oldest))
        redis.call('ZREM', KEYS[2], unpack(oldest))
        evicted = #oldest
    end
end

local latest = redis.call('ZRANGE', KEYS[2], -1, -1, 'WITHSCORES')
local ttl_seconds = math.max(1, math.ceil((tonumber(latest[2]) - now_ms) / 1000))
redis.call('EXPIRE', KEYS[1], ttl_seconds)
redis.call('EXPIRE', KEYS[2], ttl_seconds)
redis.call('EXPIRE', KEYS[3], ttl_seconds)
return evicted
