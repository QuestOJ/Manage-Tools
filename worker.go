package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/process"
)

/* Part I: typedef */

//Request describes the http request settings
type Request struct {
	URL         string
	Method      string
	Header      map[string]string
	Data        map[string]string
	NotRedirect bool

	Cookie []*http.Cookie
}

//The Response contains the response of the http request
type Response struct {
	ResponseBody   []byte
	RedirectStatus bool
}

var taskID string
var taskName string

var task Task

var uploadCnt = 0
var numbers = []rune("0123456789")

/* Part II: makes a request*/
func redirectRules(req *http.Request, via []*http.Request) error {
	if len(via) >= 0 {
		return errors.New("No Redirect")
	}
	return nil
}

func addCommonHeader(req *http.Request) {
	req.Header.Set("Accept-Encoding", "none")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set("Cache-Control", "max-age=0")
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/75.0.3770.100 Safari/537.36")
}

func addExtraHeader(header map[string]string, req *http.Request) {
	for headerName, headerValue := range header {
		req.Header.Set(headerName, headerValue)
	}
}

func addExtraCookie(Cookie []*http.Cookie, req *http.Request) {
	for _, v := range Cookie {
		req.AddCookie(v)
	}
}

func addPostBody(data map[string]string) string {
	var postdata http.Request

	postdata.ParseForm()

	for dataName, dataValue := range data {
		postdata.Form.Add(dataName, dataValue)
	}

	bodystr := strings.TrimSpace(postdata.Form.Encode())
	return bodystr
}

func sendHTTPRequest(request Request) (Response, error) {
	var response = Response{}
	var client *http.Client

	if request.NotRedirect {
		client = &http.Client{
			CheckRedirect: redirectRules,
		}
	} else {
		client = &http.Client{}
	}

	var bodystr string

	if request.Method == "POST" {
		bodystr = addPostBody(request.Data)
	}

	req, err := http.NewRequest(request.Method, request.URL, strings.NewReader(bodystr))

	if err != nil {
		return response, err
	}

	addCommonHeader(req)
	addExtraHeader(request.Header, req)
	addExtraCookie(request.Cookie, req)

	if request.Method == "POST" {
		req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3")
	}

	resp, err := client.Do(req)

	if err != nil {
		flysnowRegexp := regexp.MustCompile(`No Redirect`)
		params := flysnowRegexp.FindStringSubmatch(err.Error())

		if len(params) == 0 {
			return response, err
		}

		response.RedirectStatus = true
	}

	if resp.StatusCode >= 400 {
		return response, errors.New("Remote server return with code " + strconv.Itoa(resp.StatusCode))
	}

	if response.RedirectStatus {
		return response, nil
	}

	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return response, err
	}

	newCookie := resp.Cookies()

	for i := 0; i < len(newCookie); i++ {
		request.Cookie = append(request.Cookie, newCookie[i])
	}

	response.RedirectStatus = false
	response.ResponseBody = body

	return response, nil
}

//HTTPRequest makes a request
func HTTPRequest(request Request) (Response, error) {
	var response Response
	var err error

	for i := 1; i <= 3; i++ {
		response, err = sendHTTPRequest(request)

		if err == nil {
			return response, nil
		}
	}

	return response, err
}

/* Part III: Upload datas */
func upload(status string, jobid int, times int, comments []byte) {
	request := Request{}

	request.URL = config.Server
	request.Method = "POST"
	request.NotRedirect = false

	submitComments := base64.StdEncoding.EncodeToString(comments)
	uploadCnt++

	request.Data = map[string]string{
		"clientID":     config.ClientID,
		"clientSecret": config.ClientSecret,
		"taskID":       taskID,
		"jobID":        strconv.Itoa(jobid),
		"times":        strconv.Itoa(times),
		"status":       status,
		"comments":     submitComments,
		"uploadCnt":    strconv.Itoa(uploadCnt),
	}

	response, err := HTTPRequest(request)

	if err != nil {
		log(2, "Failed: "+err.Error())
	} else {
		log(3, "Server Return: "+string(response.ResponseBody))
	}
}

/* Part IV: Run program */

func outputStream(outputStream io.ReadCloser, outputType string, runDir string, id int, attempt int) {
	lastStamp := time.Now().UnixNano() / 1e6
	var str strings.Builder

	file, _ := os.Create(runDir + "/" + outputType + ".log")

	for {
		nowStamp := time.Now().UnixNano() / 1e6

		if nowStamp-lastStamp > 1000 {
			go func(str strings.Builder) {
				upload(outputType, id, attempt, []byte(str.String()))
				file.WriteString(str.String())
			}(str)

			str.Reset()
			lastStamp = nowStamp
		}

		tmp := make([]byte, 1024)
		_, err := outputStream.Read(tmp)

		n := bytes.Index(tmp, []byte{0})
		str.WriteString(string(tmp[:n]))

		if err != nil {
			break
		}
	}

	go func(str strings.Builder) {
		upload(outputType, id, attempt, []byte(str.String()))
		file.WriteString(str.String())
	}(str)
}

func getRunUser(username string) (uint32, uint32, error) {
	user, err := user.Lookup(username)

	if err != nil {
		return 0, 0, err
	}

	uid, _ := strconv.Atoi(user.Uid)
	gid, _ := strconv.Atoi(user.Gid)

	return uint32(uid), uint32(gid), nil
}

