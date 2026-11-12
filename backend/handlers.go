package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"encoding/json"
	"strconv"

	"github.com/bwmarrin/discordgo"
)

func handleHello(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, dmChannelID string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "DMにメッセージを送信しました！確認してください。",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	greetingMsg := fmt.Sprintf("こんにちは、%sさん！\nDiscoderのコンテストシステムへようこそ。\nコードを提出する際は、このDMで `/submit` と入力するか、サーバーのチャンネルで実行してください！", user.Username)
	s.ChannelMessageSend(dmChannelID, greetingMsg)
}

func handleSubmit(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, dmChannelID string, data discordgo.ApplicationCommandInteractionData) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "提出処理を開始しました。以降の通知はDMを確認してください！",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg {
		s.ChannelMessageSend(dmChannelID, "❌ エラー: 先に `/register` を実行してシステムに登録してください。")
		return
	}

	var problemID, contestID string
	var attachment *discordgo.MessageAttachment

	for _, opt := range data.Options {
		switch opt.Name {
		case "problem":
			problemID = opt.StringValue()
		case "file":
			attachment = data.Resolved.Attachments[opt.Value.(string)]
		case "contest":
			contestID = opt.StringValue()
		}
	}

	// 💡 追加: サーバー側でもコンテストの未指定を確実に弾く
	if contestID == "" {
		s.ChannelMessageSend(dmChannelID, "❌ エラー: 提出先のコンテストIDが指定されていません。")
		return
	}

	if !isProblemExists(problemID) {
		s.ChannelMessageSend(dmChannelID, fmt.Sprintf("エラー: 問題ID `%s` はデータベースに存在しません。", problemID))
		return
	}

	if !isContestExists(contestID) {
		s.ChannelMessageSend(dmChannelID, fmt.Sprintf("エラー: コンテスト `%s` は存在しません。", contestID))
		return
	}
	
	if !isUserJoinedContest(contestID, user.ID) {
		s.ChannelMessageSend(dmChannelID, fmt.Sprintf("エラー: コンテスト `%s` に参加登録していません。先に `/join_contest` を実行してください。", contestID))
		return
	}

	startTime, endTime, err := getContestTimes(contestID)
	if err == nil {
		now := time.Now()
		isTimeValid := now.After(startTime) && now.Before(endTime)

		if !isTimeValid {
			if role != "admin" {
				if now.Before(startTime) {
					s.ChannelMessageSend(dmChannelID, fmt.Sprintf("⚠️ 提出却下: コンテスト `%s` はまだ開始していません。\n開始予定時刻: %s", contestID, startTime.Format("2006/01/02 15:04:05")))
				} else if now.After(endTime) {
					s.ChannelMessageSend(dmChannelID, fmt.Sprintf("⚠️ 提出却下: コンテスト `%s` はすでに終了しました。\nお疲れ様でした！", contestID))
				}
				return
			} else {
				s.ChannelMessageSend(dmChannelID, "🔧 **[ADMIN INFO]** 現在コンテスト期間外ですが、Admin権限により特例で提出を受け付けました。")
			}
		}
	}

	if _, err := os.Stat(filepath.Join("contests", contestID, "problems", problemID)); os.IsNotExist(err) {
		s.ChannelMessageSend(dmChannelID, fmt.Sprintf("エラー: コンテスト `%s` 内に問題 `%s` が見つかりません。", contestID, problemID))
		return
	}

	ext := filepath.Ext(attachment.Filename)
	if _, ok := SupportedLanguages[ext]; !ok {
		var exts []string
		for k := range SupportedLanguages { exts = append(exts, k) }
		s.ChannelMessageSend(dmChannelID, fmt.Sprintf("エラー: 許可されていないファイル形式です。対応言語: `%s`", strings.Join(exts, ", ")))
		return
	}

	submitDir := filepath.Join("submissions", fmt.Sprintf("%s_%s", user.Username, user.ID), problemID)
	os.MkdirAll(submitDir, os.ModePerm)
	filePath := filepath.Join(submitDir, attachment.Filename)
	
	if err := downloadAttachment(attachment.URL, filePath); err != nil {
		s.ChannelMessageSend(dmChannelID, "エラー: ファイルの保存に失敗しました。")
		return
	}

	submissionData := Submission{
		UserID:      user.ID,
		Username:    user.Username,
		ProblemID:   problemID,
		Filename:    attachment.Filename,
		FilePath:    filePath,
		DMChannelID: dmChannelID,
		SubmittedAt: time.Now(),
		IsTest:      false,
		ContestID:   contestID,
	}

	s.ChannelMessageSend(dmChannelID, fmt.Sprintf("✅ **%s** への提出を受け付けました！\n問題: `%s`\nファイル: `%s`\n現在ジャッジ中です...", contestID, problemID, attachment.Filename))
	
	JudgeQueue <- submissionData
}

