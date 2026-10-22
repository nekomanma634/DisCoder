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
		log.Fatal("ユーザーテーブルの作成に失敗しました:", err)
	}

	createProblemsTableSQL := `CREATE TABLE IF NOT EXISTS problems (
		name TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		author_id TEXT NOT NULL,
		created_at DATETIME NOT NULL
	);`

	if _, err := db.Exec(createProblemsTableSQL); err != nil {
		log.Fatal("問題テーブルの作成に失敗しました:", err)
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

// 問題の登録
func registerProblemToDB(name, title, authorID string) error {
	insertSQL := `INSERT INTO problems (name, title, author_id, created_at) VALUES (?, ?, ?, ?)`
	_, err := db.Exec(insertSQL, name, title, authorID, time.Now())
	return err
}

// 問題が存在するかチェック
func isProblemExists(name string) bool {
	var dummy string
	err := db.QueryRow("SELECT name FROM problems WHERE name = ?", name).Scan(&dummy)
	return err == nil
}

// 取得する問題データを格納する構造体
type ProblemData struct {
	Name      string
	Title     string
	AuthorID  string
	CreatedAt time.Time
}

// 登録されている全問題を取得する
func getProblemList() ([]ProblemData, error) {
	// 新しいものから順に取得
	rows, err := db.Query("SELECT name, title, author_id, created_at FROM problems ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var problems []ProblemData
	for rows.Next() {
		var p ProblemData
		if err := rows.Scan(&p.Name, &p.Title, &p.AuthorID, &p.CreatedAt); err != nil {
			return nil, err
		}
		problems = append(problems, p)
	}
	return problems, nil
}