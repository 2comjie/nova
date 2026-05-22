local hashKey = KEYS[1]
local expireKey = KEYS[2]
local nameSetKey = KEYS[3]
local bindKey = ARGV[1]
local bindValue = ARGV[2]
local ttl = tonumber(ARGV[3])
local name = ARGV[4]

local exists = redis.call("HEXISTS", hashKey, bindKey)
redis.call("HSET", hashKey, bindKey, bindValue)
redis.call("SET", expireKey, bindKey, "EX", ttl)
redis.call("SADD", nameSetKey, name)
return exists == 1 and 2 or 1
