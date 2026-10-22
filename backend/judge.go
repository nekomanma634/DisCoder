package main

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type Submission struct {
	UserID      string
	Username    string
	Filename    string
	FilePath    string
	DMChannelID string
	SubmittedAt time.Time
}

var problemQueues map[string]chan Submission

func initJudgeQueues(dg *discordgo.Session) {
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
}

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