local aliveKey = KEYS[1]
local hashKey = KEYS[2]
local instanceID = ARGV[1]

if redis.call("EXISTS", aliveKey) == 0 then
    return redis.call("HDEL", hashKey, instanceID)
end
return 0