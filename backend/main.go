package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	setupLogger()
	log.Println("=== Discoder System Starting ===")

	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .envファイルが見つかりません。")
	}

	initDB()
	defer db.Close()

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("エラー: DISCORD_TOKEN が設定されていません。")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal("Discordセッションの作成に失敗しました:", err)
	}

	// ジャッジ用キューとワーカーの初期化
	initJudgeQueues(dg)

	// イベントハンドラの登録とIntent設定
	dg.AddHandler(interactionCreate)

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.GuildMemberAdd) {
		log.Printf("[JOIN] %s さんがサーバーに参加しました。未設定ロールを付与します。\n", m.User.Username)
		syncDiscordRole(s, m.GuildID, m.User.ID, "未設定")
	})

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsDirectMessages | discordgo.IntentsGuildMembers

	if err := dg.Open(); err != nil {
		log.Fatal("Discordへの接続に失敗しました:", err)
	}
	defer dg.Close()

	// スラッシュコマンドの登録
	registerCommands(dg)

	fmt.Println("Botが起動しました！終了するには CTRL-C を押してください。")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}