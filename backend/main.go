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

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	// .envファイルから環境変数を読み込む
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: .envファイルが見つかりません。システムの環境変数を使用します。")
	}

	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		log.Fatal("エラー: DISCORD_TOKEN が設定されていません。")
	}

	// Discordセッションの初期化
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatal("Discordセッションの作成に失敗しました:", err)
	}

	// メッセージ受信時のイベントハンドラを登録
	dg.AddHandler(messageCreate)

	// メッセージのコンテンツ（テキスト内容）を読み取る権限（Intent）を要求
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// Discordへ接続
	err = dg.Open()
	if err != nil {
		log.Fatal("Discordへの接続に失敗しました:", err)
	}
	defer dg.Close()

	fmt.Println("Botが起動しました！終了するには CTRL-C を押してください。")
	
	// 終了シグナルを待機
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc
}

// メッセージが送信されたときに実行される関数
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Bot自身のメッセージは無視する
	if m.Author.ID == s.State.User.ID {
		return
	}

	// "!submit" コマンドで始まっているか判定
	if strings.HasPrefix(m.Content, "!submit") {
		// 添付ファイルが存在するかチェック
		if len(m.Attachments) == 0 {
			s.ChannelMessageSend(m.ChannelID, "エラー: ソースコードのファイルを添付してください！")
			return
		}

		attachment := m.Attachments[0]

		// 提出コードを保存するディレクトリを作成（存在しなければ）
		submitDir := "submissions"
		if err := os.MkdirAll(submitDir, os.ModePerm); err != nil {
			s.ChannelMessageSend(m.ChannelID, "サーバーエラー: 保存先ディレクトリを作成できませんでした。")
			return
		}

		// ファイル名の生成 (ユーザーID_元のファイル名 にして重複を防ぐ)
		fileName := fmt.Sprintf("%s_%s", m.Author.ID, attachment.Filename)
		filePath := filepath.Join(submitDir, fileName)

		// ファイルをダウンロードして保存
		err := downloadAttachment(attachment.URL, filePath)
		if err != nil {
			s.ChannelMessageSend(m.ChannelID, "エラー: ファイルの保存に失敗しました。")
			log.Println("ダウンロードエラー:", err)
			return
		}

		// 完了メッセージを送信
		msg := fmt.Sprintf("<@%s> 提出を受け付けました！\n保存名: `%s`", m.Author.ID, fileName)
		s.ChannelMessageSend(m.ChannelID, msg)
	}
}

// 添付ファイルをHTTPリクエストで取得し、ローカルに保存する関数
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