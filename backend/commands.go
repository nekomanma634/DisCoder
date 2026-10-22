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