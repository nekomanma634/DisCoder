package main

import (
	"encoding/json"
	"log"
	"os"
)

type LangConfig struct {
	Package    string `json:"package"`
	CheckCmd   string `json:"check_cmd"`
	VersionCmd string `json:"version_cmd"`
	CompileCmd string `json:"compile_cmd"`
	RunCmd     string `json:"run_cmd"`
}

var SupportedLanguages map[string]LangConfig

func initLanguages() {
	data, err := os.ReadFile("languages.json")
	if err != nil {
		log.Fatal("エラー: languages.json の読み込みに失敗しました:", err)
	}

	if err := json.Unmarshal(data, &SupportedLanguages); err != nil {
		log.Fatal("エラー: languages.json のフォーマットが不正です:", err)
	}

	log.Printf("%d 個の言語設定 (languages.json) をロードしました。\n", len(SupportedLanguages))
}