func handleTestProblem(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, dmChannelID string, data discordgo.ApplicationCommandInteractionData) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "テスト処理を開始しました。結果はDMに送信されます。",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || role != "admin" {
		s.ChannelMessageSend(dmChannelID, "⛔ 権限がありません。このコマンドは Admin 専用です。")
		return
	}

	var problemID string
	var attachment *discordgo.MessageAttachment
	for _, opt := range data.Options {
		if opt.Name == "problem" {
			problemID = opt.StringValue()
		} else if opt.Name == "file" {
			attachment = data.Resolved.Attachments[opt.Value.(string)]
		}
	}

	if !isProblemExists(problemID) {
		s.ChannelMessageSend(dmChannelID, "エラー: 問題が存在しません。")
		return
	}

	testDir := filepath.Join("submissions", "admin_test", problemID)
	os.MkdirAll(testDir, os.ModePerm)
	filePath := filepath.Join(testDir, attachment.Filename)
	downloadAttachment(attachment.URL, filePath)

	submissionData := Submission{
		UserID:      user.ID,
		Username:    user.Username,
		ProblemID:   problemID,
		Filename:    attachment.Filename,
		FilePath:    filePath,
		DMChannelID: dmChannelID,
		SubmittedAt: time.Now(),
		IsTest:      true, // テストモードフラグ
	}

	s.ChannelMessageSend(dmChannelID, fmt.Sprintf("🔧 **[ADMIN TEST]**\n問題: `%s` に対してテストを実行中...", problemID))
	JudgeQueue <- submissionData
}

func handleRegister(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	_, isRegistered, err := getUserRole(user.ID)
	if err != nil {
		respondEphemeral(s, i, "データベースエラーが発生しました。")
		return
	}
	if isRegistered {
		respondEphemeral(s, i, "既に登録されています！ `/profile` コマンドで情報を確認してください。")
		return
	}

	// Discordのユーザー名をそのまま利用する
	username := user.Username

	role := "user"
	if user.ID == os.Getenv("OWNER_ID") {
		role = "admin"
	}

	if err := registerUserToDB(user.ID, username, role); err != nil {
		log.Println("ユーザー登録エラー:", err)
		respondEphemeral(s, i, "登録中にエラーが発生しました。")
		return
	}

	go syncDiscordRole(s, i.GuildID, user.ID, role)

	msg := fmt.Sprintf("✅ 登録が完了しました！\nシステム内のユーザー名: `%s`\n付与されたロール: `%s`", username, role)
	respondEphemeral(s, i, msg)
}

func handleSetRole(s *discordgo.Session, i *discordgo.InteractionCreate, executor *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	executorRole, isReg, err := getUserRole(executor.ID)
	if err != nil || !isReg || executorRole != "admin" {
		respondEphemeral(s, i, "⛔ 権限がありません。このコマンドはAdmin専用です。")
		return
	}

	targetUser := data.Options[0].UserValue(s)
	newRole := data.Options[1].StringValue()

	_, isTargetReg, err := getUserRole(targetUser.ID)
	if err != nil || !isTargetReg {
		respondEphemeral(s, i, "対象のユーザーはまだDiscoderシステムに `/register` していません。")
		return
	}

	if err := updateUserRole(targetUser.ID, newRole); err != nil {
		respondEphemeral(s, i, "データベースの更新に失敗しました。")
		return
	}

	go syncDiscordRole(s, i.GuildID, targetUser.ID, newRole)

	respondEphemeral(s, i, fmt.Sprintf("✅ %s さんの権限を `%s` に変更しました。", targetUser.Username, newRole))
}

func handleProfile(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User) {
	role, isRegistered, err := getUserRole(user.ID)
	if err != nil {
		respondEphemeral(s, i, "データベースエラーが発生しました。")
		return
	}
	if !isRegistered {
		respondEphemeral(s, i, "未登録です。先に `/register` を実行してください。")
		return
	}
	msg := fmt.Sprintf("👤 **あなたのプロフィール**\nDiscord ID: `%s`\nロール: `%s`", user.ID, role)
	respondEphemeral(s, i, msg)
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}

