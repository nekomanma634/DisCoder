package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// 提出データをまとめる構造体
type Submission struct {
	UserID      string
	Username    string
	Filename    string
	FilePath    string
	DMChannelID string
	SubmittedAt time.Time
}

var problemQueues map[string]chan Submission

func main() {
	// ログファイルのセットアップ
	logFile, err := os.OpenFile("discoder.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("ログファイルの作成に失敗しました:", err)
	}
	defer logFile.Close()

	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	log.Println("=== Discoder System Starting ===")

	err = godotenv.Load()
	if err != nil {
		log.Println("Warning: .envファイルが見つかりません。")
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("エラー: DISCORD_TOKEN が設定されていません。")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal("Discordセッションの作成に失敗しました:", err)
	}

	allowedFiles := os.Getenv("ALLOWED_FILENAMES")
	if allowedFiles == "" {
		log.Fatal("エラー: ALLOWED_FILENAMES が設定されていません。")
	}

	problemQueues = make(map[string]chan Submission)
	for _, f := range strings.Split(allowedFiles, ",") {
		filename := strings.TrimSpace(f)
		problemQueues[filename] = make(chan Submission, 100)
		go judgeWorker(dg, filename, problemQueues[filename])
	}

	// スラッシュコマンド用のハンドラを登録
	dg.AddHandler(interactionCreate)

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsDirectMessages

	err = dg.Open()
	if err != nil {
		log.Fatal("Discordへの接続に失敗しました:", err)
	}
	defer dg.Close()

	// --- 変更: 複数のスラッシュコマンドを登録する ---
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

	// コマンドを一括で登録
	for _, cmd := range commands {
		_, err = dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd)
		if err != nil {
			log.Panicf("スラッシュコマンド '%s' の作成に失敗しました: %v", cmd.Name, err)
		}
	}
	log.Println("スラッシュコマンドの登録が完了しました。")

	fmt.Println("Botが起動しました！終了するには CTRL-C を押してください。")
	
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

func interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()

	// コマンド実行者を取得
	var user *discordgo.User
	if i.Member != nil {
		user = i.Member.User
	} else {
		user = i.User
	}

	// DMチャンネルをあらかじめ取得しておく
	dmChannel, err := s.UserChannelCreate(user.ID)
	if err != nil {
		log.Println("DMチャンネル作成エラー:", err)
		return
	}

	// --- 変更: コマンド名(data.Name)によって処理を分岐する ---
	switch data.Name {

	case "hello":
		// チャンネル上には実行者だけに見えるメッセージを返す
		s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "DMにメッセージを送信しました！確認してください。",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

		// ユーザーのDMへ挨拶メッセージを送信
		greetingMsg := fmt.Sprintf("こんにちは、%sさん！\nDiscoderのコンテストシステムへようこそ。\nコードを提出する際は、このDMで `/submit` と入力するか、サーバーのチャンネルで実行してください！", user.Username)
		s.ChannelMessageSend(dmChannel.ID, greetingMsg)

	case "submit":
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
			s.ChannelMessageSend(dmChannel.ID, "エラー: ファイルが添付されていません。")
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
				s.ChannelMessageSend(dmChannel.ID, errMsg)
				return
			}
		}

		userDirName := fmt.Sprintf("%s_%s", user.Username, user.ID)
		submitDir := filepath.Join("submissions", userDirName)

		if err := os.MkdirAll(submitDir, os.ModePerm); err != nil {
			s.ChannelMessageSend(dmChannel.ID, "サーバーエラー: 保存先ディレクトリを作成できませんでした。")
			return
		}

		filePath := filepath.Join(submitDir, attachment.Filename)

		err = downloadAttachment(attachment.URL, filePath)
		if err != nil {
			s.ChannelMessageSend(dmChannel.ID, "エラー: ファイルの保存に失敗しました。")
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
			DMChannelID: dmChannel.ID,
			SubmittedAt: now,
		}

		msg := fmt.Sprintf("提出を受け付けました！\nファイル名: `%s`\n現在ジャッジの順番待ちキューに入っています...", attachment.Filename)
		s.ChannelMessageSend(dmChannel.ID, msg)

		if q, exists := problemQueues[attachment.Filename]; exists {
			q <- submissionData
		}
	}
}

// 添付ファイルのダウンロード（変更なし）
func downloadAttachment(url string, filepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	out, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

// ジャッジワーカー（変更なし）
func judgeWorker(s *discordgo.Session, problemName string, queue <-chan Submission) {
	log.Printf("[Worker: %s] 待機を開始しました。\n", problemName)

	for sub := range queue {
		log.Printf("[Worker: %s] %s さんの提出を処理開始します...\n", problemName, sub.Username)

		time.Sleep(2 * time.Second) // ダミーの処理時間

		judgeFinishTime := time.Now()
		duration := judgeFinishTime.Sub(sub.SubmittedAt)
		log.Printf("[JUDGE FINISHED] User: %s, File: %s, Elapsed: %v\n", sub.Username, sub.Filename, duration)

		resultMsg := fmt.Sprintf("✅ ジャッジ完了\n問題: `%s`\n結果: **AC** (仮)\n所要時間: %v", sub.Filename, duration)
		s.ChannelMessageSend(sub.DMChannelID, resultMsg)
	}
}