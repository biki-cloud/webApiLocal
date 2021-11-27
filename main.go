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
	"strconv"
	"strings"
	"sync"
)

// outputInfo はコマンド実行の結果を格納する。
type outputInfo struct {
	OutputURLs []string `json:"outURLs"`
	Stdout     string   `json:"stdout"`
	Stderr     string   `json:"stderr"`
	StaTus     string   `json:"status"`
	Errormsg string `json:"errmsg"`
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
func processFileOnServer(url string, uploadedFile string, parameta string, myLogger *log.Logger) (outputInfo, error) {

	myLogger.Printf("url: %v\n", url)
	myLogger.Printf("uploadFile: %v\n", uploadedFile)

	// 値をリクエストボディにセットする
	reqBody := RequestBody{Filename: uploadedFile, Parameta: parameta}

	// jsonに変換
	requestBody, err := json.Marshal(reqBody)
	myLogger.Printf("requestBody: %v\n", string(requestBody))
	if err != nil {
		return outputInfo{}, err
	}

	body := bytes.NewReader(requestBody)

	// POSTリクエストを作成
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return outputInfo{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return outputInfo{}, err
	}

	defer resp.Body.Close()

	// レスポンスを受け取り、格納する。
	var res outputInfo
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

func download(downloadURL, outputDir string, done chan error, wg *sync.WaitGroup){
	defer wg.Done()   // 関数終了時にデクリメント
	command := "curl -OL " + downloadURL
	_, _, err := Exec(command)
	if err != nil {
		done <- err
		return
	}

	// 引数で指定された出力ディレクトリに移動させる
	basename := filepath.Base(downloadURL)
	newLocation := filepath.Join(outputDir, basename)
	err = os.Rename(basename, newLocation)
	if err != nil {
		done <- err
		return
	}
	done <- nil
	return
}

func main() {
	var (
		baseURL   string
		proName   string
		inputFile string
		outputDir string
		parameta  string
		LogFlag   bool
		outJSONFLAG bool
		displayAllProgramFlag bool
	)
	flag.StringVar(&baseURL, "url", "", "(required) サーバのURLを指定してください。(必須) 例 -> -url http://127.0.0.1:8082")
	flag.StringVar(&proName, "name", "", "(required) 登録プログラムの名称を入れてください。(必須) 登録されているプログラムは-aで参照できます。例 -> -name convertToJson")
	flag.StringVar(&inputFile, "i", "", "(required) 登録プログラムに処理させる入力ファイルのパスを指定してください。(必須) 例 -> -i ./input/test.txt")
	flag.StringVar(&outputDir, "o", "", "(required) 登録プログラムの出力ファイルを出力するディレクトリを指定してください。(必須) 例 -> -o ./proOut")
	parametaUsage := "(option) 登録プログラムに使用するパラメータを指定してください。例 -> -p " + strconv.Quote("-name mike")
	flag.StringVar(&parameta, "p", "", parametaUsage)
	flag.BoolVar(&LogFlag, "l", false, "(option) -lを付与すると詳細なログを出力します。通常は使用しません。")
	flag.BoolVar(&displayAllProgramFlag, "a", false, fmt.Sprintf("(option) -aを付与するとwebサーバに登録されているプログラムのリストを表示します。-urlでurlの指定も必要。使用例 -> %s -url <http://IP:PORT> -a", flag.CommandLine.Name()))
	jsonExample := `
	{
		"status": "program timeout or program error or server error or ok",
		"stdout": "作成プログラムの標準出力",
		"stderr": "作成プログラムの標準エラー出力",
		"outURLs": [作成プログラムの出力ファイルのURLのリスト(この値は気にしなくて大丈夫です。)],
		"errmsg": "サーバ内のプログラムで起きたエラーメッセージ"
	}
	statusの各項目
	program timeout -> 登録プログラムがサーバー内で実行された際にタイムアウトになった場合
	program error   -> 登録プログラムがサーバー内で実行された際にエラーになった場合
	server error    -> サーバー内のプログラムがエラーを起こした場合
	ok              -> エラーを起こさなかった場合
	`
	flag.BoolVar(&outJSONFLAG, "j", false, "(option, but recommend) -j を付与するとコマンド結果の出力がJSON形式になり、次のように出力します。" + jsonExample)

	flag.CommandLine.Usage = func() {
		o := flag.CommandLine.Output()
		fmt.Fprintf(o, "\nUsage: %s -url <http://IP:PORT> -name <プログラム名> -i <入力ファイル> -o <出力ディレクトリ>\n", flag.CommandLine.Name())
		fmt.Fprintf(o, "\nDescription: webサーバに登録してあるプログラムを起動し、サーバ上で処理させ出力を返す。\n例:%s -url http://127.0.0.1:8082 -name convertToJson -i test.txt -o out -p %v\n \n\nOptions:\n",flag.CommandLine.Name(), strconv.Quote("-s ss -d dd"))
		flag.PrintDefaults()
		fmt.Fprintf(o, "\nCreated date 2021.11.21 by morituka. \n\n")
	}
	flag.Parse()

	isRequiredArgs := baseURL == "" || proName == "" || inputFile == "" || outputDir == ""

	if len(os.Args) == 1 {
		flag.CommandLine.Usage()
		os.Exit(2)
	}

	if displayAllProgramFlag && baseURL == "" {
		fmt.Println("パラメータが不足しています。")
		flag.CommandLine.Usage()
		os.Exit(2)
	}

	if isRequiredArgs {
		fmt.Println("必須のパラメータが不足しています。")
		fmt.Println("-------------------------------------------")
		fmt.Printf("- url : %v\n", baseURL)
		fmt.Printf("- name: %v\n", proName)
		fmt.Printf("- i   : %v\n", inputFile)
		fmt.Printf("- o   : %v\n", outputDir)
		fmt.Println("-------------------------------------------")
		flag.CommandLine.Usage()
		os.Exit(2)
	}

	if displayAllProgramFlag {
		command := fmt.Sprintf("curl %v/pro/all", baseURL)
		stdout, stderr, err := Exec(command)
		if err != nil {
			fmt.Println(err.Error())
		}else if stdout != "" {
			fmt.Println(stdout)
		} else {
			fmt.Println(stderr)
		}
		return
	}

	myLogger := GetLogger("./log.txt", LogFlag)

	// ローカルファイルをサーバーにアップロードする
	// curl -X POST -F "file=@<ファイル名>" localhost:8080/upload
	myLogger.Println("-- File Upload to server --")
	command := fmt.Sprintf("curl -X POST -F file=@%v %v/upload", inputFile, baseURL)
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
	proURL := fmt.Sprintf("%v/pro/%v", baseURL, proName)
	res, err := processFileOnServer(proURL, basename, parameta, myLogger)
	if outJSONFLAG {
		b, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fmt.Println(err.Error())
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


	// サーバ内で出力されたファイルをローカルに取得し、出力ディレクトリへ出力させる
	done := make(chan error, len(res.OutputURLs))
	var wg sync.WaitGroup
	for _, getOutFileURL := range res.OutputURLs {
		wg.Add(1) // ゴルーチン起動のたびにインクリメント
		go download(getOutFileURL, outputDir, done, &wg)
	}
	wg.Wait() // ゴルーチンでAddしたものが全てDoneされたら次に処理がいく
	close(done) // ゴルーチンが全て終了したのでチャネルをクローズする。

	for e := range done {
		if e != nil {
			fmt.Println(e.Error())
		}
	}


}
