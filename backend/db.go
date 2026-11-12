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
		score INTEGER DEFAULT 100,
		created_at DATETIME NOT NULL
	);`
	if _, err := db.Exec(createProblemsTableSQL); err != nil {
		log.Fatal("問題テーブルの作成に失敗しました:", err)
	}

	createContestsTableSQL := `CREATE TABLE IF NOT EXISTS contests (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		start_time DATETIME NOT NULL,
		end_time DATETIME NOT NULL,
		created_at DATETIME NOT NULL
	);`
	if _, err := db.Exec(createContestsTableSQL); err != nil {
		log.Fatal("コンテストテーブルの作成に失敗しました:", err)
	}

	createContestParticipantsTableSQL := `CREATE TABLE IF NOT EXISTS contest_participants (
		contest_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		score INTEGER DEFAULT 0,
		joined_at DATETIME NOT NULL,
		PRIMARY KEY (contest_id, user_id)
	);`

	if _, err := db.Exec(createContestParticipantsTableSQL); err != nil {
		log.Fatal("コンテスト参加者テーブルの作成に失敗しました:", err)
	}

	createACRecordsTableSQL := `CREATE TABLE IF NOT EXISTS ac_records (
		user_id TEXT,
		problem_id TEXT,
		contest_id TEXT,
		ac_time DATETIME,
		PRIMARY KEY (user_id, problem_id, contest_id)
	);`
	if _, err := db.Exec(createACRecordsTableSQL); err != nil {
		log.Fatal("AC記録テーブルの作成に失敗しました:", err)
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
func registerProblemToDB(name, title, authorID string, score int) error {
	insertSQL := `INSERT INTO problems (name, title, author_id, score, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := db.Exec(insertSQL, name, title, authorID, score, time.Now())
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
	Score     int
	CreatedAt time.Time
}

// 登録されている全問題を取得する
func getProblemList() ([]ProblemData, error) {
	rows, err := db.Query("SELECT name, title, author_id, score, created_at FROM problems ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var problems []ProblemData
	for rows.Next() {
		var p ProblemData
		if err := rows.Scan(&p.Name, &p.Title, &p.AuthorID, &p.Score, &p.CreatedAt); err != nil {
			return nil, err
		}
		problems = append(problems, p)
	}
	return problems, nil
}

func registerContestToDB(id, title string, startTime, endTime time.Time) error {
	insertSQL := `INSERT INTO contests (id, title, start_time, end_time, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := db.Exec(insertSQL, id, title, startTime, endTime, time.Now())
	return err
}

func isContestExists(id string) bool {
	var dummy string
	err := db.QueryRow("SELECT id FROM contests WHERE id = ?", id).Scan(&dummy)
	return err == nil
}

func getContestTimes(contestID string) (time.Time, time.Time, error) {
	var startTime, endTime time.Time
	err := db.QueryRow("SELECT start_time, end_time FROM contests WHERE id = ?", contestID).Scan(&startTime, &endTime)
	return startTime, endTime, err
}

// コンテスト参加の登録
func joinContestDB(contestID, userID string) error {
	insertSQL := `INSERT INTO contest_participants (contest_id, user_id, score, joined_at) VALUES (?, ?, 0, ?)`
	_, err := db.Exec(insertSQL, contestID, userID, time.Now())
	return err
}

// 既に参加しているかチェック
func isUserJoinedContest(contestID, userID string) bool {
	var dummy string
	err := db.QueryRow("SELECT user_id FROM contest_participants WHERE contest_id = ? AND user_id = ?", contestID, userID).Scan(&dummy)
	return err == nil
}

// ランキング取得用の構造体
type ContestParticipant struct {
	Username string
	Score    int
	JoinedAt time.Time
	LastAC   *time.Time
}

// コンテストのランキングを取得する (スコア降順、同点は参加日時の昇順)
func getContestRankingDB(contestID string) ([]ContestParticipant, error) {
	query := `
		SELECT 
			u.username, 
			cp.score, 
			cp.joined_at,
			MAX(ar.ac_time) as last_ac
		FROM contest_participants cp
		JOIN users u ON cp.user_id = u.discord_id
		LEFT JOIN ac_records ar ON cp.contest_id = ar.contest_id AND cp.user_id = ar.user_id
		WHERE cp.contest_id = ?
		GROUP BY cp.user_id, u.username, cp.score, cp.joined_at
		ORDER BY 
			cp.score DESC, 
			CASE WHEN MAX(ar.ac_time) IS NULL THEN 1 ELSE 0 END ASC,
			MAX(ar.ac_time) ASC, 
			cp.joined_at ASC
	`
	rows, err := db.Query(query, contestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var participants []ContestParticipant
	for rows.Next() {
		var p ContestParticipant
		var lastACStr *string

		if err := rows.Scan(&p.Username, &p.Score, &p.JoinedAt, &lastACStr); err != nil {
			return nil, err
		}

		if lastACStr != nil {
			// SQLiteドライバーが生成する可能性のある複数のフォーマットに対応
			layouts := []string{
				time.RFC3339Nano,
				time.RFC3339,
				"2006-01-02 15:04:05.999999999-07:00",
				"2006-01-02 15:04:05-07:00",
				"2006-01-02 15:04:05.999999999",
				"2006-01-02 15:04:05",
			}
			for _, layout := range layouts {
				if parsedTime, err := time.Parse(layout, *lastACStr); err == nil {
					p.LastAC = &parsedTime
					break
				}
			}
		}

		participants = append(participants, p)
	}
	return participants, nil
}

func AddScoreIfFirstAC(userID, problemID, contestID string) (bool, int, error) {
	if contestID == "" {
		return false, 0, nil
	}
	
	// 1. すでにこのコンテストでAC済みかチェック
	var dummy string
	err := db.QueryRow("SELECT user_id FROM ac_records WHERE user_id = ? AND problem_id = ? AND contest_id = ?", userID, problemID, contestID).Scan(&dummy)
	if err == nil {
		return false, 0, nil // 既にAC済みなのでスコア加算なし
	}

	// 2. 問題の配点を取得
	var score int
	err = db.QueryRow("SELECT score FROM problems WHERE name = ?", problemID).Scan(&score)
	if err != nil {
		score = 100 // 万が一DBに配点がない場合のフェイルセーフ
	}

	// 3. トランザクションで安全にスコア加算とAC記録を同時に行う
	tx, err := db.Begin()
	if err != nil { return false, 0, err }
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE contest_participants SET score = score + ? WHERE contest_id = ? AND user_id = ?", score, contestID, userID)
	if err != nil { return false, 0, err }

	_, err = tx.Exec("INSERT INTO ac_records (user_id, problem_id, contest_id, ac_time) VALUES (?, ?, ?, ?)", userID, problemID, contestID, time.Now())
	if err != nil { return false, 0, err }

	tx.Commit()
	return true, score, nil
}