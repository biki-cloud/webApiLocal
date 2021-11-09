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
)


func GetLogger(filename string) *log.Logger {
	// file open
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)

	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(file)

	// my logger for this program
	myLogger := log.New(file, "INFO: ", log.Ldate|log.Ltime|log.Lshortfile)

	// 標準出力にも結果を表示するため
	//myLogger.SetOutput(io.MultiWriter(os.Stdout))
	mw := io.MultiWriter(os.Stdout, file)
	myLogger.SetOutput(mw)

	return myLogger
}

var (
	myLogger *log.Logger = GetLogger("./log.txt")
)

func execCommand(cmd *exec.Cmd) error {

	var (
		stdout bytes.Buffer
		stderr bytes.Buffer
	)

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	fmt.Printf("Stdout: %s\n", stdout.String())
	fmt.Printf("Stderr: %s\n", stderr.String())

	return err

}

type RequestBody struct {
	Filename string `json:"filename"`
	Parameta string `json:"parameta"`
}

type Res struct {
	OutPathURLs []string `json:"outURLs"`
}

// processFileOnServerはサーバにアップロードしたファイルを処理させる。
// サーバのurl, アップロードしたuploadedFile、サーバ上でコマンドを実行するためのparametaを受け取る
// 返り値はサーバー内で出力したファイルを取得するためのURLパスのリストを返す。
func processFileOnServer(url string, uploadedFile string, parameta string) ([]string, error) {
	myLogger.Printf("url: %v\n", url)
	myLogger.Printf("uploadFile: %v\n", uploadedFile)

	// 値をリクエストボディにセットする
	reqBody := RequestBody{Filename: uploadedFile, Parameta: parameta}

	// jsonに変換
	requestBody, err := json.Marshal(reqBody)
	myLogger.Printf("requestBody: %v\n", string(requestBody))
	if err != nil {
		return nil, err
	}

	body := bytes.NewReader(requestBody)

	// POSTリクエストを作成
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	// レスポンスを受け取り、格納する。
	var res Res
	b, err := ioutil.ReadAll(resp.Body)
	myLogger.Printf("Response body: %v\r", string(b))
	if err := json.Unmarshal(b, &res); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%v\n", res.OutPathURLs)

	return res.OutPathURLs, err

}

func main() {
	var (
		url       string
		inputFile string
		outputDir string
		parameta string
	)
	flag.StringVar(&url, "url", "default", "please url")
	flag.StringVar(&inputFile, "i", "default in file", "please input file")
	flag.StringVar(&outputDir, "o", "default out dir", "please output dir")
	flag.StringVar(&parameta, "p", "default parameta", "please parameta")

	flag.Parse()

	// ローカルファイルをサーバーに配置する
	// curl -X POST -F "file=@<ファイル名>" localhost:8080/upload
	myLogger.Println("-- File Upload to server --")
	fileStr := "file=@" + inputFile
	cmd := exec.Command(`curl`, `-X`, `POST`, `-F`, fileStr, "localhost:8080/upload")
	myLogger.Printf("commands: %v\n", cmd.Args)
	err := execCommand(cmd)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println()

	// アップロードしたファイルをサーバー上で処理する。
	basename := filepath.Base(inputFile)
	getOutFileUrls, err := processFileOnServer(url, basename, parameta)
	if err != nil {
		myLogger.Fatalln(err)
	}

	fmt.Println()

	// processFileOnServer関数の引数で出力されたファイルたちを受け取るURLをもらうのでローカルに取得する
	myLogger.Println("-- Get File from server --")
	for _, getOutFileUrl := range getOutFileUrls {
		cmd = exec.Command(`curl`, `-OL`, getOutFileUrl)
		fmt.Println(cmd.Args)
		err = execCommand(cmd)

		if err != nil {
			log.Fatal(err)
		}
		// move output dir
		fmt.Println("-- Move File --")
		basename := filepath.Base(getOutFileUrl)
		fmt.Println(basename)
		newLocation := filepath.Join(outputDir, basename)
		fmt.Printf("%v -> %v\n", basename, newLocation)
		err = os.Rename(basename, newLocation)

		if err != nil {
			log.Fatal(err)
		}

		//err = os.Remove(newLocation)
		//if err != nil {
		//log.Fatal(err)
		//}
	}

}
