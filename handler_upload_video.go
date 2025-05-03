package main

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
	"github.com/google/uuid"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	maxVideoMemory = 1 << 30 // 1 GB
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	http.MaxBytesReader(w, r.Body, maxVideoMemory)

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

	fmt.Println("uploading video", videoID, "by user", userID)

	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "Video not found", err)
			return
		}

		respondWithError(w, http.StatusInternalServerError, "Couldn't get video", err)
		return
	}

	if video.UserID != userID {
		respondWithError(w, http.StatusForbidden, "You don't have permission to upload this video", nil)
		return
	}

	file, fileHeader, err := r.FormFile("video")
	if file == nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't get file from form", err)
		return
	}
	defer file.Close()

	mediaType := fileHeader.Header.Get("Content-Type")
	mimeType, _, err := mime.ParseMediaType(mediaType)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Couldn't parse media type", err)
		return
	}

	if mimeType != "video/mp4" {
		respondWithError(w, http.StatusBadRequest, "media type must be video/mp4", err)
		return
	}

	localFile, err := os.CreateTemp(
		"",
		fmt.Sprintf(
			"video-%s%s",
			time.Now().Format("20060102150405"),
			GetMediaExtension(mediaType),
		),
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't create temp file", err)
		return
	}

	defer os.Remove(localFile.Name())
	defer localFile.Close()

	_, err = io.Copy(localFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't save video", err)
		return
	}

	fileName, err := GetRandomFileName(GetMediaExtension(mediaType))
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't generate file name", err)
		return
	}

	ratio, err := GetVideoAspectRatio(localFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't get video aspect ratio", err)
		return
	}

	prefixS3 := "other"
	if ratio == "16:9" {
		prefixS3 = "landscape"
	} else if ratio == "9:16" {
		prefixS3 = "portrait"
	}

	processedVideoPath, err := ProcessVideoForFastStart(localFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't process video", err)
		return
	}
	defer os.Remove(processedVideoPath)

	processedFile, err := os.Open(processedVideoPath)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't open processed video", err)
		return
	}
	defer processedFile.Close()

	fullFileName := fmt.Sprintf("%s/%s", prefixS3, fileName)

	_, err = cfg.s3Client.PutObject(
		r.Context(),
		&s3.PutObjectInput{
			Bucket:      aws.String(cfg.s3Bucket),
			Key:         aws.String(fullFileName),
			Body:        processedFile,
			ContentType: aws.String(mimeType),
		},
	)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't upload video", err)
		return
	}

	videoURL := cfg.GetS3URL(fullFileName)
	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Couldn't update video", err)
		return
	}

	signedVideo, err := cfg.dbVideoToSignedVideo(video)

	respondWithJSON(w, http.StatusOK, signedVideo)
	return
}

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (*database.Video, error) {
	if video.VideoURL == nil {
		return &video, nil
	}

	parts := strings.Split(*video.VideoURL, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("Invalid video URL: %s", *video.VideoURL)
	}

	presignURL, err := GeneratePresignS3URL(
		cfg.s3Client,
		parts[0],
		parts[1],
		time.Hour*24,
	)
	if err != nil {
		return nil, fmt.Errorf("Couldn't generate presign URL: %w", err)
	}

	video.VideoURL = &presignURL
	return &video, nil
}

func (cfg *apiConfig) GetS3URL(object string) string {
	return fmt.Sprintf("%s,%s", cfg.s3Bucket, object)
}
