-- 原子校验前台用户会话 token、过期时间和认证版本。
-- KEYS[1]: 用户 session Hash；KEYS[2]: 用户 session ZSET 索引；KEYS[3]: 用户认证版本 String。
-- ARGV: expected_auth_version, sid, token, now_ms。
local expected_version = ARGV[1]
local sid = ARGV[2]
local token = ARGV[3]
local now_ms = tonumber(ARGV[4])
if expected_version == '' or sid == '' or token == '' or not now_ms then
    return 0
end
if redis.call('GET', KEYS[3]) ~= expected_version then
    return 0
end

local saved_token = redis.call('HGET', KEYS[1], sid)
local expires_at_ms = tonumber(redis.call('ZSCORE', KEYS[2], sid))
if not saved_token or saved_token ~= token or not expires_at_ms then
    return 0
end
if expires_at_ms <= now_ms then
    redis.call('HDEL', KEYS[1], sid)
    redis.call('ZREM', KEYS[2], sid)
    return 0
end
return 1
