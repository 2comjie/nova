local hashKey = KEYS[1]
local expireKey = KEYS[2]
local bindKey = ARGV[1]

if redis.call("EXISTS", expireKey) == 0 then
    return redis.call("HDEL", hashKey, bindKey)
end
return 0