func handleCreateProblem(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	respondEphemeral(s, i, "⏳ 問題の登録処理を開始しています。ファイルのダウンロードと展開中です...")

	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || (role != "admin" && role != "staff") {
		editEphemeral(s, i, "⛔ 権限がありません。このコマンドは Admin または Staff 専用です。")
		return
	}

	var problemName, problemTitle string
	var mdAtt, zipAtt, configAtt *discordgo.MessageAttachment

	for _, opt := range data.Options {
		switch opt.Name {
		case "name":
			problemName = opt.StringValue()
		case "title":
			problemTitle = opt.StringValue()
		case "statement":
			mdAtt = data.Resolved.Attachments[opt.Value.(string)]
		case "testcases":
			zipAtt = data.Resolved.Attachments[opt.Value.(string)]
		case "config":
			configAtt = data.Resolved.Attachments[opt.Value.(string)]
		}
	}

	validName := regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	if !validName.MatchString(problemName) {
		editEphemeral(s, i, "エラー: 問題ID (`name`) は半角英数字とアンダースコア(_)のみ使用可能です。")
		return
	}
	if isProblemExists(problemName) {
		editEphemeral(s, i, fmt.Sprintf("エラー: 問題ID `%s` は既に登録されています。", problemName))
		return
	}

	baseDir := filepath.Join("problems", problemName)
	os.MkdirAll(baseDir, os.ModePerm)

	downloadAttachment(mdAtt.URL, filepath.Join(baseDir, "statement.md"))
	zipPath := filepath.Join(baseDir, "testcases.zip")
	downloadAttachment(zipAtt.URL, zipPath)
	unzip(zipPath, baseDir)
	os.Remove(zipPath)

	type ProblemConfig struct {
		TimeLimit   float64 `json:"time_limit"`
		MemoryLimit int     `json:"memory_limit"`
		Score       int     `json:"score"`
	}

	var pConfig ProblemConfig
	configPath := filepath.Join(baseDir, "config.json")
	if configAtt != nil && strings.HasSuffix(strings.ToLower(configAtt.Filename), ".json") {
		downloadAttachment(configAtt.URL, configPath)
		configData, _ := os.ReadFile(configPath)
		json.Unmarshal(configData, &pConfig)
	}

	// 欠けている値の自動補完
	if pConfig.TimeLimit <= 0 { pConfig.TimeLimit = 2.0 }
	if pConfig.MemoryLimit <= 0 { pConfig.MemoryLimit = 1024 }
	if pConfig.Score <= 0 { pConfig.Score = 100 } // デフォルト100点

	// 補完された状態のコンフィグをファイルに書き出し直す
	configBytes, _ := json.MarshalIndent(pConfig, "", "  ")
	os.WriteFile(configPath, configBytes, os.ModePerm)

	inDir := filepath.Join(baseDir, "in")
	outDir := filepath.Join(baseDir, "out")
	if _, err := os.Stat(inDir); os.IsNotExist(err) {
		entries, _ := os.ReadDir(baseDir)
		for _, e := range entries {
			if e.IsDir() && e.Name() != "in" && e.Name() != "out" {
				nestedIn := filepath.Join(baseDir, e.Name(), "in")
				nestedOut := filepath.Join(baseDir, e.Name(), "out")
				if _, errIn := os.Stat(nestedIn); errIn == nil {
					os.Rename(nestedIn, inDir)
					os.Rename(nestedOut, outDir)
					os.RemoveAll(filepath.Join(baseDir, e.Name()))
					break
				}
			}
		}
	}

	if err := registerProblemToDB(problemName, problemTitle, user.ID, pConfig.Score); err != nil {
		editEphemeral(s, i, "エラー: データベースへの登録に失敗しました。")
		return
	}

	log.Printf("[INFO] [SYSTEM] 問題 %s が作成されました。配点: %d点\n", problemName, pConfig.Score)
	successMsg := fmt.Sprintf("✅ **問題の登録が完了しました！**\n\n**問題ID**: `%s`\n**表示名**: %s\n**配点**: %d点\n\nディレクトリ (`problems/%s/`) に展開されました。",
		problemName, problemTitle, pConfig.Score, problemName)
	editEphemeral(s, i, successMsg)
}