func runProgram(command JobCommand, timeout int, runDir string, id int, attempt int) (bool, error) {
	var scriptsCommand strings.Builder

	scriptsCommand.WriteString("#!/bin/bash\nset -e\n")
	scriptsCommand.WriteString(command.Program)

	for _, args := range command.Args {
		scriptsCommand.WriteString(" ")
		scriptsCommand.WriteString(args)
	}

	scriptsCommand.WriteString("\nsleep 2")

	err := ioutil.WriteFile(runDir+"/scripts.sh", []byte(scriptsCommand.String()), 0644)
	if err != nil {
		return false, err
	}

	uid, gid, err := getRunUser(command.User)

	if err != nil {
		return false, err
	}

	subProcess := exec.Command("bash", runDir+"/scripts.sh")
	subProcess.SysProcAttr = &syscall.SysProcAttr{}
	subProcess.SysProcAttr.Credential = &syscall.Credential{Uid: uid, Gid: gid}

	stdout, err := subProcess.StdoutPipe()
	if err != nil {
		return false, err
	}

	stderr, err := subProcess.StderrPipe()
	if err != nil {
		return false, err
	}

	startStamp := time.Now().UnixNano() / 1e6
	if err = subProcess.Start(); err != nil {
		return false, err
	}

	timelimitInt := timeout * 1000

	go func(processHandle *os.Process) {
		pid := int32(processHandle.Pid)

		isRun, _ := process.PidExists(pid)

		for isRun == true {
			if int((time.Now().UnixNano()/1e6)-startStamp) >= timelimitInt+2010 {
				processHandle.Kill()
			}

			isRun, _ = process.PidExists(pid)
		}
	}(subProcess.Process)

	go outputStream(stdout, "stdout", runDir, id, attempt)
	go outputStream(stderr, "stderr", runDir, id, attempt)

	err = subProcess.Wait()

	if err != nil {
		fmt.Println(err)
		return false, err
	}

	stdout.Close()
	stderr.Close()

	return true, nil
}

/* Part V: Routine */
func loadWorker(dataDir string, taskName string) {
	var err error

	config, err = loadConfig(configFile)

	if err != nil {
		log(1, err.Error())
	}

	log(3, "Parse config.json success")

	task, err = loadJobs(dataDir + "/tasks/" + taskName + ".json")
	loadArgs(&task)

	log(3, "Parse "+taskName+".json success")

	if err != nil {
		log(1, err.Error())
	}
}

func runJob(job Job, id int, attempt int) bool {
	jobInfo, _ := json.Marshal(job.Info)

	log(3, "Running job "+strconv.Itoa(id)+" attempt "+strconv.Itoa(attempt))
	upload("new-job", id, attempt, jobInfo)

	stringID := strconv.Itoa(id)
	stringAttempt := strconv.Itoa(attempt)

	runDir := logDir + "/" + stringID + "-" + string(stringAttempt)

	os.Mkdir(runDir, 0755)

	status, err := runProgram(job.Command, job.Settings.Timeout, runDir, id, attempt)

	if err != nil {
		status = false
		log(2, err.Error())
	}

	if status == true {
		log(3, "Running job "+strconv.Itoa(id)+" attempt "+strconv.Itoa(attempt)+" success")
		upload("success", id, attempt, jobInfo)
		return true
	}

	log(3, "Running job "+strconv.Itoa(id)+" attempt "+strconv.Itoa(attempt)+" failed")
	upload("failed", id, attempt, jobInfo)
	return false
}

// Initworker function
func Initworker() {
	realPath, _ := filepath.Abs(dataDir)
	logDir = dataDir + "/log/worker/" + string(taskID)
	logFile = logDir + "/main.log"
	configFile = dataDir + "/config.json"

	os.Mkdir(dataDir+"/log", 0755)
	os.Mkdir(dataDir+"/log/worker", 0755)
	os.Mkdir(logDir, 0755)

	log(3, "New task starting...")
	log(3, "Task ID "+taskID)
	log(3, "Task name "+taskName)
	log(3, "Data path "+realPath)

	//Load Config and Task
	loadWorker(dataDir, taskName)
	taskInfo, _ := json.Marshal(task.Info)

	//Task begin
	log(3, "Running task "+taskName+" ["+task.Info.Description+"]")
	upload("new-task", 0, 0, taskInfo)

	runFlag := true

	for i := 0; i < len(task.Jobs); i++ {
		job := task.Jobs[i]

		if job.Settings.AlwaysRun == false && runFlag == false {
			continue
		}

		log(3, "Running job "+strconv.Itoa(i+1)+" ["+job.Info.Description+"]")

		successFlag := false

		for j := 0; j < job.Settings.Retry+1; j++ {
			successFlag = runJob(job, i+1, j+1)

			if successFlag {
				break
			}
		}

		if successFlag == false {
			runFlag = false
		}
	}

	log(3, "Finished task "+taskName+" ["+task.Info.Description+"]")

	if runFlag == false {
		upload("finished-failed", 0, 0, taskInfo)
	} else {
		upload("finished-success", 0, 0, taskInfo)
	}
}
