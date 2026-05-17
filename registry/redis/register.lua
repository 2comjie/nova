local hashKey = KEYS[1]
local expireKey = KEYS[2]
local serviceData = ARGV[1]
local fieldKey = ARGV[2]
local ttl = tonumber(ARGV[3])
redis.call('HSET', hashKey, fieldKey, serviceData)
redis.call('SET', expireKey, '1', 'EX', ttl)
return 1
