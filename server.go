package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

type taskExec struct {
	TaskName string   `json:"taskname"`
	RunArgs  []string `json:"args"`
}

var exectask taskExec

func validateTaskname(taskname string) bool {
	_, err := os.Stat(dataDir + "/tasks/" + taskname + ".json")
	if err != nil {
		if os.IsExist(err) {
			return true
		}
		return false
	}
	return true
}

func randSeq(n int) string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]rune, n)
	for i := range b {
		b[i] = numbers[r.Intn(len(numbers))]
	}
	return string(b)
}

func runWorker(generateID string) {
	runArgs := []string{"-worker", "-d", dataDir, exectask.TaskName, generateID}
	runArgs = append(runArgs, exectask.RunArgs...)

	selfPath, _ := filepath.Abs(os.Args[0])
	subProcess := exec.Command(selfPath, runArgs...)

	if err := subProcess.Start(); err != nil {
		log(2, err.Error())
	}

	err := subProcess.Wait()

	if err != nil {
		log(2, err.Error())
	}
}

func loadServer(dataDir string) {
	var err error

	config, err = loadConfig(configFile)

	if err != nil {
		log(1, err.Error())
	}

	log(3, "Parse config.json success")
}

func Initserver() {
	logDir = dataDir + "/log/server/" + time.Now().Format("20060102")
	logFile = logDir + "/main.log"
	configFile = dataDir + "/config.json"

	os.Mkdir(dataDir+"/log", 0755)
	os.Mkdir(dataDir+"/log/server", 0755)
	os.Mkdir(logDir, 0755)

	loadServer(dataDir)
	startServer()
}

func startServer() {
	log(2, "Listening on 0.0.0.0:"+config.Port)

	http.HandleFunc("/", sayHello)
	http.HandleFunc("/submit", submit)
	err := http.ListenAndServe("0.0.0.0:"+config.Port, nil)

	if err != nil {
		log(1, err.Error())
	}
}

func sayHello(writer http.ResponseWriter, request *http.Request) {
	log(3, "Recevie request from "+request.RemoteAddr+" path "+request.URL.Path)
	writer.Write([]byte(VERSION))
}

func submit(writer http.ResponseWriter, request *http.Request) {
	log(3, "Recevie request from "+request.RemoteAddr+" path "+request.URL.Path)

	id := request.PostFormValue("id")
	token := request.PostFormValue("token")

	storePassword := md5.Sum([]byte(config.ClientID + config.ClientSecret))

	if id != config.ClientID && token != string(storePassword[:]) {
		writer.WriteHeader(403)
		log(3, "403 Authentication password required")
		return
	}

	taskJson := request.PostFormValue("task")
	err := json.Unmarshal([]byte(taskJson), &exectask)

	if err != nil {
		writer.WriteHeader(400)
		writer.Write([]byte(err.Error()))
		log(3, err.Error())
		return
	}

	fmt.Println(exectask)

	if validateTaskname(exectask.TaskName) == false {
		writer.WriteHeader(400)
		writer.Write([]byte("Invailed task name " + exectask.TaskName))
		log(3, "Invailed task name "+exectask.TaskName)
		return
	}

	generateID := strconv.FormatInt(time.Now().Unix(), 10) + randSeq(6)

	go runWorker(generateID)
	writer.Write([]byte(generateID))

}
