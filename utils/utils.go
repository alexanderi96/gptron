package utils

import "regexp"

func IsNumber(s string) bool {
	return regexp.MustCompile(`^\d+$`).MatchString(s)
}

func IsUUID(s string) bool {
	return regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(s)
}