// 既存のEphemeralメッセージを更新（上書き）するヘルパー関数
func editEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, newContent string) {
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &newContent,
	})
}

func handleProblems(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User) {
	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || (role != "admin" && role != "staff") {
		respondEphemeral(s, i, "⛔ 権限がありません。このコマンドは Admin または Staff 専用です。")
		return
	}

	problems, err := getProblemList()
	if err != nil {
		respondEphemeral(s, i, "データベースエラーが発生しました。")
		return
	}

	if len(problems) == 0 {
		respondEphemeral(s, i, "現在、登録されている問題はありません。")
		return
	}

	var embed discordgo.MessageEmbed
	embed.Title = "📚 登録済みの問題一覧"
	embed.Color = 0x3498DB
	embed.Description = fmt.Sprintf("現在 **%d** 問がプールに登録されています。\n", len(problems))

	for _, p := range problems {
		dateStr := p.CreatedAt.Format("2006/01/02")
		field := &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("ID: `%s`", p.Name),
			// 一覧表示にも配点を表示
			Value:  fmt.Sprintf("**%s** (配点: %d点)\n登録日: %s", p.Title, p.Score, dateStr),
			Inline: false,
		}
		embed.Fields = append(embed.Fields, field)
		if len(embed.Fields) >= 25 {
			embed.Description += "\n※表示上限(25件)に達したため、古い問題は省略されています。"
			break
		}
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{&embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func handleShowProblem(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	// 1. 権限チェック (Admin または Staff のみ)
	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || (role != "admin" && role != "staff") {
		respondEphemeral(s, i, "⛔ 権限がありません。このコマンドは Admin または Staff 専用です。")
		return
	}

	problemName := data.Options[0].StringValue()

	// 2. 問題の存在チェック
	if !isProblemExists(problemName) {
		respondEphemeral(s, i, fmt.Sprintf("エラー: 問題 `%s` は登録されていません。", problemName))
		return
	}

	// 3. ファイルの読み込み
	filePath := filepath.Join("problems", problemName, "statement.md")
	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Println("ファイル読み込みエラー:", err)
		respondEphemeral(s, i, "サーバーエラー: 問題文ファイルの読み込みに失敗しました。")
		return
	}

	// 4. 文字数制限のハンドリング (DiscordのEmbed Descriptionは最大4096文字)
	textRune := []rune(string(content))
	var displayText string
	if len(textRune) > 4000 {
		displayText = string(textRune[:4000]) + "\n\n... (文字数制限のため省略)"
	} else {
		displayText = string(textRune)
	}

	// 5. Embed形式で表示
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("📝 問題: %s", problemName),
		Description: displayText,
		Color:       0x2ECC71, // 緑色
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral,
		},
	})
}

