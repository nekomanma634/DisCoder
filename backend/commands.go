package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

func registerCommands(dg *discordgo.Session) {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "submit",
			Description: "ソースコードを提出します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "file",
					Description: "提出するファイル",
					Required:    true,
				},
			},
		},
		{
			Name:        "hello",
			Description: "BotとのDM（ダイレクトメッセージ）を開始します",
		},
		{
			Name:        "register",
			Description: "Discoderシステムにユーザー登録します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "username",
					Description: "コンテストで使用するユーザー名（半角英数字など）",
					Required:    true, // 必須入力にする
				},
			},
		},
		{
			Name:        "profile",
			Description: "自分の登録情報と現在の権限（ロール）を確認します",
		},
		{
			Name:        "setrole",
			Description: "【Admin専用】ユーザーの権限(staff等)を変更します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "target",
					Description: "権限を変更するユーザー",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "role",
					Description: "付与する権限 (admin / staff / user)",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Admin (管理者)", Value: "admin"},
						{Name: "Staff (運営)", Value: "staff"},
						{Name: "User (一般参加者)", Value: "user"},
					},
				},
			},
		},
	}

	for _, cmd := range commands {
		if _, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd); err != nil {
			log.Panicf("スラッシュコマンド '%s' の作成に失敗しました: %v", cmd.Name, err)
		}
	}
	log.Println("スラッシュコマンドの登録が完了しました。")
}

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()
	user := i.User
	if i.Member != nil {
		user = i.Member.User
	}

	dmChannel, err := s.UserChannelCreate(user.ID)
	if err != nil {
		log.Println("DMチャンネル作成エラー:", err)
		return
	}

	switch data.Name {
	case "hello":
		handleHello(s, i, user, dmChannel.ID)
	case "submit":
		handleSubmit(s, i, user, dmChannel.ID, data)
	case "register":
		handleRegister(s, i, user, data)
	case "profile":
		handleProfile(s, i, user)
	case "setrole":
		handleSetRole(s, i, user, data)
	}
}

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

	var attachment *discordgo.MessageAttachment
	for _, att := range data.Resolved.Attachments {
		attachment = att
		break
	}

	if attachment == nil {
		s.ChannelMessageSend(dmChannelID, "エラー: ファイルが添付されていません。")
		return
	}

	allowedFiles := os.Getenv("ALLOWED_FILENAMES")
	if allowedFiles != "" {
		isValidFile := false
		for _, allowed := range strings.Split(allowedFiles, ",") {
			if attachment.Filename == strings.TrimSpace(allowed) {
				isValidFile = true
				break
			}
		}
		if !isValidFile {
			errMsg := fmt.Sprintf("エラー: 提出できるファイル名は `%s` のみです。\nあなたの提出ファイル: `%s`", allowedFiles, attachment.Filename)
			s.ChannelMessageSend(dmChannelID, errMsg)
			return
		}
	}

	userDirName := fmt.Sprintf("%s_%s", user.Username, user.ID)
	submitDir := filepath.Join("submissions", userDirName)
	if err := os.MkdirAll(submitDir, os.ModePerm); err != nil {
		s.ChannelMessageSend(dmChannelID, "サーバーエラー: 保存先ディレクトリを作成できませんでした。")
		return
	}

	filePath := filepath.Join(submitDir, attachment.Filename)
	if err := downloadAttachment(attachment.URL, filePath); err != nil {
		s.ChannelMessageSend(dmChannelID, "エラー: ファイルの保存に失敗しました。")
		log.Println("ダウンロードエラー:", err)
		return
	}

	now := time.Now()
	log.Printf("[SUBMIT] User: %s (ID: %s), File: %s, Path: %s\n", user.Username, user.ID, attachment.Filename, filePath)

	submissionData := Submission{
		UserID:      user.ID,
		Username:    user.Username,
		Filename:    attachment.Filename,
		FilePath:    filePath,
		DMChannelID: dmChannelID,
		SubmittedAt: now,
	}

	msg := fmt.Sprintf("提出を受け付けました！\nファイル名: `%s`\n現在ジャッジの順番待ちキューに入っています...", attachment.Filename)
	s.ChannelMessageSend(dmChannelID, msg)

	if q, exists := problemQueues[attachment.Filename]; exists {
		q <- submissionData
	}
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

	// ユーザーが入力した任意のユーザー名を取得
	customUsername := data.Options[0].StringValue()

	role := "user"
	if user.ID == os.Getenv("OWNER_ID") {
		role = "admin"
	}

	if err := registerUserToDB(user.ID, customUsername, role); err != nil {
		log.Println("ユーザー登録エラー:", err)
		respondEphemeral(s, i, "登録中にエラーが発生しました。")
		return
	}

	// Discordのロールを自動付与
	go syncDiscordRole(s, i.GuildID, user.ID, role)

	msg := fmt.Sprintf("✅ 登録が完了しました！\nシステム内のユーザー名: `%s`\n付与されたロール: `%s`", customUsername, role)
	respondEphemeral(s, i, msg)
}

func handleSetRole(s *discordgo.Session, i *discordgo.InteractionCreate, executor *discordgo.User, data discordgo.ApplicationCommandInteractionData) {
	// 実行者がAdminかどうかチェック
	executorRole, isReg, err := getUserRole(executor.ID)
	if err != nil || !isReg || executorRole != "admin" {
		respondEphemeral(s, i, "⛔ 権限がありません。このコマンドはAdmin専用です。")
		return
	}

	targetUser := data.Options[0].UserValue(s)
	newRole := data.Options[1].StringValue()

	// 対象者がDBにいるか確認
	_, isTargetReg, err := getUserRole(targetUser.ID)
	if err != nil || !isTargetReg {
		respondEphemeral(s, i, "対象のユーザーはまだDiscoderシステムに `/register` していません。")
		return
	}

	// DB更新
	if err := updateUserRole(targetUser.ID, newRole); err != nil {
		respondEphemeral(s, i, "データベースの更新に失敗しました。")
		return
	}

	// Discordロールも同期
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

// チャンネルを汚さない（実行者にのみ見える）レスポンス用のヘルパー関数
func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: content,
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}