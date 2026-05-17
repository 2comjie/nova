local hashKey = KEYS[1]
local expireKey = KEYS[2]
local fieldKey = ARGV[1]
redis.call('HDEL', hashKey, fieldKey)
redis.call('DEL', expireKey)
