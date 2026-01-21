package utils

// BuildKey constructs a key with the given prefix
func BuildKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + key
}

// BuildKeys constructs multiple keys with the given prefix
func BuildKeys(prefix string, keys ...string) []string {
	result := make([]string, len(keys))
	for i, key := range keys {
		result[i] = BuildKey(prefix, key)
	}
	return result
}
