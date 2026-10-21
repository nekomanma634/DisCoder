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
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent | discordgo.IntentsDirectMessages

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
		// 送信元がどこであれ、ユーザーとのDMチャンネルを取得する
		dmChannel, err := s.UserChannelCreate(m.Author.ID)
		if err != nil {
			log.Println("DMチャンネルの作成に失敗しました:", err)
			s.ChannelMessageSend(m.ChannelID, "エラー: DMを送信できませんでした。")
			return
		}

		// 添付ファイルが存在するかチェック
		if len(m.Attachments) == 0 {
			s.ChannelMessageSend(dmChannel.ID, "エラー: ソースコードのファイルを添付してください！")
			return
		}

		attachment := m.Attachments[0]

		allowedFiles := os.Getenv("ALLOWED_FILENAMES")
		if allowedFiles != "" {
			isValidFile := false
			// カンマ区切りで複数指定されている場合を考慮してチェック
			for _, allowed := range strings.Split(allowedFiles, ",") {
				if attachment.Filename == strings.TrimSpace(allowed) {
					isValidFile = true
					break
				}
			}

			// 一致するファイル名がなかった場合はエラーを返して終了
			if !isValidFile {
				errMsg := fmt.Sprintf("エラー: 提出できるファイル名は `%s` のみです。\nあなたの提出ファイル: `%s`", allowedFiles, attachment.Filename)
				s.ChannelMessageSend(dmChannel.ID, errMsg)
				return
			}
		}

		userDirName := fmt.Sprintf("%s_%s", m.Author.Username, m.Author.ID)
		submitDir := filepath.Join("submissions", userDirName)

		// 2. ユーザー専用ディレクトリを作成
		if err := os.MkdirAll(submitDir, os.ModePerm); err != nil {
			s.ChannelMessageSend(dmChannel.ID, "サーバーエラー: 保存先ディレクトリを作成できませんでした。")
			return
		}

		// 3. ディレクトリが分かれたので、ファイル名はそのまま（元の名前）で保存
		filePath := filepath.Join(submitDir, attachment.Filename)

		// ファイルのダウンロード
		err = downloadAttachment(attachment.URL, filePath)
		if err != nil {
			s.ChannelMessageSend(dmChannel.ID, "エラー: ファイルの保存に失敗しました。")
			log.Println("ダウンロードエラー:", err)
			return
		}

		// DMへ完了メッセージを送信
		msg := fmt.Sprintf("提出を受け付けました！\nファイル名: `%s`\nジャッジが完了次第、こちらに結果を通知します。", attachment.Filename)
		s.ChannelMessageSend(dmChannel.ID, msg)

		// サーバー（公開チャンネル）からの提出だった場合のみ、元のメッセージにリアクションをつける
		// m.GuildID が空文字列の場合はDMからの送信を意味します
		if m.GuildID != "" {
			s.MessageReactionAdd(m.ChannelID, m.ID, "✅")
		}
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