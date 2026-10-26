package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
	"regexp"

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

	var problemID string
	var attachment *discordgo.MessageAttachment

	for _, opt := range data.Options {
		switch opt.Name {
		case "problem":
			problemID = opt.StringValue()
		case "file":
			attachment = data.Resolved.Attachments[opt.Value.(string)]
		}
	}

	// 問題が存在するかチェック
	if !isProblemExists(problemID) {
		s.ChannelMessageSend(dmChannelID, fmt.Sprintf("エラー: 問題ID `%s` は存在しません。", problemID))
		return
	}

	// 拡張子チェック (例: main.cpp -> .cpp)
	ext := filepath.Ext(attachment.Filename)
	allowedExts := os.Getenv("ALLOWED_FILENAMES") // .envには ".cpp,.py,.go" のように記述するように変更
	if allowedExts != "" && !strings.Contains(allowedExts, ext) {
		errMsg := fmt.Sprintf("エラー: 許可されていないファイル形式です。許可されている拡張子: `%s`", allowedExts)
		s.ChannelMessageSend(dmChannelID, errMsg)
		return
	}

	userDirName := fmt.Sprintf("%s_%s", user.Username, user.ID)
	submitDir := filepath.Join("submissions", userDirName, problemID)
	os.MkdirAll(submitDir, os.ModePerm)

	filePath := filepath.Join(submitDir, attachment.Filename)
	if err := downloadAttachment(attachment.URL, filePath); err != nil {
		s.ChannelMessageSend(dmChannelID, "エラー: ファイルの保存に失敗しました。")
		return
	}

	now := time.Now()
	submissionData := Submission{
		UserID:      user.ID,
		Username:    user.Username,
		ProblemID:   problemID,
		Filename:    attachment.Filename,
		FilePath:    filePath,
		DMChannelID: dmChannelID,
		SubmittedAt: now,
		IsTest:      false,
	}

	s.ChannelMessageSend(dmChannelID, fmt.Sprintf("✅ 提出を受け付けました！\n問題: `%s`\nファイル: `%s`\n現在ジャッジ中です...", problemID, attachment.Filename))
	
	// ジャッジキューに送信
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
	var mdAtt, zipAtt *discordgo.MessageAttachment

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

	if !strings.HasSuffix(strings.ToLower(mdAtt.Filename), ".md") {
		editEphemeral(s, i, "エラー: 問題文は `.md` ファイルである必要があります。")
		return
	}
	if !strings.HasSuffix(strings.ToLower(zipAtt.Filename), ".zip") {
		editEphemeral(s, i, "エラー: テストケースは `.zip` ファイルである必要があります。")
		return
	}

	baseDir := filepath.Join("problems", problemName)
	if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
		editEphemeral(s, i, "サーバーエラー: ディレクトリの作成に失敗しました。")
		return
	}

	// 問題文(.md)の保存
	if err := downloadAttachment(mdAtt.URL, filepath.Join(baseDir, "statement.md")); err != nil {
		editEphemeral(s, i, "エラー: 問題文のダウンロードに失敗しました。")
		return
	}

	// ZIPのダウンロードと解凍
	zipPath := filepath.Join(baseDir, "testcases.zip")
	if err := downloadAttachment(zipAtt.URL, zipPath); err != nil {
		editEphemeral(s, i, "エラー: テストケース(ZIP)のダウンロードに失敗しました。")
		return
	}

	if err := unzip(zipPath, baseDir); err != nil {
		editEphemeral(s, i, fmt.Sprintf("エラー: ZIPの解凍に失敗しました: %v", err))
		return
	}

	// 展開が終わった元のZIPファイルは不要なので削除
	os.Remove(zipPath)

	inDir := filepath.Join(baseDir, "in")
	outDir := filepath.Join(baseDir, "out")
	
	// 直下に in フォルダが無い場合、1つ下の階層を探索する
	if _, err := os.Stat(inDir); os.IsNotExist(err) {
		entries, _ := os.ReadDir(baseDir)
		for _, e := range entries {
			if e.IsDir() && e.Name() != "in" && e.Name() != "out" {
				nestedIn := filepath.Join(baseDir, e.Name(), "in")
				nestedOut := filepath.Join(baseDir, e.Name(), "out")
				
				// 余分なフォルダの中に in があれば、直下に引き上げる
				if _, errIn := os.Stat(nestedIn); errIn == nil {
					os.Rename(nestedIn, inDir)
					os.Rename(nestedOut, outDir)
					// 不要になった空の親フォルダを丸ごと削除
					os.RemoveAll(filepath.Join(baseDir, e.Name()))
					log.Printf("[AUTO FIX] %s のテストケース階層ズレ (%s) を自動補正しました。\n", problemName, e.Name())
					break
				}
			}
		}
	}

	if err := registerProblemToDB(problemName, problemTitle, user.ID); err != nil {
		log.Println("DB登録エラー:", err)
		editEphemeral(s, i, "エラー: データベースへの登録に失敗しました。")
		return
	}

	log.Printf("[PROBLEM CREATED] %s (ID: %s) が作成されました。\n", problemTitle, problemName)
	successMsg := fmt.Sprintf("✅ **問題の登録が完了しました！**\n\n**問題ID**: `%s`\n**表示名**: %s\n\nディレクトリ (`problems/%s/`) に展開されました。", 
		problemName, problemTitle, problemName)
	editEphemeral(s, i, successMsg)
}

// 既存のEphemeralメッセージを更新（上書き）するヘルパー関数
func editEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, newContent string) {
	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content: &newContent,
	})
}

func handleProblems(s *discordgo.Session, i *discordgo.InteractionCreate, user *discordgo.User) {
	// 1. 権限チェック (Admin または Staff のみ)
	role, isReg, err := getUserRole(user.ID)
	if err != nil || !isReg || (role != "admin" && role != "staff") {
		respondEphemeral(s, i, "⛔ 権限がありません。このコマンドは Admin または Staff 専用です。")
		return
	}

	// 2. データベースから問題一覧を取得
	problems, err := getProblemList()
	if err != nil {
		log.Println("問題一覧取得エラー:", err)
		respondEphemeral(s, i, "データベースエラーが発生しました。")
		return
	}

	if len(problems) == 0 {
		respondEphemeral(s, i, "現在、登録されている問題はありません。\n`/create_problem` で最初の問題を作成してください。")
		return
	}

	// 3. 一覧をEmbed形式でフォーマットして送信
	var embed discordgo.MessageEmbed
	embed.Title = "📚 登録済みの問題一覧"
	embed.Color = 0x3498DB // 青色
	embed.Description = fmt.Sprintf("現在 **%d** 問がプールに登録されています。\n", len(problems))

	for _, p := range problems {
		// 日付を見やすくフォーマット (例: 2026/10/22)
		dateStr := p.CreatedAt.Format("2006/01/02")
		
		field := &discordgo.MessageEmbedField{
			Name:   fmt.Sprintf("ID: `%s`", p.Name),
			Value:  fmt.Sprintf("**%s**\n登録日: %s", p.Title, dateStr),
			Inline: false,
		}
		embed.Fields = append(embed.Fields, field)

		// DiscordのEmbedはフィールド上限が25個のため、超える場合は打ち切る
		if len(embed.Fields) >= 25 {
			embed.Description += "\n※表示上限(25件)に達したため、古い問題は省略されています。"
			break
		}
	}

	// Embedを送信（実行者にのみ表示）
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