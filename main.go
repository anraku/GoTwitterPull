package main // stop変数の競合を防ぐため

import (
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	nsq "github.com/bitly/go-nsq"

	mgo "gopkg.in/mgo.v2"
)

func main() {
	var stoplock sync.Mutex // stop変数の競合を防ぐため
	stop := false
	// startTwitterStreamの終了シグナルを受信するためのチャネル
	stopChan := make(chan struct{}, 1)
	// プログラムが終了した時にUnixシグナルを受け取るためのチャネル
	signalChan := make(chan os.Signal, 1)
	go func() {
		// Unixシグナルを受け取るまで処理をブロック
		<-signalChan
		stoplock.Lock()
		// 46行目で定義されているgoroutineを終了させるためフラグを立てる
		stop = true
		stoplock.Unlock()
		log.Println("停止します...")
		// startTwitterStreamに終了させるためのシグナルを送る
		stopChan <- struct{}{}
		closeConn()
	}()
	// プログラムが終了された際、チャネルにシグナルを送る
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)

	// DBへ接続
	if err := dialdb(); err != nil {
		log.Fatalln("MongoDBへのダイアルに失敗しました：", err)
	}
	defer closedb()

	// 処理を開始します
	votes := make(chan string) // 投票結果のためのチャネル
	// votesが受信したtweetデータをNSQにパブリッシュする
	publisherStoppedChan := publishVotes(votes)
	// 10秒ごとにTwitterに問い合わせ、DBの選択肢と比較、NSQにパブリッシュを行う
	twitterStoppedChan := startTwitterStream(stopChan, votes)
	go func() {
		for {
			// 1分ごとにTwitterとの接続を切る
			time.Sleep(1 * time.Minute)
			closeConn()
			stoplock.Lock()
			// プログラムが終了しているならば、forループから抜ける
			if stop {
				stoplock.Unlock()
				break
			}
			stoplock.Unlock()
		}
	}()
	<-twitterStoppedChan
	close(votes)
	<-publisherStoppedChan
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
		options = append(options, p.Options...)
	}
	iter.Close()
	return options, iter.Err()
}

func publishVotes(votes <-chan string) <-chan struct{} {
	stopchan := make(chan struct{}, 1)
	// NSQの接続先
	pub, err := nsq.NewProducer("localhost:4161", nsq.NewConfig())
	if err != nil {
		log.Println("NSQへの接続に失敗しました：", err)
	}
	go func() {
		// votesにデータが送信されるまでfor文で処理がブロックされる
		// goroutineが終了すると、ループを脱出する
		for vote := range votes {
			pub.Publish("votes", []byte(vote))
		}
		log.Println("Publisher: 停止中です")
		pub.Stop()
		log.Println("Publisher: 停止しました")
		// goroutine内で終了時のシグナルを送信しているが、
		// deferを使って処理を書いてもよい
		stopchan <- struct{}{}
	}()
	return stopchan
}
