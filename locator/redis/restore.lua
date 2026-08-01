local hashKey = KEYS[1]
local nameSetKey = KEYS[2]
local bindKey = ARGV[1]
local currentValue = ARGV[2]
local previousValue = ARGV[3]
local ttl = tonumber(ARGV[4])
local name = ARGV[5]

if redis.call("HGET", hashKey, bindKey) ~= currentValue then
    return 0
end

redis.call("HSET", hashKey, bindKey, previousValue)
redis.call("HEXPIRE", hashKey, ttl, "FIELDS", 1, bindKey)
redis.call("SADD", nameSetKey, name)
return 1
