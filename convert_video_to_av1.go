package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type Config struct {
	BasePath   string `json:"BasePath"`
	FfmpegPath string `json:"FfmpegPath"`
}

var videoExtensions = map[string]bool{
	".mp4":  true,
	".avi":  true,
	".mkv":  true,
	".mov":  true,
	".wmv":  true,
	".flv":  true,
	".webm": true,
}

func loadConfig(filename string) (Config, error) {
	var config Config
	configFile, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("설정 파일 '%s'을(를) 읽는 중 오류 발생: %w", filename, err)
	}
	err = json.Unmarshal(configFile, &config)
	if err != nil {
		return config, fmt.Errorf("설정 파일 '%s'의 JSON 파싱 중 오류 발생: %w", filename, err)
	}

	if config.BasePath == "" {
		return config, fmt.Errorf("설정 파일에 'BasePath'가 지정되지 않았습니다")
	}

	if config.FfmpegPath == "" {
		log.Println("'FfmpegPath'가 설정 파일에 지정되지 않았습니다. 기본값 'ffmpeg'를 사용합니다 (PATH 환경 변수에서 검색).")
		config.FfmpegPath = "ffmpeg"
	}

	if _, err := os.Stat(config.BasePath); os.IsNotExist(err) {
		return config, fmt.Errorf("설정에 지정된 BasePath '%s'이(가) 존재하지 않습니다", config.BasePath)
	}

	if _, err := exec.LookPath(config.FfmpegPath); err != nil {
		if _, statErr := os.Stat(config.FfmpegPath); os.IsNotExist(statErr) {
			return config, fmt.Errorf("설정에 지정된 FfmpegPath '%s'를 찾을 수 없습니다. PATH 환경 변수를 확인하거나 올바른 전체 경로를 지정해주세요: %w", config.FfmpegPath, err)
		} else if statErr != nil {
			return config, fmt.Errorf("FfmpegPath '%s' 확인 중 오류 발생: %w", config.FfmpegPath, statErr)
		}
		log.Printf("경고: FfmpegPath '%s' 파일은 존재하지만 실행 가능한 상태인지 확인하지 못했습니다. PATH 또는 권한 문제를 확인하세요.", config.FfmpegPath)
	}

	return config, nil
}

// 디렉토리 이름이 yyyymmdd 형식인지 확인하는 함수
func isDateFormatDir(dirName string) bool {
	datePattern := regexp.MustCompile(`^\d{8}$`) // yyyymmdd 형식 확인
	return datePattern.MatchString(dirName)
}

// 특정 디렉토리 내의 비디오 파일 찾기
func findVideosInDir(dirPath string) ([]string, error) {
	var videoFiles []string

	err := filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("경로 접근 중 오류 발생 '%s': %v", path, err)
			return nil
		}
		if !d.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if _, ok := videoExtensions[ext]; ok {
				log.Printf("비디오 파일 발견: %s", path)
				videoFiles = append(videoFiles, path)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("디렉토리 '%s' 탐색 중 오류 발생: %w", dirPath, err)
	}

	return videoFiles, nil
}

func findVideoFiles(basePath string) ([]string, error) {
	var allVideoFiles []string
	log.Printf("yyyymmdd 형식의 디렉토리 검색 시작: %s", basePath)

	// basePath 내의 항목들을 읽음
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, fmt.Errorf("기본 경로 '%s' 읽기 중 오류 발생: %w", basePath, err)
	}

	// yyyymmdd 형식의 디렉토리 찾기
	var dateFormatDirs []string
	for _, entry := range entries {
		if entry.IsDir() && isDateFormatDir(entry.Name()) {
			dateFormatDir := filepath.Join(basePath, entry.Name())
			log.Printf("날짜 형식 디렉토리 발견: %s", dateFormatDir)
			dateFormatDirs = append(dateFormatDirs, dateFormatDir)
		}
	}

	if len(dateFormatDirs) == 0 {
		log.Printf("yyyymmdd 형식의 디렉토리를 찾지 못했습니다.")
		return nil, nil
	}

	// 각 날짜 형식 디렉토리 내에서 비디오 파일 검색
	for _, dateDir := range dateFormatDirs {
		log.Printf("디렉토리 내 비디오 파일 검색 중: %s", dateDir)
		videoFiles, err := findVideosInDir(dateDir)
		if err != nil {
			log.Printf("경고: 디렉토리 '%s' 검색 중 오류 발생: %v", dateDir, err)
			continue
		}
		allVideoFiles = append(allVideoFiles, videoFiles...)
	}

	log.Printf("총 %d개의 날짜 형식 디렉토리와 %d개의 비디오 파일을 찾았습니다.", len(dateFormatDirs), len(allVideoFiles))
	return allVideoFiles, nil
}

