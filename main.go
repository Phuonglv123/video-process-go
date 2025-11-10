package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// Config holds all application configuration
type Config struct {
	MinIOEndpoint  string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOBucket    string
	MinIOSecure    bool
	BackupDir      string
	ProcessedDir   string
	LogDir         string
	Workers        int
}

// ProcessLog represents a single video processing log entry
type ProcessLog struct {
	File          string    `json:"file"`
	OriginalCodec string    `json:"original_codec"`
	NewCodec      string    `json:"new_codec"`
	SizeBefore    int64     `json:"size_before"`
	SizeAfter     int64     `json:"size_after"`
	ProcessedAt   time.Time `json:"processed_at"`
	Status        string    `json:"status"`
}

// VideoProcessor handles video processing operations
type VideoProcessor struct {
	config      *Config
	minioClient *minio.Client
	logMutex    sync.Mutex
	stats       struct {
		sync.Mutex
		total     int
		converted int
		skipped   int
		failed    int
	}
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Load configuration
	config := loadConfig()

	// Create necessary directories
	if err := createDirectories(config); err != nil {
		log.Fatalf("Failed to create directories: %v", err)
	}

	// Initialize MinIO client
	minioClient, err := initMinIO(config)
	if err != nil {
		log.Fatalf("Failed to initialize MinIO: %v", err)
	}

	processor := &VideoProcessor{
		config:      config,
		minioClient: minioClient,
	}

	// Process videos
	if err := processor.processAllVideos(); err != nil {
		log.Fatalf("Failed to process videos: %v", err)
	}

	// Print final statistics
	processor.stats.Lock()
	fmt.Printf("\n✅ Completed all tasks (Total: %d, Converted: %d, Skipped: %d, Failed: %d)\n",
		processor.stats.total, processor.stats.converted, processor.stats.skipped, processor.stats.failed)
	processor.stats.Unlock()
}

