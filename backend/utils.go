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
	"bufio"

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

func parseFrontmatter(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	meta := make(map[string]string)
	scanner := bufio.NewScanner(file)
	
	inFrontmatter := false
	for scanner.Scan() {
		// UTF-8のBOM (\xef\xbb\xbf) を見えない文字ごと除去
		text := strings.TrimPrefix(scanner.Text(), "\xef\xbb\xbf")
		line := strings.TrimSpace(text)

		if !inFrontmatter {
			if line == "---" {
				inFrontmatter = true
			} else if line != "" {
				// 空行以外の文字が "---" より前に来たらメタデータなしと判断して終了
				break
			}
			continue
		}

		// inFrontmatter == true の場合
		if line == "---" {
			break // メタデータブロック終了
		}

		// "Key: Value" の形を解析
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			meta[strings.ToLower(key)] = val // 判別しやすいようキーをすべて小文字化
		}
	}
	return meta, nil
}

// ファイルを移動する (別ドライブ間でも機能する安全な移動)
func moveFile(srcPath, dstPath string) error {
	inputFile, err := os.Open(srcPath)
	if err != nil { return fmt.Errorf("ソースを開けません: %w", err) }
	
	outputFile, err := os.Create(dstPath)
	if err != nil {
		inputFile.Close()
		return fmt.Errorf("保存先を作成できません: %w", err)
	}
	
	_, err = io.Copy(outputFile, inputFile)
	inputFile.Close()
	outputFile.Close()
	
	if err != nil { return fmt.Errorf("コピーに失敗: %w", err) }
	
	// コピー完了後に元ファイルを削除
	return os.Remove(srcPath)
}

// ファイル単体をコピーする関数
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	// 権限(パーミッション)も引き継ぐ
	if srcInfo, err := os.Stat(src); err == nil {
		os.Chmod(dst, srcInfo.Mode())
	}
	return nil
}

// フォルダとその中身(in/outフォルダやファイル群)を再帰的にコピーする関数
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// コピー先のフォルダを作成
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// ディレクトリの場合は再帰的にコピー
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// ファイルの場合はコピー
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}
	return nil
}