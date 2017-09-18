package main

import "net"

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

var (
	authClient *oauth.Client
	creds      *oauth.Credentials
)

func setupTwitterAuth() {
	var ts struct {
		ConsumerKey    string `env:"SP_TWITTER_KEY"`
		ConsumerSecret string `env:"SP_TWITTER_SECRET"`
		AccessToken    string `env:"SP_TWITTER_ACCESSTOKEN"`
		AccessSecret   string `env:"SP_TWITTER_ACCESSSECRET"`
	}
	if err := envdecode.Decode(&ts); err != nil {
		log.Fatalln(err)
	}
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
