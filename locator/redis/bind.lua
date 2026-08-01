local hashKey = KEYS[1]
local nameSetKey = KEYS[2]
local bindKey = ARGV[1]
local bindValue = ARGV[2]
local ttl = tonumber(ARGV[3])
local name = ARGV[4]

local previous = redis.call("HGET", hashKey, bindKey)
redis.call("HSET", hashKey, bindKey, bindValue)
redis.call("HEXPIRE", hashKey, ttl, "FIELDS", 1, bindKey)
redis.call("SADD", nameSetKey, name)
return previous or ""
