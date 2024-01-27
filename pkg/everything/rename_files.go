package everything

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
)

func RenameFiles() {
	path := "C:\\Users\\mipa0717\\Documents\\ericsson\\pmic2\\XML"
	pathNew := "C:\\Users\\mipa0717\\Documents\\ericsson\\pmic2.1\\XML"
	if _, err := os.Stat(pathNew); os.IsNotExist(err) {
		_ = os.Mkdir(pathNew, os.ModePerm)
	}
	neDirs, err := ioutil.ReadDir(path)
	if err != nil {
		log.Fatal(err)
	}

	for _, neDir := range neDirs {
		neName := neDir.Name()
		//newFileName := strings.ReplaceAll(strings.ReplaceAll(fileName, "-", ""), " ", "_")
		fmt.Println(neName)
		if neDir.IsDir() {
			fileDir := path + "\\" + neName
			files, err := ioutil.ReadDir(fileDir)
			if err != nil {
				log.Fatal(err)
			}
			newNeDir := pathNew + "\\" + neName
			if _, err := os.Stat(newNeDir); os.IsNotExist(err) {
				_ = os.Mkdir(newNeDir, os.ModePerm)
			}
			for _, file := range files {
				if !file.IsDir() && strings.Contains(file.Name(), "A20220113") {
					e := os.Rename(fmt.Sprintf("%s\\%s\\%s", path, neName, file.Name()), fmt.Sprintf("%s\\%s\\%s", pathNew, neName, file.Name()))
					if e != nil {
						log.Fatal(e)
					}
				}
			}
		}
	}
}
