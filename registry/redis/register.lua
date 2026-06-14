local hashKey = KEYS[1]
local serviceData = ARGV[1]
local fieldKey = ARGV[2]
local ttl = tonumber(ARGV[3])
redis.call('HSET', hashKey, fieldKey, serviceData)
redis.call('HEXPIRE', hashKey, ttl, 'FIELDS', 1, fieldKey)
return 1
