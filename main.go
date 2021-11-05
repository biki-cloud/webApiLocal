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

type Payload struct {
	Filename string `json:"filename"`
}

type Out struct {
	OutPathUrls []string `json:"outUrls"`
}

func processFileOnServer(url string, uploadedFile string) ([]string, error) {
	myLogger.Printf("url: %v\n", url)
	myLogger.Printf("uploadFile: %v\n", uploadedFile)

	data := Payload{Filename: uploadedFile}

	payloadBytes, err := json.Marshal(data)
	myLogger.Printf("payloadBytes: %v\n", string(payloadBytes))
	if err != nil {
		return nil, err
	}

	body := bytes.NewReader(payloadBytes)

	//req, err := http.NewRequest("POST", "http://127.0.0.1:8080/pro/convertToJson", body)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	//resp, err := http.DefaultClient.Do(req)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	var out Out
	b, err := ioutil.ReadAll(resp.Body)
	fmt.Println(string(b))
	if err := json.Unmarshal(b, &out); err != nil {
		log.Fatal(err)
	}

	fmt.Printf("%v\n", out.OutPathUrls)

	return out.OutPathUrls, err

}

func main() {
	var (
		url       string
		inputFile string
		outputDir string
	)
	flag.StringVar(&url, "url", "default", "please url")
	flag.StringVar(&inputFile, "in", "default", "please input file")
	flag.StringVar(&outputDir, "out", "default", "please output dir")

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

	basename := filepath.Base(inputFile)
	getOutFileUrls, err := processFileOnServer(url, basename)

	// curl -X POST file={inputFile} localhost:8080/pro/convertToJson
	fmt.Println()

	// 上で受け取るコマンドを用いてファイルをサーバーからローカルに取得する
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
