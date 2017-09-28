package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/garyburd/go-oauth/oauth"
	"github.com/joeshaw/envdecode"
)

var conn net.Conn

func dial(netw, addr string) (net.Conn, error) {
	// すでに接続が切れているか確認
	if conn != nil {
		conn.Close()
		conn = nil
	}
	// 接続を試みる
	netc, err := net.DialTimeout(netw, addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	// 接続が成功
	conn = netc
	return netc, nil
}

// 検索結果のレスポンスを保持するための変数
var reader io.ReadCloser

// Twitterとの接続をクリーンアップするための関数
func closeConn() {
	if conn != nil {
		conn.Close()
	}
	if reader != nil {
		reader.Close()
	}
}

// TwitterAPIを使ったリクエストを送るためのオブジェクト
var (
	authClient *oauth.Client
	creds      *oauth.Credentials
)

func setupTwitterAuth() {
	// TwitterAPIを使うための認証情報
	var ts struct {
		ConsumerKey    string `env:"SP_TWITTER_KEY"`
		ConsumerSecret string `env:"SP_TWITTER_SECRET"`
		AccessToken    string `env:"SP_TWITTER_ACCESSTOKEN"`
		AccessSecret   string `env:"SP_TWITTER_ACCESSSECRET"`
	}
	// 環境変数が有効かどうかチェック
	if err := envdecode.Decode(&ts); err != nil {
		log.Fatalln(err)
	}
	// 各種認証情報をオブジェクトに設定
	creds = &oauth.Credentials{
		Token:  ts.AccessToken,
		Secret: ts.AccessSecret,
	}
	authClient = &oauth.Client{
		Credentials: oauth.Credentials{
			Token:  ts.ConsumerKey,
			Secret: ts.ConsumerSecret,
		},
	}
}

// Twitterにリクエストを送るためのリクエスト
var (
	authSetupOnce sync.Once
	httpClient    *http.Client
)

// Twitterにリクエストを送信
func makeRequest(req *http.Request, params url.Values) (*http.Response, error) {
	// 初期化処理をsync.Onceでラップすることで1回のみ実行とする
	authSetupOnce.Do(func() {
		setupTwitterAuth()
		httpClient = &http.Client{
			Transport: &http.Transport{
				Dial: dial,
			},
		}
	})
	formEnc := params.Encode()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", strconv.Itoa(len(formEnc)))
	//req.Header.Set("Authorization", authClient.AuthorizationHeader(creds, "POST", req.URL, params))
	req.Header.Set("Authorization", authClient.AuthorizationHeader(creds, "GET", req.URL, params))
	return httpClient.Do(req)
}

// Twitterのつぶやきのデータを保持する
type tweet struct {
	Text string
}

// データベースから回答情報を取得し、Twitterで検索を行う
func readFromTwitter(votes chan<- string) {
	// DBに格納されている投票情報を取得
	options, err := loadOptions()
	if err != nil {
		log.Println("選択肢の読み込みに失敗しました：", err)
		return
	}
	//u, err := url.Parse("https://stream.twitter.com/1.1/statuses/filter.json")
	u, err := url.Parse("https://api.twitter.com/1.1/statuses/home_timeline.json")
	if err != nil {
		log.Println("URLの解析に失敗しました：", err)
		return
	}
	query := make(url.Values)
	query.Set("track", strings.Join(options, ","))
	req, err := http.NewRequest("GET", u.String(), strings.NewReader(query.Encode()))
	if err != nil {
		log.Println("検索のリクエストの作成に失敗しました：", err)
		return
	}
	// URL表示
	log.Println("Twitterに検索をするURL：", req.URL)
	// Twitter APIへの検索リクエスト送信
	resp, err := makeRequest(req, query)
	// レスポンスの中身を確認
	b, err := ioutil.ReadAll(resp.Body)
	if err == nil {
		fmt.Println(string(b))
	}

	if err != nil {
		log.Println("検索のリクエストに失敗しました", err)
		return
	}

	reader = resp.Body
	decoder := json.NewDecoder(reader)
	for {
		var tweet tweet
		if err := decoder.Decode(&tweet); err != nil {
			break
		}
		// DBから取得した選択肢と、tweetの内容を比較
		for _, option := range options {
			if strings.Contains(strings.ToLower(tweet.Text), strings.ToLower(option)) {
				log.Println("投票:", option)
				votes <- option
			}
		}
	}
}

// goroutineの中でreadFromTwitterメソッドを繰り返し呼び出す
func startTwitterStream(stopchan <-chan struct{}, votes chan<- string) <-chan struct{} {
	// goroutineが完全に終了したことを知らせるためのチャネル
	stoppedchan := make(chan struct{}, 1)
	// goroutineの中で繰り返しTwitterに問い合わせを行う
	go func() {
		// goroutineが終了した際、シグナルとしてからのデータをstoppedchanに送る
		defer func() {
			stoppedchan <- struct{}{}
		}()
		for {
			select {
			// stopchanにデータが渡された（終了を指示された）場合
			case <-stopchan:
				log.Println("Twitterへの問い合わせを終了します...")
				return
			// 一定間感覚でTwitterに投票情報を問い合わせる
			default:
				log.Println("Twitterに問い合わせます...")
				readFromTwitter(votes)
				log.Println(" (待機中) ")
				time.Sleep(10 * time.Second) // 待機してから再接続します
			}
		}
	}()
	// goroutine終了を知らせるチャネルを返す
	return stoppedchan
}
