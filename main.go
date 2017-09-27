package main

import (
	"fmt"
	"log"

	mgo "gopkg.in/mgo.v2"
)

func main() {
	fmt.Println("vim-go")
}

var db *mgo.Session

func dialdb() error {
	var err error
	log.Println("MongoDBにダイアル中：localhost")
	// MongoDBに接続
	db, err = mgo.Dial("localhost") // HACK: URLを指定できる形にしたい
	return err
}
func closedb() {
	// MongoDBを閉じる
	db.Close()
	log.Println("データベース接続が閉じられました")
}

// 投票情報を取得するための構造体
type poll struct {
	Options []string // 選択肢
}

func loadOptions() ([]string, error) {
	var options []string                                 // 投票情報を格納するためのスライス
	iter := db.DB("ballots").C("polls").Find(nil).Iter() // 検索結果のiteratorを取得
	// 検索結果を保持するオブジェクト
	// AllではなくFindの結果を保持することで、1つのオブジェクトで結果を保持できる
	var p poll
	for iter.Next(&p) {
		// 検索結果を一つずつoptionsに追加する
		options = append(options, p.Optilns...)
	}
	iter.Close()
	return option, iter.Err()
}
