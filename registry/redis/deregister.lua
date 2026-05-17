local hashKey = KEYS[1]
local expireKey = KEYS[2]
local fieldKey = ARGV[1]

redis.Call('HDEL', hashKey, fieldKey)
redis.Call('DEL', expireKey)