-- 原子删除指定前台用户会话及其索引成员。
-- KEYS[1]: 用户 session Hash；KEYS[2]: 用户 session ZSET 索引。
-- ARGV[1]: 待删除 sid。
local sid = ARGV[1]
if not sid or sid == '' then
    return redis.error_reply('invalid user session delete arguments')
end

local deleted = redis.call('HDEL', KEYS[1], sid)
redis.call('ZREM', KEYS[2], sid)
if redis.call('ZCARD', KEYS[2]) == 0 then
    redis.call('DEL', KEYS[1], KEYS[2])
end
return deleted
