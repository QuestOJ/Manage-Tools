package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
)

//Config is a data field for connection settings, which data parsed from config.json
type Config struct {
	Server       string `json:"server"`
	ClientID     string `json:"id"`
	ClientSecret string `json:"secret"`
	Port         string `json:"port"`
}

//BasicInfo describes a basic information
type BasicInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

//JobCommand contains the scripts the job wants to run
type JobCommand struct {
	Program string   `json:"program"`
	Args    []string `json:"args"`
	User    string   `json:"user"`
}

//JobSettings describes options for a job
type JobSettings struct {
	Timeout   int  `json:"timeout"`
	AlwaysRun bool `json:"alwaysRun"`
	Retry     int  `json:"retry"`
}

//Job describes a job
type Job struct {
	Info     BasicInfo   `json:"info"`
	Command  JobCommand  `json:"command"`
	Settings JobSettings `json:"settings"`
}

//Task describes a task, which contains lots of jobs
type Task struct {
	Info BasicInfo `json:"info"`
	Jobs []Job     `json:"jobs"`
}

var configFile string
var config Config

func readFile(filePath string) ([]byte, error) {
	Data, err := ioutil.ReadFile(filePath)

	if err != nil {
		return nil, err
	}

	return Data, nil
}

func loadConfig(configFilePath string) (Config, error) {
	config := Config{}

	configData, err := readFile(configFilePath)

	if err != nil {
		return config, err
	}

	err = json.Unmarshal(configData, &config)

	if err != nil {
		return config, err
	}

	return config, nil
}

func loadJobs(configFilePath string) (Task, error) {
	tasks := Task{}

	configData, err := readFile(configFilePath)

	if err != nil {
		return tasks, err
	}

	err = json.Unmarshal(configData, &tasks)

	if err != nil {
		return tasks, err
	}

	return tasks, nil
}

func loadArgs(task *Task) {
	argReg := regexp.MustCompile(`^{\$[\d+]}$`)
	argNumberReg := regexp.MustCompile(`[\d+]`)

	for i := 0; i < len(task.Jobs); i++ {
		for j := 0; j < len(task.Jobs[i].Command.Args); j++ {
			argResult := argReg.FindAllString(task.Jobs[i].Command.Args[j], -1)

			if len(argResult) > 0 {
				argNumberResult := argNumberReg.FindAllString(argResult[0], -1)
				argNumber, _ := strconv.Atoi(argNumberResult[0])

				if argNumber >= len(flag.Args())-1 {
					log(1, "Arg"+argNumberResult[0]+" not provided")
					fmt.Println("Usage: tools [-d data dir] [task] [id] [arg1] [arg2] ... \n\nOptions:")
					flag.PrintDefaults()
					os.Exit(1)
				}

				task.Jobs[i].Command.Args[j] = flag.Arg(argNumber + 1)
			}
		}
	}
}
