local hashKey = KEYS[1]
local fieldKey = ARGV[1]
redis.call('HDEL', hashKey, fieldKey)
return 1