func loadConfig() *Config {
	secure, _ := strconv.ParseBool(getEnv("MINIO_SECURE", "true"))
	workers, _ := strconv.Atoi(getEnv("WORKERS", "8"))

	return &Config{
		MinIOEndpoint:  getEnv("MINIO_ENDPOINT", "play.min.io"),
		MinIOAccessKey: getEnv("MINIO_ACCESS_KEY", ""),
		MinIOSecretKey: getEnv("MINIO_SECRET_KEY", ""),
		MinIOBucket:    getEnv("MINIO_BUCKET", "videos"),
		MinIOSecure:    secure,
		BackupDir:      getEnv("BACKUP_DIR", "./backup"),
		ProcessedDir:   getEnv("PROCESSED_DIR", "./processed"),
		LogDir:         getEnv("LOG_DIR", "./logs"),
		Workers:        workers,
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func createDirectories(config *Config) error {
	dirs := []string{config.BackupDir, config.ProcessedDir, config.LogDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func initMinIO(config *Config) (*minio.Client, error) {
	client, err := minio.New(config.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.MinIOAccessKey, config.MinIOSecretKey, ""),
		Secure: config.MinIOSecure,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	// Check if bucket exists
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, config.MinIOBucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("bucket %s does not exist", config.MinIOBucket)
	}

	return client, nil
}

func (p *VideoProcessor) processAllVideos() error {
	ctx := context.Background()

	// List all video objects
	videoExtensions := map[string]bool{
		".mp4": true, ".mov": true, ".avi": true,
		".mkv": true, ".flv": true, ".wmv": true,
	}

	var videos []string
	objectCh := p.minioClient.ListObjects(ctx, p.config.MinIOBucket, minio.ListObjectsOptions{
		Recursive: true,
	})

	for object := range objectCh {
		if object.Err != nil {
			log.Printf("Error listing objects: %v", object.Err)
			continue
		}

		ext := strings.ToLower(filepath.Ext(object.Key))
		if videoExtensions[ext] {
			videos = append(videos, object.Key)
		}
	}

	if len(videos) == 0 {
		log.Println("No videos found in bucket")
		return nil
	}

	log.Printf("Found %d videos to process\n", len(videos))

	// Create worker pool
	videoChan := make(chan string, len(videos))
	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < p.config.Workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for videoKey := range videoChan {
				p.processVideo(workerID, videoKey)
			}
		}(i + 1)
	}

	// Feed videos to workers
	for _, videoKey := range videos {
		videoChan <- videoKey
	}
	close(videoChan)

	// Wait for all workers to finish
	wg.Wait()

	return nil
}

func (p *VideoProcessor) processVideo(workerID int, videoKey string) {
	p.stats.Lock()
	p.stats.total++
	p.stats.Unlock()

	ctx := context.Background()

	// Get object info
	objInfo, err := p.minioClient.StatObject(ctx, p.config.MinIOBucket, videoKey, minio.StatObjectOptions{})
	if err != nil {
		log.Printf("[Worker %d] ❌ Failed to stat %s: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:        videoKey,
			ProcessedAt: time.Now(),
			Status:      "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}

	// Download video to temporary location for probing
	tempProbeFile := filepath.Join(os.TempDir(), fmt.Sprintf("probe_%d_%s", workerID, filepath.Base(videoKey)))
	if err := p.downloadFile(videoKey, tempProbeFile); err != nil {
		log.Printf("[Worker %d] ❌ Failed to download %s for probing: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:        videoKey,
			SizeBefore:  objInfo.Size,
			ProcessedAt: time.Now(),
			Status:      "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}
	defer os.Remove(tempProbeFile)

	// Check codec using ffprobe
	codec, err := p.getVideoCodec(tempProbeFile)
	if err != nil {
		log.Printf("[Worker %d] ⚠️ Failed to probe %s: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:        videoKey,
			SizeBefore:  objInfo.Size,
			ProcessedAt: time.Now(),
			Status:      "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}

	// If codec is already h264, skip
	if strings.ToLower(codec) == "h264" {
		log.Printf("[Worker %d] ✅ %s OK (h264)\n", workerID, videoKey)
		p.logProcess(ProcessLog{
			File:          videoKey,
			OriginalCodec: codec,
			NewCodec:      "h264",
			SizeBefore:    objInfo.Size,
			SizeAfter:     objInfo.Size,
			ProcessedAt:   time.Now(),
			Status:        "skipped",
		})
		p.stats.Lock()
		p.stats.skipped++
		p.stats.Unlock()
		return
	}

	log.Printf("[Worker %d] ⚠️ %s codec=%s → processing...\n", workerID, videoKey, codec)

	// Download to backup
	backupPath := filepath.Join(p.config.BackupDir, videoKey)
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		log.Printf("[Worker %d] ❌ Failed to create backup directory for %s: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:          videoKey,
			OriginalCodec: codec,
			SizeBefore:    objInfo.Size,
			ProcessedAt:   time.Now(),
			Status:        "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}

	if err := p.downloadFile(videoKey, backupPath); err != nil {
		log.Printf("[Worker %d] ❌ Failed to backup %s: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:          videoKey,
			OriginalCodec: codec,
			SizeBefore:    objInfo.Size,
			ProcessedAt:   time.Now(),
			Status:        "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}

	// Convert video
	processedPath := filepath.Join(p.config.ProcessedDir, videoKey)
	if err := os.MkdirAll(filepath.Dir(processedPath), 0755); err != nil {
		log.Printf("[Worker %d] ❌ Failed to create processed directory for %s: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:          videoKey,
			OriginalCodec: codec,
			SizeBefore:    objInfo.Size,
			ProcessedAt:   time.Now(),
			Status:        "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}

	if err := p.convertToH264(backupPath, processedPath); err != nil {
		log.Printf("[Worker %d] ❌ Failed to convert %s: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:          videoKey,
			OriginalCodec: codec,
			SizeBefore:    objInfo.Size,
			ProcessedAt:   time.Now(),
			Status:        "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}

	// Get size of processed file
	processedInfo, err := os.Stat(processedPath)
	if err != nil {
		log.Printf("[Worker %d] ⚠️ Failed to stat processed file %s: %v\n", workerID, videoKey, err)
		processedInfo = nil
	}

	// Upload back to MinIO
	if err := p.uploadFile(processedPath, videoKey); err != nil {
		log.Printf("[Worker %d] ❌ Failed to upload %s: %v\n", workerID, videoKey, err)
		p.logProcess(ProcessLog{
			File:          videoKey,
			OriginalCodec: codec,
			NewCodec:      "h264",
			SizeBefore:    objInfo.Size,
			SizeAfter:     processedInfo.Size(),
			ProcessedAt:   time.Now(),
			Status:        "failed",
		})
		p.stats.Lock()
		p.stats.failed++
		p.stats.Unlock()
		return
	}

	var sizeAfter int64
	if processedInfo != nil {
		sizeAfter = processedInfo.Size()
	}

	log.Printf("[Worker %d] ✅ %s converted and uploaded\n", workerID, videoKey)
	p.logProcess(ProcessLog{
		File:          videoKey,
		OriginalCodec: codec,
		NewCodec:      "h264",
		SizeBefore:    objInfo.Size,
		SizeAfter:     sizeAfter,
		ProcessedAt:   time.Now(),
		Status:        "success",
	})
	p.stats.Lock()
	p.stats.converted++
	p.stats.Unlock()
}

func (p *VideoProcessor) downloadFile(objectKey, destPath string) error {
	ctx := context.Background()
	return p.minioClient.FGetObject(ctx, p.config.MinIOBucket, objectKey, destPath, minio.GetObjectOptions{})
}

func (p *VideoProcessor) uploadFile(sourcePath, objectKey string) error {
	ctx := context.Background()
	_, err := p.minioClient.FPutObject(ctx, p.config.MinIOBucket, objectKey, sourcePath, minio.PutObjectOptions{
		ContentType: "video/mp4",
	})
	return err
}

func (p *VideoProcessor) getVideoCodec(videoPath string) (string, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ffprobe failed: %w", err)
	}

	codec := strings.TrimSpace(string(output))
	return codec, nil
}

func (p *VideoProcessor) convertToH264(inputPath, outputPath string) error {
	cmd := exec.Command("ffmpeg",
		"-i", inputPath,
		"-c:v", "libx264",
		"-c:a", "aac",
		"-y", // Overwrite output file
		outputPath,
	)

	// Capture stderr for error messages
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
	}

	return nil
}

func (p *VideoProcessor) logProcess(entry ProcessLog) {
	p.logMutex.Lock()
	defer p.logMutex.Unlock()

	logPath := filepath.Join(p.config.LogDir, "process.log")
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("Failed to open log file: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(entry); err != nil {
		log.Printf("Failed to write log entry: %v", err)
	}
}
