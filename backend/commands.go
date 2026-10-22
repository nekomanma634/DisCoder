package main

import (
	"log"

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
			Description: "Discoderシステムにユーザー登録します", // オプション入力を削除
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