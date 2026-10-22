package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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

func unzip(src string, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// zip内ファイルの保存先パスを生成
		fpath := filepath.Join(dest, f.Name)

		// ディレクトリトラバーサル（Zip Slip）攻撃対策
		if !strings.HasPrefix(fpath, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("不正なファイルパスが含まれています: %s", fpath)
		}

		// ディレクトリの場合は作成して次へ
		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, os.ModePerm)
			continue
		}

		// ファイルが格納されるディレクトリがまだ無ければ作成
		if err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm); err != nil {
			return err
		}

		// ファイルを展開して書き込む
		outFile, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			return err
		}
	}
	return nil
}