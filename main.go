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

// OutputInfo はコマンド実行の結果を格納する。
type OutputInfo struct {
	StaTus string `json:"status"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	OutputURLs []string `json:"outURLs"`
}

// NullWriter は何も書かない
// これをloggerのSetOutputにセットしたらログを吐かない
type NullWriter int

func (NullWriter) Write([]byte) (int, error) { return 0, nil }

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

// RequestBody はアップロードされた情報をサーバーに送る際の構造体
type RequestBody struct {
	Filename string `json:"filename"`
	Parameta string `json:"parameta"`
}

//ResponseBody はプロセスがサーバの中で走り、その結果を受け取るための構造体
type ResponseBody struct {
	OutPathURLs     []string `json:"outURLs"`
	StdOut          string   `json:"stdout"`
	StdErr          string   `json:"stderr"`
	CmdStartSuccess bool     `json:"cmdStartSuccess"`
	CmdEndSuccess   bool     `json:"cmdEndSuccess"`
}

// processFileOnServerはサーバにアップロードしたファイルを処理させる。
// サーバのurl, アップロードしたuploadedFile、サーバ上でコマンドを実行するためのparametaを受け取る
// 返り値はサーバー内で出力したファイルを取得するためのURLパスのリストを返す。
func processFileOnServer(url string, uploadedFile string, parameta string, myLogger *log.Logger) (OutputInfo, error) {

	myLogger.Printf("url: %v\n", url)
	myLogger.Printf("uploadFile: %v\n", uploadedFile)

	// 値をリクエストボディにセットする
	reqBody := RequestBody{Filename: uploadedFile, Parameta: parameta}

	// jsonに変換
	requestBody, err := json.Marshal(reqBody)
	myLogger.Printf("requestBody: %v\n", string(requestBody))
	if err != nil {
		return OutputInfo{}, err
	}

	body := bytes.NewReader(requestBody)

	// POSTリクエストを作成
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return OutputInfo{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return OutputInfo{}, err
	}

	defer resp.Body.Close()

	// レスポンスを受け取り、格納する。
	var res OutputInfo
	b, err := ioutil.ReadAll(resp.Body)
	myLogger.Printf("Response body: %v\r", string(b))
	if err := json.Unmarshal(b, &res); err != nil {
		log.Fatal(err)
	}
	myLogger.Printf("res: %v\n", res)

	if res.OutputURLs == nil {
		res.OutputURLs = []string{}
	}

	return res, err
}

func main() {
	// example -> go run main.go -url http://127.0.0.1:8081 -name convertToJson -i test.txt -o ./out -p "-s ss -d dd" -l
	var (
		baseURL   string
		proName   string
		inputFile string
		outputDir string
		parameta  string
		LogFlag   bool
		outJSONFLAG bool
	)
	flag.StringVar(&baseURL, "url", "", "サーバのURLを指定してください。例 -> -url http://127.0.0.1:8082")
	flag.StringVar(&proName, "name", "allみたいな感じにしてその場合はプログラム一覧を出してもいい。", "登録プログラムの名称を入れてください。例 -> -name convertToJson")
	flag.StringVar(&inputFile, "i", "", "登録プログラムに処理させる入力ファイルのパスを指定してください。例 -> -i ./input/test.txt")
	flag.StringVar(&outputDir, "o", "", "登録プログラムの出力ファイルを出力するディレクトリを指定してください。例 -> -o ./proOut")
	flag.StringVar(&parameta, "p", "", `登録プログラムに使用するパラメータを指定してください。例 -> -p "-name mike"`)
	flag.BoolVar(&LogFlag, "l", false, "-lを付与すると詳細なログを出力します。通常は使用しません。")
	jsonExample := `
	{
		"status": "program timeout or program error or server error or ok",
		"stdout": "作成プログラムの標準出力",
		"stderr": "作成プログラムの標準エラー出力",
		"outURLs": [作成プログラムの出力ファイルのURLのリスト(この値は気にしなくて大丈夫です。)]
	}
	program timeout -> 作成プログラムがサーバー内で実行された際にタイムアウトになった場合
	program error   -> 作成プログラムがサーバー内で実行された際にエラーになった場合
	server error    -> サーバー内のプログラムがエラーを起こした場合
	ok              -> エラーを起こさなかった場合
	`
	flag.BoolVar(&outJSONFLAG, "j", false, "-j を付与するとコマンド結果の出力がJSON形式になり、次のように出力します。" + jsonExample)

	flag.Parse()

	myLogger := GetLogger("./log.txt", LogFlag)

	// ローカルファイルをサーバーにアップロードする
	// curl -X POST -F "file=@<ファイル名>" localhost:8080/upload
	myLogger.Println("-- File Upload to server --")
	command := "curl -X POST -F file=@" + inputFile + " " + baseURL + "/upload"
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
	proURL := baseURL + "/pro/" + proName
	res, err := processFileOnServer(proURL, basename, parameta, myLogger)
	if outJSONFLAG {
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Println(err)
		}
		fmt.Print(string(b))
	} else {
		fmt.Println(res.Stdout)
		fmt.Println(res.Stderr)
	}

	if err != nil {
		fmt.Println("err occur")
		myLogger.Fatalln(err)
	}

	// processFileOnServer関数で処理されアウトプットされたファイルをcurlコマンドで取得する
	for _, getOutFileURL := range res.OutputURLs {
		myLogger.Println("-- Get File from server --")
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
