package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func (cfg apiConfig) GetAssetPath(fileName string) string {
	return fmt.Sprintf("%s/%s", cfg.assetsRoot, fileName)
}

func (cfg apiConfig) GetAssetURL(fileName string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, fileName)
}

func GetMediaExtension(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}

	return "." + parts[1]
}

func GetRandomFileName(extension string) (string, error) {
	randomFileNameB := make([]byte, 32)
	_, err := rand.Read(randomFileNameB)
	if err != nil {
		return "", err
	}
	randomFileName := base64.RawURLEncoding.EncodeToString(randomFileNameB)
	fileName := fmt.Sprintf("%s%s", randomFileName, extension)
	return fileName, nil
}
