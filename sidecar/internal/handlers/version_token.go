package handlers

// isSafeVersionToken limits values embedded in trampoline and Compose commands.
func isSafeVersionToken(value string) bool {
	if value == "" || len(value) > 200 {
		return false
	}
	for _, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case char == '.' || char == '_' || char == '-' || char == ':':
		default:
			return false
		}
	}
	return true
}
