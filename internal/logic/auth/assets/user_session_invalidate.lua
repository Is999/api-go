-- 按数据库认证版本原子失效用户全部登录态，禁止旧版本覆盖新版本。
-- KEYS[1]: 用户 session Hash；KEYS[2]: 用户 session ZSET 索引；KEYS[3]: 用户认证版本 String。
-- ARGV: 已由业务用户表提交的新 auth_version、版本栅栏 TTL 秒数。
local expected_version = ARGV[1]
local version_ttl_seconds = tonumber(ARGV[2])
if not expected_version or not string.match(expected_version, '^%d+$') or not string.match(expected_version, '[1-9]') or not version_ttl_seconds or version_ttl_seconds < 1 then
    return redis.error_reply('invalid user session invalidate arguments')
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
    if compare_uint(current_version_text, expected_version) > 0 then
        return -1
    end
end

local invalidated = redis.call('HLEN', KEYS[1])
redis.call('DEL', KEYS[1], KEYS[2])
redis.call('SET', KEYS[3], expected_version, 'EX', version_ttl_seconds)
return invalidated
