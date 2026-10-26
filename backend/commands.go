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
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "problem",
					Description: "提出先の問題ID (例: abc001_a)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "file",
					Description: "提出するソースコード",
					Required:    true,
				},
			},
		},
		{
			Name:        "test_problem",
			Description: "【Admin専用】登録された問題のテストケースに対してコードを実行・検証します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "problem",
					Description: "テストする問題ID",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "file",
					Description: "正解と想定されるソースコード",
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
		{
			Name:        "create_problem",
			Description: "【Admin/Staff専用】新しい問題を作成・登録します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "問題ID (半角英数字のみ。例: abc001_a)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "title",
					Description: "問題の表示名 (例: はじめての足し算)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "statement",
					Description: "問題文ファイル (.md)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "testcases",
					Description: "テストケース (.zip形式。中に in/ と out/ フォルダを含める)",
					Required:    true,
				},
			},
		},
		{
			Name:        "problems",
			Description: "【Admin/Staff専用】登録されている問題の一覧を確認します",
		},
		{
			Name:        "show_problem",
			Description: "【Admin/Staff専用】登録されている問題の本文(.md)を表示します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "name",
					Description: "表示する問題ID (例: abc001_a)",
					Required:    true,
				},
			},
		},
		{
			Name:        "sandbox",
			Description: "【Admin専用】ジャッジ用サンドボックスコンテナの管理を行います",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "action",
					Description: "実行する操作",
					Required:    true,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "状態確認 (status)", Value: "status"},
						{Name: "起動・構築 (start)", Value: "start"},
						{Name: "停止 (stop)", Value: "stop"},
						{Name: "完全削除 (delete)", Value: "delete"},
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
	case "test_problem":
		handleTestProblem(s, i, user, dmChannel.ID, data)
	case "register":
		handleRegister(s, i, user, data)
	case "profile":
		handleProfile(s, i, user)
	case "setrole":
		handleSetRole(s, i, user, data)
	case "create_problem":
		handleCreateProblem(s, i, user, data)
	case "problems":
		handleProblems(s, i, user)
	case "show_problem":
		handleShowProblem(s, i, user, data)
	case "sandbox":
		handleSandbox(s, i, user, data)
	}
}
