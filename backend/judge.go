package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Submission struct {
	UserID      string
	Username    string
	ProblemID   string
	Filename    string
	FilePath    string
	DMChannelID string
	SubmittedAt time.Time
	IsTest      bool
}

var JudgeQueue chan Submission

// ジャッジシステムの初期化
func initJudgeQueues(dg *discordgo.Session) {
	// コンテナの自動構築・起動
	ensureSandbox()

	JudgeQueue = make(chan Submission, 1000)
	
	// .envからWORKER_COUNTを読み込む（デフォルトは2）
	workerCount := 2
	if countStr := os.Getenv("WORKER_COUNT"); countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil && count > 0 {
			workerCount = count
		} else {
			log.Printf("警告: WORKER_COUNT の値 '%s' が不正です。デフォルトの %d を使用します。\n", countStr, workerCount)
		}
	}

	for i := 1; i <= workerCount; i++ {
		go judgeWorker(dg, i)
	}
	log.Printf("ジャッジシステムをワーカー数 %d で起動しました。\n", workerCount)
}

// サンドボックスコンテナの自動セットアップ
func ensureSandbox() {
	containerName := "discoder-sandbox"
	log.Println("サンドボックスコンテナの状態を確認しています...")

	// 1. コンテナの存在チェック
	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}", "-f", "name="+containerName)
	out, _ := cmd.Output()
	
	if !strings.Contains(string(out), containerName) {
		log.Println("サンドボックスコンテナを新規作成しています...")
		cwd, _ := os.Getwd()
		volumePath := filepath.ToSlash(cwd) + ":/host"
		
		runCmd := exec.Command("docker", "run", "-d", "--name", containerName, "-v", volumePath, "ubuntu:latest", "sleep", "infinity")
		if err := runCmd.Run(); err != nil {
			log.Fatalf("コンテナの作成に失敗しました: %v\n(Dockerが起動しているか、権限があるか確認してください)", err)
		}
	} else {
		// 存在する場合は起動（すでに起動済みの場合は何もしない）
		exec.Command("docker", "start", containerName).Run()
	}

	// 2. コンパイラ環境のチェックと自動インストール
	checkCmd := exec.Command("docker", "exec", containerName, "which", "g++")
	if err := checkCmd.Run(); err != nil {
		log.Println("コンテナ内にコンパイラが見つかりません。自動インストールを開始します (1〜2分かかります)...")
		
		exec.Command("docker", "exec", containerName, "apt-get", "update").Run()
		
		installCmd := exec.Command("docker", "exec", containerName, "apt-get", "install", "-y", "g++", "python3", "golang")
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			log.Fatalf("コンパイラのインストールに失敗しました: %v", err)
		}
		log.Println("環境のセットアップが完了しました。")
	} else {
		log.Println("サンドボックス環境は正常にセットアップされています。")
	}
}

func judgeWorker(s *discordgo.Session, workerID int) {
	for sub := range JudgeQueue {
		log.Printf("[Worker %d] %s さんの提出 (問題: %s) を処理開始...\n", workerID, sub.Username, sub.ProblemID)

		result := runJudgeLogic(sub)

		duration := time.Since(sub.SubmittedAt)
		log.Printf("[JUDGE FINISHED] User: %s, Problem: %s, Result: %s, Elapsed: %v\n", sub.Username, sub.ProblemID, result, duration)

		prefix := ""
		if sub.IsTest {
			prefix = "🔧 **[ADMIN TEST RESULT]**\n"
		}

		resultMsg := fmt.Sprintf("%s✅ ジャッジ完了\n問題: `%s`\n結果: **%s**\n所要時間: %v", prefix, sub.ProblemID, result, duration)
		s.ChannelMessageSend(sub.DMChannelID, resultMsg)
	}
}

func runJudgeLogic(sub Submission) string {
	ext := filepath.Ext(sub.Filename)
	problemDir := filepath.Join("problems", sub.ProblemID)
	inDir := filepath.Join(problemDir, "in")
	outDir := filepath.Join(problemDir, "out")

	inFiles, err := os.ReadDir(inDir)
	if err != nil {
		log.Printf("[ERROR] テストケースが見つかりません。探したパス: %s, エラー詳細: %v\n", inDir, err)
		return "IE (Internal Error: inディレクトリがありません)"
	}

	hasInFile := false
	for _, f := range inFiles {
		if strings.HasSuffix(f.Name(), ".in") {
			hasInFile = true
			break
		}
	}
	
	if !hasInFile {
		log.Printf("[ERROR] %s の中に .in 拡張子のファイルが1つもありません。\n", inDir)
		return "IE (Internal Error: .inファイルが見つかりません)"
	}

	var compileCmd, runCmd string
	sandboxContainer := "discoder-sandbox"

	switch ext {
	case ".cpp":
		compileCmd = fmt.Sprintf("g++ -O3 /host/%s -o /tmp/a.out", filepath.ToSlash(sub.FilePath))
		runCmd = "/tmp/a.out"
	case ".py":
		runCmd = fmt.Sprintf("python3 /host/%s", filepath.ToSlash(sub.FilePath))
	case ".go":
		compileCmd = fmt.Sprintf("go build -o /tmp/prog /host/%s", filepath.ToSlash(sub.FilePath))
		runCmd = "/tmp/prog"
	default:
		return "CE (コンパイルエラー: サポートされていない言語)"
	}

	// 3. コンパイルの実行
	if compileCmd != "" {
		cmd := exec.Command("docker", "exec", sandboxContainer, "sh", "-c", compileCmd)
		out, err := cmd.CombinedOutput()
		
		if err != nil {
			errMsg := strings.TrimSpace(string(out))
			if errMsg == "" {
				errMsg = "エラーメッセージがありません。コンテナ内で処理が失敗しました。"
			}
			if len(errMsg) > 1000 {
				errMsg = string([]rune(errMsg)[:1000]) + "\n... (以降省略)"
			}
			return fmt.Sprintf("CE (コンパイルエラー)\n```text\n%s\n```", errMsg)
		}
	}

	// 4. 全テストケースに対して実行・照合
	allAC := true
	for _, file := range inFiles {
		if !strings.HasSuffix(file.Name(), ".in") {
			continue
		}
		
		baseName := strings.TrimSuffix(file.Name(), ".in")
		inPath := filepath.Join(inDir, file.Name())
		outPath := filepath.Join(outDir, baseName+".out")

		expectedOutputBytes, err := os.ReadFile(outPath)
		if err != nil {
			return "IE (出力ファイル読込エラー)"
		}
		expectedOutput := strings.TrimSpace(string(expectedOutputBytes))

		inFile, _ := os.Open(inPath)
		cmd := exec.Command("docker", "exec", "-i", sandboxContainer, "sh", "-c", runCmd)
		cmd.Stdin = inFile
		
		var outBuf bytes.Buffer
		cmd.Stdout = &outBuf

		cmd.Start()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		select {
		case <-time.After(2 * time.Second):
			cmd.Process.Kill()
			inFile.Close()
			return "TLE (実行時間制限超過)"
		case err := <-done:
			inFile.Close()
			if err != nil {
				return "RE (実行時エラー)"
			}
		}

		actualOutput := strings.TrimSpace(outBuf.String())
		if actualOutput != expectedOutput {
			allAC = false
			break
		}
	}

	if allAC {
		return "AC (正解)"
	}
	return "WA (不正解)"
}