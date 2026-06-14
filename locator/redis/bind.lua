local hashKey = KEYS[1]
local nameSetKey = KEYS[2]
local bindKey = ARGV[1]
local bindValue = ARGV[2]
local ttl = tonumber(ARGV[3])
local name = ARGV[4]

local exists = redis.call("HEXISTS", hashKey, bindKey)
redis.call("HSET", hashKey, bindKey, bindValue)
redis.call("HEXPIRE", hashKey, ttl, "FIELDS", 1, bindKey)
redis.call("SADD", nameSetKey, name)
return exists == 1 and 2 or 1
