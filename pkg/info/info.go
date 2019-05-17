package info

import (
	"encoding/json"
	"fmt"
)

var (
	commitSha = ""
	driver    = ""
	goVersion = ""
	buildDate = ""
	os        = ""
	arch      = ""
)

type Version struct {
	Commit    string `json:"commit"`
	Driver    string `json:"driver"`
	GoVersion string `json:"goVersion"`
	BuildDate string `json:"buildDate"`
	Os        string `json:"os"`
	Arch      string `json:"arch"`
}

func Print() {
	ver := &Version{
		Commit:    commitSha,
		Driver:    driver,
		GoVersion: goVersion,
		BuildDate: buildDate,
		Os:        os,
		Arch:      arch,
	}
	ver2, _ := json.Marshal(ver)
	fmt.Println(string(ver2))
}
