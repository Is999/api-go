-- 原子维护认证入口窗口计数和超限锁，避免 INCR 后未设置 TTL 的永久计数。
-- KEYS[1]: 窗口计数 String；KEYS[2]: 超限锁 String。
-- ARGV: window_seconds, max_attempts, lock_seconds。
local window_seconds = tonumber(ARGV[1])
local max_attempts = tonumber(ARGV[2])
local lock_seconds = tonumber(ARGV[3])
if not window_seconds or window_seconds < 1 or not max_attempts or max_attempts < 1 or not lock_seconds or lock_seconds < 1 then
    return redis.error_reply('invalid auth rate limit arguments')
end
if redis.call('EXISTS', KEYS[2]) == 1 then
    return -1
end

local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], window_seconds)
end
if count > max_attempts then
    redis.call('SET', KEYS[2], '1', 'EX', lock_seconds)
    redis.call('DEL', KEYS[1])
    return -1
end
return count
