package strings

func ValueOrDefault(str, def string) string {
	if str == "" {
		return def
	}

	return str
}
