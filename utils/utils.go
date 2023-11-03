package utils

import (
	"os"
	"path/filepath"
	"regexp"
)

func IsNumber(s string) bool {
	return regexp.MustCompile(`^\d+$`).MatchString(s)
}

func IsUUID(s string) bool {
	return regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(s)
}

func SaveToTempFile(data []byte, prefix string) (string, error) {
	tmpFile, err := os.CreateTemp("", prefix)
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	if _, err := tmpFile.Write(data); err != nil {
		return "", err
	}

	return filepath.Abs(tmpFile.Name())
}