func handleSandbox(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	respondEphemeral(s, i, "⏳ サンドボックスの操作を実行中です...")

	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || role != "admin" {
		editEphemeral(s, i, "⛔ 権限がありません。このコマンドは Admin 専用です。")
		return
	}

	action := data.Options[0].StringValue()
	containerName := "discoder-sandbox"

	switch action {
	case "status":
		cmd := exec.Command("docker", "ps", "-a", "--filter", "name="+containerName, "--format", "コンテナID: {{.ID}}\nイメージ: {{.Image}}\n状態: {{.Status}}\n起動時間: {{.RunningFor}}")
		out, _ := cmd.CombinedOutput()
		if string(out) == "" {
			editEphemeral(s, i, "ℹ️ コンテナ `discoder-sandbox` は現在存在しません。（未作成または削除済み）")
			return
		}

		statusText := strings.TrimSpace(string(out))

		if strings.Contains(statusText, "状態: Up") {
			statsCmd := exec.Command("docker", "stats", "--no-stream", "--format", "CPU使用率: {{.CPUPerc}}\nメモリ使用量: {{.MemUsage}}", containerName)
			statsOut, _ := statsCmd.CombinedOutput()
			if string(statsOut) != "" {
				statusText += "\n" + strings.TrimSpace(string(statsOut))
			}

			statusText += "\n\n[インストール済み環境]"

			for ext, config := range SupportedLanguages {
				if config.VersionCmd == "" {
					continue
				}
				vcCmd := exec.Command("docker", "exec", containerName, "sh", "-c", config.VersionCmd)
				if vcOut, err := vcCmd.CombinedOutput(); err == nil && string(vcOut) != "" {
					statusText += fmt.Sprintf("\n・[%s] %s", ext, strings.TrimSpace(string(vcOut)))
				}
			}
		}

		editEphemeral(s, i, fmt.Sprintf("📊 **サンドボックス詳細ステータス**\n```text\n%s\n```", statusText))

	case "stop":
		exec.Command("docker", "stop", containerName).Run()
		log.Println("[INFO] [DOCKER] サンドボックスコンテナを手動で一時停止しました。")
		editEphemeral(s, i, "✅ サンドボックスコンテナを一時停止しました。")

	case "delete":
		exec.Command("docker", "rm", "-f", containerName).Run()
		log.Println("[INFO] [DOCKER] サンドボックスコンテナを手動で削除しました。")
		editEphemeral(s, i, "🗑️ サンドボックスコンテナを完全に削除しました。\n※次回 `start` 実行時、またはシステム再起動時に初期状態から再構築されます。")

	case "start":
		cmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}", "-f", "name="+containerName)
		out, _ := cmd.Output()

		if strings.Contains(string(out), containerName) {
			exec.Command("docker", "start", containerName).Run()
			log.Println("[INFO] [DOCKER] 既存のサンドボックスコンテナを手動で起動しました。")
			editEphemeral(s, i, "✅ 既存のサンドボックスコンテナを起動しました。")
		} else {
			pkgs, _ := getRequiredPackages()
			pkgListStr := strings.Join(pkgs, ", ")

			editEphemeral(s, i, "⏳ 新規コンテナを作成しています...")
			log.Println("[INFO] [DOCKER] サンドボックスの新規構築を開始します。")

			go func() {
				cwd, _ := os.Getwd()
				volumePath := filepath.ToSlash(cwd) + ":/host"

				exec.Command("docker", "run", "-d", "--name", containerName, "-v", volumePath, "ubuntu:latest", "sleep", "infinity").Run()

				editEphemeral(s, i, fmt.Sprintf("⏳ コンテナを作成しました。パッケージリストを更新中...\nインストール予定: `%s`", pkgListStr))
				// エラーを防ぐ非対話設定
				exec.Command("docker", "exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "apt-get", "update").Run()

				editEphemeral(s, i, fmt.Sprintf("⏳ コンパイラをインストールしています...\n対象: `%s`\n※1〜2分かかります。このままお待ちください。", pkgListStr))
				installArgs := append([]string{"exec", "-e", "DEBIAN_FRONTEND=noninteractive", containerName, "apt-get", "install", "-y", "--no-install-recommends"}, pkgs...)
				exec.Command("docker", installArgs...).Run()

				log.Println("[INFO] [DOCKER] サンドボックスの新規構築とセットアップが完了しました。")
				editEphemeral(s, i, "🎉 サンドボックスの新規構築とセットアップが完了しました！ジャッジ可能な状態です。")
			}()
		}
	}
}

