package main

import (
	"database/sql"
	"log"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

var db *sql.DB

// データベースとテーブルの初期化
func initDB() {
	var err error
	db, err = sql.Open("sqlite", "./discoder.db")
	if err != nil {
		log.Fatal("データベースの接続に失敗しました:", err)
	}

	// ユーザーテーブルの作成
	// roleは 'admin', 'staff', 'user' のいずれか
	createTableSQL := `CREATE TABLE IF NOT EXISTS users (
		discord_id TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		role TEXT NOT NULL,
		registered_at DATETIME NOT NULL
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		log.Fatal("テーブルの作成に失敗しました:", err)
	}
	log.Println("データベース (SQLite) の初期化が完了しました。")
}

// ユーザーの登録
func registerUserToDB(discordID, username, role string) error {
	insertSQL := `INSERT INTO users (discord_id, username, role, registered_at) VALUES (?, ?, ?, ?)`
	_, err := db.Exec(insertSQL, discordID, username, role, time.Now())
	return err
}

// ユーザーの取得（登録されているか確認）
// 戻り値: (role, isRegistered, error)
func getUserRole(discordID string) (string, bool, error) {
	var role string
	err := db.QueryRow("SELECT role FROM users WHERE discord_id = ?", discordID).Scan(&role)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil // 登録されていない
		}
		return "", false, err // その他のエラー
	}
	return role, true, nil
}

func updateUserRole(discordID, newRole string) error {
	_, err := db.Exec("UPDATE users SET role = ? WHERE discord_id = ?", newRole, discordID)
	return err
}