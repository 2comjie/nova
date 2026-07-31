-- KEYS[1] lock key
-- ARGV[1] lock token
-- ARGV[2] ttl milliseconds
if redis.call("GET", KEYS[1]) == ARGV[1] then
    return redis.call("PEXPIRE", KEYS[1], ARGV[2])
end

return 0
