package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"encoding/json"

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
	// コンテナの自動構築・起動 (確認プロンプト付き)
	ensureSandbox()

	JudgeQueue = make(chan Submission, 1000)

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

// ユーザーに Y/n で確認を求めるヘルパー関数
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

// サンドボックスコンテナの自動セットアップ
func ensureSandbox() {
	containerName := "discoder-sandbox"
	log.Println("サンドボックスコンテナの状態を確認しています...")

	cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}", "-f", "name="+containerName)
	out, _ := cmd.Output()

	if !strings.Contains(string(out), containerName) {
		if !askForConfirmation("サンドボックスコンテナが見つかりません。新規作成してよろしいですか？") {
			log.Fatal("セットアップを中止しました。ジャッジシステムを起動できません。")
		}

		log.Println("サンドボックスコンテナを新規作成しています...")
		cwd, _ := os.Getwd()
		volumePath := filepath.ToSlash(cwd) + ":/host"

		runCmd := exec.Command("docker", "run", "-d", "--name", containerName, "-v", volumePath, "ubuntu:latest", "sleep", "infinity")
		if err := runCmd.Run(); err != nil {
			log.Fatalf("コンテナの作成に失敗しました: %v", err)
		}
	} else {
		exec.Command("docker", "start", containerName).Run()
	}

	// 必要なパッケージの取得
	pkgs, cmds := getRequiredPackages()
	if len(pkgs) == 0 {
		log.Println("※ .env の ALLOWED_FILENAMES に対応するコンパイラが設定されていません。")
		return
	}

	// 全ての必須コマンドがコンテナ内に存在するかチェック
	needsInstall := false
	for _, cmdName := range cmds {
		if err := exec.Command("docker", "exec", containerName, "which", cmdName).Run(); err != nil {
			needsInstall = true
			break
		}
	}

	if needsInstall {
		prompt := fmt.Sprintf("必要なコンパイラが不足しています。自動インストール(%s)を開始してよろしいですか？ (1〜2分かかります)", strings.Join(pkgs, ", "))
		if !askForConfirmation(prompt) {
			log.Fatal("セットアップを中止しました。")
		}

		log.Printf("自動インストールを開始します: %v\n", pkgs)
		exec.Command("docker", "exec", containerName, "apt-get", "update").Run()

		installArgs := append([]string{"exec", containerName, "apt-get", "install", "-y"}, pkgs...)
		installCmd := exec.Command("docker", installArgs...)
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

		// 戻り値を「メインの結果」と「詳細テキスト」の2つに変更
		result, details := runJudgeLogic(sub)

		duration := time.Since(sub.SubmittedAt)
		log.Printf("[JUDGE FINISHED] User: %s, Problem: %s, Result: %s, Elapsed: %v\n", sub.Username, sub.ProblemID, result, duration)

		prefix := ""
		if sub.IsTest {
			prefix = "🔧 **[ADMIN TEST RESULT]**\n"
		}

		// Discordへ送信するメッセージの組み立て
		resultMsg := fmt.Sprintf("%s✅ ジャッジ完了\n問題: `%s`\n結果: **%s**\n所要時間: %v", prefix, sub.ProblemID, result, duration)
		if details != "" {
			resultMsg += "\n\n" + details
		}

		s.ChannelMessageSend(sub.DMChannelID, resultMsg)
	}
}

func runJudgeLogic(sub Submission) (string, string) {
	ext := filepath.Ext(sub.Filename)
	problemDir := filepath.Join("problems", sub.ProblemID)
	inDir := filepath.Join(problemDir, "in")
	outDir := filepath.Join(problemDir, "out")

	inFiles, err := os.ReadDir(inDir)
	if err != nil {
		log.Printf("[ERROR] テストケースが見つかりません。パス: %s\n", inDir)
		return "IE (Internal Error)", ""
	}

	sandboxContainer := "discoder-sandbox"
	langConfig, ok := SupportedLanguages[ext]
	if !ok {
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

		expectedOutputBytes, _ := os.ReadFile(outPath)
		expectedOutput := strings.TrimSpace(string(expectedOutputBytes))

		// --- 💡変更点: メモリ計測コマンドの構築 ---
		// ワーカーの同時実行でファイルが衝突しないよう、一意のファイル名にする
		memFile := fmt.Sprintf("/tmp/mem_%s_%s_%s.txt", sub.UserID, sub.ProblemID, baseName)
		// /usr/bin/time -f '%M' でピークメモリ(KB)をファイルに出力
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
				}
			}
		}
		inFile.Close()
		
		elapsedMs = time.Since(caseStart).Milliseconds()
		if elapsedMs > maxTimeMs { maxTimeMs = elapsedMs }

		// --- 💡変更点: メモリ使用量の読み取りと後片付け ---
		memCmd := exec.Command("docker", "exec", sandboxContainer, "cat", memFile)
		memOut, _ := memCmd.Output()
		memKb, _ := strconv.ParseInt(strings.TrimSpace(string(memOut)), 10, 64)
		if memKb > maxMemoryKB { maxMemoryKB = memKb }
		memMb := float64(memKb) / 1024.0
		
		// 読み終わったメモリファイルをコンテナから削除
		exec.Command("docker", "exec", sandboxContainer, "rm", "-f", memFile).Run()

		// エラー判定とDiscord表示の作成
		if isIE {
			log.Printf("[CASE] %s (問題: %s, %s) -> IE (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: 💀 IE (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			return "IE (ジャッジシステム遅延エラー)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
		}
		if isTimeout {
			log.Printf("[CASE] %s (問題: %s, %s) -> TLE (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: ⏳ TLE (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			return "TLE (実行時間制限超過)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
		}
		if isRE {
			log.Printf("[CASE] %s (問題: %s, %s) -> RE (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: ⚠️ RE (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			return "RE (実行時エラー / メモリ制限超過)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
		}

		actualOutput := strings.TrimSpace(outBuf.String())
		if actualOutput != expectedOutput {
			log.Printf("[CASE] %s (問題: %s, %s) -> WA (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
			detailLines = append(detailLines, fmt.Sprintf("`%s`: ❌ WA (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
			allAC = false
			break // WAで打ち切り
		}

		log.Printf("[CASE] %s (問題: %s, %s) -> AC (%d ms, %.1f MB)", sub.Username, sub.ProblemID, baseName, elapsedMs, memMb)
		detailLines = append(detailLines, fmt.Sprintf("`%s`: ✅ AC (%d ms, %.1f MB)", baseName, elapsedMs, memMb))
	}

	if allAC {
		return "AC (正解)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
	}
	return "WA (不正解)", formatDetails(maxTimeMs, maxMemoryKB, pConfig.TimeLimit, pConfig.MemoryLimit, detailLines)
}

// 文字数超過を防ぐための詳細テキストフォーマット関数
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
	// --- 追加: メモリ計測用の 'time' パッケージを強制追加 ---
	if !pkgSet["time"] {
		pkgSet["time"] = true
		pkgs = append(pkgs, "time")
		cmds = append(cmds, "/usr/bin/time")
	}
	return pkgs, cmds
}