// 비디오 파일의 코덱 정보를 가져오는 함수 (ffprobe 사용)
func getVideoCodec(filePath string, ffmpegPath string) (string, error) {
	// ffprobe 경로 얻기 (ffmpeg 경로를 기반으로)
	ffmpegDir := filepath.Dir(ffmpegPath)
	ffprobePath := filepath.Join(ffmpegDir, "ffprobe")
	
	// Windows 환경의 경우 .exe 확장자 추가
	if runtime.GOOS == "windows" {
		ffprobePath += ".exe"
	}
	
	// ffprobe 실행 파일 존재 여부 확인
	if _, err := os.Stat(ffprobePath); os.IsNotExist(err) {
		log.Printf("경고: ffprobe를 찾을 수 없습니다: %s, 시스템 PATH에서 찾기 시도", ffprobePath)
		ffprobePath = "ffprobe"
	}
	
	// ffprobe 명령어 실행하여 코덱 정보 추출
	cmd := exec.Command(ffprobePath,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_name",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("코덱 정보 추출 중 오류 발생: %w, 명령어: %s", err, cmd.String())
	}

	// 결과에서 공백 제거
	codec := strings.TrimSpace(string(output))
	log.Printf("파일 '%s'의 코덱: %s", filePath, codec)

	return codec, nil
}

func convertVideoToAV1(inputPath string, ffmpegPath string) error {
	// 파일의 현재 코덱 확인
	codec, err := getVideoCodec(inputPath, ffmpegPath)
	if err != nil {
		log.Printf("경고: 파일 '%s'의 코덱을 확인할 수 없습니다, 변환을 진행합니다: %v", inputPath, err)
	} else if strings.Contains(strings.ToLower(codec), "av1") {
		log.Printf("스킵: 파일 '%s'는 이미 AV1 코덱입니다. 변환이 필요하지 않습니다.", inputPath)
		return nil
	}

	dir := filepath.Dir(inputPath)
	baseName := filepath.Base(inputPath)
	ext := filepath.Ext(baseName)
	outputFileName := fmt.Sprintf("%s_av1.mkv", strings.TrimSuffix(baseName, ext))
	fullOutputPath := filepath.Join(dir, outputFileName)

	log.Printf("변환 시작: '%s' -> '%s' (using %s)", inputPath, fullOutputPath, ffmpegPath)

	cmd := exec.Command(ffmpegPath,
		"-i", inputPath,
		"-c:v", "av1_qsv",
		"-c:a", "copy",
		"-y",
		fullOutputPath,
	)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Printf("실행할 FFmpeg 명령어: %s", cmd.String())

	err = cmd.Run()
	if err != nil {
		errMsg := fmt.Sprintf("FFmpeg 실행 중 오류 발생 (파일: %s): %v", inputPath, err)
		if exitError, ok := err.(*exec.ExitError); ok {
			errMsg = fmt.Sprintf("%s, 종료 코드: %d, 에러 출력: %s", errMsg, exitError.ExitCode(), string(exitError.Stderr))
		}
		log.Println(errMsg)
		return fmt.Errorf(errMsg)
	}

	log.Printf("변환 완료: '%s'", fullOutputPath)
	return nil
}

func main() {
	log.Println("프로그램 시작")

	config, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("설정 로드 실패: %v", err)
	}
	log.Printf("설정 로드 완료: BasePath='%s', FfmpegPath='%s'", config.BasePath, config.FfmpegPath)

	videoFiles, err := findVideoFiles(config.BasePath)
	if err != nil {
		log.Fatalf("비디오 파일 검색 실패: %v", err)
	}

	if len(videoFiles) == 0 {
		log.Println("yyyymmdd 형식의 디렉토리 내에서 변환할 비디오 파일을 찾지 못했습니다.")
		log.Println("프로그램 종료")
		return
	}

	successCount := 0
	errorCount := 0
	skippedCount := 0
	for _, file := range videoFiles {
		// 변환 전 코덱 확인
		codec, checkErr := getVideoCodec(file, config.FfmpegPath)
		if checkErr == nil && strings.Contains(strings.ToLower(codec), "av1") {
			log.Printf("스킵: 파일 '%s'는 이미 AV1 코덱입니다.", file)
			skippedCount++
			continue
		}
		
		err := convertVideoToAV1(file, config.FfmpegPath)
		if err != nil {
			log.Printf("파일 변환 실패: %s - 오류: %v", file, err)
			errorCount++
		} else {
			successCount++
		}
	}

	log.Printf("모든 작업 완료. 성공: %d, 실패: %d, 스킵(이미 AV1): %d", successCount, errorCount, skippedCount)
	log.Println("프로그램 종료")
}
