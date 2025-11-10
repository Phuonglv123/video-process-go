# Video Processor - Go CLI Tool

A Go CLI tool that automatically processes videos stored in MinIO, converting non-h264 encoded videos to h264 format using FFmpeg.

## Features

- 🔌 **MinIO Integration**: Connects to MinIO object storage to process videos
- 🎥 **Smart Codec Detection**: Uses `ffprobe` to detect video codecs
- 🔄 **Automatic Conversion**: Converts non-h264 videos to h264 using `ffmpeg`
- 📦 **Backup System**: Backs up original videos before processing
- 📊 **Detailed Logging**: JSON-formatted logs for all processing operations
- ⚡ **Parallel Processing**: Multi-threaded worker pool for efficient processing
- 🐳 **Docker Ready**: Easy deployment with Docker
- ⚙️ **Flexible Configuration**: All settings configurable via `.env` file

## Project Structure

```
video-processor/
├── main.go              # Main application code
├── go.mod              # Go module dependencies
├── go.sum              # Go module checksums
├── Dockerfile          # Docker build configuration
├── .env.example        # Example environment configuration
├── .gitignore          # Git ignore rules
├── logs/               # Processing logs (JSON format)
│   └── process.log
├── backup/             # Backup of original videos
└── processed/          # Processed videos (before upload)
```

## Prerequisites

### Local Development
- Go 1.22 or higher
- FFmpeg and FFprobe installed
- Access to a MinIO instance

### Docker
- Docker installed
- Access to a MinIO instance

## Installation

### 1. Clone the repository

```bash
git clone https://github.com/Phuonglv123/video-process-go.git
cd video-process-go
```

### 2. Configure Environment

Copy the example environment file and configure it with your MinIO credentials:

```bash
cp .env.example .env
```

Edit `.env` with your settings:

```bash
# MinIO config
MINIO_ENDPOINT=play.min.io
MINIO_ACCESS_KEY=YOUR_ACCESS_KEY
MINIO_SECRET_KEY=YOUR_SECRET_KEY
MINIO_BUCKET=videos
MINIO_SECURE=true

# Path local
BACKUP_DIR=./backup
PROCESSED_DIR=./processed
LOG_DIR=./logs

# Worker setting
WORKERS=8
```

### 3. Install Dependencies

```bash
go mod download
```

## Usage

### Local Execution

1. **Build the application:**

```bash
go build -o video-processor .
```

2. **Run the processor:**

```bash
./video-processor
```

### Docker Execution

1. **Build the Docker image:**

```bash
docker build -t video-processor .
```

2. **Run with Docker:**

```bash
docker run --rm \
  -v $(pwd)/.env:/app/.env \
  -v $(pwd)/logs:/app/logs \
  -v $(pwd)/backup:/app/backup \
  video-processor
```

Or use docker-compose:

```bash
docker-compose up
```

Or run in detached mode:

```bash
docker-compose up -d
```

## Configuration Options

| Variable | Description | Default |
|----------|-------------|---------|
| `MINIO_ENDPOINT` | MinIO server endpoint | `play.min.io` |
| `MINIO_ACCESS_KEY` | MinIO access key | *(required)* |
| `MINIO_SECRET_KEY` | MinIO secret key | *(required)* |
| `MINIO_BUCKET` | Bucket name containing videos | `videos` |
| `MINIO_SECURE` | Use HTTPS (true/false) | `true` |
| `BACKUP_DIR` | Local backup directory | `./backup` |
| `PROCESSED_DIR` | Local processed files directory | `./processed` |
| `LOG_DIR` | Log files directory | `./logs` |
| `WORKERS` | Number of parallel workers | `8` |

## How It Works

1. **Initialization**: Loads configuration from `.env` file and connects to MinIO
2. **Video Discovery**: Lists all video files in the specified bucket (supports: `.mp4`, `.mov`, `.avi`, `.mkv`, `.flv`, `.wmv`)
3. **Codec Detection**: Uses `ffprobe` to check the video codec of each file
4. **Processing Decision**:
   - If codec is already `h264`: Skips processing and logs as "skipped"
   - If codec is not `h264`: Proceeds with conversion
5. **Conversion Process**:
   - Downloads original video to backup directory
   - Converts to h264 using `ffmpeg -c:v libx264 -c:a aac`
   - Uploads processed video back to MinIO (overwrites original)
6. **Logging**: Records all operations to `logs/process.log` in JSON format

## Log Format

Each processed video generates a JSON log entry:

```json
{
  "file": "path/in/bucket.mp4",
  "original_codec": "hevc",
  "new_codec": "h264",
  "size_before": 123456789,
  "size_after": 98765432,
  "processed_at": "2025-11-10T17:00:00Z",
  "status": "success"
}
```

**Status Values:**
- `success`: Video was successfully converted and uploaded
- `skipped`: Video already uses h264 codec
- `failed`: An error occurred during processing

## Example Output

```
Found 125 videos to process
[Worker 1] ✅ videos/intro.mp4 OK (h264)
[Worker 3] ⚠️ videos/demo.mov codec=hevc → processing...
[Worker 3] ✅ videos/demo.mov converted and uploaded
[Worker 2] ✅ videos/tutorial.mp4 OK (h264)
[Worker 5] ⚠️ videos/promo.avi codec=mpeg4 → processing...
[Worker 5] ✅ videos/promo.avi converted and uploaded

✅ Completed all tasks (Total: 125, Converted: 43, Skipped: 82, Failed: 0)
```

## Supported Video Formats

- `.mp4` - MPEG-4 Part 14
- `.mov` - QuickTime File Format
- `.avi` - Audio Video Interleave
- `.mkv` - Matroska Video
- `.flv` - Flash Video
- `.wmv` - Windows Media Video

## Troubleshooting

### FFmpeg not found
**Error**: `ffprobe failed` or `ffmpeg failed`

**Solution**: Install FFmpeg:
- **Ubuntu/Debian**: `sudo apt-get install ffmpeg`
- **macOS**: `brew install ffmpeg`
- **Docker**: Already included in the Docker image

### MinIO connection failed
**Error**: `Failed to initialize MinIO`

**Solution**: 
- Check your MinIO endpoint, access key, and secret key in `.env`
- Verify network connectivity to MinIO server
- Ensure the bucket exists

### Permission denied
**Error**: `Failed to create directory`

**Solution**: Ensure the application has write permissions for backup, processed, and logs directories

## Development

### Running Tests

```bash
go test ./...
```

### Code Formatting

```bash
go fmt ./...
```

### Building for Production

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o video-processor-linux .

# Windows
GOOS=windows GOARCH=amd64 go build -o video-processor.exe .

# macOS
GOOS=darwin GOARCH=amd64 go build -o video-processor-mac .
```

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Support

For issues and questions, please open an issue on GitHub.