func handleUploadDoc(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	respondEphemeral(s, i, "⏳ ファイルを受信しました。内容を解析しています...")
	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || (role != "admin" && role != "staff") {
		editEphemeral(s, i, "⛔ 権限がありません。")
		return
	}

	var attachment *discordgo.MessageAttachment
	for _, opt := range data.Options {
		if opt.Name == "file" {
			attachment = data.Resolved.Attachments[opt.Value.(string)]
		}
	}

	if !strings.HasSuffix(strings.ToLower(attachment.Filename), ".md") {
		editEphemeral(s, i, "エラー: アップロードできるのは `.md` ファイルのみです。")
		return
	}

	tempDir := "temp"
	os.MkdirAll(tempDir, os.ModePerm)
	tempFilePath := filepath.Join(tempDir, fmt.Sprintf("%s_%s", user.ID, attachment.Filename))
	downloadAttachment(attachment.URL, tempFilePath)

	meta, err := parseFrontmatter(tempFilePath)
	if err != nil || len(meta) == 0 {
		os.Remove(tempFilePath)
		editEphemeral(s, i, "❌ エラー: ファイル先頭のメタデータ (--- で囲まれた部分) を読み取れませんでした。")
		return
	}

	docType, docID, docTitle := meta["type"], meta["id"], meta["title"]
	if docID == "" || docTitle == "" {
		os.Remove(tempFilePath)
		editEphemeral(s, i, "❌ エラー: メタデータに必須項目 (`ID`, `Title`) が含まれていません。")
		return
	}

	switch docType {
	case "contest":
		if isContestExists(docID) {
			os.Remove(tempFilePath)
			editEphemeral(s, i, fmt.Sprintf("⚠️ コンテスト `%s` は既に存在します。", docID))
			return
		}

		startTimeStr := meta["start_time"]
		endTimeStr := meta["end_time"]

		if startTimeStr == "" || endTimeStr == "" {
			os.Remove(tempFilePath)
			editEphemeral(s, i, "❌ エラー: コンテストには `Start_Time` と `End_Time` の指定が必須です。(例: 2026-11-15 21:00:00)")
			return
		}

		layout := "2006-01-02 15:04:05"
		startTimeStr = strings.ReplaceAll(startTimeStr, "/", "-")
		endTimeStr = strings.ReplaceAll(endTimeStr, "/", "-")

		// タイムゾーンをJST (Asia/Tokyo) として正確に解釈
		loc, _ := time.LoadLocation("Asia/Tokyo")
		startTime, err1 := time.ParseInLocation(layout, startTimeStr, loc)
		endTime, err2 := time.ParseInLocation(layout, endTimeStr, loc)

		if err1 != nil || err2 != nil {
			os.Remove(tempFilePath)
			editEphemeral(s, i, "❌ エラー: 時刻のフォーマットが不正です。`YYYY-MM-DD HH:MM:SS` 形式で記述してください。")
			return
		}

		contestDir := filepath.Join("contests", docID)
		os.MkdirAll(contestDir, os.ModePerm)
		os.MkdirAll(filepath.Join(contestDir, "problems"), os.ModePerm)
		moveFile(tempFilePath, filepath.Join(contestDir, "contest.md"))
		
		registerContestToDB(docID, docTitle, startTime, endTime)
		
		editEphemeral(s, i, fmt.Sprintf("🎉 **コンテストを新規作成しました！**\n\n**ID**: `%s`\n**タイトル**: %s\n**期間**: %s 〜 %s", 
			docID, docTitle, startTime.Format("2006/01/02 15:04"), endTime.Format("15:04")))

	case "problem":
		contestID := meta["contestid"]
		if contestID == "" || !isContestExists(contestID) {
			os.Remove(tempFilePath)
			editEphemeral(s, i, "❌ エラー: 所属する `ContestID` が未指定か存在しません。")
			return
		}

		problemDir := filepath.Join("contests", contestID, "problems", docID)
		os.MkdirAll(problemDir, os.ModePerm)
		os.MkdirAll(filepath.Join(problemDir, "in"), os.ModePerm)
		os.MkdirAll(filepath.Join(problemDir, "out"), os.ModePerm)
		moveFile(tempFilePath, filepath.Join(problemDir, "statement.md"))

		score := 100
		if sVal, err := strconv.Atoi(meta["score"]); err == nil && sVal > 0 { score = sVal }
		
		timeLimit := 2.0
		if tVal, err := strconv.ParseFloat(meta["time_limit"], 64); err == nil && tVal > 0 { timeLimit = tVal }
		
		memoryLimit := 1024
		if mVal, err := strconv.Atoi(meta["memory_limit"]); err == nil && mVal > 0 { memoryLimit = mVal }

		configMap := map[string]interface{}{
			"time_limit":   timeLimit,
			"memory_limit": memoryLimit,
			"score":        score,
		}
		configBytes, _ := json.MarshalIndent(configMap, "", "  ")
		os.WriteFile(filepath.Join(problemDir, "config.json"), configBytes, os.ModePerm)

		if !isProblemExists(docID) {
			registerProblemToDB(docID, docTitle, user.ID, score)
		}

		editEphemeral(s, i, fmt.Sprintf("✅ **問題をコンテストに追加しました！**\n\n**問題ID**: `%s`\n**タイトル**: %s\n**配点**: %d点", docID, docTitle, score))

	default:
		os.Remove(tempFilePath)
		editEphemeral(s, i, fmt.Sprintf("❌ エラー: 未知のType `%s` です。", docType))
	}
}

