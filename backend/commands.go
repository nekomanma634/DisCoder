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
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "contest",
					Description: "参加中のコンテストID (コンテスト問題に提出する場合)",
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
			Description: "Discoderシステムにユーザー登録します",
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
				// --- 今回追加した config オプション ---
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "config",
					Description: "問題の設定ファイル (.json形式 / 時間・メモリ制限)",
					Required:    false, // 必須ではない（未指定時はデフォルト値を使用）
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
		{
			Name:        "upload_doc",
			Description: "【Admin/Staff専用】MDファイルをアップロードし、コンテストや問題を自動構築します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionAttachment,
					Name:        "file",
					Description: "メタデータを含むMarkdownファイル (.md)",
					Required:    true,
				},
			},
		},
		{
			Name:        "copy_problem",
			Description: "【Admin/Staff専用】既存の問題を指定したコンテストにコピー(追加)します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "problem",
					Description: "コピーする問題のID (例: abc001_a)",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "contest",
					Description: "コピー先のコンテストID (例: dbc001)",
					Required:    true,
				},
			},
		},
		{
			Name:        "join_contest",
			Description: "指定したコンテストに参加登録(エントリー)します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "contest",
					Description: "参加するコンテストID (例: dbc001)",
					Required:    true,
				},
			},
		},
		{
			Name:        "contest_ranking",
			Description: "コンテストの参加者一覧と現在のランキング(順位表)を表示します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "contest",
					Description: "ランキングを見るコンテストID (例: dbc001)",
					Required:    true,
				},
			},
		},
		{
			Name:        "export_web",
			Description: "【Admin/Staff専用】フロントエンド(Web)用に指定したコンテストのデータを抽出出力します",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "contest",
					Description: "出力するコンテストID (例: dbc001)",
					Required:    true,
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
	case "upload_doc":
		handleUploadDoc(s, i, user, data)
	case "copy_problem":
		handleCopyProblem(s, i, user, data)
	case "join_contest":
		handleJoinContest(s, i, user, data)
	case "contest_ranking":
		handleContestRanking(s, i, user, data)
	case "export_web":
		handleExportWeb(s, i, user, data)
	}
}
