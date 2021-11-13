package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// NullWriter は何も書かない
// これをloggerのSetOutputにセットしたらログを吐かない
type NullWriter int
func (NullWriter) Write([]byte) (int, error) {return 0, nil}

// GetLogger はファイル名を入れてロガーを返す関数
// loggerFlagがfalseならログは書かない。
func GetLogger(filename string, loggerFlag bool) *log.Logger {
	// file open
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)

	if err != nil {
		log.Fatalln(err)
	}

	// my logger for this program
	myLogger := log.New(file, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)

	// 標準出力があるなら表示する
	// ないなら標準エラー出力を出す
	if loggerFlag {
		mw := io.MultiWriter(os.Stdout, file)
		myLogger.SetOutput(mw)
	} else {
		myLogger.SetOutput(new(NullWriter))
	}

	return myLogger
}


// Exec は実行するためのコマンドをもらい、実行し、stdout, stderr, errを返す
func Exec(command string) (stdoutStr string, stderrStr string, cmderr error) {

	commands := strings.Split(command, " ")

	cmd := exec.Command(commands[0], commands[1:]...)

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	stdoutStr = stdout.String()
	stderrStr = stderr.String()
	cmderr = err

	return
}
func execCommand(cmd *exec.Cmd) error {

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	return err

}

// RequestBody はアップロードされた情報をサーバーに送る際の構造体
type RequestBody struct {
	Filename string `json:"filename"`
	Parameta string `json:"parameta"`
}

//ResponseBody はプロセスがサーバの中で走り、その結果を受け取るための構造体
type ResponseBody struct {
	OutPathURLs []string `json:"outURLs"`
	StdOut string `json:"stdout"`
	StdErr string `json:"stderr"`
}

// processFileOnServerはサーバにアップロードしたファイルを処理させる。
// サーバのurl, アップロードしたuploadedFile、サーバ上でコマンドを実行するためのparametaを受け取る
// 返り値はサーバー内で出力したファイルを取得するためのURLパスのリストを返す。
func processFileOnServer(url string, uploadedFile string, parameta string, myLogger *log.Logger) (ResponseBody, error) {

	myLogger.Printf("url: %v\n", url)
	myLogger.Printf("uploadFile: %v\n", uploadedFile)

	// 値をリクエストボディにセットする
	reqBody := RequestBody{Filename: uploadedFile, Parameta: parameta}

	// jsonに変換
	requestBody, err := json.Marshal(reqBody)
	myLogger.Printf("requestBody: %v\n", string(requestBody))
	if err != nil {
		return ResponseBody{}, err
	}

	body := bytes.NewReader(requestBody)

	// POSTリクエストを作成
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return ResponseBody{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return ResponseBody{}, err
	}

	defer resp.Body.Close()

	// レスポンスを受け取り、格納する。
	var res ResponseBody
	b, err := ioutil.ReadAll(resp.Body)
	myLogger.Printf("Response body: %v\r", string(b))
	if err := json.Unmarshal(b, &res); err != nil {
		log.Fatal(err)
	}

	return res, err

}

func main() {
	var (
		url       string
		inputFile string
		outputDir string
		parameta string
		LogFlag bool
	)
	flag.StringVar(&url, "url", "default", "please url")
	flag.StringVar(&inputFile, "i", "default in file", "please input file")
	flag.StringVar(&outputDir, "o", "default out dir", "please output dir")
	flag.StringVar(&parameta, "p", "default parameta", "please parameta")
	flag.BoolVar(&LogFlag, "l", false, "please log flag")

	flag.Parse()

	myLogger := GetLogger("./log.txt", LogFlag)

	// ローカルファイルをサーバーにアップロードする
	// curl -X POST -F "file=@<ファイル名>" localhost:8080/upload
	myLogger.Println("-- File Upload to server --")
	command := "curl -X POST -F file=@" + inputFile + " localhost:8080/upload"
	stdout, stderr, err := Exec(command)
	myLogger.Printf("commands: %v\n", command)
	myLogger.Printf("stdout: %v\n", stdout)
	myLogger.Printf("stderr: %v\n", stderr)
	if err != nil {
		log.Fatal(err)
	}

	// アップロードしたファイル情報を送信しサーバー上で処理する。
	// サーバでの実行結果を表示する。
	basename := filepath.Base(inputFile)
	res, err := processFileOnServer(url, basename, parameta, myLogger)
	if res.StdOut == "" {
		fmt.Println(res.StdErr)
	} else {
		fmt.Println(res.StdOut)
	}
	if err != nil {
		myLogger.Fatalln(err)
	}


	// processFileOnServer関数で処理されアウトプットされたファイルをcurlコマンドで取得する
	myLogger.Println("-- Get File from server --")
	for _, getOutFileURL := range res.OutPathURLs {
		command := "curl -OL " + getOutFileURL
		stdout, stderr, err = Exec(command)
		myLogger.Printf("commands: %v\n", command)
		myLogger.Printf("stdout: %v\n", stdout)
		myLogger.Printf("stderr: %v\n", stderr)
		if err != nil {
			log.Fatal(err)
		}

		// 引数で指定された出力ディレクトリに移動させる
		myLogger.Println("-- Move File --")
		basename := filepath.Base(getOutFileURL)
		newLocation := filepath.Join(outputDir, basename)
		myLogger.Printf("move %v -> %v\n", basename, newLocation)
		err = os.Rename(basename, newLocation)
		if err != nil {
			log.Fatal(err)
		}
	}

}