func handleCopyProblem(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	respondEphemeral(s, i, "⏳ 問題のコピー処理を実行中です...")

	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || (role != "admin" && role != "staff") {
		editEphemeral(s, i, "⛔ 権限がありません。このコマンドは Admin または Staff 専用です。")
		return
	}

	var problemID, contestID string
	for _, opt := range data.Options {
		if opt.Name == "problem" {
			problemID = opt.StringValue()
		} else if opt.Name == "contest" {
			contestID = opt.StringValue()
		}
	}

	// 1. 存在チェック
	if !isProblemExists(problemID) {
		editEphemeral(s, i, fmt.Sprintf("❌ エラー: 問題 `%s` はデータベースに登録されていません。", problemID))
		return
	}
	if !isContestExists(contestID) {
		editEphemeral(s, i, fmt.Sprintf("❌ エラー: コンテスト `%s` は存在しません。", contestID))
		return
	}

	// 2. コピー元とコピー先のパスを生成
	srcPath := filepath.Join("problems", problemID)
	dstPath := filepath.Join("contests", contestID, "problems", problemID)

	// コピー元フォルダが存在するか確認
	if _, err := os.Stat(srcPath); os.IsNotExist(err) {
		editEphemeral(s, i, fmt.Sprintf("⚠️ フォルダ `%s` が見つかりません。直接削除された可能性があります。", srcPath))
		return
	}

	// コピー先にすでに同名の問題が存在しないかチェック
	if _, err := os.Stat(dstPath); err == nil {
		editEphemeral(s, i, fmt.Sprintf("⚠️ 指定されたコンテストには、すでに `%s` というIDの問題が追加されています。", problemID))
		return
	}

	// 3. フォルダの再帰的コピー処理
	if err := copyDir(srcPath, dstPath); err != nil {
		log.Printf("[ERROR] [SYSTEM] 問題フォルダのコピーに失敗 (%s -> %s): %v\n", srcPath, dstPath, err)
		editEphemeral(s, i, "サーバーエラー: フォルダのコピーに失敗しました。")
		return
	}

	log.Printf("[INFO] [SYSTEM] 問題 %s をコンテスト %s にコピーしました。\n", problemID, contestID)
	editEphemeral(s, i, fmt.Sprintf("✅ **問題のコンテストへの追加が完了しました！**\n\n**問題ID**: `%s`\n**追加先**: コンテスト `%s`\n\n元の問題(`problems/`)はそのまま残っているため、他のコンテストでも再利用可能です。", problemID, contestID))
}

func handleJoinContest(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	_, isReg, err := getUserRole(user.ID)
	if err != nil {
		respondEphemeral(s, i, "データベースエラーが発生しました。")
		return
	}
	if !isReg {
		respondEphemeral(s, i, "❌ 未登録です。コンテストに参加するには、先に `/register` を実行してシステムに登録してください。")
		return
	}

	contestID := data.Options[0].StringValue()

	if !isContestExists(contestID) {
		respondEphemeral(s, i, fmt.Sprintf("❌ エラー: コンテスト `%s` は存在しません。", contestID))
		return
	}

	if isUserJoinedContest(contestID, user.ID) {
		respondEphemeral(s, i, fmt.Sprintf("⚠️ すでにコンテスト `%s` に参加登録済みです。健闘を祈ります！", contestID))
		return
	}

	if err := joinContestDB(contestID, user.ID); err != nil {
		log.Printf("[ERROR] [DB] コンテスト参加登録に失敗 (User: %s, Contest: %s): %v\n", user.ID, contestID, err)
		respondEphemeral(s, i, "サーバーエラー: 参加登録に失敗しました。")
		return
	}

	log.Printf("[INFO] [CONTEST] %s さんがコンテスト %s に参加しました。\n", user.Username, contestID)
	
	// 参加成功時はみんなが見えるチャンネル全体に通知する
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("🎉 **%s** さんがコンテスト `%s` に参加登録しました！", user.Username, contestID),
		},
	})
}

