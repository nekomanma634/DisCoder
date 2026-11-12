package main

import (
	"bufio"
	"bytes"
	"encoding/json"
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
	ContestID   string
}

var JudgeQueue chan Submission

func initJudgeQueues(dg *discordgo.Session) {
	ensureSandbox()

	JudgeQueue = make(chan Submission, 1000)

	workerCount := 2
	if countStr := os.Getenv("WORKER_COUNT"); countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil && count > 0 {
			workerCount = count
		} else {
			log.Printf("[WARN] WORKER_COUNT の値 '%s' が不正です。デフォルトの %d を使用します。\n", countStr, workerCount)
		}
	}

	for i := 1; i <= workerCount; i++ {
		go judgeWorker(dg, i)
	}
	log.Printf("[INFO] ジャッジシステムをワーカー数 %d で起動しました。\n", workerCount)
}

func askForConfirmation(prompt string) bool {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("%s [Y/n]: ", prompt)
		response, err := reader.ReadString('\n')
		if err != nil {
			log.Fatal(err)
		}
		response = strings.ToLower(strings.TrimSpace(response))

		if response == "y" || response == "yes" || response == "" {
			return true
		} else if response == "n" || response == "no" {
			return false
		}
	}
}

func ensureSandbox() {
	containerName := "discoder-sandbox"
	log.Println("[DOCKER] サンドボックスコンテナの状態を確認しています...")

	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}", "-f", "name="+containerName)
	out, _ := cmd.Output()

	if !strings.Contains(string(out), containerName) {
		if !askForConfirmation("[DOCKER] サンドボックスコンテナが見つかりません。新規作成してよろしいですか？") {
			log.Fatal("[FATAL] セットアップを中止しました。ジャッジシステムを起動できません。")
		}

		log.Println("[DOCKER] サンドボックスコンテナを新規作成しています...")
		cwd, _ := os.Getwd()
		volumePath := filepath.ToSlash(cwd) + ":/host"

		runCmd := exec.Command("docker", "run", "-d", "--name", containerName, "-v", volumePath, "ubuntu:latest", "sleep", "infinity")
		if err := runCmd.Run(); err != nil {
			log.Fatalf("[FATAL] コンテナの作成に失敗しました: %v", err)
		}
	} else {
		exec.Command("docker", "start", containerName).Run()
	}

	pkgs, cmds := getRequiredPackages()
	if len(pkgs) == 0 {
		log.Println("[WARN] languages.json に有効なパッケージ情報がありません。")
		return
	}

	needsInstall := false
	for _, cmdName := range cmds {
		if err := exec.Command("docker", "exec", containerName, "which", cmdName).Run(); err != nil {
			needsInstall = true
			break
		}
	}

	if needsInstall {
		prompt := fmt.Sprintf("[DOCKER] 必要なパッケージが不足しています。自動インストール(%s)を開始してよろしいですか？ (1〜3分)", strings.Join(pkgs, ", "))
		if !askForConfirmation(prompt) {
			log.Fatal("[FATAL] セットアップを中止しました。")
		}

		log.Printf("[DOCKER] 自動インストールを開始します: %v\n", pkgs)
		exec.Command("docker", "exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "apt-get", "update").Run()

		installArgs := append([]string{"exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "apt-get", "install", "-y", "--no-install-recommends"}, pkgs...)
		installCmd := exec.Command("docker", installArgs...)
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			log.Fatalf("[FATAL] コンパイラのインストールに失敗しました: %v", err)
		}
		log.Println("[DOCKER] 環境のセットアップが完了しました。")
	} else {
		log.Println("[DOCKER] サンドボックス環境は正常にセットアップされています。")
	}
}

func judgeWorker(s *discordgo.Session, workerID int) {
	for sub := range JudgeQueue {
		log.Printf("[JUDGE] [Worker %d] %s さんの提出 (問題: %s) を処理開始...\n", workerID, sub.Username, sub.ProblemID)

		result, details := runJudgeLogic(sub)
		duration := time.Since(sub.SubmittedAt)
		log.Printf("[JUDGE] [FINISHED] User: %s, Problem: %s, Result: %s, Elapsed: %v\n", sub.Username, sub.ProblemID, result, duration)

		prefix := ""
		if sub.IsTest {
			prefix = "🔧 **[ADMIN TEST RESULT]**\n"
		}

		resultMsg := fmt.Sprintf("%s✅ ジャッジ完了\n問題: `%s`\n結果: **%s**\n所要時間: %v", prefix, sub.ProblemID, result, duration)

		if strings.HasPrefix(result, "AC") && sub.ContestID != "" && !sub.IsTest {
			isFirst, addedScore, err := AddScoreIfFirstAC(sub.UserID, sub.ProblemID, sub.ContestID)
			if err != nil {
				log.Printf("[ERROR] [DB] スコア加算失敗: %v\n", err)
			} else if isFirst {
				resultMsg += fmt.Sprintf("\n\n🎉 **初AC！** コンテストスコアに **+%d点** 加算されました！", addedScore)
			} else {
				resultMsg += "\n\nℹ️ (この問題はすでにAC済みのため、スコア加算はありません)"
			}
		}

		if details != "" {
			resultMsg += "\n\n" + details
		}

		s.ChannelMessageSend(sub.DMChannelID, resultMsg)
	}
}

func runJudgeLogic(sub Submission) (string, string) {
	ext := filepath.Ext(sub.Filename)
	
	problemDir := filepath.Join("problems", sub.ProblemID)
	if sub.ContestID != "" {
		problemDir = filepath.Join("contests", sub.ContestID, "problems", sub.ProblemID)
	}
	
	inDir := filepath.Join(problemDir, "in")
	outDir := filepath.Join(problemDir, "out")

	inFiles, err := os.ReadDir(inDir)
	if err != nil {
		log.Printf("[ERROR] [JUDGE] テストケース(in)が見つかりません。パス: %s, 詳細: %v\n", inDir, err)
		return "IE (Internal Error)", ""
	}

	sandboxContainer := "discoder-sandbox"
	langConfig, ok := SupportedLanguages[ext]
	if !ok {
		log.Printf("[WARN] [JUDGE] 未対応の言語での提出: %s (ユーザー: %s)\n", ext, sub.Username)
		return "CE (未対応の言語)", ""
	}

	compileCmd := strings.ReplaceAll(langConfig.CompileCmd, "{filepath}", filepath.ToSlash(sub.FilePath))
	runCmd := strings.ReplaceAll(langConfig.RunCmd, "{filepath}", filepath.ToSlash(sub.FilePath))

	if compileCmd != "" {
		cmd := exec.Command("docker", "exec", sandboxContainer, "sh", "-c", compileCmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			errMsg := strings.TrimSpace(string(out))
			if len(errMsg) > 1000 {
				errMsg = string([]rune(errMsg)[:1000]) + "\n... (以降省略)"
			}
			log.Printf("[ERROR] [JUDGE] CE: %s さんの提出 (問題: %s)\n--- コンパイルエラー詳細 ---\n%s\n--------------------------\n", sub.Username, sub.ProblemID, errMsg)
			return "CE (コンパイルエラー)", fmt.Sprintf("```text\n%s\n```", errMsg)
		}
	}

	type ProblemConfig struct {
		TimeLimit   float64 `json:"time_limit"`
		MemoryLimit int     `json:"memory_limit"`
	}

	configPath := filepath.Join(problemDir, "config.json")
	configData, err := os.ReadFile(configPath)
	var pConfig ProblemConfig
	if err == nil {
		json.Unmarshal(configData, &pConfig)
	}
	if pConfig.TimeLimit <= 0 { pConfig.TimeLimit = 2.0 }
	if pConfig.MemoryLimit <= 0 { pConfig.MemoryLimit = 1024 }

	memoryLimitKB := pConfig.MemoryLimit * 1024
	goTimeout := time.Duration((pConfig.TimeLimit + 1.0) * float64(time.Second))

	var detailLines []string
	var maxTimeMs int64 = 0
	var maxMemoryKB int64 = 0
	allAC := true

	for _, file := range inFiles {
		if !strings.HasSuffix(file.Name(), ".in") { continue }
		
		baseName := strings.TrimSuffix(file.Name(), ".in")
		inPath := filepath.Join(inDir, file.Name())
		outPath := filepath.Join(outDir, baseName+".out")

		expectedOutputBytes, err := os.ReadFile(outPath)
		if err != nil {
			log.Printf("[ERROR] [JUDGE] 出力ファイル(.out)の読み込みに失敗: %s\n", outPath)
			return "IE (Internal Error)", ""
		}
		expectedOutput := strings.TrimSpace(string(expectedOutputBytes))

		memFile := fmt.Sprintf("/tmp/mem_%s_%s_%s.txt", sub.UserID, sub.ProblemID, baseName)
		runCmdWithLimit := fmt.Sprintf("ulimit -v %d; /usr/bin/time -f '%%M' -o %s timeout %f %s", memoryLimitKB, memFile, pConfig.TimeLimit, runCmd)
		
		inFile, _ := os.Open(inPath)
		cmd := exec.Command("docker", "exec", "-i", sandboxContainer, "sh", "-c", runCmdWithLimit)
		var outBuf bytes.Buffer
		cmd.Stdin = inFile
		cmd.Stdout = &outBuf

		caseStart := time.Now()
		cmd.Start()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		var elapsedMs int64
		var isIE, isTimeout, isRE bool

		select {
		case <-time.After(goTimeout):
			cmd.Process.Kill()
			isIE = true
		case err := <-done:
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 124 {
					isTimeout = true
				} else {
					isRE = true
					log.Printf("[WARN] [JUDGE] RE発生詳細: %v\n", err)
				}
			}
		}
		inFile.Close()
		
		elapsedMs = time.Since(caseStart).Milliseconds()
		if elapsedMs > maxTimeMs { maxTimeMs = elapsedMs }

		memCmd := exec.Command("docker", "exec", sandboxContainer, "cat", memFile)
		memOut, _ := memCmd.Output()
		memKb, _ := strconv.ParseInt(strings.TrimSpace(string(memOut)), 10, 64)
		if memKb > maxMemoryKB { maxMemoryKB = memKb }
		memMb := float64(memKb) / 1024.0
		
		exec.Command("docker", "exec", sandboxContainer, "rm", "-f", memFile).Run()

		if isIE {
			log.Printf("[JUDGE] [CASE] %s (問題: %s, %s) -> IE (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: 💀 IE (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			return "IE (ジャッジシステム遅延エラー)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
		}
		if isTimeout {
			log.Printf("[JUDGE] [CASE] %s (問題: %s, %s) -> TLE (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: ⏳ TLE (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			return "TLE (実行時間制限超過)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
		}
		if isRE {
			log.Printf("[JUDGE] [CASE] %s (問題: %s, %s) -> RE (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: ⚠️ RE (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			return "RE (実行時エラー / メモリ制限超過)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
		}

		actualOutput := strings.TrimSpace(outBuf.String())
		if actualOutput != expectedOutput {
			log.Printf("[JUDGE] [CASE] %s (問題: %s, %s) -> WA (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: ❌ WA (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			allAC = false
			break
		}

		log.Printf("[JUDGE] [CASE] %s (問題: %s, %s) -> AC (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
		detailLines = append(detailLines, fmt.Sprintf("`%s`: ✅ AC (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
	}

	if allAC {
		return "AC (正解)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
	}
	return "WA (不正解)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
}

func formatDetails(maxTimeMs int64, maxMemKB int64, limitTime float64, limitMem int, lines []string) string {
	maxMemMB := float64(maxMemKB) / 1024.0
	limitTimeMs := int64(limitTime * 1000)
	
	detailStr := fmt.Sprintf("⏱️ **実行時間**: `%d ms` / 制限 `%d ms`\n💾 **メモリ**: `%.1f MB` / 制限 `%d MB`\n**[テストケース詳細]**\n%s", 
		maxTimeMs, limitTimeMs, maxMemMB, limitMem, strings.Join(lines, "\n"))
	
	if len(detailStr) > 1500 {
		return detailStr[:1500] + "\n... (Discordの文字数制限のため以降省略)"
	}
	return detailStr
}

func getRequiredPackages() (pkgs []string, cmds []string) {
	pkgSet := make(map[string]bool)
	for _, config := range SupportedLanguages {
		if config.Package != "" && !pkgSet[config.Package] {
			pkgSet[config.Package] = true
			pkgs = append(pkgs, config.Package)
			cmds = append(cmds, config.CheckCmd)
		}
	}
	if !pkgSet["time"] {
		pkgSet["time"] = true
		pkgs = append(pkgs, "time")
		cmds = append(cmds, "/usr/bin/time")
	}
	return pkgs, cmds
}