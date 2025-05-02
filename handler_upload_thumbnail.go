package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"os"
)

const (
	maxMemory = 10 << 20 // 10 MB
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	err = r.ParseMultipartForm(maxMemory)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse multipart form", err)
		return
	}

	file, fileHeader, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file from form", err)
		return
	}

	mediaType := fileHeader.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse media type", err)
		return
	}

	if mimeType != "image/jpeg" && mimeType != "image/png" {
		respondWithError(w, http.StatusBadRequest, "media type must be image/jpeg, image/png", err)
		return
	}

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "Couldn't find video", err)
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You don't have permission to upload this video", nil)
		return
	}

	extension := GetMediaExtension(mimeType)
	randomFileNameB := make([]byte, 32)
	_, err = rand.Read(randomFileNameB)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate random file name", err)
		return
	}

	randomFileName := base64.RawURLEncoding.EncodeToString(randomFileNameB)
	fileName := fmt.Sprintf("%s%s", randomFileName, extension)
	filePath := cfg.GetAssetPath(fileName)

	f, err := os.Create(filePath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create file", err)
		return
	}

	_, err = io.Copy(f, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't copy file", err)
		return
	}

	thumbnailURL := cfg.GetAssetURL(fileName)
	video.ThumbnailURL = &thumbnailURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
