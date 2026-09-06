if redis.call("HGET", KEYS[1], ARGV[1]) ~= ARGV[2] then
    return 0
end
redis.call("HEXPIRE", KEYS[1], ARGV[3], "FIELDS", 1, ARGV[1])
return 1
