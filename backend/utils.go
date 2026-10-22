package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
)

// 自動作成するロールの定義
type RoleData struct {
	Name  string
	Color int
}

var definedRoles = []RoleData{
	{"Admin", 0xE74C3C},  // 赤
	{"Staff", 0xF1C40F},  // 黄色
	{"User", 0x3498DB},   // 青
	{"未設定", 0x95A5A6}, // 灰色
}

func setupLogger() {
	logFile, err := os.OpenFile("discoder.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("ログファイルの作成に失敗しました:", err)
	}
	log.SetOutput(io.MultiWriter(os.Stdout, logFile))
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
}

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

// サーバーに必須ロールが存在するか確認し、無ければ自動作成する
func ensureGuildRoles(s *discordgo.Session, guildID string) {
	roles, err := s.GuildRoles(guildID)
	if err != nil {
		log.Println("ロール一覧の取得に失敗:", err)
		return
	}

	existingRoles := make(map[string]bool)
	for _, r := range roles {
		existingRoles[strings.ToLower(r.Name)] = true
	}

	for _, dr := range definedRoles {
		if !existingRoles[strings.ToLower(dr.Name)] {
			name := dr.Name
			color := dr.Color
			hoist := true // 右側のオンラインリストでロールごとに分けて表示する

			_, err := s.GuildRoleCreate(guildID, &discordgo.RoleParams{
					Name:  name,
					Color: &color,
					Hoist: &hoist,
				})
			if err != nil {
				log.Printf("❌ ロール '%s' の自動作成に失敗: %v\n", dr.Name, err)
			} else {
				log.Printf("✅ ロール '%s' を自動作成しました。\n", dr.Name)
			}
		}
	}
}

// データベースの権限に合わせてDiscordのロールを付与・剥奪する
func syncDiscordRole(s *discordgo.Session, guildID string, userID string, roleName string) {
	if guildID == "" {
		return
	}

	// 1. ロールがなければ作成する
	ensureGuildRoles(s, guildID)

	roles, err := s.GuildRoles(guildID)
	if err != nil {
		return
	}

	targetRoleName := "User"
	if roleName == "admin" {
		targetRoleName = "Admin"
	} else if roleName == "staff" {
		targetRoleName = "Staff"
	} else if roleName == "未設定" {
		targetRoleName = "未設定"
	}

	// 2. 過去に付与された管理対象のロール（Admin, Staff, User, 未設定）を一旦すべて外す
	for _, r := range roles {
		for _, dr := range definedRoles {
			if strings.EqualFold(r.Name, dr.Name) {
				s.GuildMemberRoleRemove(guildID, userID, r.ID)
			}
		}
	}

	// 3. 目的の新しいロールだけを付与する
	for _, r := range roles {
		if strings.EqualFold(r.Name, targetRoleName) {
			err = s.GuildMemberRoleAdd(guildID, userID, r.ID)
			if err != nil {
				log.Printf("ユーザーにロール %s の付与を試みましたが失敗しました (Botの権限不足の可能性があります): %v\n", targetRoleName, err)
			}
			return
		}
	}
}