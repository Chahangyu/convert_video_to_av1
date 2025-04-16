package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Config struct {
	BasePath   string `json:"BasePath"`
	OutputPath string `json:"OutputPath"`
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
	if config.OutputPath == "" {
		return config, fmt.Errorf("설정 파일에 'OutputPath'가 지정되지 않았습니다")
	}

	if config.FfmpegPath == "" {
		log.Println("'FfmpegPath'가 설정 파일에 지정되지 않았습니다. 기본값 'ffmpeg'를 사용합니다 (PATH 환경 변수에서 검색).")
		config.FfmpegPath = "ffmpeg"
	}

	if _, err := os.Stat(config.BasePath); os.IsNotExist(err) {
		return config, fmt.Errorf("설정에 지정된 BasePath '%s'이(가) 존재하지 않습니다", config.BasePath)
	}
	if _, err := os.Stat(config.OutputPath); os.IsNotExist(err) {
		log.Printf("출력 경로 '%s'이(가) 존재하지 않아 생성을 시도합니다.", config.OutputPath)
		err = os.MkdirAll(config.OutputPath, 0755)
		if err != nil {
			return config, fmt.Errorf("출력 경로 '%s' 생성 중 오류 발생: %w", config.OutputPath, err)
		}
		log.Printf("출력 경로 '%s'을(를) 성공적으로 생성했습니다.", config.OutputPath)
	} else if err != nil {
		return config, fmt.Errorf("출력 경로 '%s' 확인 중 오류 발생: %w", config.OutputPath, err)
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

func findVideoFiles(basePath string) ([]string, error) {
	var videoFiles []string
	log.Printf("비디오 파일 검색 시작: %s", basePath)

	err := filepath.WalkDir(basePath, func(path string, d fs.DirEntry, err error) error {
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
		return nil, fmt.Errorf("디렉토리 '%s' 탐색 중 오류 발생: %w", basePath, err)
	}

	log.Printf("총 %d개의 비디오 파일을 찾았습니다.", len(videoFiles))
	return videoFiles, nil
}

func convertVideoToAV1(inputPath string, outputPath string, ffmpegPath string) error {
	baseName := filepath.Base(inputPath)
	ext := filepath.Ext(baseName)
	outputFileName := fmt.Sprintf("%s_av1.mkv", strings.TrimSuffix(baseName, ext))
	fullOutputPath := filepath.Join(outputPath, outputFileName)

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

	err := cmd.Run()
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
	log.Printf("설정 로드 완료: BasePath='%s', OutputPath='%s', FfmpegPath='%s'", config.BasePath, config.OutputPath, config.FfmpegPath)

	videoFiles, err := findVideoFiles(config.BasePath)
	if err != nil {
		log.Fatalf("비디오 파일 검색 실패: %v", err)
	}

	if len(videoFiles) == 0 {
		log.Println("변환할 비디오 파일을 찾지 못했습니다.")
		log.Println("프로그램 종료")
		return
	}

	successCount := 0
	errorCount := 0
	for _, file := range videoFiles {
		err := convertVideoToAV1(file, config.OutputPath, config.FfmpegPath)
		if err != nil {
			log.Printf("파일 변환 실패: %s - 오류: %v", file, err)
			errorCount++
		} else {
			successCount++
		}
	}

	log.Printf("모든 작업 완료. 성공: %d, 실패: %d", successCount, errorCount)
	log.Println("프로그램 종료")
}
