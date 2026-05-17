local hashKey = KEYS[1] -- hash key
local expireKey = KEYS[2] -- expire key
local serviceData = ARGV[1] -- service
local fieldKey = ARGV[2] -- fieldKey
local ttl = ARGV[3] -- ttl
redis.Call('HSET', hashKey, fieldKey, serviceData)
redis.Call('SET', expireKey, '1', ttl)