func handleContestRanking(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	contestID := data.Options[0].StringValue()

	if !isContestExists(contestID) {
		respondEphemeral(s, i, fmt.Sprintf("❌ エラー: コンテスト `%s` は存在しません。", contestID))
		return
	}

	participants, err := getContestRankingDB(contestID)
	if err != nil {
		log.Printf("[ERROR] [DB] ランキング取得に失敗 (Contest: %s): %v\n", contestID, err)
		respondEphemeral(s, i, "サーバーエラー: ランキングの取得に失敗しました。")
		return
	}

	if len(participants) == 0 {
		respondEphemeral(s, i, fmt.Sprintf("ℹ️ コンテスト `%s` にはまだ参加者がいません。\n一番乗りで `/join_contest` してみましょう！", contestID))
		return
	}

	// ランキングの埋め込みメッセージを作成
	var embed discordgo.MessageEmbed
	embed.Title = fmt.Sprintf("🏆 コンテスト順位表: %s", contestID)
	embed.Color = 0xF1C40F // ゴールド色
	embed.Description = fmt.Sprintf("現在の参加者: **%d** 名\n\n", len(participants))

	// 上位者をリスト化
	rankText := ""
	for idx, p := range participants {
		rank := idx + 1
		medal := "🏅"
		if rank == 1 { medal = "🥇" } else if rank == 2 { medal = "🥈" } else if rank == 3 { medal = "🥉" }
		
		// 最後にスコアが変動した時刻を表示 (未提出の場合は --:--)
		timeStr := "--:--"
		if p.LastAC != nil {
			timeStr = p.LastAC.Local().Format("15:04:05")
		}
		
		rankText += fmt.Sprintf("%s **%d位**: %s `Score: %d` (🕒 %s)\n", medal, rank, p.Username, p.Score, timeStr)
		
		if rank >= 30 {
			rankText += "\n... (以降省略)"
			break
		}
	}
	
	embed.Description += rankText

	// ランキングは公開メッセージとして誰でも見れるように送信
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{&embed},
		},
	})
}

func handleExportWeb(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	respondEphemeral(s, i, "⏳ フロントエンド用のデータ抽出とエクスポートを開始しています...")

	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || (role != "admin" && role != "staff") {
		editEphemeral(s, i, "⛔ 権限がありません。このコマンドは Admin または Staff 専用です。")
		return
	}

	var contestID string
	for _, opt := range data.Options {
		if opt.Name == "contest" {
			contestID = opt.StringValue()
		}
	}

	if !isContestExists(contestID) {
		editEphemeral(s, i, fmt.Sprintf("❌ エラー: コンテスト `%s` は存在しません。", contestID))
		return
	}

	// 💡 フロントエンド(Vite)の public フォルダに出力する設計
	srcContestDir := filepath.Join("contests", contestID)
	dstContestDir := filepath.Join("..", "frontend", "public", "contests", contestID)

	if _, err := os.Stat(srcContestDir); os.IsNotExist(err) {
		editEphemeral(s, i, fmt.Sprintf("❌ エラー: エクスポートするコンテストフォルダ `%s` が見つかりません。", srcContestDir))
		return
	}

	// Web用のコンテストフォルダの作成
	os.MkdirAll(dstContestDir, os.ModePerm)

	// 1. contest.md の安全なコピー
	srcContestMD := filepath.Join(srcContestDir, "contest.md")
	if _, err := os.Stat(srcContestMD); err == nil {
		copyFile(srcContestMD, filepath.Join(dstContestDir, "contest.md"))
	}

	// 2. problems フォルダの走査
	srcProblemsDir := filepath.Join(srcContestDir, "problems")
	dstProblemsDir := filepath.Join(dstContestDir, "problems")

	pEntries, err := os.ReadDir(srcProblemsDir)
	probCount := 0
	if err == nil {
		for _, pEntry := range pEntries {
			if !pEntry.IsDir() { continue }
			
			problemID := pEntry.Name()
			srcProb := filepath.Join(srcProblemsDir, problemID)
			dstProb := filepath.Join(dstProblemsDir, problemID)
			
			os.MkdirAll(dstProb, os.ModePerm)

			// ⚠️ 厳重注意ポイント
			// テストケース(in/out)は機密情報なので絶対にコピーしない！
			copyFile(filepath.Join(srcProb, "statement.md"), filepath.Join(dstProb, "statement.md"))
			copyFile(filepath.Join(srcProb, "config.json"), filepath.Join(dstProb, "config.json"))
			probCount++
		}
	}

	log.Printf("[INFO] [SYSTEM] コンテスト %s (問題数: %d) をWeb用にエクスポートしました。\n", contestID, probCount)
	
	successMsg := fmt.Sprintf("✅ **Web向けエクスポート完了！**\n\nコンテスト **`%s`** と内包される **%d問** のデータを抽出しました。\n\n📂 **出力先**: `%s`\n※不要なテストケース(`in`/`out`)は自動で除外されています。\n※Viteの `public` フォルダに配置したため、フロント側からは `fetch('/contests/%s/...')` で直接読み込めます！", contestID, probCount, filepath.Join("..", "frontend", "public", "contests"), contestID)
	editEphemeral(s, i, successMsg)
}