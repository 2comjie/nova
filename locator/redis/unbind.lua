local hashKey = KEYS[1]
local expireKey = KEYS[2]
local nameSetKey = KEYS[3]
local bindKey = ARGV[1]
local name = ARGV[2]

redis.call("HDEL", hashKey, bindKey)
redis.call("DEL", expireKey)

if redis.call("HLEN", hashKey) == 0 then
    redis.call("SREM", nameSetKey, name)
end
return 1
