local hashKey = KEYS[1]
local nameSetKey = KEYS[2]
local bindKey = ARGV[1]
local instanceID = ARGV[2]
local name = ARGV[3]

if redis.call("HGET", hashKey, bindKey) ~= instanceID then
    return 0
end

redis.call("HDEL", hashKey, bindKey)

if redis.call("HLEN", hashKey) == 0 then
    redis.call("SREM", nameSetKey, name)
end
return 1
